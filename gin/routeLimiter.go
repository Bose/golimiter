package gin

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Bose/golimiter"
	"github.com/gin-gonic/gin"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// ErrorResponse - define the structure of errors sent to consumers
type ErrorResponse struct {
	Code      int    `json:"code" binding:"required"`
	Subcode   int    `json:"subcode" binding:"required"`
	Message   string `json:"message" binding:"required"`
	RequestID string `json:"requestid" binding:"required"`
}

// RateLimiter - define a rate limiter type
type RateLimiter struct {
	LimiterGUID      string                  // GUID for the limiter, since routes can have multiple limiters, this ensures the key is uniq - typically should be method + route, example == GET::/hello
	Limiter          *golimiter.Limiter      // The limiter
	Store            *golimiter.LimiterStore // Store for the limiter (* may include a central store of redis)
	KeyGetter        KeyGetter               // Func that gets the Key for the request (default is the clientIP)
	Reporter         RateExceededReporter    // func that logs/traces/etc when the rate is exceeded
	Type             string                  // the type of rate limiter (user, product, IP, etc)
	Behavior         string                  // the string you want to send back to the client for the behavior you'd prefer from the retry logic
	Rate             string                  // the Rate of the limiter
	ErrorCode        int                     // the primary error code returned when the rate is exceeded
	ErrorSubCode     int                     // the secondary error code returned when the rate is exceeded
	MetricLabel      string                  // prometheus label for the type of limiter to increment (user, IP, product, etc)
	Metric           *prometheus.CounterVec  // prometheus metric for observing exceeded rates
	MetricIncremeter MetricIncrementer       // func called to increment the prometheus metric with the required labels
}

// KeyGetter will define the rate limiter key given the gin Context
type KeyGetter func(c *gin.Context) (string, bool)

// RateExceededReporter - will define what/how things will get reported when rates are exceeded
type RateExceededReporter func(c *gin.Context, l RateLimiter, span opentracing.Span, doTracing bool) string

// MetricIncrementer - will define how the metric for the limiter is incremented (and deal with labels)
type MetricIncrementer func(c *gin.Context, l RateLimiter, metric *prometheus.CounterVec)

// NewLimiter - create a new one
func NewLimiter(
	limiterGUID string,
	rateStr string,
	rateType string,
	keyGetter KeyGetter,
	reporter RateExceededReporter,
	maxEntriesInMemory int,
	behavior string,
	errorCode int,
	errorSubCode int,
	metricLabel string,
	metric *prometheus.CounterVec,
	metricIncrementer MetricIncrementer,
	opts ...golimiter.Option) (l RateLimiter, err error) {
	rate, err := golimiter.NewRateFromFormatted(rateStr)
	if err != nil {
		return l, err
	}
	// gotta override the default exp with the rate's exp (regardless of opts)
	newOpts := append([]golimiter.Option{golimiter.WithLimiterExpiration(rate.Period)}, opts...)
	store := golimiter.NewInMemoryLimiterStore(maxEntriesInMemory, newOpts...)
	if limiterGUID == "" {
		limiterGUID = uuid.New().String()
	}
	l = RateLimiter{
		LimiterGUID:      limiterGUID,
		Limiter:          golimiter.New(store, rate),
		Store:            store,
		KeyGetter:        keyGetter,
		Reporter:         reporter,
		Type:             rateType,
		Behavior:         behavior,
		Rate:             rateStr,
		ErrorCode:        errorCode,
		ErrorSubCode:     errorCode,
		MetricLabel:      metricLabel,
		Metric:           metric,
		MetricIncremeter: metricIncrementer,
	}
	return l, nil
}

// LimitRoute -  Decorator
func LimitRoute(limiters []RateLimiter, handle gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestid, ok := c.Get("request-id")
		if !ok {
			requestid = "unknown"
		}
		doTracing := false
		rateSpan, err := traceSpan(c, "limitRoute", false)
		if err != nil {
			doTracing = false
		} else {
			defer rateSpan.Finish()
			defer timeTrack(time.Now(), "LimitRoute")
		}

		for _, l := range limiters {
			key, ok := l.KeyGetter(c)
			if !ok {
				logrus.Debugf("LimitRoute: no key found for %s rate limiter - nothing to enforce", l.Type)
				continue
			}
			context, err := l.Limiter.Get(c, l.LimiterGUID+":"+key)
			if err != nil {
				err := fmt.Errorf("Error getting rate limiter info: %s", err.Error())
				errStr := err.Error()
				logrus.Error(errStr)
				e := ErrorResponse{Code: l.ErrorCode, Subcode: l.ErrorSubCode, Message: errStr, RequestID: requestid.(string)}
				c.JSON(503, e)
				return
			}
			logrus.Debugf("doRateLimiting: limit: %d, remaining: %d, reset: %d, delay: %d", context.Limit, context.Remaining, context.Reset, context.Delay)
			if context.Delay == 0 {
				c.Header("X-RateLimit-Limit", strconv.FormatInt(context.Limit, 10))
				c.Header("X-RateLimit-Remaining", strconv.FormatInt(context.Remaining, 10))
				c.Header("X-RateLimit-Reset", strconv.FormatInt(context.Reset, 10))
				c.Header("X-Retry-Behavior", l.Behavior)
				c.Header("Retry-After", strconv.FormatInt(context.Reset-time.Now().Unix(), 10))

				if context.Reached {
					errStr := l.Reporter(c, l, rateSpan, doTracing)
					logrus.Errorf(errStr)
					e := ErrorResponse{Code: l.ErrorCode, Subcode: l.ErrorSubCode, Message: errStr, RequestID: requestid.(string)}
					c.JSON(http.StatusTooManyRequests, e)
					return
				}
			}
			// context reached and we're just delaying the response
			if context.Reached {
				errStr := l.Reporter(c, l, rateSpan, doTracing)
				logrus.Errorf(errStr)
				time.Sleep(time.Duration(context.Delay) * time.Millisecond)
			}
		}
		// we didn't hit any rate limits so handle the request
		handle(c)
		return
	}
}

// DefaultKeyGetter - returns the Client IP
func DefaultKeyGetter(c *gin.Context) (string, bool) {
	return c.ClientIP(), true
}

// DefaultRateExceededReporter - handle all the reporting/logging/tagging for rate limiting
func DefaultRateExceededReporter(c *gin.Context, l RateLimiter, span opentracing.Span, doTracing bool) string {
	if doTracing {
		span.SetTag("rate-limited", true)
		span.SetTag("rate-limiter-type", l.Type)
		span.SetTag("rate-limiiter-rate", l.Rate)
	}
	if l.Metric != nil {
		l.MetricIncremeter(c, l, l.Metric)
	}
	return fmt.Sprintf("Rate limited: type == %s and rate == %s", l.Type, l.Rate)
}

// DefaultMetricIncrementer - increment the metric when a rate is exceeded
func DefaultMetricIncrementer(c *gin.Context, l RateLimiter, metric *prometheus.CounterVec) {
	metric.WithLabelValues(l.MetricLabel, l.Type, l.Rate).Inc()
}

// NewDefaultMetric - define what a metric is
func NewDefaultMetric(subsystem string, name string, help string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      name,
			Help:      help,
		},
		[]string{"label", "type", "rate"},
	)
	if err := prometheus.Register(c); err != nil {
		logrus.Errorf("%s could not be registered: %s", name, err.Error())
	} else {
		logrus.Infof("%s registered.", name)
	}
	return c
}

// timeTrack - spill some timing into the debug log stream
func timeTrack(start time.Time, name string) float64 {
	elapsed := time.Since(start)
	logrus.Debugf("%s took %s", name, elapsed)
	return float64(elapsed / time.Millisecond) // convert to milliseconds
}

func traceSpan(c *gin.Context, operationName string, doAuthSpan bool) (s opentracing.Span, e error) {
	parentSpan, _ := c.Get("tracing-context")
	if parentSpan == nil {
		return s, errors.New("unable to Get tracing-context")
	}
	return startSpanWithParent(parentSpan.(opentracing.Span).Context(), operationName, c.Request.Method, c.Request.URL.Path), nil
}

// startSpanWithParent will start a new span with a parent span.
// example:
//      span:= StartSpanWithParent(c.Get("tracing-context"),
func startSpanWithParent(parent opentracing.SpanContext, operationName, method, path string) opentracing.Span {
	options := []opentracing.StartSpanOption{
		opentracing.Tag{Key: ext.SpanKindRPCServer.Key, Value: ext.SpanKindRPCServer.Value},
		opentracing.Tag{Key: string(ext.HTTPMethod), Value: method},
		opentracing.Tag{Key: string(ext.HTTPUrl), Value: path},
		opentracing.Tag{Key: "current-goroutines", Value: runtime.NumGoroutine()},
	}

	if parent != nil {
		options = append(options, opentracing.ChildOf(parent))
	}

	return opentracing.StartSpan(operationName, options...)
}

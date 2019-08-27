package main

import (
	"os"

	"github.com/Bose/golimiter"

	ginlimiter "github.com/Bose/golimiter/gin"
	ginprometheus "github.com/zsais/go-gin-prometheus"

	goCache "github.com/Bose/go-cache"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const maxConnectionsAllowed = 50 // max connections allowed to the read-only redis cluster from this service
const maxEntries = 1000          // max entries allowed in the LRU + expiry in memory store

const (
	redisTestServer       = "redis://localhost:6379"
	sentinelTestServer    = "redis://localhost:26379"
	redisMasterIdentifier = "mymaster"
	sharedSecret          = "test-secret-must"
	useSentinel           = true
	defExpSeconds         = 0
	redisOpsTimeout       = 50  // millisecods
	redisConnTimeout      = 500 // milliseconds
)

func main() {
	// use the JSON formatter
	// logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.DebugLevel)

	r := gin.Default()
	r.Use(gin.Recovery()) // add Recovery middleware
	p := ginprometheus.NewPrometheus("go_limiter_example")
	p.Use(r)

	redisCache := setupRedisCentralStore()

	l, err := ginlimiter.NewLimiter(
		"GET::/helloworld",                     // define the GUID that identifies this rate limiter as GET on the route
		"10-2-M",                               // defines the rate limit based on a std format
		"ip-address",                           // a label that defines the type of rate limiter it is
		ginlimiter.DefaultKeyGetter,            // get the key for this request: IP, request USER identifier, etc
		ginlimiter.DefaultRateExceededReporter, // func that does all the required reporting when a rate is exceeded
		maxEntries,                             // max inmemory entries
		"constant",                             // this hint is sent back to the client: constant, exponential, etc
		1,                                      // primary error code
		2,                                      // error sub code
		"hello-world",                          // prometheus metric label for this route
		ginlimiter.NewDefaultMetric("limtier_example", "helloworld", "count times helloworld is rate limited"), // prometheus.ConterVec for rate limit exceeded
		ginlimiter.DefaultMetricIncrementer, // how-to increment the prometheus.CounterVec with the required labels
		golimiter.WithUnsyncCounterLimit(1), // use this option to limit how far the mem and central store can drift
		golimiter.WithUnsyncTimeLimit(5),    // use this option to update the central store at least every 10ms
	)
	if err != nil {
		panic("failed to create limiter " + err.Error())
	}
	l.Store.DistributedRedisCache = redisCache

	l2, err := ginlimiter.NewLimiter(
		"GET::/helloworld-delayed",             // define the GUID that identifies this rate limiter as GET on the route
		"10-2-M-1000",                          // defines the rate limit based on a std format with a delay of 1s
		"ip-address",                           // a label that defines the type of rate limiter it is
		ginlimiter.DefaultKeyGetter,            // get the key for this request: IP, request USER identifier, etc
		ginlimiter.DefaultRateExceededReporter, // func that does all the required reporting when a rate is exceeded
		maxEntries,                             // max inmemory entries
		"constant",                             // this hint is sent back to the client: constant, exponential, etc
		1,                                      // primary error code
		2,                                      // error sub code
		"hello-world",                          // prometheus metric label for this route
		ginlimiter.NewDefaultMetric("limiter_example", "helloworld_delayed", "count times helloworld is rate limited"), // prometheus.ConterVec for rate limit exceeded
		ginlimiter.DefaultMetricIncrementer) // how-to increment the prometheus.CounterVec with the required labels
	if err != nil {
		panic("failed to create limiter " + err.Error())
	}
	l2.Store.DistributedRedisCache = redisCache

	// add the rate limiter decorator (which uses a slice of rate limiters)
	r.GET("/helloworld", ginlimiter.LimitRoute([]ginlimiter.RateLimiter{l}, func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "Hello world!\n"})
	}))

	// add the rate limiter decorator (which uses a slice of rate limiters)
	r.GET("/helloworld-delayed", ginlimiter.LimitRoute([]ginlimiter.RateLimiter{l2}, func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "Hello world!\n"})
	}))
	r.GET("nolimits", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "Hello world!\n"})
	})

	if err := r.Run(":9090"); err != nil {
		logrus.Error(err)
	}
}

func setupRedisCentralStore() *goCache.GenericCache {
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithFields(logrus.Fields{"method": "newInMemoryLimiter"})

	logger.Debug("setting envVar TRACE_SYNC= true will turn-on trace logging to the central store for the limiters, you must call this before creating any limiters")
	os.Setenv("TRACE_SYNC", "true")

	os.Setenv("NO_REDIS_PASSWORD", "true")
	// os.Setenv("REDIS_READ_ONLY_ADDRESS","redis://localhost:6379")
	// os.Setenv("REDIS_SENTINEL_ADDRESS", "redis://localhost:26379")
	selectDatabase := 3
	useSentinel := true
	cacheWritePool, err := goCache.InitRedisCache(useSentinel, defExpSeconds, nil, defExpSeconds, defExpSeconds, selectDatabase, logger)
	if err != nil {
		logrus.Errorf("couldn't connect to redis on %s", redisTestServer)
		panic("")
	}
	logger.Info("cacheWritePool initialized")
	readOnlyPool, err := goCache.InitReadOnlyRedisCache(redisTestServer, "", redisOpsTimeout, redisConnTimeout, defExpSeconds, maxConnectionsAllowed, selectDatabase, logger)
	if err != nil {
		logrus.Errorf("couldn't connect to redis on %s", redisTestServer)
		panic("")
	}
	logger.Info("cacheReadPool initialized")
	return goCache.NewCacheWithMultiPools(cacheWritePool, readOnlyPool, goCache.L2, sharedSecret, defExpSeconds, []byte("test"), false)
}

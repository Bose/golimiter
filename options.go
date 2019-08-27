package golimiter

import "time"

const (
	defaultLimiterExpSeconds  = -100 // DefaultLimiterExpSeconds - default entry expiry to an invalid value, so we know if it hasn't been set
	defaultUnsyncCounterLimit = 50   // max diff between inmemory and central store before a sync is triggered by Get()
	defaultUnsyncTimeLimit    = 500  // how often to sync to the distributed store
)

// Option - defines a func interface for passing in options to the NewInMemoryLimiterStore()
type Option func(*options)

// options - defines the available options for NewInMemoryLimiterStore()
type options struct {
	unsyncCounterLimit uint64
	unsyncTimeLimit    uint64
	limiterExpiration  time.Duration
	workerNumber       int
	rate               Rate
	increment          int
}

// defaultOptions - defines the defaults for options to NewInMemoryLimiterStore()
var defaultOptions = options{
	unsyncCounterLimit: defaultUnsyncCounterLimit,
	unsyncTimeLimit:    defaultUnsyncTimeLimit,
	limiterExpiration:  defaultLimiterExpSeconds * time.Second,
	workerNumber:       -1, // since there are real worker with 0 (when interating in batch), we're using -1
	increment:          0,
}

// WithLimiterExpSeconds sets the limiter's expiry in seconds
func WithLimiterExpiration(exp time.Duration) Option {
	return func(o *options) {
		o.limiterExpiration = exp
	}
}

// WithUnsyncCounterLimit - allows you to override the default limit to how far the in memory counter can drift from the central store
func WithUnsyncCounterLimit(deltaLimit uint64) Option {
	return func(o *options) {
		o.unsyncCounterLimit = deltaLimit
	}
}

// WithUnsyncTimeLimit - allows you to override the default time limit (milliseconds) between syncs to the central store.
// this cannot be 0 or it will default to defaultUnsyncTimeLimit
func WithUnsyncTimeLimit(timeLimit uint64) Option {
	return func(o *options) {
		if timeLimit > 0 {
			o.unsyncTimeLimit = timeLimit
		}
	}
}

// WithWorkerNumber passes an optional worker number
func WithWorkerNumber(worker int) Option {
	return func(o *options) {
		o.workerNumber = worker
	}
}

// WithRate passes an optional rate
func WithRate(r Rate) Option {
	return func(o *options) {
		o.rate = r
	}
}

// WithIncrement passes an optional increment
func WithIncrement(i int) Option {
	return func(o *options) {
		o.increment = i
	}
}

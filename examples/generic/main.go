package main

// Create a rate with the given limit (number of requests) for the given
// period (a time.Duration of your choice).
import (
	"context"
	"fmt"
	"os"
	"time"

	goCache "github.com/Bose/go-cache"
	"github.com/Bose/golimiter"
	"github.com/sirupsen/logrus"
)

const (
	maxEntriesInMemory    = 100
	redisTestServer       = "redis://localhost:6379"
	sentinelTestServer    = "redis://localhost:26379"
	redisMasterIdentifier = "mymaster"
	sharedSecret          = "test-secret-must"
	useSentinel           = true
	defExpSeconds         = 0
	//myEnv                 = "local"
	maxConnectionsAllowed = 5
	redisOpsTimeout       = 50  // millisecods
	redisConnTimeout      = 500 // milliseconds
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithFields(logrus.Fields{"method": "main"})

	rate := golimiter.Rate{
		Period: 1 * time.Hour,
		Limit:  1000,
	}

	logger.Debug("setting envVar TRACE_SYNC= true will turn-on trace logging to the central store for the limiters, you must call this before creating any limiters")
	os.Setenv("TRACE_SYNC", "true")

	ctx := context.Background()

	rateStore := golimiter.New(golimiter.NewInMemoryLimiterStore(maxEntriesInMemory, golimiter.WithLimiterExpiration(rate.Period)), rate)

	c, err := rateStore.Get(ctx, "test-rate-object")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Limit: %d\nReached: %v\nRemaining: %d\nReset: %d\n", c.Limit, c.Reached, c.Remaining, c.Reset)

	// You can also use the simplified format "<limit>-<duration>-<period>"", with the given
	// periods:
	//
	// * "S": second
	// * "M": minute
	// * "H": hour
	//
	// You can also add an optional delay to the format "<limit>-<duration>-<period>-<delay>":
	// "1-1-S-10"  represents 1 req in 1 second with 10ms delay
	//
	// Examples:
	//
	// * 5 reqs in 10 seconds: "5-10-S"
	// * 10 reqs in 5 minutes: "10-5-M"
	// * 1000 reqs in 1 hour: "1000-1-H"
	// * 5 reqs in 10 seconds with 20ms delay: "5-10-S-20"
	//

	rate, err = golimiter.NewRateFromFormatted("1000-1-H")
	if err != nil {
		panic(err)

	}

	// let's make a limiter that uses redis for cross process coordiation
	store := golimiter.NewInMemoryLimiterStore(maxEntriesInMemory, golimiter.WithLimiterExpiration(rate.Period))
	store.DistributedRedisCache = setupRedisCentralStore(logger)
	rateStore = golimiter.New(store, rate)
	c, err = rateStore.Get(ctx, "test-rate-object")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Limit: %d\nReached: %v\nRemaining: %d\nReset: %d\nDelay: %d\n", c.Limit, c.Reached, c.Remaining, c.Reset, c.Delay)
	time.Sleep(500 * time.Millisecond) // since it's eventually consistent with redis, you need to give it a sec to sync

	rateWithDelay, err := golimiter.NewRateFromFormatted("1000-1-H-1000")
	if err != nil {
		panic(err)

	}
	rateStore = golimiter.New(store, rateWithDelay)
	c, err = rateStore.Get(ctx, "test-rate-object")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Limit: %d\nReached: %v\nRemaining: %d\nReset: %d\nDelay: %d\n", c.Limit, c.Reached, c.Remaining, c.Reset, c.Delay)
	time.Sleep(500 * time.Millisecond) // since it's eventually consistent with redis, you need to give it a sec to sync

}

func setupRedisCentralStore(logger *logrus.Entry) *goCache.GenericCache {
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
	readOnlyPool, err := goCache.InitReadOnlyRedisCache("redis://localhost:6379", "", redisOpsTimeout, redisConnTimeout, defExpSeconds, maxConnectionsAllowed, selectDatabase, logger)
	if err != nil {
		logrus.Errorf("couldn't connect to redis on %s", redisTestServer)
		panic("")
	}
	logger.Info("cacheReadPool initialized")
	return goCache.NewCacheWithMultiPools(cacheWritePool, readOnlyPool, goCache.L2, sharedSecret, defExpSeconds, []byte("test"), false)
}

package benchmarks

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	goCache "github.com/Bose/go-cache"
	"github.com/Bose/golimiter"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

var benchInMemoryWithRedisStore *golimiter.Limiter
var benchLimitWithRedis int64
var benchKeyWithRedis string

func init() {
	benchLimitWithRedis = 1000000000
	benchKeyWithRedis = uuid.New().String()
	benchInMemoryWithRedisStore = newBenchStoreInMemoryWithRedis(strconv.Itoa(int(benchLimitWithRedis)) + "-10-M")
}
func benchmarkTypicalIncrInMemoryWithRedis(i int, b *testing.B) {
	for n := 0; n < b.N; n++ {
		typicalBenchRateIncr(b, benchInMemoryWithRedisStore, benchKeyWithRedis)
	}
	_ = benchInMemoryWithRedisStore.Store.CentralStoreUpdates()
}
func benchmarkTypicalUpdateRedis(i int, b *testing.B) {
	for n := 0; n < b.N; n++ {
		typicalUpdateRedis(b, benchInMemoryWithRedisStore, benchKeyWithRedis)
	}
}

func BenchmarkIncrInMemoryWithRedis1(b *testing.B)  { benchmarkTypicalIncrInMemoryWithRedis(1, b) }
func BenchmarkIncrInMemoryWithRedis2(b *testing.B)  { benchmarkTypicalIncrInMemoryWithRedis(2, b) }
func BenchmarkIncrInMemoryWithRedis3(b *testing.B)  { benchmarkTypicalIncrInMemoryWithRedis(3, b) }
func BenchmarkIncrInMemoryWithRedis10(b *testing.B) { benchmarkTypicalIncrInMemoryWithRedis(10, b) }
func BenchmarkIncrInMemoryWithRedis20(b *testing.B) { benchmarkTypicalIncrInMemoryWithRedis(20, b) }
func BenchmarkIncrInMemoryWithRedis40(b *testing.B) { benchmarkTypicalIncrInMemoryWithRedis(40, b) }

func BenchmarkUpdateCentralStore(b *testing.B)   { benchmarkTypicalUpdateRedis(1, b) }
func BenchmarkUpdateCentralStore2(b *testing.B)  { benchmarkTypicalUpdateRedis(2, b) }
func BenchmarkUpdateCentralStore3(b *testing.B)  { benchmarkTypicalUpdateRedis(3, b) }
func BenchmarkUpdateCentralStore10(b *testing.B) { benchmarkTypicalUpdateRedis(10, b) }
func BenchmarkUpdateCentralStore20(b *testing.B) { benchmarkTypicalUpdateRedis(20, b) }
func BenchmarkUpdateCentralStore40(b *testing.B) { benchmarkTypicalUpdateRedis(40, b) }

const (
	maxBenchEntriesWithRedis = 100
	useSentinel              = true
	// maxEntries               = 1000
	// These tests require redis server running on localhost:6379 (the default)
	redisTestServer = "redis://localhost:6379"
	// sentinelTestServer    = "redis://localhost:26379"
	// redisMasterIdentifier = "mymaster"
	sharedSecret  = "test-secret-must"
	defExpSeconds = 0
	// myEnv                 = "local"
	maxConnectionsAllowed = 5
)

func newBenchStoreInMemoryWithRedis(rateLimit string) *golimiter.Limiter {
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithFields(logrus.Fields{"method": "newInMemoryLimiter"})

	os.Setenv("NO_REDIS_PASSWORD", "true")
	// os.Setenv("REDIS_READ_ONLY_ADDRESS","redis://localhost:6379")
	// os.Setenv("REDIS_SENTINEL_ADDRESS", "redis://localhost:26379")
	selectDatabase := 3
	cacheWritePool, err := goCache.InitRedisCache(useSentinel, defExpSeconds, nil, defExpSeconds, defExpSeconds, selectDatabase, logger)
	if err != nil {
		logrus.Errorf("couldn't connect to redis on %s", redisTestServer)
		panic("")
	}
	logger.Info("cacheWritePool initialized")
	readOnlyPool, err := goCache.InitReadOnlyRedisCache("redis://localhost:6379", "", 50, 500, defExpSeconds, maxConnectionsAllowed, selectDatabase, logger)
	if err != nil {
		logrus.Errorf("couldn't connect to redis on %s", redisTestServer)
		panic("")
	}
	logger.Info("cacheReadPool initialized")
	c := goCache.NewCacheWithMultiPools(cacheWritePool, readOnlyPool, goCache.L2, sharedSecret, defExpSeconds, []byte("test"), false)

	rate, err := golimiter.NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}

	store := golimiter.NewInMemoryLimiterStore(maxBenchEntriesWithRedis, golimiter.WithLimiterExpiration(rate.Period))
	store.DistributedRedisCache = c
	time.Sleep(1 * time.Second)
	store.StopCentralStoreUpdates() // gotta stop these for the benchmarks to work
	time.Sleep(2 * time.Second)
	return golimiter.New(store, rate)
}

func typicalBenchRateIncr(b *testing.B, c *golimiter.Limiter, key string) {
	ctx := context.Background()
	rateContext, err := c.Get(ctx, key)
	if err != nil {
		return // this is okay..
	}
	if rateContext.Limit != benchLimitWithRedis {
		b.Errorf("Expect %d for Limit and got %d", benchLimitWithRedis, rateContext.Limit)
	}
	if rateContext.Reached != false {
		b.Errorf("Expected reached to be true: %v", rateContext.Reached)
	}
	if rateContext.Remaining == 0 {
		b.Error("Expected remainting to be greater 0")
	}
	if rateContext.Reset < time.Now().Unix() {
		b.Error("Expected reset to be before now")
	}
}

func typicalUpdateRedis(b *testing.B, c *golimiter.Limiter, key string) {
	ctx := context.Background()
	rateContext, err := c.Get(ctx, key)
	if err != nil {
		return // this is okay..
	}
	if rateContext.Limit != benchLimitWithRedis {
		b.Errorf("Expect %d for Limit and got %d", benchLimitWithRedis, rateContext.Limit)
	}
	if rateContext.Reached != false {
		b.Errorf("Expected reached to be true: %v", rateContext.Reached)
	}
	if rateContext.Remaining == 0 {
		b.Error("Expected remainting to be greater 0")
	}
	if rateContext.Reset < time.Now().Unix() {
		b.Error("Expected reset to be before now")
	}
	workerNum := 1
	c.Store.SyncEntryWithCentralStore(key, golimiter.WithWorkerNumber(workerNum))
}

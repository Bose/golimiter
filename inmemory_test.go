package golimiter

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	goCache "github.com/Bose/go-cache"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	maxEntries = 1000
	// These tests require redis server running on localhost:6379 (the default)
	redisTestServer       = "redis://localhost:6379"
	sentinelTestServer    = "redis://localhost:26379"
	redisMasterIdentifier = "mymaster"
	sharedSecret          = "test-secret-must"
	useSentinel           = true
	defExpSeconds         = 0
	myEnv                 = "local"
	maxConnectionsAllowed = 5
)

func TestInMemoryRateStore_TypicalGetSet(t *testing.T) {
	r, err := initTestRedis(t)
	if err != nil {
		t.Fatal("Unable to init test redis: ", err)
	}
	defer r.Close()

	typicalRateIncr(t, "limiter with options no delay v1", testNewInMemoryLimiterWithOptions, "1-1-S", false)
	typicalRateIncr(t, "limiter with options with delay v2", testNewInMemoryLimiterWithOptions, "1-1-S-50", true)

	typicalRateIncr(t, "limiter with options no delay v3", testNewInMemoryLimiterWithOptionsV2, "1-2-S", false)
	typicalRateIncr(t, "limiter with options with delay v4", testNewInMemoryLimiterWithOptionsV2, "1-2-S-50", true)

	typicalRateIncr(t, "one without a delay specified v5", testNewInMemoryLimiter, "1-1-S", false) // one without a delay specified
	typicalRateIncr(t, "one with a delay specified v6", testNewInMemoryLimiter, "1-1-S-50", true)  // one with a delay specified
}

func Test_TTL(t *testing.T) {
	// r, err := initTestRedis(t)
	// if err != nil {
	// 	t.Fatal("Unable to init test redis: ", err)
	// }
	// defer r.Close()
	name := "Test_TTL"
	t.Logf("TEST NAME: %s", name)
	key := uuid.New().String()

	rate, err := NewRateFromFormatted("1-10-M")
	if err != nil {
		panic(err)
	}
	store := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period), WithUnsyncCounterLimit(0), WithUnsyncTimeLimit(1))
	store.DistributedRedisCache = testNewCache(t)
	c := New(store, rate)
	rawKey := fmt.Sprintf("%s:%s", store.Prefix, key)

	store2 := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period), WithUnsyncCounterLimit(0), WithUnsyncTimeLimit(1))
	store2.DistributedRedisCache = testNewCache(t)
	c2 := New(store2, rate)

	ctx := context.Background()

	rateContext, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("%s Error %s", name, err.Error())
	}
	now := time.Now().Unix()
	dumpRateContextToLog(rateContext)
	if rateContext.Limit != 1 {
		t.Errorf("%s Expect 1 for Limit and got %d", name, rateContext.Limit)
	}
	if rateContext.Count != 1 {
		t.Errorf("%s Expect 1 for Count and got %d", name, rateContext.Count)
	}
	if rateContext.Reached != false {
		t.Errorf("%s Expected reached to be false", name)
	}
	if rateContext.Remaining != 0 {
		t.Errorf("%s Expected remainting to be 0", name)
	}
	if rateContext.Reset < now {
		t.Errorf("%s Expected reset to be before now", name)
	}
	time.Sleep(5 * time.Second)

	now2 := time.Now().Unix()
	rateContext2, err := c2.Get(ctx, key)
	dumpRateContextToLog(rateContext2)
	if rateContext2.Limit != 1 {
		t.Errorf("%s Expect 1 for Limit and got %d", name, rateContext.Limit)
	}
	if rateContext2.Count != 2 {
		t.Errorf("%s Expect 1 for Count and got %d", name, rateContext.Count)
	}
	if rateContext2.Reached != true {
		t.Errorf("%s Expected reached to be true", name)
	}
	if rateContext2.Remaining != 0 {
		t.Errorf("%s Expected remainting to be 0", name)
	}
	if rateContext2.Reset < now2 {
		t.Errorf("%s Expected reset %d to be less than now2 %d - why wasn't the TTL taken from the central store", name, rateContext2.Reset, now)
	}
	t.Logf("%s looking for TTL for %s", name, key)
	redisTTL, err := store2.EntryCentralStoreExpiresIn(rawKey)
	if err != err {
		t.Errorf("%s unexpected error %s", name, err.Error())
	}
	expiresAt := store2.convertToExpiresAt(-1, redisTTL)
	if expiresAt != rateContext2.Reset {
		t.Errorf("%s expected expires to match redis == %d != inmemory == %d", name, expiresAt, rateContext2.Reset)
	}
	t.Logf("Updated TTL == %d", rateContext2.Reset)

}

func TestInMemoryRateStore_Janitor(t *testing.T) {
	r, err := initTestRedis(t)
	if err != nil {
		t.Fatal("Unable to init test redis: ", err)
	}
	defer r.Close()

	testJanitor(t, testNewInMemoryLimiter, "1-10-M-50")
}
func testJanitor(t *testing.T, factory limiterFactory, rate string) {
	c := factory(t, rate) // let's make the 1st store

	key := uuid.New().String()
	ctx := context.Background()
	context, err := c.Get(ctx, key) // increment/get a key from the store
	if err != nil {
		t.Fatalf("Error %s", err.Error())
	}
	dumpRateContextToLog(context) // dump the rate context for the limiter

	c2 := factory(t, rate) // let's make a 2nd store
	time.Sleep(5)          // wait for the redis connection intialized

	// add some entries in the the limiter stores
	for x := 0; x < maxEntries-1; x++ {
		_, err = c2.Get(ctx, uuid.New().String())
		if err != nil {
			t.Fatalf("Error %s", err.Error())
		}
		_, err = c.Get(ctx, uuid.New().String())
		if err != nil {
			t.Fatalf("Error %s", err.Error())
		}
	}
	logrus.Debugf("Finished with setup")
	time.Sleep(2 * time.Second) // sleeping will cause the sync to happen in the background

	val, err := c.Store.CentralStorePeek(key) // check that the original key/rate made it into the central store
	if err != nil {
		logrus.Debugf("CentralStorePeek: error fetching in redis key %s: %s", key, err.Error())
		context, err := c.Peek(ctx, key)
		if err != nil {
			t.Fatalf("Error %s", err.Error())
		}
		dumpRateContextToLog(context)
	}
	if val != 1 {
		t.Errorf("Expected 1 got %d", val)
	} else {
		logrus.Debugf("synced %s successfully", key)
	}

	logrus.Debug("turn off updates")
	c.Store.StopCentralStoreUpdates()
	c2.Store.StopCentralStoreUpdates()
	time.Sleep(1 * time.Second)
}
func typicalRateIncr(t *testing.T, name string, factory limiterFactory, rate string, withDelay bool) {
	t.Logf("TEST NAME: %s", name)
	key := uuid.New().String()

	c := factory(t, rate)
	ctx := context.Background()

	defer c.Store.StopCentralStoreUpdates()

	peekContext, err := c.Peek(ctx, key)
	if peekContext.Count != 0 {
		t.Errorf("%s expected 1 for count and got %d", name, peekContext.Count)
	}
	peekContext, err = c.Peek(ctx, key)
	if peekContext.Count != 0 {
		t.Errorf("%s expected 1 for count and got %d", name, peekContext.Count)
	}

	rateContext, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("%s Error %s", name, err.Error())
	}
	now := time.Now().Unix()
	dumpRateContextToLog(rateContext)
	if rateContext.Limit != 1 {
		t.Errorf("%s Expect 1 for Limit and got %d", name, rateContext.Limit)
	}
	if rateContext.Count != 1 {
		t.Errorf("%s Expect 1 for Count and got %d", name, rateContext.Count)
	}
	if rateContext.Reached != false {
		t.Errorf("%s Expected reached to be false", name)
	}
	if rateContext.Remaining != 0 {
		t.Errorf("%s Expected remainting to be 0", name)
	}
	if rateContext.Reset < now {
		t.Errorf("%s Expected reset to be before now", name)
	}
	if withDelay && rateContext.Delay != 50 {
		t.Errorf("%s Expected delay to be 50 and got %d", name, rateContext.Delay)
	}
	// might as well sleep the right duration
	time.Sleep(time.Duration(rateContext.Delay) * time.Millisecond)

	for i := 0; i < 10; i++ {
		rateContext, err = c.Get(ctx, key)
	}
	if err != nil {
		t.Fatalf("%s Error %s", name, err.Error())
	}
	dumpRateContextToLog(rateContext)
	if rateContext.Limit != 1 {
		t.Errorf("%s Expect 1 for Limit and got %d", name, rateContext.Limit)
	}
	if rateContext.Reached == false {
		t.Errorf("%s Expected reached to NOT be false", name)
	}
	if rateContext.Remaining != 0 {
		t.Errorf("%s Expected remainting to be 0", name)
	}
	if rateContext.Reset < time.Now().Unix() {
		t.Errorf("%s Expected reset to be before now", name)
	}
	if withDelay && rateContext.Delay != 50 {
		t.Errorf("%s Expected delay to be 50 and got %d", name, rateContext.Delay)
	}
	// might as well sleep the right duration
	time.Sleep(time.Duration(rateContext.Delay) * time.Millisecond)
}

func Test_limiterStore_CentralStoreSync(t *testing.T) {
	r, err := initTestRedis(t)
	if err != nil {
		t.Fatal("Unable to init test redis: ", err)
	}
	defer r.Close()

	l, store := testGetLimiterAndStore(t, "1-5-M")
	ctx := context.Background()
	defer l.Store.StopCentralStoreUpdates()
	tests := []struct {
		name      string
		key       string
		wantEntry bool
	}{
		{
			name:      "one",
			key:       uuid.New().String(),
			wantEntry: true,
		},
	}
	for _, tt := range tests {
		context, err := l.Get(ctx, tt.key)
		if err != nil {
			t.Error(err.Error())
		}
		fmt.Println(context)

		counter := StoreEntry{CurrentCount: 1, LastSyncedCount: 0}
		if err = store.Cache.Set(tt.key, counter, 1*time.Minute); err != nil {
			t.Errorf("got error when upating inmem store: %s", err.Error())
		}

		if err := l.Store.CentralStoreSync(); err != nil {
			t.Errorf("got error when upating central store: %s", err.Error())
		}

		gotUpdatedInMemoryEntry, err := store.CentralStorePeek(tt.key)
		if tt.wantEntry && gotUpdatedInMemoryEntry != 1 {
			t.Errorf("expected entry and got %v", gotUpdatedInMemoryEntry)
		}
	}
}

func Test_limiterStore_SyncEntryWithCentralStore(t *testing.T) {
	r, err := initTestRedis(t)
	if err != nil {
		t.Fatal("Unable to init test redis: ", err)
	}
	defer r.Close()

	l, store := testGetLimiterAndStore(t, "1-5-M")
	ctx := context.Background()
	defer l.Store.StopCentralStoreUpdates()
	tests := []struct {
		name      string
		key       string
		wantEntry bool
	}{
		{
			name:      "one",
			key:       uuid.New().String(),
			wantEntry: true,
		},
	}
	for _, tt := range tests {
		context, err := l.Get(ctx, tt.key)
		if err != nil {
			t.Error(err.Error())
		}
		fmt.Println(context)

		counter := StoreEntry{CurrentCount: 1, LastSyncedCount: 0}
		if err = store.Cache.Set(tt.key, counter, 1*time.Minute); err != nil {
			t.Errorf("got error when upating inmem store: %s", err.Error())
		}
		gotUpdatedInMemoryEntry := l.Store.SyncEntryWithCentralStore(tt.key)
		if tt.wantEntry && gotUpdatedInMemoryEntry == nil {
			t.Errorf("expected entry and got %v", gotUpdatedInMemoryEntry)
		}
	}
}

type limiterFactory func(*testing.T, string) *Limiter

var testNewInMemoryLimiter = func(t *testing.T, rateLimit string) *Limiter {
	rate, err := NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}

	store := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period))
	store.DistributedRedisCache = testNewCache(t)
	return New(store, rate)
}

var testNewInMemoryLimiterWithOptions = func(t *testing.T, rateLimit string) *Limiter {
	rate, err := NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}

	store := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period), WithUnsyncCounterLimit(0), WithUnsyncTimeLimit(1))
	store.DistributedRedisCache = testNewCache(t)
	return New(store, rate)
}

var testNewInMemoryLimiterWithOptionsV2 = func(t *testing.T, rateLimit string) *Limiter {
	rate, err := NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}

	store := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period), WithUnsyncCounterLimit(0))
	store.DistributedRedisCache = testNewCache(t)
	return New(store, rate)
}

func testGetLimiterAndStore(t *testing.T, rateLimit string) (*Limiter, *LimiterStore) {
	rate, err := NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}

	store := NewInMemoryLimiterStore(maxEntries, WithLimiterExpiration(rate.Period), WithUnsyncCounterLimit(0))
	store.DistributedRedisCache = testNewCache(t)
	return New(store, rate), store
}

func testNewCache(t *testing.T) *goCache.GenericCache {
	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithFields(logrus.Fields{"method": "newInMemoryLimiter"})

	os.Setenv("NO_REDIS_PASSWORD", "true")
	// os.Setenv("REDIS_READ_ONLY_ADDRESS","redis://localhost:6379")
	// os.Setenv("REDIS_SENTINEL_ADDRESS", "redis://localhost:26379")
	selectDatabase := 3
	cacheWritePool, err := goCache.InitRedisCache(useSentinel, defExpSeconds, nil, defExpSeconds, defExpSeconds, selectDatabase, logger)
	if err != nil {
		t.Errorf("couldn't connect to redis on %s", redisTestServer)
		t.FailNow()
		panic("")
	}
	logger.Info("cacheWritePool initialized")
	readOnlyPool, err := goCache.InitReadOnlyRedisCache("redis://localhost:6379", "", 50, 500, defExpSeconds, maxConnectionsAllowed, selectDatabase, logger)
	if err != nil {
		t.Errorf("couldn't connect to redis on %s", redisTestServer)
		t.FailNow()
		panic("")
	}
	logger.Info("cacheReadPool initialized")
	return goCache.NewCacheWithMultiPools(cacheWritePool, readOnlyPool, goCache.L2, sharedSecret, defExpSeconds, []byte("test"), false)
}

func dumpRateContextToLog(r Context) {
	fmt.Printf(
		"Limit: %v\nCount: %v\nReached: %v\nRemaining: %v\nReset %v\nDelay: %v\nNow: %v\n",
		r.Limit,
		r.Count,
		r.Reached,
		r.Remaining,
		r.Reset,
		r.Delay,
		time.Now().Unix(),
	)
}

package golimiter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	goCache "github.com/Bose/go-cache"
	"github.com/Jim-Lambert-Bose/cache/persistence"
	"github.com/sirupsen/logrus"
)

// limiterStore - define how limiters interact with their state
type limiterStore struct {
	// Prefix used for the key.
	Prefix string
	// cache used to store values in-memory and a mutex for it
	Cache   *goCache.InMemoryStore
	cacheMu sync.Mutex

	// Distributed Cache and a janitor to keep the distributed cache and in memory cache in sync
	DistributedRedisCache     *goCache.GenericCache
	distributedCounterJanitor *janitor

	differedUpdateKeys   map[string]struct{} // all the keys with differed updates since the last central store sync
	differedUpdateKeysMu sync.Mutex

	UnsyncCounterLimit uint64 // how far inmemory and the central store can drift apart unsync'd
	UnsyncTimeLimit    uint64 // how long between inmemory and central store syncs
}

// LimiterStore - encapsulate the limiterStore
type LimiterStore struct {
	*limiterStore
}

const (
	defaultLimiterPrefix           = "limiter" // DefaultLimiterPrefix - default prefix to use for all keys in storage
	lruJanitorCleanupEveryNSeconds = 60        // how often to clean out expired entries from the LRU
	differedUpdateKeysInitalSize   = 25        // side of differedUpdateKeys
)

// traceSync - should we trace syncs to the central store... (very verbose logging)
var traceSync bool
var traceSyncOnce sync.Once

// NewInMemoryLimiterStore creates a new instance of memory store with defaults.
func NewInMemoryLimiterStore(maxEntries int, opt ...Option) *LimiterStore {
	traceSyncOnce.Do(func() {
		// this means you have to set this env before you create any limiterStores
		if len(os.Getenv("TRACE_SYNC")) != 0 {
			traceSync = true
		}
		logSync(logrus.Debugf, "NewInMemoryLimiterStore: traceSync == %v", traceSync)
	})
	opts := defaultOptions
	for _, o := range opt {
		o(&opts)
	}

	if opts.limiterExpiration == defaultLimiterExpSeconds*time.Second {
		// this is a fundamental misunderstanding of how limiters work.  The defaultLimiterExpSeconds is -100 intentionally to catch
		// this sort of configuration issue.  I thought about returning an error, but that could be ignored and then the client would
		// be faced with all sorts of odd behavior and have to struggle to find out why... and it will never work properly with the default Exp.
		// now, I could set a better default (something like 1 sec), but then the client would be caught with a default that doesn't likely
		// match it's limiter.Rate and all sorts of wierd behavior.  In the end, panic is the best solution for this problem.
		panic("you must set WithLimiterExpiration(t time.Duration) or the limiter will not work properly")
	}
	c, err := goCache.NewInMemoryStore(
		maxEntries,
		opts.limiterExpiration,
		time.Duration(lruJanitorCleanupEveryNSeconds)*time.Second,
		false, // no metric created
		"",    // no need for a metric label
	)
	if err != nil {
		panic(err)
	}

	ls := &limiterStore{Prefix: defaultLimiterPrefix,
		Cache:              c,
		differedUpdateKeys: make(map[string]struct{}, differedUpdateKeysInitalSize),
		UnsyncCounterLimit: opts.unsyncCounterLimit,
		UnsyncTimeLimit:    opts.unsyncTimeLimit,
	}

	// This trick ensures that the janitor goroutine (which--granted it
	// was enabled--is running DeleteExpired on c forever) does not keep
	// the returned C object from being garbage collected. When it is
	// garbage collected, the finalizer stops the janitor goroutine, after
	// which c can be collected.
	LS := &LimiterStore{ls}
	if ls.UnsyncTimeLimit > 0 {
		runJanitor(ls, time.Duration(ls.UnsyncTimeLimit)*time.Millisecond)
		runtime.SetFinalizer(LS, stopJanitor)
	}
	return LS
}

// StopCentralStoreUpdates - stop the janitor
func (store *LimiterStore) StopCentralStoreUpdates() bool {
	return stopJanitor(store)
}

// Get returns the limit for given identifier. (and increments the limiter's counter)
// if it's drifted too far from the central store, it will sync to the central store
func (store *LimiterStore) Get(ctx context.Context, key string, rate Rate) (lContext Context, err error) {
	key = fmt.Sprintf("%s:%s", store.Prefix, key)
	now := time.Now()
	// logrus.Debugf("limiter.Get: looking for key == %s", key)
	entry := goCache.GenericCacheEntry{}
	// not sure the lock is needed, but it's blistering fast for now with it.
	store.cacheMu.Lock()
	defer store.cacheMu.Unlock()
	err = store.Cache.Get(key, &entry)
	if err != nil { // can't find existing entry - it's a new entry
		// logrus.Debugf("limiter.Get: didn't find key == %s with error == %s", key, err.Error())
		counter := StoreEntry{CurrentCount: 1, LastSyncedCount: 0}
		if err = store.Cache.Set(key, counter, rate.Period); err != nil {
			// logrus.Errorf("limiter.Get: error setting increment == %s", err.Error())
			return lContext, err
		}
		if err = store.Cache.Get(key, &entry); err != nil {
			// logrus.Errorf("limiter.Get: error getting increment == %s", err.Error())
			return lContext, err
		}
		count, _, delta := EntrySyncInfo(entry) // do not increment, since it's already been done to this new entry
		if delta > store.UnsyncCounterLimit {   // new in-memory entry so handle limiters with UnsyncCounterLimit of 0
			updatedEntry := store.SyncEntryWithCentralStore(key, WithIncrement(0), WithRate(rate)) // WithIncrement 0 since it's already been done to this new entry
			if updatedEntry != nil {
				// if not nil, the in-memory entry was updated and the TTL was set to match the central store.
				// and now we need to Get it for the updated TTL
				if err = store.Cache.Get(key, &entry); err != nil {
					// logrus.Errorf("limiter.Get: error getting increment == %s", err.Error())
					return lContext, err
				}
				lctx := GetContextFromState(now, rate, time.Unix(entry.ExpiresAt, 0), int64(updatedEntry.(int64)))
				return lctx, nil
			}
		}
		// we've differed the central store update to an async batch op
		store.addDifferedUpdateKey(key)
		lctx := GetContextFromState(now, rate, time.Unix(entry.ExpiresAt, 0), int64(count))
		return lctx, nil
	}
	// okay, it's an existing entry in-memory
	existingCount, existingLastSyncCount, delta := EntrySyncInfo(entry)
	if delta+1 > store.UnsyncCounterLimit { // we needed to add 1 to the delta returned from the existing entry
		updatedEntry := store.SyncEntryWithCentralStore(key, WithIncrement(1), WithRate(rate)) // this will update the in-memory counter if needed.
		if updatedEntry != nil {
			// if not nil, the in-memory entry was updated and the TTL was set to match the central store.
			// and now we need to Get it for the updated TTL
			if err = store.Cache.Get(key, &entry); err != nil {
				// logrus.Errorf("limiter.Get: error getting increment == %s", err.Error())
				return lContext, err
			}
			lctx := GetContextFromState(now, rate, time.Unix(entry.ExpiresAt, 0), int64(updatedEntry.(int64)))
			return lctx, nil
		}
	}
	// we've differed the central store update to an async batch op
	entry.Data = StoreEntry{CurrentCount: existingCount + 1, LastSyncedCount: existingLastSyncCount}
	err = store.Cache.Update(key, entry)
	if err != nil {
		logrus.Errorf("limiter.Get: error incrementing == %s", err.Error())
		return lContext, err
	}
	store.addDifferedUpdateKey(key)
	count := entry.Data.(StoreEntry).CurrentCount
	lctx := GetContextFromState(now, rate, time.Unix(entry.ExpiresAt, 0), int64(count))
	return lctx, nil
}

// Peek returns the limit for given identifier, without modification on current values.
func (store *LimiterStore) Peek(ctx context.Context, key string, rate Rate) (lContext Context, err error) {
	key = fmt.Sprintf("%s:%s", store.Prefix, key)
	now := time.Now()

	var entry goCache.GenericCacheEntry
	err = store.Cache.Get(key, &entry)
	if err != nil {
		logrus.Errorf("limiter.Peek: error getting entry for %s - %s", key, err.Error())
		return lContext, err
	}
	count := entry.Data.(StoreEntry).CurrentCount
	lctx := GetContextFromState(now, rate, time.Unix(0, entry.ExpiresAt), int64(count))
	return lctx, nil
}

var (
	// ErrNoCentralStore - no central store for the rate limiter was initialized
	ErrNoCentralStore = errors.New("no central limiter store initialized")
)

// CentralStorePeek - peek at a value in the distributed central store
func (store *limiterStore) CentralStorePeek(key string) (uint64, error) {
	if store.DistributedRedisCache == nil {
		return 0, ErrNoCentralStore
	}
	key = fmt.Sprintf("%s:%s", store.Prefix, key)
	var v uint64
	err := store.DistributedRedisCache.Get(key, &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// addDifferedUpdateKey simply adds a key to the list of differed central store updates
func (store *limiterStore) addDifferedUpdateKey(key string) {
	store.differedUpdateKeysMu.Lock()
	_, ok := store.differedUpdateKeys[key]
	if !ok {
		store.differedUpdateKeys[key] = struct{}{}
	}
	store.differedUpdateKeysMu.Unlock()
}

// resetDifferedUpdateKeys compiles and returns the list of differed updates and then resets the queue
func (store *limiterStore) resetDifferedUpdateKeys() []string {
	store.differedUpdateKeysMu.Lock()
	var keys []string
	for k := range store.differedUpdateKeys {
		keys = append(keys, k)
	}
	store.differedUpdateKeys = make(map[string]struct{}, differedUpdateKeysInitalSize)
	store.differedUpdateKeysMu.Unlock()
	return keys
}

// CentralStoreUpdates is used by the janitor.Run to "do" the updates via background workers
func (store *limiterStore) CentralStoreUpdates() error {
	// logrus.Debugf("CentralStoreUpdates: starting")
	// if store.DistributedRedisCache == nil {
	// 	logrus.Debugf("CentralStoreUpdates: done (no store)")
	// 	return ErrNoCentralStore
	// }
	// logrus.Debugf("CentralStoreUpdates: workers setup")
	keys := store.resetDifferedUpdateKeys()
	// logrus.Debugf("CentralStoreUpdates: update %d keys", len(keys))
	if len(keys) == 0 {
		return nil
	}
	// logrus.Debugf("CentralStoreUpdates: starting goroutines %d", runtime.NumGoroutine())
	var wg sync.WaitGroup
	concurrency := 5
	in := make(chan string)
	for x := 0; x < concurrency; x++ {
		wg.Add(1)
		go worker(store, in, x, &wg)
	}

	for _, k := range keys {
		in <- k
	}
	close(in)
	wg.Wait()
	// logrus.Debugf("CentralStoreUpdates: stopping goroutines %d", runtime.NumGoroutine())
	// logrus.Debugf("CentralStoreUpdates: done")
	return nil

}

// CentralStoreSync - orchestrator for keeping the stores in sync that uses concurrency to get stuff done
// this syncs every entry, every time... see CentralStoreUpdates() which is far more efficient
func (store *limiterStore) CentralStoreSync() error {
	// logrus.Debugf("CentralStoreSync: starting")
	if store.DistributedRedisCache == nil {
		return ErrNoCentralStore
	}
	var wg sync.WaitGroup
	concurrency := 10
	in := make(chan string)
	for x := 0; x < concurrency; x++ {
		wg.Add(1)
		go worker(store, in, x, &wg)
	}
	// logrus.Debugf("CentralStoreSync: workers setup")
	for _, k := range store.Cache.Keys() {
		key := k.(string)
		in <- key
	}
	close(in)
	wg.Wait()
	// logrus.Debugf("CentralStoreSync: done")
	return nil
}

// worker - is a concurrent for the central store sync
func worker(store *limiterStore, in chan string, workerNum int, wg *sync.WaitGroup) {
	for key := range in {
		store.SyncEntryWithCentralStore(key, WithWorkerNumber(workerNum))
	}
	wg.Done()
	// logrus.Debugf("goroutines %d / worker %d done", runtime.NumGoroutine(), workerNum)
}

// EntrySyncInfo - delta between last sync count
func EntrySyncInfo(e goCache.GenericCacheEntry) (currentCount int, lastSyncCount int, delta uint64) {
	storeEntry := e.Data.(StoreEntry)
	return storeEntry.CurrentCount, storeEntry.LastSyncedCount, uint64(storeEntry.CurrentCount - storeEntry.LastSyncedCount)
}

// EntryCentralStoreExpiresIn retrieves the entries TTL from the central store (if the limiter has one)
func (store *limiterStore) EntryCentralStoreExpiresIn(key string) (int64, error) {
	if store.DistributedRedisCache == nil {
		return 0, ErrNoCentralStore
	}
	return store.DistributedRedisCache.RedisGetExpiresIn(key)
}

// SyncEntryWithCentralStore - figures out what to sync for one entry and sets the in memory TTL to match the central store TTL
func (store *limiterStore) SyncEntryWithCentralStore(key string, opt ...Option) (updatedInMemoryCounter interface{}) {
	if store.DistributedRedisCache == nil {
		return nil // nada a central store to sync with
	}
	opts := defaultOptions
	for _, o := range opt {
		o(&opts)
	}
	var redisTTL int64
	var redisCounter uint64
	var memoryCounterEntry goCache.GenericCacheEntry
	err := store.Cache.Get(key, &memoryCounterEntry)
	if err != nil {
		logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: error fetching in memory key %s: %s", opts.workerNumber, key, err.Error())
		return nil // signal that we didn't do anything to the central store
	}
	foundRedisEntry := false // we'll use this later to signal if we found the entry in Redis, or it's going to be a new entry in Redis
	err = store.DistributedRedisCache.Get(key, &redisCounter)
	if err != nil {
		logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: redis cache miss for key: %s - %s", opts.workerNumber, key, err.Error())
	} else {
		foundRedisEntry = true
	}
	if redisCounter != 0 {
		// so there's a counter for this limiter in Redis, let's find out it's TTL so we can manage it appropriately..
		// if the TTL is really low (<5ms) then we will reset our in memory counter to 0 and give it a brand new TTL
		logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: key %s has counter == %d", opts.workerNumber, key, redisCounter)
		redisTTL, err = store.DistributedRedisCache.RedisGetExpiresIn(key)
		if err != nil {
			// refactored this error into it's own func (since it's pretty long 30 LOC)
			return store.syncEntryWithCentralStore_ErrOnRedisGetExpiresIn(err, key, opts)
		}
		logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: key %s has a TTL of %d milliseconds", opts.workerNumber, key, redisTTL)
	}
	if foundRedisEntry && redisTTL < int64(store.UnsyncTimeLimit) { // less than the next time we will sync (default 500ms).. so we'll treat it like it's expired
		if err := store.Cache.Delete(key); err != nil {
			logrus.Errorf("SyncEntryWithCentralStore: error resetting in-memory entry (%s) via delete", key)
			return nil // signal that we didn't do anything to the central store
		}
		return int64(0) // we successfully set it to 0 with the default exp for the store
	}

	// okay... we've got a new redis counter or it's an existing redis counter and it's not expired
	memoryCounter, memoryLastSynced, delta := EntrySyncInfo(memoryCounterEntry)
	memoryCounter = memoryCounter + opts.increment // gotta add in the increment passed in (which can be 0 or N)
	delta = delta + uint64(opts.increment)         // gotta add in the increment passed in (which can be 0 or N)

	logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: current values for key/mem/synced/redis ==  %s/%d/%d/%d", opts.workerNumber, key, memoryCounter, memoryLastSynced, redisCounter)
	if (redisCounter + delta) < uint64(memoryCounter) {
		delta = uint64(memoryCounter) - redisCounter // makes sure the delta increases the redis value to at least the in memory value
	}
	if delta == 0 {
		// even with no delta, we need to update the in memory TTL to match the redisTTL
		if redisTTL > 1000 { // we only care about seconds for inmemory exp
			newExp := time.Duration(redisTTL) * time.Millisecond
			newMemoryCounter := StoreEntry{CurrentCount: memoryCounter, LastSyncedCount: memoryCounter}
			if err := store.Cache.Set(key, newMemoryCounter, newExp); err != nil {
				logrus.Errorf("SyncEntryWithCentralStore: error setting in-memory entry (%s) to value of: %d with new exp of: %v", key, memoryCounter, newExp)
				return memoryCounter
			}
			logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: delta == 0 so in memory entry (%s) set to valude of: %d with a new exp of: %v", opts.workerNumber, key, memoryCounter, newExp)
			return memoryCounter
		}
	}

	newRedisValue, err := store.DistributedRedisCache.RedisIncrementAtomic(key, delta)
	if err != nil {
		logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: error incrementing %s - %s", opts.workerNumber, key, err.Error())
		return nil // can't update inmemory since it didn't get to redis... signal that we didn't do anything to the central store
	}
	if !foundRedisEntry { // it's a new entry, so we have to set it's TTL to the inmemory TTL
		err = store.DistributedRedisCache.RedisExpireAt(key, uint64(memoryCounterEntry.ExpiresAt))
		if err != nil {
			logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: error updating expiry on redis entry %s - %s", opts.workerNumber, key, err.Error())
			// gotta delete it then otherwise it's a stuck limiter... it will get updated the next time if this fails though
			err := store.DistributedRedisCache.Delete(key)
			if err != nil {
				logrus.Errorf("SyncEntryWithCentralStore worker %d: error deleting entry %s with no expiry - %s", opts.workerNumber, key, err.Error())
			}
		}
		// we don't need to update expiredAt, just the counts
		memoryCounterEntry.Data = StoreEntry{CurrentCount: int(newRedisValue), LastSyncedCount: int(newRedisValue)}
		if err := store.Cache.Update(key, memoryCounterEntry); err != nil {
			logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: error update in memory %s - %s", opts.workerNumber, key, err.Error())
			// we had an err updating inmemory, but the redis store was updated... most likely cause: inmemory entry was expired or evicted
			// so we are going to count this as success (so we do NOT return nil here and let the newRedisValue be returned after this err path)
		}
		return int64(newRedisValue)
	}
	memoryCounterEntry.Data = StoreEntry{CurrentCount: int(newRedisValue), LastSyncedCount: int(newRedisValue)}
	memoryCounterEntry.ExpiresAt = store.convertToExpiresAt(opts.workerNumber, redisTTL)
	if err := store.Cache.Update(key, memoryCounterEntry); err != nil {
		logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: error update in memory %s - %s", opts.workerNumber, key, err.Error())
		// we had an err updating inmemory, but the redis store was updated... most likely cause: inmemory entry was expired or evicted
		// so we are going to count this as success
		return int64(newRedisValue)
	}
	logSync(logrus.Debugf, "SyncEntryWithCentralStore goroutines %d / worker %d: successful sync of %s", runtime.NumGoroutine(), opts.workerNumber, key)
	return int64(newRedisValue)
}

// syncEntryWithCentralStore_ErrOnRedisGetExpiresIn is just a refactoring of this specific error encountered during the sync of an entry
func (store *limiterStore) syncEntryWithCentralStore_ErrOnRedisGetExpiresIn(err error, key string, opts options) (updatedInMemoryCounter interface{}) {
	if err == persistence.ErrCacheMiss {
		// it just expired, so reset inmemory by deleting key
		if err := store.Cache.Delete(key); err != nil {
			logrus.Errorf("SyncEntryWithCentralStore: error resetting in-memory entry (%s) via delete", key)
			return nil // signal that we didn't do anything to the central store
		}
		return int64(0) // we successfully set it to 0 with the default exp for the store
	}
	if err == persistence.ErrCacheNoTTL {
		logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: key %s has no TTL, so deleting it", opts.workerNumber, key)
		// gotta delete it then otherwise it's a stuck limiter... it will get updated the next time if this fails though
		err := store.DistributedRedisCache.Delete(key)
		if err != nil {
			// we'll have to try again next time, if it still has no TTL... but for now, we'll keep going with it as-is
			// and signal that we did nothing to the central store counter
			logrus.Errorf("SyncEntryWithCentralStore worker %d: error deleting entry %s with no expiry - %s", opts.workerNumber, key, err.Error())
			return nil
		} else {
			// we deleted the key with no TTL (effectively setting the central store to zero)
			logSync(logrus.Errorf, "SyncEntryWithCentralStore worker %d: key %s with no TTL was successfully deleted", opts.workerNumber, key)
			// I think, the right thing is to just signal that we've done nothing to the central store.
			return nil
		}
	}
	// so, we didn't miss the key or hit a key with no TTL... it's some other type of error
	logrus.Errorf("SyncEntryWithCentralStore worker %d: error getting the key (%s) ttl - %s", opts.workerNumber, key, err.Error())
	return nil
}

// convertToExpiresAt takes a TTL from Redis and converts it to an EPOC exipresAt
func (store *limiterStore) convertToExpiresAt(workerNum int, redisTTL int64) int64 {
	now := time.Now().Unix()
	q, r := redisTTL/1000, redisTTL%1000
	if r > int64(store.UnsyncTimeLimit) {
		q = q + 1
	}
	e := now + q
	logSync(logrus.Debugf, "SyncEntryWithCentralStore worker %d: redisTTL %d / quotient %d / remainder %d / now %d / expiresAt %d ", workerNum, redisTTL, q, r, now, e)
	return e
}

// logSync just figures out if it should log based on TraceSync
func logSync(logger func(format string, args ...interface{}), format string, args ...interface{}) {
	if traceSync {
		logger(format, args...)
	}
}

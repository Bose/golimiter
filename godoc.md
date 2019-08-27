

# golimiter
`import "github.com/Bose/golimiter"`

* [Overview](#pkg-overview)
* [Index](#pkg-index)
* [Subdirectories](#pkg-subdirectories)

## <a name="pkg-overview">Overview</a>



## <a name="pkg-index">Index</a>
* [Variables](#pkg-variables)
* [func EntrySyncInfo(e goCache.GenericCacheEntry) (currentCount int, lastSyncCount int, delta uint64)](#EntrySyncInfo)
* [type Context](#Context)
  * [func GetContextFromState(now time.Time, rate Rate, expiration time.Time, count int64) Context](#GetContextFromState)
* [type Limiter](#Limiter)
  * [func New(store Store, rate Rate) *Limiter](#New)
  * [func (l *Limiter) Get(ctx context.Context, key string) (Context, error)](#Limiter.Get)
  * [func (l *Limiter) Peek(ctx context.Context, key string) (Context, error)](#Limiter.Peek)
* [type LimiterStore](#LimiterStore)
  * [func NewInMemoryLimiterStore(maxEntries int, opt ...Option) *LimiterStore](#NewInMemoryLimiterStore)
  * [func (store LimiterStore) CentralStorePeek(key string) (uint64, error)](#LimiterStore.CentralStorePeek)
  * [func (store LimiterStore) CentralStoreSync() error](#LimiterStore.CentralStoreSync)
  * [func (store LimiterStore) CentralStoreUpdates() error](#LimiterStore.CentralStoreUpdates)
  * [func (store LimiterStore) EntryCentralStoreExpiresIn(key string) (int64, error)](#LimiterStore.EntryCentralStoreExpiresIn)
  * [func (store *LimiterStore) Get(ctx context.Context, key string, rate Rate) (lContext Context, err error)](#LimiterStore.Get)
  * [func (store *LimiterStore) Peek(ctx context.Context, key string, rate Rate) (lContext Context, err error)](#LimiterStore.Peek)
  * [func (store *LimiterStore) StopCentralStoreUpdates() bool](#LimiterStore.StopCentralStoreUpdates)
  * [func (store LimiterStore) SyncEntryWithCentralStore(key string, opt ...Option) (updatedInMemoryCounter interface{})](#LimiterStore.SyncEntryWithCentralStore)
* [type Option](#Option)
  * [func WithIncrement(i int) Option](#WithIncrement)
  * [func WithLimiterExpiration(exp time.Duration) Option](#WithLimiterExpiration)
  * [func WithRate(r Rate) Option](#WithRate)
  * [func WithUnsyncCounterLimit(deltaLimit uint64) Option](#WithUnsyncCounterLimit)
  * [func WithUnsyncTimeLimit(timeLimit uint64) Option](#WithUnsyncTimeLimit)
  * [func WithWorkerNumber(worker int) Option](#WithWorkerNumber)
* [type Rate](#Rate)
  * [func NewRateFromFormatted(formatted string) (Rate, error)](#NewRateFromFormatted)
* [type Store](#Store)
* [type StoreEntry](#StoreEntry)
* [type StoreOptions](#StoreOptions)


#### <a name="pkg-files">Package files</a>
[context.go](/src/github.com/Bose/golimiter/context.go) [golimiter.go](/src/github.com/Bose/golimiter/golimiter.go) [inmemory.go](/src/github.com/Bose/golimiter/inmemory.go) [janitor.go](/src/github.com/Bose/golimiter/janitor.go) [options.go](/src/github.com/Bose/golimiter/options.go) [rate.go](/src/github.com/Bose/golimiter/rate.go) [store.go](/src/github.com/Bose/golimiter/store.go) 



## <a name="pkg-variables">Variables</a>
``` go
var (
    // ErrNoCentralStore - no central store for the rate limiter was initialized
    ErrNoCentralStore = errors.New("no central limiter store initialized")
)
```


## <a name="EntrySyncInfo">func</a> [EntrySyncInfo](/src/target/inmemory.go?s=11484:11583#L303)
``` go
func EntrySyncInfo(e goCache.GenericCacheEntry) (currentCount int, lastSyncCount int, delta uint64)
```
EntrySyncInfo - delta between last sync count




## <a name="Context">type</a> [Context](/src/target/context.go?s=72:196#L8)
``` go
type Context struct {
    Limit     int64
    Count     int64
    Remaining int64
    Reset     int64
    Reached   bool
    Delay     int64
}

```
Context is the limit context.







### <a name="GetContextFromState">func</a> [GetContextFromState](/src/target/context.go?s=262:355#L18)
``` go
func GetContextFromState(now time.Time, rate Rate, expiration time.Time, count int64) Context
```
GetContextFromState generate a new Context from given state.





## <a name="Limiter">type</a> [Limiter](/src/target/golimiter.go?s=78:126#L8)
``` go
type Limiter struct {
    Store Store
    Rate  Rate
}

```
Limiter is the limiter instance.







### <a name="New">func</a> [New](/src/target/golimiter.go?s=167:208#L14)
``` go
func New(store Store, rate Rate) *Limiter
```
New returns an instance of Limiter.





### <a name="Limiter.Get">func</a> (\*Limiter) [Get](/src/target/golimiter.go?s=313:384#L22)
``` go
func (l *Limiter) Get(ctx context.Context, key string) (Context, error)
```
Get returns the limit for given identifier.




### <a name="Limiter.Peek">func</a> (\*Limiter) [Peek](/src/target/golimiter.go?s=516:588#L27)
``` go
func (l *Limiter) Peek(ctx context.Context, key string) (Context, error)
```
Peek returns the limit for given identifier, without modification on current values.




## <a name="LimiterStore">type</a> [LimiterStore](/src/target/inmemory.go?s=1012:1055#L37)
``` go
type LimiterStore struct {
    // contains filtered or unexported fields
}

```
LimiterStore - encapsulate the limiterStore







### <a name="NewInMemoryLimiterStore">func</a> [NewInMemoryLimiterStore](/src/target/inmemory.go?s=1570:1643#L52)
``` go
func NewInMemoryLimiterStore(maxEntries int, opt ...Option) *LimiterStore
```
NewInMemoryLimiterStore creates a new instance of memory store with defaults.





### <a name="LimiterStore.CentralStorePeek">func</a> (LimiterStore) [CentralStorePeek](/src/target/inmemory.go?s=8310:8381#L200)
``` go
func (store LimiterStore) CentralStorePeek(key string) (uint64, error)
```
CentralStorePeek - peek at a value in the distributed central store




### <a name="LimiterStore.CentralStoreSync">func</a> (LimiterStore) [CentralStoreSync](/src/target/inmemory.go?s=10568:10619#L270)
``` go
func (store LimiterStore) CentralStoreSync() error
```
CentralStoreSync - orchestrator for keeping the stores in sync that uses concurrency to get stuff done
this syncs every entry, every time... see CentralStoreUpdates() which is far more efficient




### <a name="LimiterStore.CentralStoreUpdates">func</a> (LimiterStore) [CentralStoreUpdates](/src/target/inmemory.go?s=9467:9521#L236)
``` go
func (store LimiterStore) CentralStoreUpdates() error
```
CentralStoreUpdates is used by the janitor.Run to "do" the updates via background workers




### <a name="LimiterStore.EntryCentralStoreExpiresIn">func</a> (LimiterStore) [EntryCentralStoreExpiresIn](/src/target/inmemory.go?s=11850:11930#L309)
``` go
func (store LimiterStore) EntryCentralStoreExpiresIn(key string) (int64, error)
```
EntryCentralStoreExpiresIn retrieves the entries TTL from the central store (if the limiter has one)




### <a name="LimiterStore.Get">func</a> (\*LimiterStore) [Get](/src/target/inmemory.go?s=4073:4177#L111)
``` go
func (store *LimiterStore) Get(ctx context.Context, key string, rate Rate) (lContext Context, err error)
```
Get returns the limit for given identifier. (and increments the limiter's counter)
if it's drifted too far from the central store, it will sync to the central store




### <a name="LimiterStore.Peek">func</a> (\*LimiterStore) [Peek](/src/target/inmemory.go?s=7553:7658#L179)
``` go
func (store *LimiterStore) Peek(ctx context.Context, key string, rate Rate) (lContext Context, err error)
```
Peek returns the limit for given identifier, without modification on current values.




### <a name="LimiterStore.StopCentralStoreUpdates">func</a> (\*LimiterStore) [StopCentralStoreUpdates](/src/target/inmemory.go?s=3812:3869#L105)
``` go
func (store *LimiterStore) StopCentralStoreUpdates() bool
```
StopCentralStoreUpdates - stop the janitor




### <a name="LimiterStore.SyncEntryWithCentralStore">func</a> (LimiterStore) [SyncEntryWithCentralStore](/src/target/inmemory.go?s=12197:12313#L317)
``` go
func (store LimiterStore) SyncEntryWithCentralStore(key string, opt ...Option) (updatedInMemoryCounter interface{})
```
SyncEntryWithCentralStore - figures out what to sync for one entry and sets the in memory TTL to match the central store TTL




## <a name="Option">type</a> [Option](/src/target/options.go?s=474:500#L12)
``` go
type Option func(*options)
```
Option - defines a func interface for passing in options to the NewInMemoryLimiterStore()







### <a name="WithIncrement">func</a> [WithIncrement](/src/target/options.go?s=2161:2193#L72)
``` go
func WithIncrement(i int) Option
```
WithIncrement passes an optional increment


### <a name="WithLimiterExpiration">func</a> [WithLimiterExpiration](/src/target/options.go?s=1141:1193#L34)
``` go
func WithLimiterExpiration(exp time.Duration) Option
```
WithLimiterExpSeconds sets the limiter's expiry in seconds


### <a name="WithRate">func</a> [WithRate](/src/target/options.go?s=2038:2066#L65)
``` go
func WithRate(r Rate) Option
```
WithRate passes an optional rate


### <a name="WithUnsyncCounterLimit">func</a> [WithUnsyncCounterLimit](/src/target/options.go?s=1392:1445#L41)
``` go
func WithUnsyncCounterLimit(deltaLimit uint64) Option
```
WithUnsyncCounterLimit - allows you to override the default limit to how far the in memory counter can drift from the central store


### <a name="WithUnsyncTimeLimit">func</a> [WithUnsyncTimeLimit](/src/target/options.go?s=1704:1753#L49)
``` go
func WithUnsyncTimeLimit(timeLimit uint64) Option
```
WithUnsyncTimeLimit - allows you to override the default time limit (milliseconds) between syncs to the central store.
this cannot be 0 or it will default to defaultUnsyncTimeLimit


### <a name="WithWorkerNumber">func</a> [WithWorkerNumber](/src/target/options.go?s=1900:1940#L58)
``` go
func WithWorkerNumber(worker int) Option
```
WithWorkerNumber passes an optional worker number





## <a name="Rate">type</a> [Rate](/src/target/rate.go?s=119:216#L12)
``` go
type Rate struct {
    Formatted string
    Period    time.Duration
    Limit     int64
    Delay     int64
}

```
Rate is the rate == 1-1-S-50







### <a name="NewRateFromFormatted">func</a> [NewRateFromFormatted](/src/target/rate.go?s=291:348#L20)
``` go
func NewRateFromFormatted(formatted string) (Rate, error)
```
NewRateFromFormatted - returns the rate from the formatted version ()





## <a name="Store">type</a> [Store](/src/target/store.go?s=103:1125#L9)
``` go
type Store interface {
    // Get increments the limit for a given identtfier and returns its context
    Get(ctx context.Context, key string, rate Rate) (Context, error)
    // Peek returns the limit for given identifier, without modification on current values.
    Peek(ctx context.Context, key string, rate Rate) (Context, error)
    // CentrailStorePeek - Peek at an entry in the central store
    CentralStorePeek(key string) (uint64, error)
    // CentralStoreSync - syncs all the keys from in memory with the central store
    CentralStoreSync() error
    // CentralStoreUpdates - just sync the updated keys from in memory with the central store
    CentralStoreUpdates() error
    // SyncEntryWithCentralStore - sync just one key from in memory with the central store and return the new entry IF it's updated
    SyncEntryWithCentralStore(key string, opt ...Option) interface{}
    // StopCentralStoreUpdates - stop all janitor syncing to central store and return whether or not the message to stop as successfully sent
    StopCentralStoreUpdates() bool
}
```
Store is the common interface for limiter stores.










## <a name="StoreEntry">type</a> [StoreEntry](/src/target/store.go?s=1468:1536#L39)
``` go
type StoreEntry struct {
    CurrentCount    int
    LastSyncedCount int
}

```
StoreEntry - represent the entry in the store










## <a name="StoreOptions">type</a> [StoreOptions](/src/target/store.go?s=1166:1417#L27)
``` go
type StoreOptions struct {
    // Prefix is the prefix to use for the key.
    Prefix string

    // MaxRetry is the maximum number of retry under race conditions.
    MaxRetry int

    // CleanUpInterval is the interval for cleanup.
    CleanUpInterval time.Duration
}

```
StoreOptions are options for store.














- - -
Generated by [godoc2md](http://godoc.org/github.com/davecheney/godoc2md)

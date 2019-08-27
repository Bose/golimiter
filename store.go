package golimiter

import (
	"context"
	"time"
)

// Store is the common interface for limiter stores.
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

// StoreOptions are options for store.
type StoreOptions struct {
	// Prefix is the prefix to use for the key.
	Prefix string

	// MaxRetry is the maximum number of retry under race conditions.
	MaxRetry int

	// CleanUpInterval is the interval for cleanup.
	CleanUpInterval time.Duration
}

// StoreEntry - represent the entry in the store
type StoreEntry struct {
	CurrentCount    int
	LastSyncedCount int
}

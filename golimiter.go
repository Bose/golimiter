package golimiter

import (
	"context"
)

// Limiter is the limiter instance.
type Limiter struct {
	Store Store
	Rate  Rate
}

// New returns an instance of Limiter.
func New(store Store, rate Rate) *Limiter {
	return &Limiter{
		Store: store,
		Rate:  rate,
	}
}

// Get returns the limit for given identifier.
func (l *Limiter) Get(ctx context.Context, key string) (Context, error) {
	return l.Store.Get(ctx, key, l.Rate)
}

// Peek returns the limit for given identifier, without modification on current values.
func (l *Limiter) Peek(ctx context.Context, key string) (Context, error) {
	return l.Store.Peek(ctx, key, l.Rate)
}

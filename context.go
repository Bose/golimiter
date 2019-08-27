package golimiter

import (
	"time"
)

// Context is the limit context.
type Context struct {
	Limit     int64
	Count     int64
	Remaining int64
	Reset     int64
	Reached   bool
	Delay     int64
}

// GetContextFromState generate a new Context from given state.
func GetContextFromState(now time.Time, rate Rate, expiration time.Time, count int64) Context {
	limit := rate.Limit
	remaining := int64(0)
	reached := true
	delayResp := rate.Delay

	if count <= limit {
		remaining = limit - count
		reached = false
	}

	reset := expiration.Unix()

	return Context{
		Limit:     limit,
		Count:     count,
		Remaining: remaining,
		Reset:     reset,
		Reached:   reached,
		Delay:     delayResp,
	}
}

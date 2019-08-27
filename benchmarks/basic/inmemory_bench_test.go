package benchmarks

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Bose/golimiter"

	"github.com/google/uuid"
)

var benchInMemoryStore *golimiter.Limiter
var benchLimit int64
var benchKey string

func init() {
	benchLimit = 10000000
	benchKey = uuid.New().String()
	benchInMemoryStore = newBenchStoreInMemory(strconv.Itoa(int(benchLimit)) + "-1-S")
}
func benchmarkTypicalIncrInMemory(i int, b *testing.B) {
	for n := 0; n < b.N; n++ {
		typicalBenchRateIncr(b, benchInMemoryStore, benchKey)
	}
}

func BenchmarkIncrInMemory1(b *testing.B)  { benchmarkTypicalIncrInMemory(1, b) }
func BenchmarkIncrInMemory2(b *testing.B)  { benchmarkTypicalIncrInMemory(2, b) }
func BenchmarkIncrInMemory3(b *testing.B)  { benchmarkTypicalIncrInMemory(3, b) }
func BenchmarkIncrInMemory10(b *testing.B) { benchmarkTypicalIncrInMemory(10, b) }
func BenchmarkIncrInMemory20(b *testing.B) { benchmarkTypicalIncrInMemory(20, b) }
func BenchmarkIncrInMemory40(b *testing.B) { benchmarkTypicalIncrInMemory(40, b) }

const (
	maxBenchEntries = 100
)

func newBenchStoreInMemory(rateLimit string) *golimiter.Limiter {
	rate, err := golimiter.NewRateFromFormatted(rateLimit)
	if err != nil {
		panic(err)
	}
	return golimiter.New(golimiter.NewInMemoryLimiterStore(maxBenchEntries, golimiter.WithLimiterExpiration(1*time.Second)), rate)
}

func typicalBenchRateIncr(b *testing.B, c *golimiter.Limiter, key string) {
	ctx := context.Background()
	rateContext, err := c.Get(ctx, key)
	if err != nil {
		return // this is okay..
	}
	if rateContext.Limit != benchLimit {
		b.Errorf("Expect %d for Limit and got %d", benchLimit, rateContext.Limit)
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

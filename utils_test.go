package golimiter

import (
	"net"
	"testing"
	"time"

	"github.com/Bose/minisentinel"
	"github.com/alicebob/miniredis"
)

func testRedisPorts(t *testing.T) error {
	const timeout = 50
	conn, err := net.DialTimeout("tcp", "localhost:6379", 50*time.Millisecond)
	if err != nil {
		t.Log("Redis Connection error:", err)
		return err
	}
	t.Log("Redis connection successful")
	conn.Close()

	conn2, err := net.DialTimeout("tcp", "localhost:26379", 50*time.Millisecond)
	if err != nil {
		t.Log("Sentinel Connection error:", err)
		return err
	}
	t.Log("Sentinel connection successful")
	conn2.Close()
	return nil
}

type testRedis struct {
	m *miniredis.Miniredis
	s *minisentinel.Sentinel
}

func (t testRedis) Close() {
	if t.m != nil {
		t.m.Close()
	}
	if t.s != nil {
		t.s.Close()
	}
}

func initTestRedis(t *testing.T) (testRedis, error) {
	if err := testRedisPorts(t); err == nil {
		t.Log("Using Redis/Sentinel that's already running...")
		return testRedis{}, err
	}
	m := miniredis.NewMiniRedis()
	// m.RequireAuth("super-secret") // not required, but demonstrates how-to
	err := m.StartAddr(":6379")
	if err != nil {
		return testRedis{}, err
	}
	s := minisentinel.NewSentinel(m, minisentinel.WithReplica(m))
	err = s.StartAddr(":26379")
	if err != nil {
		m.Close()
		return testRedis{}, err
	}
	t.Log("Using miniredis and minisentinel...")

	return testRedis{m: m, s: s}, nil
}

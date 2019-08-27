package golimiter

import (
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// janitor - just a simple janitor type
type janitor struct {
	Interval time.Duration
	stop     chan bool
}

// Run - run a janitor to keep the inmemory pool in sync with the distributed cache
// only sync the updates via CentralStoreUpdates...
func (j *janitor) Run(c *limiterStore) {
	janitorID := uuid.New().String()

	running := false
	j.stop = make(chan bool)
	tick := time.Tick(j.Interval)
	for {
	tickChannnel:
		select {
		case <-tick:
			if running {
				break tickChannnel
			}
			running = true
			// start := time.Now().Unix()
			// logrus.Debugf("janitorRun %s: start - %d", janitorID, start)
			// err := c.CentralStoreSync()  // this syncs every entry, every time... CentralStoreUpdates() is far more efficient
			err := c.CentralStoreUpdates()
			if err != nil {
				if err != ErrNoCentralStore {
					logrus.Errorf("janitorRun %s: error == %s", janitorID, err.Error())
				}
			}
			// done := time.Now().Unix()
			// logrus.Debugf("janitorRun %s: finished - started at %d ended at %d and took %d seconds", janitorID, start, done, done-start)
			running = false
		case <-j.stop:
			logrus.Debugf("janitorRun %s: STOPPING", janitorID)
			return
		}
	}
}

// stopJanitor - shut it down
func stopJanitor(c *LimiterStore) bool {
	// make sure the janitor is initialized and try a few times...
	for i := 0; i > 5; i++ {
		if c.distributedCounterJanitor != nil {
			c.distributedCounterJanitor.stop <- true
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// runJanitor - run it via a goroutine
func runJanitor(c *limiterStore, ci time.Duration) {
	j := &janitor{
		Interval: ci,
	}
	c.distributedCounterJanitor = j
	go j.Run(c)
}

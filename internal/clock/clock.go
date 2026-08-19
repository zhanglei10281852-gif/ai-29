package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

type Fixed struct {
	mu  sync.RWMutex
	now time.Time
}

func NewFixed(now time.Time) *Fixed {
	return &Fixed{now: now.UTC()}
}

func (c *Fixed) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *Fixed) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

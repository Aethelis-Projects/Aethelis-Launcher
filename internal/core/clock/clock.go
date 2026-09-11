package clock

import (
	"time"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type RealClock struct{}

func NewRealClock() ports.Clock {
	return &RealClock{}
}

func (c *RealClock) Now() time.Time {
	return time.Now()
}

func (c *RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

func (c *RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

type MockClock struct {
	CurrentTime time.Time
}

func NewMockClock(t time.Time) *MockClock {
	return &MockClock{CurrentTime: t}
}

func (c *MockClock) Now() time.Time {
	return c.CurrentTime
}

func (c *MockClock) Since(t time.Time) time.Duration {
	return c.CurrentTime.Sub(t)
}

func (c *MockClock) Sleep(d time.Duration) {
	c.CurrentTime = c.CurrentTime.Add(d)
}

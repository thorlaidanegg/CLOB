package circuit

import (
	"time"

	"github.com/thorlaidanegg/clob/types"
)

const maxSamples = 10_000

// PriceSample is a single (price, timestamp) data point in the rolling window.
type PriceSample struct {
	Price     types.Decimal
	Timestamp int64
}

// RollingWindow maintains a bounded circular buffer of recent price samples.
// Samples older than the configured duration are evicted on each Add call.
type RollingWindow struct {
	buffer   []PriceSample
	head     int
	size     int
	duration time.Duration
}

// NewRollingWindow creates a window that retains samples within the given duration.
func NewRollingWindow(duration time.Duration) *RollingWindow {
	return &RollingWindow{
		buffer:   make([]PriceSample, maxSamples),
		duration: duration,
	}
}

// Add records a new price sample and evicts stale entries.
func (w *RollingWindow) Add(price types.Decimal, now int64) {
	w.buffer[w.head] = PriceSample{Price: price, Timestamp: now}
	w.head = (w.head + 1) % maxSamples

	if w.size < maxSamples {
		w.size++
	}

	cutoff := now - int64(w.duration)
	for w.size > 0 {
		oldestIdx := (w.head - w.size + maxSamples) % maxSamples
		if w.buffer[oldestIdx].Timestamp >= cutoff {
			break
		}
		w.size--
	}
}

// OldestPrice returns the oldest price still within the window.
func (w *RollingWindow) OldestPrice() (types.Decimal, bool) {
	if w.size == 0 {
		return types.Zero(2), false
	}
	idx := (w.head - w.size + maxSamples) % maxSamples
	return w.buffer[idx].Price, true
}

// NewestPrice returns the most recently added price.
func (w *RollingWindow) NewestPrice() (types.Decimal, bool) {
	if w.size == 0 {
		return types.Zero(2), false
	}
	idx := (w.head - 1 + maxSamples) % maxSamples
	return w.buffer[idx].Price, true
}

// Len returns the number of samples currently in the window.
func (w *RollingWindow) Len() int { return w.size }

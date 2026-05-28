// Package sequence provides a monotonic sequence counter.
package sequence

// Counter is a monotonic uint64 counter. Not goroutine-safe;
// owned exclusively by the single-writer goroutine.
type Counter struct {
	n uint64
}

// NewCounter creates a Counter starting at n. Pass InitialOrderSeq or
// InitialEventSeq from MarketConfig for WAL recovery.
func NewCounter(start uint64) *Counter {
	// start-1 so first Next() returns start.
	return &Counter{n: start - 1}
}

// Next increments the counter and returns the new value.
// First call on a counter initialized with NewCounter(1) returns 1.
func (c *Counter) Next() uint64 {
	c.n++
	return c.n
}

// Peek returns the current value without incrementing.
func (c *Counter) Peek() uint64 { return c.n }

// Reset sets the counter to n. Used for WAL replay recovery.
func (c *Counter) Reset(n uint64) { c.n = n }

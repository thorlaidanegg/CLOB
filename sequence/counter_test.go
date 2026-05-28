package sequence

import "testing"

func TestCounter_StartsAtOne(t *testing.T) {
	c := NewCounter(1)
	if got := c.Next(); got != 1 {
		t.Fatalf("first Next() = %d, want 1", got)
	}
}

func TestCounter_Monotonic(t *testing.T) {
	c := NewCounter(1)
	prev := c.Next()
	for i := 0; i < 100; i++ {
		next := c.Next()
		if next <= prev {
			t.Fatalf("counter not monotonic: %d followed by %d", prev, next)
		}
		prev = next
	}
}

func TestCounter_Peek(t *testing.T) {
	c := NewCounter(1)
	c.Next()
	c.Next()
	if c.Peek() != 2 {
		t.Fatalf("Peek = %d, want 2", c.Peek())
	}
	// Peek must not advance.
	if c.Peek() != 2 {
		t.Fatal("Peek advanced the counter")
	}
}

func TestCounter_Reset(t *testing.T) {
	c := NewCounter(1)
	c.Next()
	c.Next()
	c.Reset(100)
	if got := c.Next(); got != 101 {
		t.Fatalf("after Reset(100), Next() = %d, want 101", got)
	}
}

func TestCounter_InitialValue(t *testing.T) {
	c := NewCounter(42)
	if got := c.Next(); got != 42 {
		t.Fatalf("NewCounter(42).Next() = %d, want 42", got)
	}
}

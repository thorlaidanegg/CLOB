package pool

import "testing"

type testItem struct {
	poolIndex int
	value     int
}

func TestPool_AcquireRelease(t *testing.T) {
	p := New[testItem](4)
	if p.Len() != 0 {
		t.Fatalf("initial Len = %d, want 0", p.Len())
	}

	item, idx, err := p.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	item.poolIndex = idx
	item.value = 42

	if p.Len() != 1 {
		t.Fatalf("Len after Acquire = %d, want 1", p.Len())
	}

	p.Release(idx)
	if p.Len() != 0 {
		t.Fatalf("Len after Release = %d, want 0", p.Len())
	}
}

func TestPool_ZeroedOnAcquire(t *testing.T) {
	p := New[testItem](4)
	item, idx, _ := p.Acquire()
	item.poolIndex = idx
	item.value = 99
	p.Release(idx)

	// Re-acquire the same slot; value must be zeroed.
	item2, idx2, _ := p.Acquire()
	item2.poolIndex = idx2
	if item2.value != 0 {
		t.Errorf("re-acquired slot not zeroed: value = %d", item2.value)
	}
	p.Release(idx2)
}

func TestPool_Exhaustion(t *testing.T) {
	p := New[testItem](2)
	_, _, err1 := p.Acquire()
	if err1 != nil {
		t.Fatal(err1)
	}
	_, _, err2 := p.Acquire()
	if err2 != nil {
		t.Fatal(err2)
	}
	_, _, err3 := p.Acquire()
	if err3 != ErrPoolExhausted {
		t.Fatalf("expected ErrPoolExhausted, got %v", err3)
	}
}

func TestPool_LargeAcquireReleaseCycle(t *testing.T) {
	const cap = 1000
	p := New[testItem](cap)
	items := make([]int, cap)

	for i := 0; i < cap; i++ {
		item, idx, err := p.Acquire()
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		item.poolIndex = idx
		items[i] = idx
	}
	if p.Len() != cap {
		t.Fatalf("Len at full = %d, want %d", p.Len(), cap)
	}

	for _, idx := range items {
		p.Release(idx)
	}
	if p.Len() != 0 {
		t.Fatalf("Len after all releases = %d, want 0 (leak detected)", p.Len())
	}
}

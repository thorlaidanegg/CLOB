package circuit

import (
	"testing"
	"time"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

func testBreaker(maxMovePct string) *CircuitBreaker {
	return NewCircuitBreaker(config.CircuitBreakerConfig{
		WindowDuration: 60 * time.Second,
		MaxMovePercent: types.MustDecimal(maxMovePct, 4),
	})
}

func TestCircuitBreaker_NoHaltWithinThreshold(t *testing.T) {
	cb := testBreaker("0.1000") // 10%

	now := int64(0)
	cb.Check(types.MustDecimal("100.00", 2), now)
	now += int64(time.Second)
	_, halt := cb.Check(types.MustDecimal("105.00", 2), now) // 5% move
	if halt {
		t.Error("5% move should not trigger a 10% circuit breaker")
	}
}

func TestCircuitBreaker_HaltsOnExcessiveMove(t *testing.T) {
	cb := testBreaker("0.1000") // 10%

	now := int64(0)
	cb.Check(types.MustDecimal("100.00", 2), now)
	now += int64(time.Second)
	reason, halt := cb.Check(types.MustDecimal("115.00", 2), now) // 15% move
	if !halt {
		t.Error("15% move should trigger a 10% circuit breaker")
	}
	if reason == "" {
		t.Error("halt reason should not be empty")
	}
}

func TestCircuitBreaker_NotEnoughData(t *testing.T) {
	cb := testBreaker("0.0500") // 5%

	// Only one sample â€” needs at least two to compare.
	_, halt := cb.Check(types.MustDecimal("100.00", 2), 0)
	if halt {
		t.Error("single sample should never trigger halt")
	}
}

func TestCircuitBreaker_StaleDataEvicted(t *testing.T) {
	cb := testBreaker("0.0500") // 5%

	now := int64(0)
	cb.Check(types.MustDecimal("100.00", 2), now)

	// Advance 2 minutes â€” the first sample falls outside the 60s window.
	now += int64(2 * time.Minute)
	cb.Check(types.MustDecimal("102.00", 2), now)

	// Add a third sample 1s later â€” only the 2-minute sample is the baseline now.
	now += int64(time.Second)
	_, halt := cb.Check(types.MustDecimal("103.00", 2), now) // ~0.98% from 102
	if halt {
		t.Error("move from recent baseline is within threshold; should not halt")
	}
}

func TestCircuitBreaker_ExactThresholdDoesNotHalt(t *testing.T) {
	cb := testBreaker("0.1000") // 10%

	now := int64(0)
	cb.Check(types.MustDecimal("100.00", 2), now)
	now += int64(time.Second)
	// Exactly 10% move: move == MaxMovePercent, not GreaterThan â†’ no halt
	_, halt := cb.Check(types.MustDecimal("110.00", 2), now)
	if halt {
		t.Error("exact threshold should not halt (GreaterThan, not GreaterThanOrEqual)")
	}
}

func TestRollingWindow_Basic(t *testing.T) {
	w := NewRollingWindow(60 * time.Second)

	_, ok := w.OldestPrice()
	if ok {
		t.Error("empty window should return ok=false")
	}

	w.Add(types.MustDecimal("100.00", 2), 0)
	w.Add(types.MustDecimal("101.00", 2), int64(time.Second))

	oldest, ok := w.OldestPrice()
	if !ok || !oldest.Equal(types.MustDecimal("100.00", 2)) {
		t.Errorf("oldest = %s, want 100.00", oldest)
	}
	newest, ok := w.NewestPrice()
	if !ok || !newest.Equal(types.MustDecimal("101.00", 2)) {
		t.Errorf("newest = %s, want 101.00", newest)
	}
}

func TestRollingWindow_Eviction(t *testing.T) {
	w := NewRollingWindow(10 * time.Second)

	w.Add(types.MustDecimal("100.00", 2), 0)
	// Jump 15s past the window duration.
	w.Add(types.MustDecimal("200.00", 2), int64(15*time.Second))

	if w.Len() != 1 {
		t.Errorf("expected 1 sample after eviction, got %d", w.Len())
	}
	oldest, _ := w.OldestPrice()
	if !oldest.Equal(types.MustDecimal("200.00", 2)) {
		t.Errorf("oldest after eviction = %s, want 200.00", oldest)
	}
}

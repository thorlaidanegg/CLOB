package stopbook

import (
	"testing"

	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/types"
)

func testStopBook() *StopBook {
	return NewStopBook(
		pool.New[StopNode](100),
		pool.New[StopLevel](20),
		10,
	)
}

func addStop(sb *StopBook, side types.Side, triggerPrice string, convertTo types.OrderType) *StopNode {
	node, idx, _ := sb.nodePool.Acquire()
	node.PoolIndex = idx
	node.OrderID = types.NewOrderID()
	node.UserID = "user1"
	node.Side = side
	node.TriggerPrice = types.MustDecimal(triggerPrice, 2)
	node.LimitPrice = types.Zero(2)
	node.Qty = types.MustDecimal("10", 0)
	node.ConvertTo = convertTo
	node.TIF = types.GTC
	sb.AddStop(node)
	return node
}

func TestStopBook_SellDoesNotFireAboveTrigger(t *testing.T) {
	sb := testStopBook()
	addStop(sb, types.Ask, "100.00", types.Market)

	// lastTradePrice = 101 > trigger 100 â†’ sell stop should NOT fire
	triggered, err := sb.CheckTriggers(types.MustDecimal("101.00", 2), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggered) != 0 {
		t.Errorf("stop sell should not fire above trigger, got %d triggers", len(triggered))
	}
}

func TestStopBook_SellFiresAtTrigger(t *testing.T) {
	sb := testStopBook()
	node := addStop(sb, types.Ask, "100.00", types.Market)
	id := node.OrderID

	// lastTradePrice == trigger â†’ should fire
	triggered, err := sb.CheckTriggers(types.MustDecimal("100.00", 2), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggered) != 1 {
		t.Fatalf("stop sell should fire at trigger, got %d", len(triggered))
	}
	if triggered[0].OrderID != id {
		t.Error("triggered order ID mismatch")
	}
	if sb.Len() != 0 {
		t.Error("stop should be removed after trigger")
	}
}

func TestStopBook_SellFiresBelowTrigger(t *testing.T) {
	sb := testStopBook()
	addStop(sb, types.Ask, "100.00", types.Market)

	triggered, err := sb.CheckTriggers(types.MustDecimal("99.00", 2), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggered) != 1 {
		t.Fatalf("stop sell should fire below trigger, got %d", len(triggered))
	}
}

func TestStopBook_BuyFiresAtOrAboveTrigger(t *testing.T) {
	sb := testStopBook()
	addStop(sb, types.Bid, "100.00", types.Market)

	triggered, _ := sb.CheckTriggers(types.MustDecimal("100.00", 2), 0)
	if len(triggered) != 1 {
		t.Fatalf("stop buy should fire at trigger, got %d", len(triggered))
	}
}

func TestStopBook_StopLimitConvertsAtLimitPrice(t *testing.T) {
	sb := testStopBook()
	node, idx, _ := sb.nodePool.Acquire()
	node.PoolIndex = idx
	node.OrderID = types.NewOrderID()
	node.UserID = "user1"
	node.Side = types.Ask
	node.TriggerPrice = types.MustDecimal("100.00", 2)
	node.LimitPrice = types.MustDecimal("99.50", 2)
	node.Qty = types.MustDecimal("5", 0)
	node.ConvertTo = types.Limit
	node.TIF = types.GTC
	sb.AddStop(node)

	triggered, _ := sb.CheckTriggers(types.MustDecimal("100.00", 2), 0)
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered, got %d", len(triggered))
	}
	if triggered[0].ConvertTo != types.Limit {
		t.Error("should convert to Limit")
	}
	if !triggered[0].LimitPrice.Equal(types.MustDecimal("99.50", 2)) {
		t.Errorf("LimitPrice = %s, want 99.50", triggered[0].LimitPrice)
	}
}

func TestStopBook_CascadeLimit(t *testing.T) {
	sb := NewStopBook(pool.New[StopNode](100), pool.New[StopLevel](20), 2)
	addStop(sb, types.Ask, "100.00", types.Market)

	_, err := sb.CheckTriggers(types.MustDecimal("100.00", 2), 2) // depth == maxDepth
	if err != ErrCascadeLimit {
		t.Errorf("expected ErrCascadeLimit, got %v", err)
	}
}

func TestStopBook_Cancel(t *testing.T) {
	sb := testStopBook()
	node := addStop(sb, types.Ask, "100.00", types.Market)

	cancelled, ok := sb.CancelStop(node.OrderID, "user1")
	if !ok || cancelled == nil {
		t.Fatal("CancelStop should succeed")
	}
	if sb.Len() != 0 {
		t.Error("stop book should be empty after cancel")
	}
	sb.nodePool.Release(cancelled.PoolIndex)
}

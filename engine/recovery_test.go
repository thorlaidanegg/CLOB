package engine

import (
	"testing"
	"time"

	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

// TestEngine_WithInitialOrders_SeedsBook verifies that orders supplied via
// WithInitialOrders are real, matchable residents of the rebuilt book — not just
// cosmetic depth.
func TestEngine_WithInitialOrders_SeedsBook(t *testing.T) {
	recovered := []RecoveredOrder{
		{
			OrderID: "bid-1", UserID: "alice", Side: types.Bid, Type: types.Limit,
			Price: types.MustDecimal("100.00", 2), RemainQty: types.MustDecimal("5", 0),
			DisplayQty: types.MustDecimal("5", 0), TIF: types.GTC,
		},
		{
			OrderID: "bid-2", UserID: "alice", Side: types.Bid, Type: types.Limit,
			Price: types.MustDecimal("99.00", 2), RemainQty: types.MustDecimal("3", 0),
			DisplayQty: types.MustDecimal("3", 0), TIF: types.GTC,
		},
		{
			OrderID: "ask-1", UserID: "bob", Side: types.Ask, Type: types.Limit,
			Price: types.MustDecimal("101.00", 2), RemainQty: types.MustDecimal("4", 0),
			DisplayQty: types.MustDecimal("4", 0), TIF: types.GTC,
		},
		{
			OrderID: "stop-1", UserID: "carol", Side: types.Ask, Type: types.Stop,
			StopPrice: types.MustDecimal("90.00", 2), RemainQty: types.MustDecimal("2", 0),
			TIF: types.GTC,
		},
	}

	e, err := New(testConfig(), WithInitialOrders(recovered), WithCommandBuffer(256), WithEventBuffer(1024))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { e.Close() }) //nolint

	// Resting limits restored.
	st := e.Stats()
	if st.OpenOrders != 3 {
		t.Errorf("OpenOrders = %d, want 3", st.OpenOrders)
	}
	if st.StopOrders != 1 {
		t.Errorf("StopOrders = %d, want 1", st.StopOrders)
	}

	// BBO reflects the seeded book.
	bid, ask, hasBid, hasAsk := e.BBO()
	if !hasBid || !hasAsk {
		t.Fatalf("BBO missing: hasBid=%v hasAsk=%v", hasBid, hasAsk)
	}
	if bid.String() != "100.00" {
		t.Errorf("best bid = %s, want 100.00", bid.String())
	}
	if ask.String() != "101.00" {
		t.Errorf("best ask = %s, want 101.00", ask.String())
	}

	// Seeded orders are matchable: an aggressive ask at 100.00 must fill bid-1.
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "dave",
		Side: types.Ask, Price: types.MustDecimal("100.00", 2),
		Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})

	var filled bool
	for _, ev := range drainEvents(e, 500*time.Millisecond) {
		if f, ok := ev.(events.TradeFill); ok && f.OrderID == "bid-1" {
			if f.Price.String() != "100.00" || f.FilledQty.String() != "5" {
				t.Errorf("fill on bid-1 = %s @ %s, want 5 @ 100.00", f.FilledQty.String(), f.Price.String())
			}
			filled = true
		}
	}
	if !filled {
		t.Error("expected the aggressive ask to fill the recovered resting bid bid-1")
	}
}

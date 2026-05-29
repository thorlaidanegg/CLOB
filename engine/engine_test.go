package engine

import (
	"testing"
	"time"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

func testConfig() config.MarketConfig {
	return config.MarketConfig{
		MarketID:       "BTC-USD",
		PricePrecision: 2,
		QtyPrecision:   0,
		TickSize:       types.MustDecimal("0.01", 2),
		LotSize:        types.MustDecimal("1", 0),
		Features:       config.DefaultFeatures().Add(config.FeaturePostOnly).Add(config.FeatureFOK).Add(config.FeatureStopOrders),
		FeeSchedule: config.FeeSchedule{
			MakerFeeRate: types.MustDecimal("0.0010", 4),
			TakerFeeRate: types.MustDecimal("0.0020", 4),
		},
	}
}

func startEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(testConfig(), WithCommandBuffer(256), WithEventBuffer(1024))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { e.Close() }) //nolint
	return e
}

// drainEvents collects events from the channel until the given deadline.
func drainEvents(e *Engine, timeout time.Duration) []events.Event {
	var collected []events.Event
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-e.Events():
			if !ok {
				return collected
			}
			collected = append(collected, ev)
		case <-deadline:
			return collected
		}
	}
}

func TestEngine_AlreadyStarted(t *testing.T) {
	e, _ := New(testConfig())
	_ = e.Start()
	if err := e.Start(); err != ErrAlreadyStarted {
		t.Errorf("expected ErrAlreadyStarted, got %v", err)
	}
	e.Close() //nolint
}

func TestEngine_SubmitBeforeStart(t *testing.T) {
	e, _ := New(testConfig())
	err := e.Submit(CancelOrder{MarketID: "BTC-USD", OrderID: "x", UserID: "u"})
	if err != ErrNotStarted {
		t.Errorf("expected ErrNotStarted, got %v", err)
	}
}

func TestEngine_LimitOrderCross(t *testing.T) {
	e := startEngine(t)

	// Transition to Open state first.
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})

	// Place ask.
	askID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD",
		OrderID:  askID,
		UserID:   "seller",
		Side:     types.Ask,
		Price:    types.MustDecimal("100.00", 2),
		Qty:      types.MustDecimal("10", 0),
		TIF:      types.GTC,
	})

	// Place crossing bid.
	bidID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD",
		OrderID:  bidID,
		UserID:   "buyer",
		Side:     types.Bid,
		Price:    types.MustDecimal("100.00", 2),
		Qty:      types.MustDecimal("10", 0),
		TIF:      types.GTC,
	})

	evts := drainEvents(e, 200*time.Millisecond)

	// Find the TradeExecuted event.
	var tradeEvt *events.TradeExecuted
	for _, ev := range evts {
		if te, ok := ev.(events.TradeExecuted); ok {
			tradeEvt = &te
		}
	}
	if tradeEvt == nil {
		t.Fatal("expected TradeExecuted event")
	}
	if !tradeEvt.Price.Equal(types.MustDecimal("100.00", 2)) {
		t.Errorf("trade price = %s, want 100.00", tradeEvt.Price)
	}
	if !tradeEvt.Qty.Equal(types.MustDecimal("10", 0)) {
		t.Errorf("trade qty = %s, want 10", tradeEvt.Qty)
	}
}

func TestEngine_OrderRejected_MarketNotOpen(t *testing.T) {
	e := startEngine(t)
	// Engine starts in PreOpen â€” market orders should be rejected.
	_ = e.Submit(PlaceMarketOrder{
		MarketID: "BTC-USD",
		OrderID:  types.NewOrderID(),
		UserID:   "u",
		Side:     types.Bid,
		Qty:      types.MustDecimal("1", 0),
		TIF:      types.IOC,
	})

	evts := drainEvents(e, 100*time.Millisecond)
	var rejected bool
	for _, ev := range evts {
		if _, ok := ev.(events.OrderRejected); ok {
			rejected = true
		}
	}
	if !rejected {
		t.Error("expected OrderRejected for market order in PreOpen state")
	}
}

func TestEngine_CancelOrder(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})

	orderID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD",
		OrderID:  orderID,
		UserID:   "u",
		Side:     types.Ask,
		Price:    types.MustDecimal("100.00", 2),
		Qty:      types.MustDecimal("5", 0),
		TIF:      types.GTC,
	})
	_ = e.Submit(CancelOrder{
		MarketID: "BTC-USD",
		OrderID:  orderID,
		UserID:   "u",
	})

	evts := drainEvents(e, 200*time.Millisecond)
	var canceled bool
	for _, ev := range evts {
		if oc, ok := ev.(events.OrderCanceled); ok && oc.OrderID == orderID {
			canceled = true
		}
	}
	if !canceled {
		t.Error("expected OrderCanceled event")
	}
}

func TestEngine_AdminHaltResume(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(AdminHaltMarket{MarketID: "BTC-USD", Reason: "test"})

	evts := drainEvents(e, 100*time.Millisecond)
	var halted bool
	for _, ev := range evts {
		if _, ok := ev.(events.MarketHalted); ok {
			halted = true
		}
	}
	if !halted {
		t.Error("expected MarketHalted event")
	}
}

func TestEngine_BBO(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Bid, Price: types.MustDecimal("99.00", 2), Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("101.00", 2), Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})

	// Give processor time to handle.
	drainEvents(e, 100*time.Millisecond)

	bid, ask, hasBid, hasAsk := e.BBO()
	if !hasBid || !bid.Equal(types.MustDecimal("99.00", 2)) {
		t.Errorf("BBO bid = %s hasBid=%v, want 99.00 true", bid, hasBid)
	}
	if !hasAsk || !ask.Equal(types.MustDecimal("101.00", 2)) {
		t.Errorf("BBO ask = %s hasAsk=%v, want 101.00 true", ask, hasAsk)
	}
}

func TestEngine_InvalidConfig(t *testing.T) {
	_, err := New(config.MarketConfig{}) // empty config should fail Validate()
	if err == nil {
		t.Error("expected error for empty config")
	}
}

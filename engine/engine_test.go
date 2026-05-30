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

func TestEngine_Halted_LimitOrderQueuesAndMatchesOnResume(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(AdminHaltMarket{MarketID: "BTC-USD", Reason: "test halt"})
	drainEvents(e, 50*time.Millisecond) // consume AdminResumed + AdminHalted events

	// Place a resting ask while halted.
	askID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: askID, UserID: "seller",
		Side: types.Ask, Price: types.MustDecimal("100.00", 2),
		Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})

	evts := drainEvents(e, 100*time.Millisecond)

	// Must get OrderAccepted + OrderRested; must NOT get TradeExecuted.
	var gotAccepted, gotRested, gotTrade bool
	for _, ev := range evts {
		switch ev.(type) {
		case events.OrderAccepted:
			gotAccepted = true
		case events.OrderRested:
			gotRested = true
		case events.TradeExecuted:
			gotTrade = true
		}
	}
	if !gotAccepted {
		t.Error("expected OrderAccepted for limit order during halt")
	}
	if !gotRested {
		t.Error("expected OrderRested for limit order during halt")
	}
	if gotTrade {
		t.Error("must not execute trades while market is halted")
	}

	// Resume and send a crossing bid — the queued ask should now fill.
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	bidID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: bidID, UserID: "buyer",
		Side: types.Bid, Price: types.MustDecimal("100.00", 2),
		Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})

	evts2 := drainEvents(e, 100*time.Millisecond)

	var gotFill bool
	for _, ev := range evts2 {
		if te, ok := ev.(events.TradeExecuted); ok {
			if te.MakerOrderID == askID {
				gotFill = true
			}
		}
	}
	if !gotFill {
		t.Error("queued ask should fill against crossing bid after resume")
	}
}

func TestEngine_Halted_MarketOrderRejected(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(AdminHaltMarket{MarketID: "BTC-USD", Reason: "test halt"})
	drainEvents(e, 50*time.Millisecond)

	_ = e.Submit(PlaceMarketOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Bid, Qty: types.MustDecimal("1", 0), TIF: types.IOC,
	})

	evts := drainEvents(e, 100*time.Millisecond)
	var gotRejected bool
	for _, ev := range evts {
		if _, ok := ev.(events.OrderRejected); ok {
			gotRejected = true
		}
	}
	if !gotRejected {
		t.Error("market order during halt must be rejected")
	}
}

func TestEngine_Halted_StopOrderQueues(t *testing.T) {
	e := startEngine(t)
	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})
	_ = e.Submit(AdminHaltMarket{MarketID: "BTC-USD", Reason: "test halt"})
	drainEvents(e, 50*time.Millisecond)

	stopID := types.NewOrderID()
	_ = e.Submit(PlaceStopOrder{
		MarketID:     "BTC-USD",
		OrderID:      stopID,
		UserID:       "u",
		Side:         types.Bid,
		TriggerPrice: types.MustDecimal("95.00", 2),
		LimitPrice:   types.Zero(2),
		Qty:          types.MustDecimal("2", 0),
		ConvertTo:    types.Market,
		TIF:          types.GTC,
	})

	evts := drainEvents(e, 100*time.Millisecond)
	var gotAccepted bool
	for _, ev := range evts {
		if oa, ok := ev.(events.OrderAccepted); ok && oa.OrderID == stopID {
			gotAccepted = true
		}
	}
	if !gotAccepted {
		t.Error("stop order during halt must be accepted and queued")
	}
}

func TestEngine_InvalidConfig(t *testing.T) {
	_, err := New(config.MarketConfig{}) // empty config should fail Validate()
	if err == nil {
		t.Error("expected error for empty config")
	}
}

func TestEngine_Auction_EndToEnd(t *testing.T) {
	// Auction opens in 20ms, clears in 60ms.
	openTime := time.Now().Add(60 * time.Millisecond)
	cfg := testConfig()
	cfg.Features = cfg.Features.Add(config.FeatureAuctions)
	cfg.Auction = &config.AuctionConfig{
		PreOpenDuration: 20 * time.Millisecond,
		OpenTime:        openTime,
	}

	e, err := New(cfg, WithCommandBuffer(64), WithEventBuffer(256))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() }) //nolint

	// Wait for AuctionOpened (market transitions PreOpen → Auction at ~40ms).
	var auctionOpened bool
	deadline := time.After(80 * time.Millisecond)
	for !auctionOpened {
		select {
		case ev := <-e.Events():
			if _, ok := ev.(events.AuctionOpened); ok {
				auctionOpened = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for AuctionOpened")
		}
	}

	// Submit crossing orders during auction — no immediate matching.
	askID := types.NewOrderID()
	bidID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: askID, UserID: "seller",
		Side: types.Ask, Price: types.MustDecimal("100.00", 2),
		Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: bidID, UserID: "buyer",
		Side: types.Bid, Price: types.MustDecimal("105.00", 2),
		Qty: types.MustDecimal("5", 0), TIF: types.GTC,
	})

	// Collect all events until after auction clears (wait up to 200ms after openTime).
	evts := drainEvents(e, 200*time.Millisecond)

	var gotAuctionCleared bool
	var gotTradeExecuted bool
	for _, ev := range evts {
		switch ev.(type) {
		case events.AuctionCleared:
			gotAuctionCleared = true
		case events.TradeExecuted:
			gotTradeExecuted = true
		}
	}

	if !gotAuctionCleared {
		t.Error("expected AuctionCleared event")
	}
	if !gotTradeExecuted {
		t.Error("expected TradeExecuted from auction sweep")
	}
}

func TestEngine_Auction_GTCCarryover(t *testing.T) {
	// Auction opens immediately, clears in 60ms.
	openTime := time.Now().Add(60 * time.Millisecond)
	cfg := testConfig()
	cfg.Features = cfg.Features.Add(config.FeatureAuctions)
	cfg.Auction = &config.AuctionConfig{
		PreOpenDuration: 60 * time.Millisecond, // opens immediately on start
		OpenTime:        openTime,
	}

	e, err := New(cfg, WithCommandBuffer(64), WithEventBuffer(256))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() }) //nolint

	// Wait for AuctionOpened.
	var auctionOpened bool
	deadline := time.After(50 * time.Millisecond)
	for !auctionOpened {
		select {
		case ev := <-e.Events():
			if _, ok := ev.(events.AuctionOpened); ok {
				auctionOpened = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for AuctionOpened")
		}
	}

	// Submit two non-crossing bids — they won't fill in the auction.
	bid1 := types.NewOrderID()
	bid2 := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: bid1, UserID: "u1",
		Side: types.Bid, Price: types.MustDecimal("95.00", 2),
		Qty: types.MustDecimal("3", 0), TIF: types.GTC,
	})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: bid2, UserID: "u2",
		Side: types.Bid, Price: types.MustDecimal("94.00", 2),
		Qty: types.MustDecimal("2", 0), TIF: types.GTC,
	})

	// Drain events through the clear (200ms from now).
	evts := drainEvents(e, 200*time.Millisecond)

	var gotCleared bool
	restedIDs := map[types.OrderID]bool{}
	for _, ev := range evts {
		switch e := ev.(type) {
		case events.AuctionCleared:
			gotCleared = true
		case events.OrderRested:
			restedIDs[e.OrderID] = true
		}
	}

	if !gotCleared {
		t.Error("expected AuctionCleared event")
	}
	// Both GTC bids should carry over and rest in the continuous book.
	if !restedIDs[bid1] {
		t.Errorf("bid1 should have rested in continuous book after auction")
	}
	if !restedIDs[bid2] {
		t.Errorf("bid2 should have rested in continuous book after auction")
	}
}

func maxDepthConfig(mode config.DepthMode) config.MarketConfig {
	cfg := testConfig()
	cfg.MaxDepth = 2
	cfg.MaxDepthMode = mode
	return cfg
}

func TestEngine_MaxDepth_RejectOrder(t *testing.T) {
	e, err := New(maxDepthConfig(config.DepthRejectOrder), WithCommandBuffer(64), WithEventBuffer(256))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() }) //nolint

	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})

	// Fill the ask side to MaxDepth=2 with levels at 100.00 and 101.00.
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("100.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("101.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})

	// Third ask at 102.00 is outside the top 2 levels → must be rejected.
	thirdID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: thirdID, UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("102.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})

	evts := drainEvents(e, 200*time.Millisecond)
	var gotRejected bool
	for _, ev := range evts {
		if rej, ok := ev.(events.OrderRejected); ok && rej.OrderID == thirdID {
			if rej.Reason != types.RejectMaxDepth {
				t.Errorf("rejection reason = %v, want RejectMaxDepth", rej.Reason)
			}
			gotRejected = true
		}
	}
	if !gotRejected {
		t.Error("expected OrderRejected for ask beyond MaxDepth")
	}
}

func TestEngine_MaxDepth_TreatAsIOC(t *testing.T) {
	e, err := New(maxDepthConfig(config.DepthTreatAsIOC), WithCommandBuffer(64), WithEventBuffer(256))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() }) //nolint

	_ = e.Submit(AdminResumeMarket{MarketID: "BTC-USD"})

	// Fill the ask side to MaxDepth=2.
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("100.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.NewOrderID(), UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("101.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})

	// Third ask at 102.00 — treated as IOC with no crossing bids → canceled, never rests.
	thirdID := types.NewOrderID()
	_ = e.Submit(PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: thirdID, UserID: "u",
		Side: types.Ask, Price: types.MustDecimal("102.00", 2), Qty: types.MustDecimal("1", 0), TIF: types.GTC,
	})

	evts := drainEvents(e, 200*time.Millisecond)

	var rested, canceled bool
	for _, ev := range evts {
		if or, ok := ev.(events.OrderRested); ok && or.OrderID == thirdID {
			rested = true
		}
		if oc, ok := ev.(events.OrderCanceled); ok && oc.OrderID == thirdID {
			canceled = true
		}
	}
	if rested {
		t.Error("order beyond MaxDepth must not rest in book")
	}
	if !canceled {
		t.Error("expected OrderCanceled (IOC treatment) for ask beyond MaxDepth")
	}
}

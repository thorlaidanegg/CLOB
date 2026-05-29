package book

import (
	"testing"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/sequence"
	"github.com/thorlaidanegg/clob/types"
)

// testBook builds an OrderBook for testing.
func testBook() *OrderBook {
	cfg := &config.MarketConfig{
		PricePrecision:  2,
		QtyPrecision:    0,
		TickSize:        types.NewDecimal(1, 2),
		LotSize:         types.NewDecimal(1, 0),
		STPMode:         config.STPDisabled,
		MaxCascadeDepth: 10,
	}
	nodePool := pool.New[OrderNode](1000)
	levelPool := pool.New[PriceLevel](100)
	seq := sequence.NewCounter(1)
	return NewOrderBook(cfg, nodePool, levelPool, seq)
}

// acquireNode creates a node from the book's pool for testing.
func acquireNode(b *OrderBook, side types.Side, price string, qty string, tif types.TIF, orderType types.OrderType) *OrderNode {
	node, idx, err := b.nodePool.Acquire()
	if err != nil {
		panic(err)
	}
	node.PoolIndex = idx
	node.OrderID = types.NewOrderID()
	node.UserID = "user1"
	node.Side = side
	node.Type = orderType
	node.TIF = tif
	node.SeqNum = b.orderSeq.Next()

	p := types.MustDecimal(price, b.config.PricePrecision)
	q := types.MustDecimal(qty, b.config.QtyPrecision)
	node.Price = p
	node.RemainQty = q
	node.OrigQty = q
	node.FilledQty = types.Zero(b.config.QtyPrecision)
	node.DisplayQty = q
	node.HiddenQty = types.Zero(b.config.QtyPrecision)
	return node
}

// restAsk adds a resting ask to the book directly.
func restAsk(b *OrderBook, price string, qty string) *OrderNode {
	node := acquireNode(b, types.Ask, price, qty, types.GTC, types.Limit)
	b.restNode(node)
	return node
}

// restBid adds a resting bid to the book directly.
func restBid(b *OrderBook, price string, qty string) *OrderNode {
	node := acquireNode(b, types.Bid, price, qty, types.GTC, types.Limit)
	b.restNode(node)
	return node
}

// --- Basic crossing tests ---

func TestMatchLoop_LimitBuyCrossesAsk(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "10")

	incoming := acquireNode(b, types.Bid, "100.00", "10", types.GTC, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
	if !fills[0].Price.Equal(types.MustDecimal("100.00", 2)) {
		t.Errorf("fill price = %s, want 100.00", fills[0].Price)
	}
	if !fills[0].Qty.Equal(types.MustDecimal("10", 0)) {
		t.Errorf("fill qty = %s, want 10", fills[0].Qty)
	}
	if b.asks.Len() != 0 {
		t.Error("asks should be empty after full fill")
	}
}

func TestMatchLoop_LimitSellCrossesBid(t *testing.T) {
	b := testBook()
	restBid(b, "100.00", "10")

	incoming := acquireNode(b, types.Ask, "100.00", "10", types.GTC, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if len(fills) != 1 {
		t.Fatal("expected 1 fill")
	}
}

func TestMatchLoop_ExactQtyMatch_LevelRemoved(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	incoming := acquireNode(b, types.Bid, "100.00", "5", types.GTC, types.Limit)
	_, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if b.asks.Len() != 0 {
		t.Error("level should be removed after exact fill")
	}
	if b.index.Len() != 0 {
		t.Error("index should be empty")
	}
}

func TestMatchLoop_IncomingLargerThanResting(t *testing.T) {
	// incoming 10, resting 5 â†’ partial fill, incoming rests with 5
	b := testBook()
	restAsk(b, "100.00", "5")
	incoming := acquireNode(b, types.Bid, "100.00", "10", types.GTC, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != PartialFill_Rested {
		t.Errorf("disposition = %v, want PartialFill_Rested", disp)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
	// Incoming rests with 5 remaining.
	if b.bids.Len() != 1 {
		t.Error("incoming should rest in bid book")
	}
}

func TestMatchLoop_IncomingSmallerThanResting(t *testing.T) {
	// resting 10, incoming 3 â†’ incoming fully filled, resting stays with 7
	b := testBook()
	resting := restAsk(b, "100.00", "10")
	_ = resting
	incoming := acquireNode(b, types.Bid, "100.00", "3", types.GTC, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
	// Resting ask still in book with 7 remaining.
	if b.asks.Len() != 1 {
		t.Error("resting ask should still be in book")
	}
	best := b.asks.Best()
	if !best.TotalQty.Equal(types.MustDecimal("7", 0)) {
		t.Errorf("resting qty = %s, want 7", best.TotalQty)
	}
}

// --- Market order tests ---

func TestMatchLoop_MarketDrainsMultipleLevels(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	restAsk(b, "101.00", "5")
	restAsk(b, "102.00", "5")

	incoming := acquireNode(b, types.Bid, "0", "15", types.IOC, types.Market)
	fills, disp := b.PlaceMarket(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if len(fills) != 3 {
		t.Fatalf("fills = %d, want 3", len(fills))
	}
	if b.asks.Len() != 0 {
		t.Error("all ask levels should be consumed")
	}
}

func TestMatchLoop_MarketOnEmptyBook(t *testing.T) {
	b := testBook()
	incoming := acquireNode(b, types.Bid, "0", "10", types.IOC, types.Market)
	_, disp := b.PlaceMarket(incoming)

	if disp != Canceled {
		t.Errorf("disposition = %v, want Canceled", disp)
	}
}

func TestMatchLoop_MarketPartialThenCanceled(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	incoming := acquireNode(b, types.Bid, "0", "10", types.IOC, types.Market)
	fills, disp := b.PlaceMarket(incoming)

	if disp != PartialFill_Canceled {
		t.Errorf("disposition = %v, want PartialFill_Canceled", disp)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
}

// --- Price-time priority tests ---

func TestMatchLoop_PriceTimePriority_LowerSeqFirst(t *testing.T) {
	b := testBook()
	// Two resting asks at the same price; lower SeqNum should fill first.
	first := restAsk(b, "100.00", "5")
	second := restAsk(b, "100.00", "5")

	if first.SeqNum >= second.SeqNum {
		t.Fatalf("first.SeqNum (%d) must be < second.SeqNum (%d)", first.SeqNum, second.SeqNum)
	}

	incoming := acquireNode(b, types.Bid, "100.00", "5", types.GTC, types.Limit)
	fills, _ := b.PlaceLimit(incoming)

	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
	if fills[0].MakerOrderID != first.OrderID {
		t.Error("first resting order (lower SeqNum) should fill first")
	}
}

// --- IOC tests ---

func TestMatchLoop_IOC_PartialFill(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	incoming := acquireNode(b, types.Bid, "100.00", "10", types.IOC, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != PartialFill_Canceled {
		t.Errorf("disposition = %v, want PartialFill_Canceled", disp)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills))
	}
}

func TestMatchLoop_IOC_NoLiquidity(t *testing.T) {
	b := testBook()
	incoming := acquireNode(b, types.Bid, "100.00", "10", types.IOC, types.Limit)
	_, disp := b.PlaceLimit(incoming)

	if disp != Canceled {
		t.Errorf("disposition = %v, want Canceled", disp)
	}
}

func TestMatchLoop_IOC_FullyFilled(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "10")
	incoming := acquireNode(b, types.Bid, "100.00", "10", types.IOC, types.Limit)
	_, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
}

// --- FOK tests ---

func TestMatchLoop_FOK_InsufficientLiquidity_BookUnchanged(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	preLen := b.index.Len()

	incoming := acquireNode(b, types.Bid, "100.00", "10", types.FOK, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != Rejected {
		t.Errorf("disposition = %v, want Rejected", disp)
	}
	if len(fills) != 0 {
		t.Errorf("fills = %d, want 0 (book must be unchanged)", len(fills))
	}
	// Book must be completely unchanged.
	if b.index.Len() != preLen {
		t.Error("book state changed after FOK dry-run failure")
	}
	best := b.asks.Best()
	if !best.TotalQty.Equal(types.MustDecimal("5", 0)) {
		t.Errorf("ask qty changed after FOK failure: %s", best.TotalQty)
	}
}

func TestMatchLoop_FOK_SufficientLiquidity(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "10")
	incoming := acquireNode(b, types.Bid, "100.00", "10", types.FOK, types.Limit)
	_, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
}

func TestMatchLoop_FOK_AcrossMultipleLevels(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "5")
	restAsk(b, "101.00", "5")
	incoming := acquireNode(b, types.Bid, "101.00", "10", types.FOK, types.Limit)
	fills, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}
	if len(fills) != 2 {
		t.Fatalf("fills = %d, want 2", len(fills))
	}
}

// --- Iceberg tests ---

func TestMatchLoop_Iceberg_DisplayVisibleHiddenInvisible(t *testing.T) {
	b := testBook()
	// Place iceberg ask: display=3, hidden=7
	node, idx, _ := b.nodePool.Acquire()
	node.PoolIndex = idx
	node.OrderID = types.NewOrderID()
	node.UserID = "user1"
	node.Side = types.Ask
	node.Type = types.Iceberg
	node.TIF = types.GTC
	node.SeqNum = b.orderSeq.Next()
	node.Price = types.MustDecimal("100.00", 2)
	node.OrigQty = types.MustDecimal("10", 0)
	node.RemainQty = types.MustDecimal("3", 0) // display portion
	node.DisplayQty = types.MustDecimal("3", 0)
	node.HiddenQty = types.MustDecimal("7", 0)
	node.OrigDisplayQty = types.MustDecimal("3", 0)
	node.FilledQty = types.Zero(0)
	b.restNode(node)

	// Public book shows display qty only.
	depth := b.asks.Depth(1)
	if !depth[0].DisplayQty.Equal(types.MustDecimal("3", 0)) {
		t.Errorf("display qty = %s, want 3", depth[0].DisplayQty)
	}
	if !depth[0].TotalQty.Equal(types.MustDecimal("3", 0)) {
		t.Errorf("total qty = %s, want 3 (hidden not visible)", depth[0].TotalQty)
	}
}

func TestMatchLoop_Iceberg_ReplenishMovesToTail(t *testing.T) {
	b := testBook()

	// Iceberg: display=3, hidden=7
	ice, idx, _ := b.nodePool.Acquire()
	ice.PoolIndex = idx
	ice.OrderID = "ICE"
	ice.UserID = "user1"
	ice.Side = types.Ask
	ice.Type = types.Iceberg
	ice.TIF = types.GTC
	ice.SeqNum = b.orderSeq.Next()
	ice.Price = types.MustDecimal("100.00", 2)
	ice.OrigQty = types.MustDecimal("10", 0)
	ice.RemainQty = types.MustDecimal("3", 0)
	ice.DisplayQty = types.MustDecimal("3", 0)
	ice.HiddenQty = types.MustDecimal("7", 0)
	ice.OrigDisplayQty = types.MustDecimal("3", 0)
	ice.FilledQty = types.Zero(0)
	b.restNode(ice)

	// Add a normal order behind iceberg.
	normal := restAsk(b, "100.00", "5")
	normalSeq := normal.SeqNum

	// Fill the display portion (3) â€” should trigger replenishment.
	incoming := acquireNode(b, types.Bid, "100.00", "3", types.GTC, types.Limit)
	_, disp := b.PlaceLimit(incoming)

	if disp != FullyFilled {
		t.Errorf("disposition = %v, want FullyFilled", disp)
	}

	// After replenishment, iceberg should be at the tail (behind normal order).
	level := b.asks.Best()
	if level.Head.OrderID != normal.OrderID {
		t.Errorf("head should be normal order, got %s", level.Head.OrderID)
	}
	// Replenished iceberg has a new SeqNum > normalSeq.
	if level.Tail.SeqNum <= normalSeq {
		t.Errorf("replenished iceberg SeqNum (%d) should be > normal (%d)", level.Tail.SeqNum, normalSeq)
	}
}

// --- STP tests ---

func makeStpBook(mode config.STPMode) *OrderBook {
	cfg := &config.MarketConfig{
		PricePrecision:  2,
		QtyPrecision:    0,
		TickSize:        types.NewDecimal(1, 2),
		LotSize:         types.NewDecimal(1, 0),
		STPMode:         mode,
		MaxCascadeDepth: 10,
	}
	return NewOrderBook(cfg, pool.New[OrderNode](1000), pool.New[PriceLevel](100), sequence.NewCounter(1))
}

func acquireNodeUser(b *OrderBook, side types.Side, price string, qty string, userID string) *OrderNode {
	node, idx, _ := b.nodePool.Acquire()
	node.PoolIndex = idx
	node.OrderID = types.NewOrderID()
	node.UserID = types.UserID(userID)
	node.Side = side
	node.Type = types.Limit
	node.TIF = types.GTC
	node.SeqNum = b.orderSeq.Next()
	p := types.MustDecimal(price, 2)
	q := types.MustDecimal(qty, 0)
	node.Price = p
	node.RemainQty = q
	node.OrigQty = q
	node.FilledQty = types.Zero(0)
	node.DisplayQty = q
	node.HiddenQty = types.Zero(0)
	return node
}

func TestMatchLoop_STP_CancelBoth(t *testing.T) {
	b := makeStpBook(config.STPCancelBoth)
	maker := acquireNodeUser(b, types.Ask, "100.00", "10", "alice")
	b.restNode(maker)
	makerID := maker.OrderID

	taker := acquireNodeUser(b, types.Bid, "100.00", "10", "alice")
	fills, disp := b.PlaceLimit(taker)

	if len(fills) != 0 {
		t.Errorf("CancelBoth: fills = %d, want 0", len(fills))
	}
	// Neither order should be in book.
	if b.index.Has(makerID) {
		t.Error("CancelBoth: maker should be removed")
	}
	if b.bids.Len() != 0 {
		t.Error("CancelBoth: taker should not rest")
	}
	_ = disp
}

func TestMatchLoop_STP_CancelMaker(t *testing.T) {
	b := makeStpBook(config.STPCancelMaker)
	maker1 := acquireNodeUser(b, types.Ask, "100.00", "5", "alice")
	b.restNode(maker1)
	maker2 := acquireNodeUser(b, types.Ask, "100.00", "5", "bob")
	b.restNode(maker2)

	// alice is taker â€” maker1 (alice) should be canceled, then fills with maker2 (bob)
	taker := acquireNodeUser(b, types.Bid, "100.00", "5", "alice")
	fills, _ := b.PlaceLimit(taker)

	if len(fills) != 1 {
		t.Fatalf("CancelMaker: fills = %d, want 1", len(fills))
	}
	if fills[0].MakerOrderID != maker2.OrderID {
		t.Error("CancelMaker: should fill with maker2 (bob) after skipping maker1 (alice)")
	}
}

func TestMatchLoop_STP_CancelTaker(t *testing.T) {
	b := makeStpBook(config.STPCancelTaker)
	maker := acquireNodeUser(b, types.Ask, "100.00", "10", "alice")
	b.restNode(maker)
	makerID := maker.OrderID

	taker := acquireNodeUser(b, types.Bid, "100.00", "10", "alice")
	fills, _ := b.PlaceLimit(taker)

	if len(fills) != 0 {
		t.Errorf("CancelTaker: fills = %d, want 0", len(fills))
	}
	// Maker should still be in book.
	if !b.index.Has(makerID) {
		t.Error("CancelTaker: maker should remain in book")
	}
}

func TestMatchLoop_STP_DecrementCancel(t *testing.T) {
	b := makeStpBook(config.STPDecrementCancel)
	// maker has 10, taker has 3 â†’ maker reduced by 3, no fill
	maker := acquireNodeUser(b, types.Ask, "100.00", "10", "alice")
	b.restNode(maker)
	makerID := maker.OrderID

	taker := acquireNodeUser(b, types.Bid, "100.00", "3", "alice")
	fills, _ := b.PlaceLimit(taker)

	if len(fills) != 0 {
		t.Errorf("DecrementCancel: fills = %d, want 0", len(fills))
	}
	// Maker should remain with reduced qty.
	if !b.index.Has(makerID) {
		t.Error("DecrementCancel: maker should remain in book")
	}
	level := b.asks.Best()
	wantQty := types.MustDecimal("7", 0)
	if !level.TotalQty.Equal(wantQty) {
		t.Errorf("DecrementCancel: maker qty = %s, want 7", level.TotalQty)
	}
}

// --- Cancel tests ---

func TestBook_Cancel_RemovesFromBook(t *testing.T) {
	b := testBook()
	resting := restAsk(b, "100.00", "10")

	node, err := b.Cancel(resting.OrderID, "user1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if node == nil {
		t.Fatal("Cancel should return the node")
	}
	if b.index.Len() != 0 {
		t.Error("index should be empty after cancel")
	}
	if b.asks.Len() != 0 {
		t.Error("ask tree should be empty after cancel of only order")
	}
	b.nodePool.Release(node.PoolIndex) // simulate processor releasing
}

func TestBook_Cancel_NotFound(t *testing.T) {
	b := testBook()
	_, err := b.Cancel("nonexistent", "user1")
	if err == nil {
		t.Fatal("expected ErrOrderNotFound")
	}
}

func TestBook_Cancel_WrongUser(t *testing.T) {
	b := testBook()
	resting := restAsk(b, "100.00", "10")

	_, err := b.Cancel(resting.OrderID, "wronguser")
	if err == nil {
		t.Fatal("expected ErrOwnershipMismatch")
	}
}

// --- Pool utilization ---

func TestBook_PoolUtilization_ZeroAfterCycles(t *testing.T) {
	b := testBook()
	preLen := b.nodePool.Len()

	for i := 0; i < 50; i++ {
		ask := restAsk(b, "100.00", "10")
		bid := acquireNode(b, types.Bid, "100.00", "10", types.GTC, types.Limit)
		b.PlaceLimit(bid)
		_ = ask
	}

	if b.nodePool.Len() != preLen {
		t.Errorf("nodePool.Len() = %d after all fills, want %d (leaked nodes)", b.nodePool.Len(), preLen)
	}
}

// --- Benchmarks ---

func BenchmarkMatchLoop_LimitCrossingAsk(b *testing.B) {
	bk := testBook()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ask := restAsk(bk, "100.00", "10")
		_ = ask
		bid := acquireNode(bk, types.Bid, "100.00", "10", types.GTC, types.Limit)
		bk.PlaceLimit(bid)
	}
}

func BenchmarkMatchLoop_MarketOrderDrainsBook(b *testing.B) {
	bk := testBook()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		restAsk(bk, "100.00", "2")
		restAsk(bk, "101.00", "2")
		restAsk(bk, "102.00", "2")
		b.StartTimer()

		market := acquireNode(bk, types.Bid, "0", "6", types.IOC, types.Market)
		bk.PlaceMarket(market)
	}
}

func BenchmarkMatchLoop_FOKDryRun(b *testing.B) {
	bk := testBook()
	restAsk(bk, "100.00", "5") // insufficient for FOK of 10
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fok := acquireNode(bk, types.Bid, "100.00", "10", types.FOK, types.Limit)
		bk.PlaceLimit(fok)
		// Book state unchanged, re-acquire for next iteration is not needed.
	}
}

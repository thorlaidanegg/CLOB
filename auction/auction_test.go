package auction

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

var seqGen uint64

func nextSeq() uint64 {
	seqGen++
	return seqGen
}

func bid(price, qty string) AuctionOrder {
	return AuctionOrder{
		OrderID: types.NewOrderID(),
		UserID:  "buyer",
		Side:    types.Bid,
		Price:   types.MustDecimal(price, 2),
		Qty:     types.MustDecimal(qty, 0),
		TIF:     types.GTC,
		SeqNum:  nextSeq(),
	}
}

func ask(price, qty string) AuctionOrder {
	return AuctionOrder{
		OrderID: types.NewOrderID(),
		UserID:  "seller",
		Side:    types.Ask,
		Price:   types.MustDecimal(price, 2),
		Qty:     types.MustDecimal(qty, 0),
		TIF:     types.GTC,
		SeqNum:  nextSeq(),
	}
}

func TestAuction_ClearingPrice_Standard(t *testing.T) {
	a := NewAuctionBook()
	a.AddOrder(bid("105.00", "10"))
	a.AddOrder(bid("103.00", "10"))
	a.AddOrder(bid("101.00", "10"))
	a.AddOrder(ask("99.00", "10"))
	a.AddOrder(ask("101.00", "10"))
	a.AddOrder(ask("103.00", "10"))

	price, qty, ok := a.ComputeClearingPrice()
	if !ok {
		t.Fatal("expected clearing price to be found")
	}
	if qty.IsZero() {
		t.Fatal("matchable qty should be non-zero")
	}
	t.Logf("clearing price=%s matchableQty=%s", price, qty)
}

func TestAuction_ClearingPrice_AllBidsHigherThanAsks(t *testing.T) {
	a := NewAuctionBook()
	// All bids are higher than all asks â€” everything can match.
	a.AddOrder(bid("110.00", "5"))
	a.AddOrder(bid("108.00", "5"))
	a.AddOrder(ask("100.00", "5"))
	a.AddOrder(ask("102.00", "5"))

	_, qty, ok := a.ComputeClearingPrice()
	if !ok {
		t.Fatal("expected clearing price")
	}
	// 10 units total available on each side â€” 10 should match.
	want := types.MustDecimal("10", 0)
	if !qty.Equal(want) {
		t.Errorf("matchable qty = %s, want %s", qty, want)
	}
}

func TestAuction_ClearingPrice_ExactCrossing(t *testing.T) {
	a := NewAuctionBook()
	a.AddOrder(bid("100.00", "10"))
	a.AddOrder(ask("100.00", "10"))

	price, qty, ok := a.ComputeClearingPrice()
	if !ok {
		t.Fatal("expected clearing price at exact crossing")
	}
	if !price.Equal(types.MustDecimal("100.00", 2)) {
		t.Errorf("price = %s, want 100.00", price)
	}
	if !qty.Equal(types.MustDecimal("10", 0)) {
		t.Errorf("qty = %s, want 10", qty)
	}
}

func TestAuction_NoCrossing(t *testing.T) {
	a := NewAuctionBook()
	a.AddOrder(bid("95.00", "10"))
	a.AddOrder(ask("100.00", "10"))

	_, _, ok := a.ComputeClearingPrice()
	if ok {
		t.Error("no crossing orders â€” should return ok=false")
	}
}

func TestAuction_Sweep_CorrectFillsAtClearingPrice(t *testing.T) {
	a := NewAuctionBook()
	a.AddOrder(bid("105.00", "10"))
	a.AddOrder(ask("100.00", "10"))

	clearingPrice := types.MustDecimal("100.00", 2)
	fills, unmatched, canceled := a.Sweep(clearingPrice)

	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}
	if !fills[0].Price.Equal(clearingPrice) {
		t.Errorf("fill price = %s, want %s", fills[0].Price, clearingPrice)
	}
	if !fills[0].Qty.Equal(types.MustDecimal("10", 0)) {
		t.Errorf("fill qty = %s, want 10", fills[0].Qty)
	}
	if len(unmatched) != 0 {
		t.Errorf("expected no unmatched, got %d", len(unmatched))
	}
	if len(canceled) != 0 {
		t.Errorf("expected no canceled, got %d", len(canceled))
	}
}

func TestAuction_Sweep_PartialMatchLeavesGTCUnmatched(t *testing.T) {
	a := NewAuctionBook()
	a.AddOrder(bid("105.00", "15"))
	a.AddOrder(ask("100.00", "10"))

	clearingPrice := types.MustDecimal("100.00", 2)
	fills, unmatched, _ := a.Sweep(clearingPrice)

	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}
	if !fills[0].Qty.Equal(types.MustDecimal("10", 0)) {
		t.Errorf("fill qty = %s, want 10", fills[0].Qty)
	}
	if len(unmatched) != 1 {
		t.Fatalf("expected 1 unmatched GTC bid, got %d", len(unmatched))
	}
	if !unmatched[0].Qty.Equal(types.MustDecimal("5", 0)) {
		t.Errorf("unmatched qty = %s, want 5", unmatched[0].Qty)
	}
}

func TestAuction_Sweep_IOCUnmatchedDropped(t *testing.T) {
	a := NewAuctionBook()
	iocBid := bid("105.00", "5")
	iocBid.TIF = types.IOC
	a.AddOrder(iocBid)
	a.AddOrder(ask("100.00", "10")) // More ask than bid.

	clearingPrice := types.MustDecimal("100.00", 2)
	fills, unmatched, canceled := a.Sweep(clearingPrice)

	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}
	// Remaining ask is GTC so it goes to unmatched.
	for _, u := range unmatched {
		if u.TIF == types.IOC {
			t.Error("IOC order should not appear in unmatched list")
		}
	}
	// IOC bid filled completely — nothing in canceled.
	if len(canceled) != 0 {
		t.Errorf("expected no canceled orders, got %d", len(canceled))
	}
}

package book

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func TestBook_WouldCross(t *testing.T) {
	b := testBook()
	restAsk(b, "100.00", "10")

	// Bid at 100 should cross.
	if !b.WouldCross(types.MustDecimal("100.00", 2), types.Bid) {
		t.Error("100.00 bid should cross 100.00 ask")
	}
	// Bid at 99 should not cross.
	if b.WouldCross(types.MustDecimal("99.00", 2), types.Bid) {
		t.Error("99.00 bid should not cross 100.00 ask")
	}
}

func TestBook_HasOrder(t *testing.T) {
	b := testBook()
	resting := restAsk(b, "100.00", "10")

	if !b.HasOrder(resting.OrderID) {
		t.Error("HasOrder should return true for resting order")
	}
	if _, err := b.Cancel(resting.OrderID, "user1"); err != nil {
		t.Fatal(err)
	}
	if b.HasOrder(resting.OrderID) {
		t.Error("HasOrder should return false after cancel")
	}
}

func TestBook_CancelLastOrder_LevelRemoved(t *testing.T) {
	b := testBook()
	resting := restAsk(b, "100.00", "10")

	node, err := b.Cancel(resting.OrderID, "user1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	b.nodePool.Release(node.PoolIndex)

	if b.asks.Len() != 0 {
		t.Error("ask level should be removed when last order canceled")
	}
	if b.levelPool.Len() != 0 {
		t.Error("level should be released back to pool")
	}
}

func TestBook_BBO(t *testing.T) {
	b := testBook()
	restBid(b, "99.00", "5")
	restAsk(b, "101.00", "5")

	bid, ask, hasBid, hasAsk := b.BBO()
	if !hasBid || !bid.Equal(types.MustDecimal("99.00", 2)) {
		t.Errorf("BBO bid = %s hasBid=%v, want 99.00 true", bid, hasBid)
	}
	if !hasAsk || !ask.Equal(types.MustDecimal("101.00", 2)) {
		t.Errorf("BBO ask = %s hasAsk=%v, want 101.00 true", ask, hasAsk)
	}
}

func TestBook_BBO_Empty(t *testing.T) {
	b := testBook()
	_, _, hasBid, hasAsk := b.BBO()
	if hasBid || hasAsk {
		t.Error("BBO on empty book should return hasBid=false, hasAsk=false")
	}
}

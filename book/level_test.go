package book

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func newNode(id string, qty string, prec uint8) *OrderNode {
	q := types.MustDecimal(qty, prec)
	return &OrderNode{
		OrderID:    types.OrderID(id),
		RemainQty:  q,
		DisplayQty: q,
		FilledQty:  types.Zero(prec),
		HiddenQty:  types.Zero(prec),
	}
}

func newLevel(prec uint8) *PriceLevel {
	return &PriceLevel{
		Price:      types.MustDecimal("100.00", 2),
		TotalQty:   types.Zero(prec),
		DisplayQty: types.Zero(prec),
	}
}

func TestLevel_AppendUnlink(t *testing.T) {
	l := newLevel(0)
	a := newNode("A", "10", 0)
	b := newNode("B", "20", 0)

	l.Append(a)
	l.Append(b)

	if l.OrderCount != 2 {
		t.Fatalf("OrderCount = %d, want 2", l.OrderCount)
	}
	if l.Head != a || l.Tail != b {
		t.Fatal("head/tail wrong after append")
	}
	want := types.MustDecimal("30", 0)
	if !l.TotalQty.Equal(want) {
		t.Errorf("TotalQty = %s, want 30", l.TotalQty)
	}

	l.Unlink(a)
	if l.Head != b {
		t.Fatal("head should be b after unlinking a")
	}
	wantAfter := types.MustDecimal("20", 0)
	if !l.TotalQty.Equal(wantAfter) {
		t.Errorf("TotalQty after unlink = %s, want 20", l.TotalQty)
	}
	if l.OrderCount != 1 {
		t.Fatalf("OrderCount = %d, want 1", l.OrderCount)
	}
}

func TestLevel_DecrementQty(t *testing.T) {
	l := newLevel(0)
	node := newNode("A", "10", 0)
	l.Append(node)

	fillQty := types.MustDecimal("3", 0)
	l.DecrementQty(node, fillQty)

	wantRemain := types.MustDecimal("7", 0)
	wantFilled := types.MustDecimal("3", 0)
	if !node.RemainQty.Equal(wantRemain) {
		t.Errorf("RemainQty = %s, want 7", node.RemainQty)
	}
	if !node.FilledQty.Equal(wantFilled) {
		t.Errorf("FilledQty = %s, want 3", node.FilledQty)
	}
	if !l.TotalQty.Equal(wantRemain) {
		t.Errorf("level TotalQty = %s, want 7", l.TotalQty)
	}
}

func TestLevel_ReplenishIceberg(t *testing.T) {
	l := newLevel(0)
	// Iceberg: display=5, hidden=15, orig_display=5
	node := &OrderNode{
		OrderID:        "ICE",
		RemainQty:      types.MustDecimal("5", 0),
		DisplayQty:     types.MustDecimal("5", 0),
		HiddenQty:      types.MustDecimal("15", 0),
		OrigDisplayQty: types.MustDecimal("5", 0),
		FilledQty:      types.Zero(0),
		SeqNum:         1,
	}
	other := newNode("B", "3", 0)
	l.Append(node)
	l.Append(other)

	// Replenish: hidden â†’ display, node moves to tail
	l.ReplenishIceberg(node, 99)

	if l.Tail != node {
		t.Error("replenished node should be at tail")
	}
	if l.Head != other {
		t.Error("other node should be at head")
	}
	if node.SeqNum != 99 {
		t.Errorf("SeqNum = %d, want 99", node.SeqNum)
	}
	wantDisplay := types.MustDecimal("5", 0)
	if !node.DisplayQty.Equal(wantDisplay) {
		t.Errorf("DisplayQty = %s, want 5", node.DisplayQty)
	}
	wantHidden := types.MustDecimal("10", 0)
	if !node.HiddenQty.Equal(wantHidden) {
		t.Errorf("HiddenQty = %s, want 10", node.HiddenQty)
	}
}

func TestLevel_IsEmpty(t *testing.T) {
	l := newLevel(0)
	if !l.IsEmpty() {
		t.Error("new level should be empty")
	}
	node := newNode("A", "5", 0)
	l.Append(node)
	if l.IsEmpty() {
		t.Error("level with node should not be empty")
	}
	l.Unlink(node)
	if !l.IsEmpty() {
		t.Error("level should be empty after unlink")
	}
}

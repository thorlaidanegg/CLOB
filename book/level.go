package book

import "github.com/thorlaidanegg/clob/types"

// PriceLevel is a FIFO queue of orders at a single price.
// All quantity fields are maintained incrementally; no recomputation needed.
type PriceLevel struct {
	Price      types.Decimal
	TotalQty   types.Decimal // sum of RemainQty for all nodes
	DisplayQty types.Decimal // sum of DisplayQty for all nodes (public book view)
	OrderCount int

	Head *OrderNode // fills start here (oldest, highest priority)
	Tail *OrderNode // new orders append here (newest, lowest priority)

	PoolIndex int
}

// Append adds node to the tail of the level. O(1).
func (l *PriceLevel) Append(node *OrderNode) {
	if l.Tail == nil {
		l.Head = node
		l.Tail = node
		node.prev = nil
		node.next = nil
	} else {
		node.prev = l.Tail
		node.next = nil
		l.Tail.next = node
		l.Tail = node
	}
	l.TotalQty = l.TotalQty.Add(node.RemainQty)
	l.DisplayQty = l.DisplayQty.Add(node.DisplayQty)
	l.OrderCount++
	node.level = l
}

// Unlink removes node from the level without releasing it to the pool. O(1).
func (l *PriceLevel) Unlink(node *OrderNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.Head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.Tail = node.prev
	}
	l.TotalQty = l.TotalQty.Sub(node.RemainQty)
	l.DisplayQty = l.DisplayQty.Sub(node.DisplayQty)
	l.OrderCount--
	node.prev = nil
	node.next = nil
	node.level = nil
}

// DecrementQty reduces node's quantity by fillQty and updates level totals.
// For iceberg nodes, DisplayQty is decremented by min(fillQty, node.DisplayQty).
func (l *PriceLevel) DecrementQty(node *OrderNode, fillQty types.Decimal) {
	node.RemainQty = node.RemainQty.Sub(fillQty)
	node.FilledQty = node.FilledQty.Add(fillQty)

	displayFill := types.Min(fillQty, node.DisplayQty)
	node.DisplayQty = node.DisplayQty.Sub(displayFill)

	l.TotalQty = l.TotalQty.Sub(fillQty)
	l.DisplayQty = l.DisplayQty.Sub(displayFill)
}

// ReplenishIceberg promotes HiddenQty to DisplayQty (up to OrigDisplayQty),
// assigns a new SeqNum (losing time priority), and moves the node to the tail.
func (l *PriceLevel) ReplenishIceberg(node *OrderNode, newSeqNum uint64) {
	replenish := types.Min(node.HiddenQty, node.OrigDisplayQty)

	// Remove node's current contribution from level totals.
	l.TotalQty = l.TotalQty.Sub(node.RemainQty)
	l.DisplayQty = l.DisplayQty.Sub(node.DisplayQty)
	l.OrderCount--

	// Update node state.
	node.HiddenQty = node.HiddenQty.Sub(replenish)
	node.RemainQty = replenish
	node.DisplayQty = replenish
	node.SeqNum = newSeqNum

	// Unlink from current position (without touching level totals â€” already adjusted).
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.Head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.Tail = node.prev
	}
	node.prev = nil
	node.next = nil

	// Append to tail with fresh totals.
	if l.Tail == nil {
		l.Head = node
		l.Tail = node
	} else {
		node.prev = l.Tail
		l.Tail.next = node
		l.Tail = node
	}
	l.TotalQty = l.TotalQty.Add(node.RemainQty)
	l.DisplayQty = l.DisplayQty.Add(node.DisplayQty)
	l.OrderCount++
	node.level = l
}

// IsEmpty returns true if the level has no orders.
func (l *PriceLevel) IsEmpty() bool { return l.Head == nil }

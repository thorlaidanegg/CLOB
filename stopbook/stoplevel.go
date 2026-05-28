package stopbook

import "github.com/thorlaidanegg/clob/types"

// StopLevel is a FIFO queue of stop orders at a single trigger price.
type StopLevel struct {
	Price      types.Decimal
	Head       *StopNode
	Tail       *StopNode
	OrderCount int
	PoolIndex  int
}

// Append adds node to the tail of the level. O(1).
func (l *StopLevel) Append(node *StopNode) {
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
	l.OrderCount++
	node.level = l
}

// Unlink removes node from the level. O(1).
func (l *StopLevel) Unlink(node *StopNode) {
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
	l.OrderCount--
	node.prev = nil
	node.next = nil
	node.level = nil
}

// IsEmpty returns true if the level has no stop orders.
func (l *StopLevel) IsEmpty() bool { return l.Head == nil }

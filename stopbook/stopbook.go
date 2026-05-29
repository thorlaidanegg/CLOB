// Package stopbook manages stop and stop-limit orders.
//
// Stop orders rest in a [StopBook] separate from the regular order book and
// are invisible to depth queries. [StopBook.CheckTriggers] is called after
// every trade; it returns [TriggeredOrder] values for any stops whose trigger
// price was crossed, which the engine converts and re-submits as limit or
// market orders. Cascade depth is capped by [config.MarketConfig.MaxCascadeDepth].
package stopbook

import (
	"errors"

	"github.com/tidwall/btree"
	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/types"
)

// ErrCascadeLimit is returned when CheckTriggers exceeds MaxCascadeDepth.
var ErrCascadeLimit = errors.New("clob/stopbook: cascade depth limit reached")

// TriggeredOrder is the result of a stop order firing. The engine processor
// converts each TriggeredOrder into a PlaceMarketOrder or PlaceLimitOrder
// command and re-queues it into the command channel.
// Defined here (not in engine/) to avoid the circular import that would result
// from engine/ depending on itself.
type TriggeredOrder struct {
	OrderID    types.OrderID
	UserID     types.UserID
	MarketID   types.MarketID
	Side       types.Side
	ConvertTo  types.OrderType // Market or Limit
	LimitPrice types.Decimal   // zero for stop-market
	Qty        types.Decimal
	TIF        types.TIF
	Flags      types.OrderFlags
	STPMode    interface{} // config.STPMode â€” use interface to avoid config import issue
}

// StopBook holds all pending stop and stop-limit orders.
// stopSells are sorted DESC (highest trigger first â€” fires when price falls to/below).
// stopBuys are sorted ASC (lowest trigger first â€” fires when price rises to/above).
type StopBook struct {
	stopSells *btree.BTreeG[*StopLevel] // DESC: highest trigger first
	stopBuys  *btree.BTreeG[*StopLevel] // ASC:  lowest trigger first
	index     map[types.OrderID]*StopNode
	nodePool  *pool.Pool[StopNode]
	levelPool *pool.Pool[StopLevel]
	maxDepth  int
}

// NewStopBook creates a StopBook with the given pools and cascade limit.
func NewStopBook(nodePool *pool.Pool[StopNode], levelPool *pool.Pool[StopLevel], maxCascadeDepth int) *StopBook {
	// stopSells: DESC â€” highest trigger price fires first when market falls.
	sellLess := func(a, b *StopLevel) bool {
		return a.Price.GreaterThan(b.Price)
	}
	// stopBuys: ASC â€” lowest trigger price fires first when market rises.
	buyLess := func(a, b *StopLevel) bool {
		return a.Price.LessThan(b.Price)
	}
	return &StopBook{
		stopSells: btree.NewBTreeGOptions(sellLess, btree.Options{Degree: 32, NoLocks: true}),
		stopBuys:  btree.NewBTreeGOptions(buyLess, btree.Options{Degree: 32, NoLocks: true}),
		index:     make(map[types.OrderID]*StopNode),
		nodePool:  nodePool,
		levelPool: levelPool,
		maxDepth:  maxCascadeDepth,
	}
}

// AddStop inserts a stop order into the appropriate tree.
func (s *StopBook) AddStop(node *StopNode) {
	s.index[node.OrderID] = node

	var tree *btree.BTreeG[*StopLevel]
	if node.Side == types.Ask { // stop sell
		tree = s.stopSells
	} else { // stop buy
		tree = s.stopBuys
	}

	sentinel := &StopLevel{Price: node.TriggerPrice}
	existing, ok := tree.Get(sentinel)
	if !ok {
		level, idx, err := s.levelPool.Acquire()
		if err != nil {
			panic("clob/stopbook: level pool exhausted")
		}
		level.PoolIndex = idx
		level.Price = node.TriggerPrice
		tree.Set(level)
		existing = level
	}
	existing.Append(node)
}

// CancelStop removes a stop order from the book.
func (s *StopBook) CancelStop(orderID types.OrderID, userID types.UserID) (*StopNode, bool) {
	node, ok := s.index[orderID]
	if !ok {
		return nil, false
	}
	delete(s.index, orderID)

	level := node.level
	level.Unlink(node)

	var tree *btree.BTreeG[*StopLevel]
	if node.Side == types.Ask {
		tree = s.stopSells
	} else {
		tree = s.stopBuys
	}

	if level.IsEmpty() {
		tree.Delete(level)
		s.levelPool.Release(level.PoolIndex)
	}
	return node, true
}

// Has returns true if orderID is in the stop book.
func (s *StopBook) Has(orderID types.OrderID) bool {
	_, ok := s.index[orderID]
	return ok
}

// CheckTriggers fires all stops whose trigger price is reached by lastTradePrice.
// depth is the current cascade recursion depth; callers start at 0.
// Returns triggered orders for the processor to convert and re-submit.
func (s *StopBook) CheckTriggers(lastTradePrice types.Decimal, depth int) ([]TriggeredOrder, error) {
	if depth >= s.maxDepth {
		return nil, ErrCascadeLimit
	}

	var triggered []TriggeredOrder

	// Stop sells fire when lastTradePrice <= triggerPrice.
	// stopSells is DESC, so Best() = highest trigger = most likely to fire next.
	for {
		best, ok := s.stopSells.Min()
		if !ok {
			break
		}
		if !lastTradePrice.LessThanOrEqual(best.Price) {
			break
		}
		// Drain the entire level.
		node := best.Head
		for node != nil {
			next := node.next
			triggered = append(triggered, s.convertToTriggered(node))
			delete(s.index, node.OrderID)
			s.nodePool.Release(node.PoolIndex)
			node = next
		}
		s.stopSells.Delete(best)
		s.levelPool.Release(best.PoolIndex)
	}

	// Stop buys fire when lastTradePrice >= triggerPrice.
	for {
		best, ok := s.stopBuys.Min()
		if !ok {
			break
		}
		if !lastTradePrice.GreaterThanOrEqual(best.Price) {
			break
		}
		node := best.Head
		for node != nil {
			next := node.next
			triggered = append(triggered, s.convertToTriggered(node))
			delete(s.index, node.OrderID)
			s.nodePool.Release(node.PoolIndex)
			node = next
		}
		s.stopBuys.Delete(best)
		s.levelPool.Release(best.PoolIndex)
	}

	return triggered, nil
}

// convertToTriggered converts a StopNode to a TriggeredOrder.
// The same OrderID carries through â€” preserves the audit trail end-to-end.
func (s *StopBook) convertToTriggered(node *StopNode) TriggeredOrder {
	return TriggeredOrder{
		OrderID:    node.OrderID,
		UserID:     node.UserID,
		MarketID:   node.MarketID,
		Side:       node.Side,
		ConvertTo:  node.ConvertTo,
		LimitPrice: node.LimitPrice,
		Qty:        node.Qty,
		TIF:        node.TIF,
		Flags:      node.Flags,
		STPMode:    node.STPMode,
	}
}

// Len returns the number of stop orders in the book.
func (s *StopBook) Len() int { return len(s.index) }

// AcquireNode acquires a StopNode from the book's node pool.
func (s *StopBook) AcquireNode() (*StopNode, int, error) {
	return s.nodePool.Acquire()
}

// ReleaseNode returns a StopNode slot back to the book's node pool.
func (s *StopBook) ReleaseNode(idx int) {
	s.nodePool.Release(idx)
}

package engine

import (
	"time"

	"github.com/thorlaidanegg/clob/types"
)

// RecoveredOrder describes a single resting order to seed into a fresh engine at
// startup, reconstructed from a durable source (e.g. an event-log replay). It is
// the input to [WithInitialOrders].
//
// Only orders that actually rest in a book are meaningful here: limit/iceberg
// orders (which go into the continuous book) and stop/stop-limit orders (which
// go into the stop book). Market orders never rest and are ignored.
type RecoveredOrder struct {
	OrderID types.OrderID
	UserID  types.UserID
	Side    types.Side

	// Type selects where the order is seeded:
	//   Limit, Iceberg        -> continuous book (resting)
	//   Stop, StopLimit       -> stop book (pending trigger)
	//   Market                -> ignored (never rests)
	Type types.OrderType

	Price     types.Decimal // resting limit price (Limit/Iceberg/StopLimit)
	StopPrice types.Decimal // trigger price (Stop/StopLimit)

	RemainQty  types.Decimal // remaining quantity still working
	DisplayQty types.Decimal // iceberg visible portion; equals RemainQty if not iceberg

	Flags    types.OrderFlags
	TIF      types.TIF
	ExpireAt int64
}

// seedInitialOrders places recovered orders directly into the book and stop book
// without matching, without invoking the pre-order hook, and without emitting any
// events. It runs synchronously in [Engine.Start] before the processor goroutine
// launches, so the book is single-threaded-owned here and no locking is needed.
//
// Recovered orders are assigned fresh, monotonically increasing sequence numbers
// in input order, so callers must pass them in their original time priority.
func (p *CommandProcessor) seedInitialOrders(orders []RecoveredOrder) {
	now := time.Now().UnixNano()
	qp := p.cfg.QtyPrecision

	for _, ro := range orders {
		switch ro.Type {
		case types.Stop, types.StopLimit:
			node, idx, err := p.stopBook.AcquireNode()
			if err != nil {
				continue // pool exhausted — skip rather than crash recovery
			}
			node.PoolIndex = idx
			node.OrderID = ro.OrderID
			node.UserID = ro.UserID
			node.MarketID = p.cfg.MarketID
			node.Side = ro.Side
			node.TriggerPrice = ro.StopPrice
			node.Qty = ro.RemainQty
			node.TIF = ro.TIF
			node.Flags = ro.Flags
			node.ExpireAt = ro.ExpireAt
			node.Timestamp = now
			node.SeqNum = p.orderSeq.Next()
			if ro.Type == types.StopLimit {
				node.ConvertTo = types.Limit
				node.LimitPrice = ro.Price
			} else {
				node.ConvertTo = types.Market
			}
			p.stopBook.AddStop(node)

		case types.Market:
			continue // never rests

		default: // Limit, Iceberg
			node, idx, err := p.nodePool.Acquire()
			if err != nil {
				continue
			}
			node.PoolIndex = idx
			node.OrderID = ro.OrderID
			node.UserID = ro.UserID
			node.MarketID = p.cfg.MarketID
			node.Side = ro.Side
			node.Type = types.Limit
			node.Price = ro.Price
			node.OrigQty = ro.RemainQty
			node.RemainQty = ro.RemainQty
			node.FilledQty = types.Zero(qp)
			node.TIF = ro.TIF
			node.Flags = ro.Flags
			node.ExpireAt = ro.ExpireAt
			node.Timestamp = now
			node.SeqNum = p.orderSeq.Next()

			if ro.Flags.Has(types.FlagIceberg) && ro.DisplayQty.IsPositive() && ro.DisplayQty.LessThan(ro.RemainQty) {
				node.DisplayQty = ro.DisplayQty
				node.OrigDisplayQty = ro.DisplayQty
				node.HiddenQty = ro.RemainQty.Sub(ro.DisplayQty)
				node.RemainQty = ro.DisplayQty
			} else {
				node.DisplayQty = ro.RemainQty
				node.OrigDisplayQty = ro.RemainQty
				node.HiddenQty = types.Zero(qp)
			}
			p.book.PlaceResting(node)
		}
	}
}

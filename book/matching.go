package book

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// Disposition describes the outcome of a match attempt.
type Disposition uint8

const (
	FullyFilled          Disposition = 1
	PartialFill_Rested   Disposition = 2
	PartialFill_Canceled Disposition = 3
	Rested               Disposition = 4
	Canceled             Disposition = 5
	Rejected             Disposition = 6
	STPCanceled_Taker    Disposition = 7 // taker zeroed by STP CancelTaker or CancelBoth
)

// STPCanceled records the fields needed to emit an OrderCanceled event for a
// maker removed from the book by self-trade prevention. Captured before the
// node is released to the pool so the processor can safely read it.
type STPCanceled struct {
	OrderID   types.OrderID
	UserID    types.UserID
	Side      types.Side
	Price     types.Decimal
	RemainQty types.Decimal
	FilledQty types.Decimal
}

// match attempts to fill incoming against the opposite side of the book.
// For FOK orders, performs a dry-run first; if it fails, returns Rejected
// with zero state change.
// Returns the fills produced, any STP-canceled makers, and the final disposition.
func (b *OrderBook) match(incoming *OrderNode) ([]types.Fill, []STPCanceled, Disposition) {
	// FOK dry-run: check if full fill is possible before any mutation.
	if incoming.TIF == types.FOK {
		if !b.canFillFull(incoming) {
			b.nodePool.Release(incoming.PoolIndex)
			return nil, nil, Rejected
		}
	}

	fills := make([]types.Fill, 0, 8)
	var stpCancels []STPCanceled
	var stpCanceledTaker bool // set when CancelTaker/CancelBoth zeros the incoming order

	var oppTree *PriceLevelTree
	if incoming.Side == types.Bid {
		oppTree = b.asks
	} else {
		oppTree = b.bids
	}

	for incoming.RemainQty.IsPositive() {
		bestLevel := oppTree.Best()
		if bestLevel == nil {
			break
		}
		if !b.crosses(incoming, bestLevel) {
			break
		}

		node := bestLevel.Head
		for node != nil && incoming.RemainQty.IsPositive() {
			// STP check.
			if b.stpEnabled(incoming, node) {
				mode := b.effectiveSTPMode(incoming, node)
				var cont bool
				node, cont = b.applySTP(incoming, node, bestLevel, mode, &stpCancels)
				if !cont {
					// RemainQty is positive entering applySTP (loop invariant).
					// If it's now zero without a fill, STP zeroed it (CancelTaker/CancelBoth).
					stpCanceledTaker = incoming.RemainQty.IsZero()
					goto done
				}
				continue
			}

			fillQty := types.Min(node.RemainQty, incoming.RemainQty)

			// Mutate incoming.
			incoming.RemainQty = incoming.RemainQty.Sub(fillQty)
			incoming.FilledQty = incoming.FilledQty.Add(fillQty)

			// Mutate maker and level.
			bestLevel.DecrementQty(node, fillQty)

			f := types.Fill{
				MakerOrderID:   node.OrderID,
				TakerOrderID:   incoming.OrderID,
				MakerUserID:    node.UserID,
				TakerUserID:    incoming.UserID,
				MakerSide:      node.Side,
				Price:          bestLevel.Price,
				Qty:            fillQty,
				MakerRemainQty: node.RemainQty,
				TakerRemainQty: incoming.RemainQty,
				MakerSeqNum:    node.SeqNum,
				TakerSeqNum:    incoming.SeqNum,
				// Timestamp set by caller (processor) after match returns.
			}

			makerExhausted := node.RemainQty.IsZero()

			// Iceberg replenishment — level stays non-empty.
			if makerExhausted && node.HiddenQty.IsPositive() {
				bestLevel.ReplenishIceberg(node, b.orderSeq.Next())
				f.MakerLevelExists = true
				f.MakerLevelTotalQty = bestLevel.TotalQty
				f.MakerLevelDisplayQty = bestLevel.DisplayQty
				f.MakerLevelOrderCount = bestLevel.OrderCount
				fills = append(fills, f)
				node = bestLevel.Head
				continue
			}

			// Remove fully filled maker (non-iceberg).
			if makerExhausted {
				next := node.next
				bestLevel.Unlink(node)
				b.index.Delete(node.OrderID)
				b.nodePool.Release(node.PoolIndex)
				node = next
			}

			// Capture level state after all mutations for this fill.
			f.MakerLevelExists = !bestLevel.IsEmpty()
			if f.MakerLevelExists {
				f.MakerLevelTotalQty = bestLevel.TotalQty
				f.MakerLevelDisplayQty = bestLevel.DisplayQty
				f.MakerLevelOrderCount = bestLevel.OrderCount
			}
			fills = append(fills, f)

			if !makerExhausted {
				break // incoming partially filled this maker; done at this level
			}
		}

		if bestLevel.IsEmpty() {
			oppTree.Delete(bestLevel.Price)
			b.levelPool.Release(bestLevel.PoolIndex)
		}
	}

done:
	// Determine disposition.
	if stpCanceledTaker {
		b.nodePool.Release(incoming.PoolIndex)
		return fills, stpCancels, STPCanceled_Taker
	}
	if incoming.RemainQty.IsZero() {
		b.nodePool.Release(incoming.PoolIndex)
		return fills, stpCancels, FullyFilled
	}

	if incoming.TIF == types.IOC {
		b.nodePool.Release(incoming.PoolIndex)
		if len(fills) > 0 {
			return fills, stpCancels, PartialFill_Canceled
		}
		return fills, stpCancels, Canceled
	}

	if incoming.Type == types.Market {
		b.nodePool.Release(incoming.PoolIndex)
		return fills, stpCancels, Canceled
	}

	// Rest the order.
	b.restNode(incoming)
	if len(fills) > 0 {
		return fills, stpCancels, PartialFill_Rested
	}
	return fills, stpCancels, Rested
}

// crosses returns true if incoming would fill against level.
func (b *OrderBook) crosses(incoming *OrderNode, level *PriceLevel) bool {
	if incoming.Type == types.Market {
		return true
	}
	if incoming.Side == types.Bid {
		return incoming.Price.GreaterThanOrEqual(level.Price)
	}
	return incoming.Price.LessThanOrEqual(level.Price)
}

// canFillFull returns true if the book has enough liquidity to fully fill incoming.
// This is the FOK dry-run â€” it mutates nothing.
func (b *OrderBook) canFillFull(incoming *OrderNode) bool {
	var available types.Decimal
	available = types.Zero(incoming.RemainQty.Precision())

	var oppTree *PriceLevelTree
	if incoming.Side == types.Bid {
		oppTree = b.asks
	} else {
		oppTree = b.bids
	}

	oppTree.Iterate(func(level *PriceLevel) bool {
		if !b.crosses(incoming, level) {
			return false
		}
		available = available.Add(level.TotalQty)
		return available.LessThan(incoming.RemainQty)
	})
	return available.GreaterThanOrEqual(incoming.RemainQty)
}

// stpEnabled returns true if both orders share the same UserID and STP is active.
func (b *OrderBook) stpEnabled(incoming, maker *OrderNode) bool {
	if incoming.UserID != maker.UserID {
		return false
	}
	mode := b.effectiveSTPMode(incoming, maker)
	return mode != config.STPDisabled
}

// effectiveSTPMode returns the STP mode to apply for a pair of matching orders.
// Per-order override takes precedence over market default.
func (b *OrderBook) effectiveSTPMode(incoming, maker *OrderNode) config.STPMode {
	if incoming.STPMode != config.STPDisabled {
		return incoming.STPMode
	}
	if maker.STPMode != config.STPDisabled {
		return maker.STPMode
	}
	return b.config.STPMode
}

// applySTP applies the self-trade prevention rule and returns the next node
// to consider and whether matching should continue.
// It appends an STPCanceled entry to cancels for every maker node it removes.
func (b *OrderBook) applySTP(incoming, maker *OrderNode, level *PriceLevel, mode config.STPMode, cancels *[]STPCanceled) (*OrderNode, bool) {
	switch mode {
	case config.STPCancelBoth:
		*cancels = append(*cancels, b.cancelMakerNode(maker, level))
		incoming.RemainQty = types.Zero(incoming.RemainQty.Precision())
		return nil, false

	case config.STPCancelMaker:
		next := maker.next
		*cancels = append(*cancels, b.cancelMakerNode(maker, level))
		return next, true

	case config.STPCancelTaker:
		incoming.RemainQty = types.Zero(incoming.RemainQty.Precision())
		return nil, false

	case config.STPDecrementCancel:
		if maker.RemainQty.GreaterThan(incoming.RemainQty) {
			level.DecrementQty(maker, incoming.RemainQty)
			incoming.RemainQty = types.Zero(incoming.RemainQty.Precision())
			return nil, false
		}
		incoming.RemainQty = incoming.RemainQty.Sub(maker.RemainQty)
		incoming.FilledQty = incoming.FilledQty.Add(maker.RemainQty)
		next := maker.next
		*cancels = append(*cancels, b.cancelMakerNode(maker, level))
		return next, true

	default:
		return maker.next, true
	}
}

// cancelMakerNode removes maker from the book due to STP and returns a
// snapshot of its identity for OrderCanceled event emission. The node is
// released to the pool before returning, so callers must not dereference it.
func (b *OrderBook) cancelMakerNode(maker *OrderNode, level *PriceLevel) STPCanceled {
	sc := STPCanceled{
		OrderID:   maker.OrderID,
		UserID:    maker.UserID,
		Side:      maker.Side,
		Price:     level.Price,
		RemainQty: maker.RemainQty,
		FilledQty: maker.FilledQty,
	}
	level.Unlink(maker)
	b.index.Delete(maker.OrderID)
	if level.IsEmpty() {
		var oppTree *PriceLevelTree
		if maker.Side == types.Bid {
			oppTree = b.bids
		} else {
			oppTree = b.asks
		}
		oppTree.Delete(level.Price)
		b.levelPool.Release(level.PoolIndex)
	}
	b.nodePool.Release(maker.PoolIndex)
	return sc
}

// restNode adds incoming to the resting book on its own side.
func (b *OrderBook) restNode(node *OrderNode) {
	var tree *PriceLevelTree
	if node.Side == types.Bid {
		tree = b.bids
	} else {
		tree = b.asks
	}

	// Initialize level quantity fields with the right precision on first use.
	level, created := tree.GetOrCreate(node.Price, b.levelPool)
	if created {
		level.TotalQty = types.Zero(node.RemainQty.Precision())
		level.DisplayQty = types.Zero(node.DisplayQty.Precision())
	}
	level.Append(node)
	b.index.Put(node.OrderID, node)
}

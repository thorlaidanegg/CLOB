// Package book implements the continuous limit order book.
//
// [OrderBook] holds a bid-side and ask-side [PriceLevelTree], an [OrderIndex]
// for O(1) cancel, and a shared [pool.Pool] for zero-alloc node reuse.
// All operations are single-threaded; the engine's command goroutine is the
// sole owner.
package book

import (
	"errors"
	"fmt"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/sequence"
	"github.com/thorlaidanegg/clob/types"
)

// Error sentinels for book operations.
var (
	ErrOrderNotFound     = errors.New("clob/book: order not found")
	ErrOwnershipMismatch = errors.New("clob/book: cancel rejected, wrong user")
	ErrDuplicateOrderID  = errors.New("clob/book: duplicate order id")
)

// OrderBook is the central limit order book.
// All methods must be called from a single goroutine â€” no locking.
type OrderBook struct {
	bids      *PriceLevelTree
	asks      *PriceLevelTree
	index     *OrderIndex
	nodePool  *pool.Pool[OrderNode]
	levelPool *pool.Pool[PriceLevel]
	config    *config.MarketConfig
	// orderSeq is shared with the CommandProcessor so SeqNums are globally monotonic.
	orderSeq *sequence.Counter
}

// NewOrderBook constructs an OrderBook. nodePool, levelPool, and orderSeq must
// be the same instances shared with the engine's CommandProcessor.
func NewOrderBook(cfg *config.MarketConfig, nodePool *pool.Pool[OrderNode], levelPool *pool.Pool[PriceLevel], orderSeq *sequence.Counter) *OrderBook {
	return &OrderBook{
		bids:      NewPriceLevelTree(types.Bid),
		asks:      NewPriceLevelTree(types.Ask),
		index:     newOrderIndex(),
		nodePool:  nodePool,
		levelPool: levelPool,
		config:    cfg,
		orderSeq:  orderSeq,
	}
}

// PlaceLimit submits a limit order (or iceberg) to the book.
// node must be acquired from nodePool by the caller before calling this.
// Returns the fills produced and the final disposition.
func (b *OrderBook) PlaceLimit(node *OrderNode) ([]types.Fill, Disposition) {
	return b.match(node)
}

// PlaceResting adds node directly to the resting book without running the match
// loop. Used when the market is halted — orders queue but do not execute.
func (b *OrderBook) PlaceResting(node *OrderNode) {
	b.restNode(node)
}

// PlaceMarket submits a market order to the book.
// Market orders always cross; they are never rested.
func (b *OrderBook) PlaceMarket(node *OrderNode) ([]types.Fill, Disposition) {
	return b.match(node)
}

// Cancel removes an order from the book. Returns errors for not-found or ownership mismatch.
// On success, releases the node back to the pool.
func (b *OrderBook) Cancel(orderID types.OrderID, userID types.UserID) (*OrderNode, error) {
	node, ok := b.index.Get(orderID)
	if !ok {
		return nil, ErrOrderNotFound
	}
	if node.UserID != userID {
		return nil, fmt.Errorf("%w: order %s belongs to %s, not %s",
			ErrOwnershipMismatch, orderID, node.UserID, userID)
	}

	level := node.level
	level.Unlink(node)
	b.index.Delete(orderID)

	if level.IsEmpty() {
		var tree *PriceLevelTree
		if node.Side == types.Bid {
			tree = b.bids
		} else {
			tree = b.asks
		}
		tree.Delete(level.Price)
		b.levelPool.Release(level.PoolIndex)
	}

	// Return node to caller for event emission; caller releases to pool.
	return node, nil
}

// HasOrder returns true if orderID is resting in the book.
func (b *OrderBook) HasOrder(id types.OrderID) bool {
	return b.index.Has(id)
}

// WouldCross returns true if a limit order at price on side would immediately
// fill against the resting book. Used for PostOnly pre-check.
func (b *OrderBook) WouldCross(price types.Decimal, side types.Side) bool {
	if side == types.Bid {
		best := b.asks.Best()
		if best == nil {
			return false
		}
		return price.GreaterThanOrEqual(best.Price)
	}
	best := b.bids.Best()
	if best == nil {
		return false
	}
	return price.LessThanOrEqual(best.Price)
}

// BBO returns the best bid and ask prices.
func (b *OrderBook) BBO() (bid, ask types.Decimal, hasBid, hasAsk bool) {
	if best := b.bids.Best(); best != nil {
		bid = best.Price
		hasBid = true
	}
	if best := b.asks.Best(); best != nil {
		ask = best.Price
		hasAsk = true
	}
	return
}

// Snapshot returns depth levels for both sides.
func (b *OrderBook) Snapshot(levels int) (bids, asks []DepthLevel) {
	return b.bids.Depth(levels), b.asks.Depth(levels)
}

// ExpireGTD scans all resting orders and removes those with ExpireAt <= now.
// Returns the expired nodes (caller emits events and releases nodes to pool).
func (b *OrderBook) ExpireGTD(now int64) []*OrderNode {
	var expired []*OrderNode
	b.index.IterateExpired(now, func(node *OrderNode) {
		level := node.level
		level.Unlink(node)
		b.index.Delete(node.OrderID)

		if level.IsEmpty() {
			var tree *PriceLevelTree
			if node.Side == types.Bid {
				tree = b.bids
			} else {
				tree = b.asks
			}
			tree.Delete(level.Price)
			b.levelPool.Release(level.PoolIndex)
		}
		expired = append(expired, node)
	})
	return expired
}

// LevelInfo returns the current state of a price level for DepthUpdate emission.
// Returns exists=false when the level has been fully consumed.
func (b *OrderBook) LevelInfo(side types.Side, price types.Decimal) (totalQty, displayQty types.Decimal, orderCount int, exists bool) {
	var tree *PriceLevelTree
	if side == types.Bid {
		tree = b.bids
	} else {
		tree = b.asks
	}
	level, ok := tree.Get(price)
	if !ok {
		return types.Zero(b.config.QtyPrecision), types.Zero(b.config.QtyPrecision), 0, false
	}
	return level.TotalQty, level.DisplayQty, level.OrderCount, true
}

// WouldExceedMaxDepth returns true if placing an order at price on side would
// create a new price level beyond the configured MaxDepth.
// Returns false when MaxDepth is 0 (unlimited), when the price already has a
// level, or when the new price is within the top MaxDepth levels by price.
func (b *OrderBook) WouldExceedMaxDepth(price types.Decimal, side types.Side) bool {
	if b.config.MaxDepth == 0 {
		return false
	}
	var tree *PriceLevelTree
	if side == types.Bid {
		tree = b.bids
	} else {
		tree = b.asks
	}
	if tree.Len() < b.config.MaxDepth {
		return false
	}
	// Price already has a level — same level, no depth increase.
	if _, ok := tree.Get(price); ok {
		return false
	}
	// New price would add a level. Check if it falls within top MaxDepth by price.
	levels := tree.Depth(b.config.MaxDepth)
	if len(levels) < b.config.MaxDepth {
		return false
	}
	worst := levels[b.config.MaxDepth-1].Price
	if side == types.Bid {
		return price.LessThan(worst) // worse than the Nth-best bid
	}
	return price.GreaterThan(worst) // worse than the Nth-best ask
}

// OpenOrderCount returns the number of resting orders.
func (b *OrderBook) OpenOrderCount() int { return b.index.Len() }

// BidLevelCount returns the number of bid price levels.
func (b *OrderBook) BidLevelCount() int { return b.bids.Len() }

// AskLevelCount returns the number of ask price levels.
func (b *OrderBook) AskLevelCount() int { return b.asks.Len() }

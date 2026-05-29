package book

import (
	"github.com/tidwall/btree"

	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/types"
)

// PriceLevelTree is a B-Tree of PriceLevels, ordered by price.
// Bid side: descending order (Best() = highest bid).
// Ask side: ascending order (Best() = lowest ask).
// Both sides use tree.Min() for the best price; the comparator handles direction.
type PriceLevelTree struct {
	tree  btree.BTreeG[*PriceLevel]
	side  types.Side
	count int
}

// NewPriceLevelTree creates a tree for the given side with degree 32.
// Degree 32 is cache-friendly: a node fits in 2â€“3 cache lines and provides
// O(log n) access at 1M price levels with only ~5 levels of depth.
func NewPriceLevelTree(side types.Side) *PriceLevelTree {
	var less func(a, b *PriceLevel) bool
	if side == types.Bid {
		// Descending: higher price is "less" so tree.Min() returns best bid.
		less = func(a, b *PriceLevel) bool {
			return a.Price.GreaterThan(b.Price)
		}
	} else {
		// Ascending: lower price is "less" so tree.Min() returns best ask.
		less = func(a, b *PriceLevel) bool {
			return a.Price.LessThan(b.Price)
		}
	}
	return &PriceLevelTree{
		tree: *btree.NewBTreeGOptions(less, btree.Options{Degree: 32, NoLocks: true}),
		side: side,
	}
}

// Best returns the best-priced level (highest bid or lowest ask), or nil.
func (t *PriceLevelTree) Best() *PriceLevel {
	item, ok := t.tree.Min()
	if !ok {
		return nil
	}
	return item
}

// Insert adds a price level to the tree.
func (t *PriceLevelTree) Insert(level *PriceLevel) {
	t.tree.Set(level)
	t.count++
}

// Delete removes the level at the given price from the tree.
func (t *PriceLevelTree) Delete(price types.Decimal) {
	t.tree.Delete(&PriceLevel{Price: price})
	t.count--
}

// Get returns the level at the given price, or nil if not found.
func (t *PriceLevelTree) Get(price types.Decimal) (*PriceLevel, bool) {
	return t.tree.Get(&PriceLevel{Price: price})
}

// GetOrCreate returns the existing level at price, or acquires a new one from
// levelPool. Returns the level and true if a new level was created.
// Panics if levelPool is exhausted â€” size the pool generously.
func (t *PriceLevelTree) GetOrCreate(price types.Decimal, levelPool *pool.Pool[PriceLevel]) (*PriceLevel, bool) {
	if level, ok := t.Get(price); ok {
		return level, false
	}
	level, idx, err := levelPool.Acquire()
	if err != nil {
		panic("clob/book: level pool exhausted â€” increase WithLevelPoolSize")
	}
	level.PoolIndex = idx
	level.Price = price
	// Initialize quantity fields to zero at the right precision.
	// They will be set correctly when the first node is appended.
	t.Insert(level)
	return level, true
}

// DepthLevel is a snapshot of a single price level for external consumption.
type DepthLevel struct {
	Price      types.Decimal
	TotalQty   types.Decimal
	DisplayQty types.Decimal
	OrderCount int
}

// Depth returns the top-n levels from best to worst price.
func (t *PriceLevelTree) Depth(n int) []DepthLevel {
	if n <= 0 {
		return nil
	}
	result := make([]DepthLevel, 0, n)
	t.tree.Scan(func(level *PriceLevel) bool {
		result = append(result, DepthLevel{
			Price:      level.Price,
			TotalQty:   level.TotalQty,
			DisplayQty: level.DisplayQty,
			OrderCount: level.OrderCount,
		})
		return len(result) < n
	})
	return result
}

// Iterate calls fn for each level from best to worst price.
// Stops if fn returns false.
func (t *PriceLevelTree) Iterate(fn func(*PriceLevel) bool) {
	t.tree.Scan(fn)
}

// Len returns the number of price levels in the tree.
func (t *PriceLevelTree) Len() int { return t.count }

// IsEmpty returns true if the tree has no price levels.
func (t *PriceLevelTree) IsEmpty() bool { return t.count == 0 }

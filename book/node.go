// Package book implements the order book data structures and matching logic.
package book

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// OrderNode is a single order resting in the book.
// All instances are acquired from the node pool; never allocate directly.
type OrderNode struct {
	// Identity
	OrderID  types.OrderID
	UserID   types.UserID
	MarketID types.MarketID
	Side     types.Side
	Type     types.OrderType
	TIF      types.TIF
	Flags    types.OrderFlags
	STPMode  config.STPMode // per-order override; zero = market default

	// Pricing
	Price     types.Decimal // zero for market orders
	StopPrice types.Decimal // trigger price (only meaningful while in stop book)

	// Quantity
	OrigQty        types.Decimal
	RemainQty      types.Decimal
	FilledQty      types.Decimal
	DisplayQty     types.Decimal // iceberg: visible portion; non-iceberg: equals RemainQty
	HiddenQty      types.Decimal // iceberg: reserve; non-iceberg: always zero
	OrigDisplayQty types.Decimal // iceberg: initial display lot for replenishment

	// Time priority â€” SeqNum is the ONLY determinant of time priority.
	// Lower SeqNum = earlier in queue = fills first.
	// Timestamp is stored for reporting only; NEVER used for ordering.
	SeqNum    uint64
	Timestamp int64

	// Expiry (unix ns, zero for GTC/IOC/FOK)
	ExpireAt int64

	// Linked list within a PriceLevel
	prev *OrderNode
	next *OrderNode

	// Back-pointer to owning PriceLevel â€” enables O(1) cancel
	level *PriceLevel

	// Pool slot index â€” set by caller after pool.Acquire()
	PoolIndex int
}

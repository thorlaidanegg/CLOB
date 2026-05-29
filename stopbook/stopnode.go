// Package stopbook manages stop orders pending price trigger.
package stopbook

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// StopNode holds a pending stop or stop-limit order in the stop book.
// All instances are acquired from the node pool.
type StopNode struct {
	OrderID  types.OrderID
	UserID   types.UserID
	MarketID types.MarketID
	Side     types.Side
	TIF      types.TIF
	Flags    types.OrderFlags
	STPMode  config.STPMode

	TriggerPrice types.Decimal // activation threshold
	LimitPrice   types.Decimal // zero for stop-market
	Qty          types.Decimal
	ConvertTo    types.OrderType // Market | Limit (what it becomes on trigger)

	ExpireAt  int64
	Timestamp int64
	SeqNum    uint64

	prev *StopNode
	next *StopNode

	level     *StopLevel
	PoolIndex int
}

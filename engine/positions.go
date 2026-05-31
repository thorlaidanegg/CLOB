package engine

import "github.com/thorlaidanegg/clob/types"

// positionTracker maintains a signed net quantity position per user.
//
// Positive value = net long (cumulative buys exceed sells).
// Negative value = net short (cumulative sells exceed buys).
//
// Single-goroutine use only — no mutex.
type positionTracker struct {
	positions map[types.UserID]types.Decimal
	precision uint8
}

func newPositionTracker(qtyPrecision uint8) *positionTracker {
	return &positionTracker{
		positions: make(map[types.UserID]types.Decimal),
		precision: qtyPrecision,
	}
}

func (pt *positionTracker) get(userID types.UserID) types.Decimal {
	if pos, ok := pt.positions[userID]; ok {
		return pos
	}
	return types.Zero(pt.precision)
}

// applyFill updates both the maker's and taker's net positions after a trade.
func (pt *positionTracker) applyFill(f types.Fill) {
	// MakerSide is the side of the resting order.
	// A bid maker bought; an ask maker sold.
	makerDelta := f.Qty
	takerDelta := f.Qty
	if f.MakerSide == types.Ask {
		makerDelta = makerDelta.Neg() // maker sold → decreases position
	} else {
		takerDelta = takerDelta.Neg() // taker sold (opposite of bid maker) → decreases position
	}
	pt.add(f.MakerUserID, makerDelta)
	pt.add(f.TakerUserID, takerDelta)
}

func (pt *positionTracker) add(userID types.UserID, delta types.Decimal) {
	pos := pt.get(userID)
	pt.positions[userID] = pos.Add(delta)
}

// wouldIncrease reports whether placing a new order of qty on side for userID
// would increase the absolute magnitude of the user's position.
//
// A bid increases position when it takes the user from short/flat toward long
// and the resulting net position crosses zero.
// An ask increases position symmetrically.
func (pt *positionTracker) wouldIncrease(userID types.UserID, side types.Side, qty types.Decimal) bool {
	pos := pt.get(userID)
	if side == types.Bid {
		// Buying adds qty to position. Reject if result > 0 (goes long or stays long).
		return pos.Add(qty).IsPositive()
	}
	// Selling subtracts qty from position. Reject if result < 0 (goes short or stays short).
	return pos.Sub(qty).IsNegative()
}

package auction

import (
	"github.com/thorlaidanegg/clob/types"
)

// Sweep matches orders at clearingPrice and returns:
//   - fills: all matched fills (each executes at clearingPrice regardless of limit price)
//   - unmatched: GTC orders with remaining quantity â€” the processor converts these to PlaceLimitOrder
//     commands for the continuous book. IOC/FOK unmatched orders are excluded (caller emits OrderCanceled).
func (a *AuctionBook) Sweep(clearingPrice types.Decimal) (fills []types.Fill, unmatched []AuctionOrder) {
	// Eligible bids: price >= clearingPrice
	var eligBids []AuctionOrder
	for _, o := range a.bids {
		if o.Price.GreaterThanOrEqual(clearingPrice) {
			eligBids = append(eligBids, o)
		}
	}
	// Eligible asks: price <= clearingPrice
	var eligAsks []AuctionOrder
	for _, o := range a.asks {
		if o.Price.LessThanOrEqual(clearingPrice) {
			eligAsks = append(eligAsks, o)
		}
	}

	// Track remaining quantities.
	bidRemain := make([]types.Decimal, len(eligBids))
	for i, o := range eligBids {
		bidRemain[i] = o.Qty
	}
	askRemain := make([]types.Decimal, len(eligAsks))
	for i, o := range eligAsks {
		askRemain[i] = o.Qty
	}

	now := int64(0) // Timestamp filled in by the engine/processor when re-emitting events.

	bi, ai := 0, 0
	for bi < len(eligBids) && ai < len(eligAsks) {
		if bidRemain[bi].IsZero() {
			bi++
			continue
		}
		if askRemain[ai].IsZero() {
			ai++
			continue
		}

		fillQty := bidRemain[bi]
		if askRemain[ai].LessThan(fillQty) {
			fillQty = askRemain[ai]
		}

		fills = append(fills, types.Fill{
			MakerOrderID:   eligAsks[ai].OrderID,
			TakerOrderID:   eligBids[bi].OrderID,
			MakerUserID:    eligAsks[ai].UserID,
			TakerUserID:    eligBids[bi].UserID,
			MakerSide:      types.Ask,
			Price:          clearingPrice,
			Qty:            fillQty,
			MakerRemainQty: askRemain[ai].Sub(fillQty),
			TakerRemainQty: bidRemain[bi].Sub(fillQty),
			MakerSeqNum:    eligAsks[ai].SeqNum,
			TakerSeqNum:    eligBids[bi].SeqNum,
			Timestamp:      now,
		})

		bidRemain[bi] = bidRemain[bi].Sub(fillQty)
		askRemain[ai] = askRemain[ai].Sub(fillQty)
	}

	// Collect GTC orders with remaining quantity for the continuous book.
	for i, o := range eligBids {
		if !bidRemain[i].IsZero() && o.TIF == types.GTC {
			remaining := o
			remaining.Qty = bidRemain[i]
			unmatched = append(unmatched, remaining)
		}
	}
	for i, o := range eligAsks {
		if !askRemain[i].IsZero() && o.TIF == types.GTC {
			remaining := o
			remaining.Qty = askRemain[i]
			unmatched = append(unmatched, remaining)
		}
	}

	// Non-eligible orders (price outside the clearing range) that are GTC also go to continuous book.
	for _, o := range a.bids {
		if o.Price.LessThan(clearingPrice) && o.TIF == types.GTC {
			unmatched = append(unmatched, o)
		}
	}
	for _, o := range a.asks {
		if o.Price.GreaterThan(clearingPrice) && o.TIF == types.GTC {
			unmatched = append(unmatched, o)
		}
	}

	return fills, unmatched
}

package auction

import "github.com/thorlaidanegg/clob/types"

// ComputeClearingPrice finds the price that maximises matched volume.
// Tiebreakers (in order): maximum executable quantity, minimum imbalance,
// closest to refPrice, then the reference price itself.
// Pass a zero refPrice to skip the reference-price tiebreaker.
// Returns zero-value Decimals with ok=false when there are no crossing orders.
func (a *AuctionBook) ComputeClearingPrice(refPrice types.Decimal) (price types.Decimal, matchableQty types.Decimal, ok bool) {
	if len(a.bids) == 0 || len(a.asks) == 0 {
		return types.Zero(2), types.Zero(0), false
	}

	// Collect unique price levels present across both sides.
	priceSet := make(map[string]types.Decimal)
	for _, o := range a.bids {
		priceSet[o.Price.String()] = o.Price
	}
	for _, o := range a.asks {
		priceSet[o.Price.String()] = o.Price
	}

	var prices []types.Decimal
	for _, p := range priceSet {
		prices = append(prices, p)
	}
	// Sort prices ascending for iteration.
	for i := 0; i < len(prices)-1; i++ {
		for j := i + 1; j < len(prices); j++ {
			if prices[i].GreaterThan(prices[j]) {
				prices[i], prices[j] = prices[j], prices[i]
			}
		}
	}

	hasRef := !refPrice.IsZero()

	bestPrice := types.Zero(2)
	bestQty := types.Zero(0)
	bestImbalance := types.Zero(0)
	found := false

	for _, candidate := range prices {
		// Cumulative bid quantity at or above candidate.
		cumBid := types.Zero(0)
		for _, o := range a.bids {
			if o.Price.GreaterThanOrEqual(candidate) {
				cumBid = cumBid.Add(o.Qty)
			}
		}
		// Cumulative ask quantity at or below candidate.
		cumAsk := types.Zero(0)
		for _, o := range a.asks {
			if o.Price.LessThanOrEqual(candidate) {
				cumAsk = cumAsk.Add(o.Qty)
			}
		}

		execQty := cumBid
		if cumAsk.LessThan(execQty) {
			execQty = cumAsk
		}

		if execQty.IsZero() {
			continue
		}

		imbalance := cumBid.Sub(cumAsk).Abs()

		betterRef := false
		if hasRef && found && execQty.Equal(bestQty) && imbalance.Equal(bestImbalance) {
			distCand := candidate.Sub(refPrice).Abs()
			distBest := bestPrice.Sub(refPrice).Abs()
			if distCand.LessThan(distBest) {
				betterRef = true
			} else if distCand.Equal(distBest) {
				// Equidistant: prefer the reference price itself.
				betterRef = candidate.Equal(refPrice)
			}
		}

		if !found ||
			execQty.GreaterThan(bestQty) ||
			(execQty.Equal(bestQty) && imbalance.LessThan(bestImbalance)) ||
			betterRef {
			bestPrice = candidate
			bestQty = execQty
			bestImbalance = imbalance
			found = true
		}
	}

	return bestPrice, bestQty, found
}

package auction

import "github.com/thorlaidanegg/clob/types"

// ComputeClearingPrice finds the price that maximises matched volume.
// Tiebreakers (in order): maximum executable quantity, minimum imbalance.
// Returns zero-value Decimals with ok=false when there are no crossing orders.
func (a *AuctionBook) ComputeClearingPrice() (price types.Decimal, matchableQty types.Decimal, ok bool) {
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

		// Imbalance: absolute difference between cumBid and cumAsk at this price.
		imbalance := cumBid.Sub(cumAsk).Abs()

		if !found ||
			execQty.GreaterThan(bestQty) ||
			(execQty.Equal(bestQty) && imbalance.LessThan(bestImbalance)) {
			bestPrice = candidate
			bestQty = execQty
			bestImbalance = imbalance
			found = true
		}
	}

	return bestPrice, bestQty, found
}

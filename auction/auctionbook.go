// Package auction implements the call auction (batch auction) mechanism.
//
// Orders accumulate in an [AuctionBook] while the market is in Auction state.
// [AuctionBook.ComputeClearingPrice] finds the equilibrium price that maximises
// executable volume. [AuctionBook.Sweep] executes all eligible orders at that
// price and returns unmatched GTC orders for transfer to the continuous book.
package auction

import (
	"sort"

	"github.com/thorlaidanegg/clob/types"
)

// AuctionOrder is a limit order submitted during the auction phase.
type AuctionOrder struct {
	OrderID types.OrderID
	UserID  types.UserID
	Side    types.Side
	Price   types.Decimal
	Qty     types.Decimal
	TIF     types.TIF
	SeqNum  uint64
}

// AuctionBook accumulates orders during the pre-open/auction phase.
// Orders are held in sorted slices; the book is built once and read once.
type AuctionBook struct {
	bids []AuctionOrder // DESC by price, then ASC by SeqNum
	asks []AuctionOrder // ASC  by price, then ASC by SeqNum
}

// NewAuctionBook returns an empty AuctionBook.
func NewAuctionBook() *AuctionBook {
	return &AuctionBook{}
}

// AddOrder appends an order to the appropriate side and keeps the slice sorted.
func (a *AuctionBook) AddOrder(order AuctionOrder) {
	if order.Side == types.Bid {
		a.bids = append(a.bids, order)
		sort.Slice(a.bids, func(i, j int) bool {
			if a.bids[i].Price.Equal(a.bids[j].Price) {
				return a.bids[i].SeqNum < a.bids[j].SeqNum
			}
			return a.bids[i].Price.GreaterThan(a.bids[j].Price)
		})
	} else {
		a.asks = append(a.asks, order)
		sort.Slice(a.asks, func(i, j int) bool {
			if a.asks[i].Price.Equal(a.asks[j].Price) {
				return a.asks[i].SeqNum < a.asks[j].SeqNum
			}
			return a.asks[i].Price.LessThan(a.asks[j].Price)
		})
	}
}

// BidCount returns the number of bid orders in the auction book.
func (a *AuctionBook) BidCount() int { return len(a.bids) }

// AskCount returns the number of ask orders in the auction book.
func (a *AuctionBook) AskCount() int { return len(a.asks) }

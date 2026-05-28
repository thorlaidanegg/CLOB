package types

import (
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

// OrderID is a unique identifier for an order.
type OrderID string

// UserID is a unique identifier for a user. Opaque to the engine.
type UserID string

// MarketID is the identifier for a market (e.g. "AAPL", "BTC-USD").
type MarketID string

// TradeID is a unique identifier for a trade (shared between maker and taker fills).
type TradeID string

// FillID is a unique identifier for an individual fill event.
type FillID string

// NewOrderID generates a new unique order ID with the "ord_" prefix.
func NewOrderID() OrderID {
	return OrderID("ord_" + newULID())
}

// NewTradeID generates a new unique trade ID with the "trd_" prefix.
func NewTradeID() TradeID {
	return TradeID("trd_" + newULID())
}

// NewFillID generates a new unique fill ID with the "fil_" prefix.
func NewFillID() FillID {
	return FillID("fil_" + newULID())
}

func newULID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

package types

import "fmt"

// OrderType classifies the order's matching behavior.
type OrderType uint8

const (
	Limit     OrderType = 1 // price required, rests if not immediately filled
	Market    OrderType = 2 // no price, takes best available, never rests
	Stop      OrderType = 3 // in stop book, triggers as Market
	StopLimit OrderType = 4 // in stop book, triggers as Limit at LimitPrice
	Iceberg   OrderType = 5 // Limit with visible display qty and hidden reserve
)

// IsLimit returns true for order types that require a price and can rest.
func (t OrderType) IsLimit() bool {
	return t == Limit || t == StopLimit || t == Iceberg
}

// IsMarket returns true for order types that are market-taking.
func (t OrderType) IsMarket() bool {
	return t == Market
}

// NeedsPrice returns true if the order type requires a limit price field.
func (t OrderType) NeedsPrice() bool {
	return t == Limit || t == StopLimit || t == Iceberg
}

// NeedsStopPrice returns true if the order type requires a stop/trigger price.
func (t OrderType) NeedsStopPrice() bool {
	return t == Stop || t == StopLimit
}

// String returns a human-readable name.
func (t OrderType) String() string {
	switch t {
	case Limit:
		return "limit"
	case Market:
		return "market"
	case Stop:
		return "stop"
	case StopLimit:
		return "stop_limit"
	case Iceberg:
		return "iceberg"
	default:
		return fmt.Sprintf("OrderType(%d)", uint8(t))
	}
}

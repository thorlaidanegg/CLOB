package types

import "fmt"

// Side represents the direction of an order.
type Side uint8

const (
	Bid Side = 1
	Ask Side = 2
)

// Opposite returns the opposing side.
func (s Side) Opposite() Side {
	if s == Bid {
		return Ask
	}
	return Bid
}

// IsBid returns true if s == Bid.
func (s Side) IsBid() bool { return s == Bid }

// IsAsk returns true if s == Ask.
func (s Side) IsAsk() bool { return s == Ask }

// String returns the lowercase string representation.
func (s Side) String() string {
	switch s {
	case Bid:
		return "bid"
	case Ask:
		return "ask"
	default:
		return fmt.Sprintf("Side(%d)", uint8(s))
	}
}

// SideFromString parses "bid" or "ask" into a Side.
func SideFromString(s string) (Side, error) {
	switch s {
	case "bid":
		return Bid, nil
	case "ask":
		return Ask, nil
	default:
		return 0, fmt.Errorf("clob/types: unknown side %q", s)
	}
}

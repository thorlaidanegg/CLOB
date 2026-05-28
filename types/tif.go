package types

import "fmt"

// TIF is the Time-In-Force instruction for an order.
type TIF uint8

const (
	GTC TIF = 1 // Good Till Canceled
	IOC TIF = 2 // Immediate Or Cancel
	FOK TIF = 3 // Fill Or Kill
	GTD TIF = 4 // Good Till Date (has expiry timestamp)
	DAY TIF = 5 // expires end of session
)

// CanRest returns true if orders with this TIF are eligible to rest in the book.
func (t TIF) CanRest() bool {
	return t == GTC || t == GTD || t == DAY
}

// String returns the human-readable TIF name.
func (t TIF) String() string {
	switch t {
	case GTC:
		return "GTC"
	case IOC:
		return "IOC"
	case FOK:
		return "FOK"
	case GTD:
		return "GTD"
	case DAY:
		return "DAY"
	default:
		return fmt.Sprintf("TIF(%d)", uint8(t))
	}
}

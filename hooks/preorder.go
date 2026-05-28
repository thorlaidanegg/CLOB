// Package hooks defines the pre-trade and post-fill extension points for the engine.
package hooks

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// OrderContext carries all information about an incoming order before it is
// submitted to the matching engine. Passed to PreOrderHook for validation.
type OrderContext struct {
	MarketID  types.MarketID
	UserID    types.UserID
	OrderID   types.OrderID
	Side      types.Side
	OrderType types.OrderType
	Price     types.Decimal
	Qty       types.Decimal
	TIF       types.TIF
	Flags     types.OrderFlags
	Config    *config.MarketConfig
}

// ValidationResult is the outcome of a PreOrderHook check.
type ValidationResult struct {
	OK      bool
	Reason  types.RejectionReason
	Message string
}

// OK returns a ValidationResult indicating the order is accepted.
func OK() ValidationResult { return ValidationResult{OK: true} }

// Reject returns a ValidationResult indicating the order is rejected.
func Reject(reason types.RejectionReason, message string) ValidationResult {
	return ValidationResult{OK: false, Reason: reason, Message: message}
}

// PreOrderHook is called before any order reaches the matching engine.
// The hook is the correct injection point for credit checks, risk limits,
// fat-finger guards, and any pre-trade validation that requires external state.
//
// If Validate returns OK=false, the engine emits OrderRejected and the order
// never enters the book.
type PreOrderHook interface {
	Validate(ctx OrderContext) ValidationResult
}

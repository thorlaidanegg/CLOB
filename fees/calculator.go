// Package fees implements fee calculation for the CLOB engine.
package fees

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// FeeResult holds the computed fees for a single fill.
type FeeResult struct {
	MakerFee types.Decimal
	TakerFee types.Decimal
	Currency string
}

// FeeCalculator computes maker and taker fees for a fill.
type FeeCalculator interface {
	Calculate(schedule config.FeeSchedule, fill types.Fill) FeeResult
}

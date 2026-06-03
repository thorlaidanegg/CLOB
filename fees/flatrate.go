package fees

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// FlatRateFeeCalculator applies a fixed maker/taker rate to notional value.
// Negative MakerFeeRate means the maker receives a rebate.
type FlatRateFeeCalculator struct{}

// Calculate computes fees as notional Ã— rate.
// notional = price Ã— qty (at price precision).
// Fee rates are at precision 4; result is normalized back to price precision.
func (FlatRateFeeCalculator) Calculate(schedule config.FeeSchedule, fill types.Fill) FeeResult {
	// notional = price × qty at price precision. MulQty handles the differing
	// price/qty precisions (and uses a 128-bit intermediate for overflow safety).
	notionalValue := fill.Price.MulQty(fill.Qty).Value()

	makerFeeValue := notionalValue * schedule.MakerFeeRate.Value() / 10000
	takerFeeValue := notionalValue * schedule.TakerFeeRate.Value() / 10000

	return FeeResult{
		MakerFee: types.NewDecimal(makerFeeValue, fill.Price.Precision()),
		TakerFee: types.NewDecimal(takerFeeValue, fill.Price.Precision()),
		Currency: schedule.FeeCurrency,
	}
}

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
	// Cross-precision notional: price_value * qty_value / 10^qty.precision, at price.precision.
	// Avoids assertSamePrecision panic since price and qty have different precisions.
	notionalValue := fill.Price.Value() * fill.Qty.Value()
	// Normalize: divide by 10^qty.precision to get result at price.precision
	qtyScale := int64(1)
	for i := uint8(0); i < fill.Qty.Precision(); i++ {
		qtyScale *= 10
	}
	notionalValue /= qtyScale

	makerFeeValue := notionalValue * schedule.MakerFeeRate.Value() / 10000
	takerFeeValue := notionalValue * schedule.TakerFeeRate.Value() / 10000

	return FeeResult{
		MakerFee: types.NewDecimal(makerFeeValue, fill.Price.Precision()),
		TakerFee: types.NewDecimal(takerFeeValue, fill.Price.Precision()),
		Currency: schedule.FeeCurrency,
	}
}

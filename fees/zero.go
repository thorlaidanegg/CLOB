package fees

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// ZeroFeeCalculator always returns zero fees. Default when no fee schedule is configured.
type ZeroFeeCalculator struct{}

// Calculate returns zero fees for both maker and taker.
func (ZeroFeeCalculator) Calculate(schedule config.FeeSchedule, fill types.Fill) FeeResult {
	p := fill.Price.Precision()
	return FeeResult{
		MakerFee: types.Zero(p),
		TakerFee: types.Zero(p),
		Currency: schedule.FeeCurrency,
	}
}

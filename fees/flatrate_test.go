package fees

import (
	"testing"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

func TestFlatRate_BasicFee(t *testing.T) {
	calc := FlatRateFeeCalculator{}
	// notional = 100.00 * 10 = 1000.00
	// taker fee = 1000.00 * 0.0030 = 3.00
	// maker fee = 1000.00 * -0.0010 = -1.00 (rebate)
	fill := types.Fill{
		Price: types.MustDecimal("100.00", 2),
		Qty:   types.MustDecimal("10", 0),
	}
	schedule := config.FeeSchedule{
		MakerFeeRate: types.NewDecimal(-10, 4), // -0.0010
		TakerFeeRate: types.NewDecimal(30, 4),  // 0.0030
		FeeCurrency:  "USD",
	}

	result := calc.Calculate(schedule, fill)

	wantMaker := types.MustDecimal("-1.00", 2)
	wantTaker := types.MustDecimal("3.00", 2)

	if !result.MakerFee.Equal(wantMaker) {
		t.Errorf("MakerFee = %s, want %s", result.MakerFee, wantMaker)
	}
	if !result.TakerFee.Equal(wantTaker) {
		t.Errorf("TakerFee = %s, want %s", result.TakerFee, wantTaker)
	}
}

func TestFlatRate_ZeroFee(t *testing.T) {
	calc := FlatRateFeeCalculator{}
	fill := types.Fill{
		Price: types.MustDecimal("50.00", 2),
		Qty:   types.MustDecimal("5", 0),
	}
	schedule := config.FeeSchedule{
		MakerFeeRate: types.Zero(4),
		TakerFeeRate: types.Zero(4),
		FeeCurrency:  "USD",
	}
	result := calc.Calculate(schedule, fill)
	if !result.MakerFee.IsZero() || !result.TakerFee.IsZero() {
		t.Errorf("expected zero fees, got maker=%s taker=%s", result.MakerFee, result.TakerFee)
	}
}

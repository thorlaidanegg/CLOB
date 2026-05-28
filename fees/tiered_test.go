package fees

import (
	"testing"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

type fakeVolume struct{ vol types.Decimal }

func (f fakeVolume) GetVolume(_ types.UserID, _ types.MarketID) types.Decimal { return f.vol }

func TestTiered_BaseTier(t *testing.T) {
	// Volume 0 â†’ base rates
	calc := TieredFeeCalculator{Volume: fakeVolume{vol: types.Zero(0)}}
	fill := types.Fill{
		Price: types.MustDecimal("100.00", 2),
		Qty:   types.MustDecimal("10", 0),
	}
	schedule := config.FeeSchedule{
		MakerFeeRate: types.NewDecimal(-10, 4),
		TakerFeeRate: types.NewDecimal(30, 4),
		FeeCurrency:  "USD",
		FeeModel:     config.FeeModelTiered,
		Tiers: []config.FeeTier{
			{
				MinVolume:    types.NewDecimal(10000, 0),
				MakerFeeRate: types.NewDecimal(-20, 4),
				TakerFeeRate: types.NewDecimal(20, 4),
			},
		},
	}
	result := calc.Calculate(schedule, fill)
	wantMaker := types.MustDecimal("-1.00", 2)
	if !result.MakerFee.Equal(wantMaker) {
		t.Errorf("base tier MakerFee = %s, want %s", result.MakerFee, wantMaker)
	}
}

func TestTiered_HigherTier(t *testing.T) {
	// Volume 50000 â†’ tier 2 rates
	calc := TieredFeeCalculator{Volume: fakeVolume{vol: types.NewDecimal(50000, 0)}}
	fill := types.Fill{
		Price: types.MustDecimal("100.00", 2),
		Qty:   types.MustDecimal("10", 0),
	}
	schedule := config.FeeSchedule{
		MakerFeeRate: types.NewDecimal(-10, 4),
		TakerFeeRate: types.NewDecimal(30, 4),
		FeeCurrency:  "USD",
		FeeModel:     config.FeeModelTiered,
		Tiers: []config.FeeTier{
			{
				MinVolume:    types.NewDecimal(10000, 0),
				MakerFeeRate: types.NewDecimal(-20, 4),
				TakerFeeRate: types.NewDecimal(20, 4),
			},
		},
	}
	result := calc.Calculate(schedule, fill)
	// notional = 1000.00, maker = -0.0020 * 1000 = -2.00
	wantMaker := types.MustDecimal("-2.00", 2)
	if !result.MakerFee.Equal(wantMaker) {
		t.Errorf("tier1 MakerFee = %s, want %s", result.MakerFee, wantMaker)
	}
}

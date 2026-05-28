package fees

import (
	"testing"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

func TestZeroFeeCalculator(t *testing.T) {
	calc := ZeroFeeCalculator{}
	fill := types.Fill{
		Price: types.MustDecimal("100.00", 2),
		Qty:   types.MustDecimal("10", 0),
	}
	schedule := config.FeeSchedule{FeeCurrency: "USD"}
	result := calc.Calculate(schedule, fill)

	if !result.MakerFee.IsZero() {
		t.Errorf("MakerFee = %s, want 0", result.MakerFee)
	}
	if !result.TakerFee.IsZero() {
		t.Errorf("TakerFee = %s, want 0", result.TakerFee)
	}
	if result.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", result.Currency, "USD")
	}
}

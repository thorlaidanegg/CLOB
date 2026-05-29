package fees

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// VolumeProvider returns a user's 30-day traded volume for a market.
// The server implements this by querying Postgres.
type VolumeProvider interface {
	GetVolume(userID types.UserID, marketID types.MarketID) types.Decimal
}

// TieredFeeCalculator applies tier-based rates based on 30-day volume.
type TieredFeeCalculator struct {
	Volume VolumeProvider
}

// Calculate finds each user's tier by volume and applies tier-specific rates.
func (c TieredFeeCalculator) Calculate(schedule config.FeeSchedule, fill types.Fill) FeeResult {
	makerTier := findTier(schedule.Tiers, c.Volume.GetVolume(fill.MakerUserID, ""))
	takerTier := findTier(schedule.Tiers, c.Volume.GetVolume(fill.TakerUserID, ""))

	makerRate := schedule.MakerFeeRate
	takerRate := schedule.TakerFeeRate
	if makerTier != nil {
		makerRate = makerTier.MakerFeeRate
	}
	if takerTier != nil {
		takerRate = takerTier.TakerFeeRate
	}

	notionalValue := fill.Price.Value() * fill.Qty.Value()
	qtyScale := int64(1)
	for i := uint8(0); i < fill.Qty.Precision(); i++ {
		qtyScale *= 10
	}
	notionalValue /= qtyScale

	return FeeResult{
		MakerFee: types.NewDecimal(notionalValue*makerRate.Value()/10000, fill.Price.Precision()),
		TakerFee: types.NewDecimal(notionalValue*takerRate.Value()/10000, fill.Price.Precision()),
		Currency: schedule.FeeCurrency,
	}
}

// findTier returns the highest tier the volume qualifies for, or nil for base rates.
func findTier(tiers []config.FeeTier, volume types.Decimal) *config.FeeTier {
	var match *config.FeeTier
	for i := range tiers {
		if volume.GreaterThanOrEqual(tiers[i].MinVolume) {
			match = &tiers[i]
		}
	}
	return match
}

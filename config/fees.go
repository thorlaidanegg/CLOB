package config

import "github.com/thorlaidanegg/clob/types"

// FeeModel describes how fees are computed for a market.
type FeeModel uint8

const (
	FeeModelFlat   FeeModel = 1
	FeeModelTiered FeeModel = 2
)

// FeeTier defines a volume-based fee tier.
type FeeTier struct {
	MinVolume    types.Decimal // 30-day volume threshold to enter this tier
	MakerFeeRate types.Decimal // precision 4; negative = rebate
	TakerFeeRate types.Decimal // precision 4; >= 0
}

// FeeSchedule defines the fee structure for a market.
type FeeSchedule struct {
	MakerFeeRate types.Decimal // precision 4; negative = rebate
	TakerFeeRate types.Decimal // precision 4; >= 0
	FeeCurrency  string
	FeeModel     FeeModel
	Tiers        []FeeTier // used when FeeModel == FeeModelTiered, sorted ascending by MinVolume
}

package config

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func validConfig() MarketConfig {
	return MarketConfig{
		MarketID:       "TEST",
		PricePrecision: 2,
		QtyPrecision:   0,
		TickSize:       types.NewDecimal(1, 2),
		LotSize:        types.NewDecimal(1, 0),
		Features:       DefaultFeatures(),
		FeeSchedule: FeeSchedule{
			MakerFeeRate: types.NewDecimal(-10, 4),
			TakerFeeRate: types.NewDecimal(30, 4),
			FeeCurrency:  "USD",
			FeeModel:     FeeModelFlat,
		},
		MaxCascadeDepth: 10,
		InitialOrderSeq: 1,
		InitialEventSeq: 1,
	}
}

func TestConfig_ValidPasses(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config failed: %v", err)
	}
}

func TestConfig_MissingMarketID(t *testing.T) {
	cfg := validConfig()
	cfg.MarketID = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing MarketID")
	}
}

func TestConfig_TickSizePrecisionMismatch(t *testing.T) {
	cfg := validConfig()
	cfg.TickSize = types.NewDecimal(1, 3) // wrong precision
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for TickSize precision mismatch")
	}
}

func TestConfig_TickSizeZero(t *testing.T) {
	cfg := validConfig()
	cfg.TickSize = types.Zero(2)
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for zero TickSize")
	}
}

func TestConfig_NegativeTakerFee(t *testing.T) {
	cfg := validConfig()
	cfg.FeeSchedule.TakerFeeRate = types.NewDecimal(-1, 4)
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative TakerFeeRate")
	}
}

func TestConfig_FeePrecisionWrong(t *testing.T) {
	cfg := validConfig()
	cfg.FeeSchedule.MakerFeeRate = types.NewDecimal(10, 2) // wrong precision
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for wrong MakerFeeRate precision")
	}
}

func TestConfig_MinQtyGreaterThanMax(t *testing.T) {
	cfg := validConfig()
	cfg.MinOrderQty = types.NewDecimal(100, 0)
	cfg.MaxOrderQty = types.NewDecimal(10, 0)
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for MinOrderQty > MaxOrderQty")
	}
}

func TestConfig_AuctionFeatureWithoutConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Features = cfg.Features.Add(FeatureAuctions)
	cfg.Auction = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for auction feature without AuctionConfig")
	}
}

func TestConfig_TieredFeesRequiresTiers(t *testing.T) {
	cfg := validConfig()
	cfg.FeeSchedule.FeeModel = FeeModelTiered
	cfg.FeeSchedule.Tiers = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for tiered model without tiers")
	}
}

func TestConfig_TiersUnsorted(t *testing.T) {
	cfg := validConfig()
	cfg.FeeSchedule.FeeModel = FeeModelTiered
	cfg.FeeSchedule.Tiers = []FeeTier{
		{MinVolume: types.NewDecimal(1000, 0)},
		{MinVolume: types.NewDecimal(500, 0)}, // out of order
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unsorted tiers")
	}
}

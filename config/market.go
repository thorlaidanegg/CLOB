// Package config defines [MarketConfig] and all supporting types needed to
// configure a market before passing it to the engine.
//
// Call [MarketConfig.Validate] before creating an engine — it checks tick/lot
// precision consistency, fee rate bounds, sorted tier lists, and more.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/thorlaidanegg/clob/types"
)

// ErrInvalidConfig is the sentinel wrapped by every error returned from Validate.
// Callers can detect any validation failure with errors.Is(err, config.ErrInvalidConfig).
var ErrInvalidConfig = errors.New("clob/config: invalid market config")

// DepthMode controls what happens when an order would exceed MaxDepth.
type DepthMode uint8

const (
	DepthRejectOrder DepthMode = 1 // reject the order outright
	DepthTreatAsIOC  DepthMode = 2 // treat the order as IOC
)

// STPMode is the self-trade prevention mode for a market or individual order.
type STPMode uint8

const (
	STPDisabled        STPMode = 0
	STPCancelBoth      STPMode = 1
	STPCancelMaker     STPMode = 2
	STPCancelTaker     STPMode = 3
	STPDecrementCancel STPMode = 4
)

// RefMode is the reference price mode used by the auction and circuit breaker.
type RefMode uint8

const (
	RefFirstTrade RefMode = 1
	RefOpenPrice  RefMode = 2
	RefPrevClose  RefMode = 3
)

// HaltType classifies the reason for a market halt.
type HaltType uint8

const (
	HaltCircuitBreaker HaltType = 1
	HaltCascadeLimit   HaltType = 2
	HaltAdmin          HaltType = 3
)

func (h HaltType) String() string {
	switch h {
	case HaltCircuitBreaker:
		return "circuit_breaker"
	case HaltCascadeLimit:
		return "cascade_limit"
	case HaltAdmin:
		return "admin"
	default:
		return fmt.Sprintf("HaltType(%d)", uint8(h))
	}
}

func (h HaltType) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h *HaltType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "circuit_breaker":
		*h = HaltCircuitBreaker
	case "cascade_limit":
		*h = HaltCascadeLimit
	case "admin":
		*h = HaltAdmin
	default:
		return fmt.Errorf("config: unknown HaltType %q", s)
	}
	return nil
}

// CircuitBreakerConfig configures the rolling-window price movement guard.
type CircuitBreakerConfig struct {
	WindowDuration time.Duration
	MaxMovePercent types.Decimal // precision 4, e.g. Decimal{1000,4} = 10%
	CooldownPeriod time.Duration
	ReferenceMode  RefMode
}

// AuctionConfig configures the opening auction phase.
type AuctionConfig struct {
	PreOpenDuration time.Duration
	OpenTime        time.Time
}

// MarketConfig is the complete, immutable configuration for a market.
// Call Validate() before using.
type MarketConfig struct {
	// Identity
	MarketID    types.MarketID
	BaseAsset   string
	QuoteAsset  string
	Description string

	// Precision â€” immutable after market creation
	PricePrecision uint8
	QtyPrecision   uint8

	// Tick and lot
	TickSize types.Decimal
	LotSize  types.Decimal

	// Order bounds (Zero = disabled)
	MinOrderQty   types.Decimal
	MaxOrderQty   types.Decimal
	MaxOrderValue types.Decimal

	// Book depth
	MaxDepth     int
	MaxDepthMode DepthMode

	// Features
	Features FeatureSet

	// Self-trade prevention
	STPMode STPMode

	// Fee schedule
	FeeSchedule FeeSchedule

	// Circuit breaker (nil = disabled)
	CircuitBreaker *CircuitBreakerConfig

	// Auction (nil = continuous only)
	Auction *AuctionConfig

	// Cascade limit for stop order chain reactions
	MaxCascadeDepth int

	// Recovery â€” used for WAL replay
	InitialOrderSeq uint64
	InitialEventSeq uint64

	// Admin metadata (ignored by engine)
	CreatedByUserID types.UserID
	CreatedAt       int64
	UpdatedAt       int64
}

// Validate checks all invariants in the config.
func (c *MarketConfig) Validate() error {
	if c.MarketID == "" {
		return fmt.Errorf("%w: MarketID is required", ErrInvalidConfig)
	}
	if c.TickSize.Precision() != c.PricePrecision {
		return fmt.Errorf("%w: TickSize precision %d != PricePrecision %d", ErrInvalidConfig, c.TickSize.Precision(), c.PricePrecision)
	}
	if !c.TickSize.IsPositive() {
		return fmt.Errorf("%w: TickSize must be > 0", ErrInvalidConfig)
	}
	if c.LotSize.Precision() != c.QtyPrecision {
		return fmt.Errorf("%w: LotSize precision %d != QtyPrecision %d", ErrInvalidConfig, c.LotSize.Precision(), c.QtyPrecision)
	}
	if !c.LotSize.IsPositive() {
		return fmt.Errorf("%w: LotSize must be > 0", ErrInvalidConfig)
	}
	if !c.MinOrderQty.IsZero() && !c.MaxOrderQty.IsZero() {
		if c.MinOrderQty.GreaterThan(c.MaxOrderQty) {
			return fmt.Errorf("%w: MinOrderQty > MaxOrderQty", ErrInvalidConfig)
		}
	}
	if c.FeeSchedule.MakerFeeRate.Precision() != 4 {
		return fmt.Errorf("%w: MakerFeeRate precision must be 4, got %d", ErrInvalidConfig, c.FeeSchedule.MakerFeeRate.Precision())
	}
	if c.FeeSchedule.TakerFeeRate.Precision() != 4 {
		return fmt.Errorf("%w: TakerFeeRate precision must be 4, got %d", ErrInvalidConfig, c.FeeSchedule.TakerFeeRate.Precision())
	}
	if c.FeeSchedule.TakerFeeRate.IsNegative() {
		return fmt.Errorf("%w: TakerFeeRate must be >= 0", ErrInvalidConfig)
	}
	if c.FeeSchedule.FeeModel == FeeModelTiered && len(c.FeeSchedule.Tiers) == 0 {
		return fmt.Errorf("%w: tiered fee model requires at least one tier", ErrInvalidConfig)
	}
	// Tiers must be sorted ascending by MinVolume.
	for i := 1; i < len(c.FeeSchedule.Tiers); i++ {
		if !c.FeeSchedule.Tiers[i].MinVolume.GreaterThan(c.FeeSchedule.Tiers[i-1].MinVolume) {
			return fmt.Errorf("%w: fee tiers must be sorted ascending by MinVolume", ErrInvalidConfig)
		}
	}
	if c.Features.Has(FeatureAuctions) && c.Auction == nil {
		return fmt.Errorf("%w: auctions feature enabled but Auction config is nil", ErrInvalidConfig)
	}
	depth := c.MaxCascadeDepth
	if depth == 0 {
		c.MaxCascadeDepth = 10
	}
	if c.InitialOrderSeq == 0 {
		c.InitialOrderSeq = 1
	}
	if c.InitialEventSeq == 0 {
		c.InitialEventSeq = 1
	}
	return nil
}

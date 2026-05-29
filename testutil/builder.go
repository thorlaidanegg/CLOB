// Package testutil provides helpers for engine integration tests.
package testutil

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/engine"
	"github.com/thorlaidanegg/clob/types"
)

// LimitOrder builds a PlaceLimitOrder command with GTC default.
func LimitOrder(marketID types.MarketID, userID types.UserID, side types.Side, price, qty string, pricePrecision, qtyPrecision uint8) engine.PlaceLimitOrder {
	return engine.PlaceLimitOrder{
		MarketID: marketID,
		OrderID:  types.NewOrderID(),
		UserID:   userID,
		Side:     side,
		Price:    types.MustDecimal(price, pricePrecision),
		Qty:      types.MustDecimal(qty, qtyPrecision),
		TIF:      types.GTC,
	}
}

// MarketOrder builds a PlaceMarketOrder command with IOC default.
func MarketOrder(marketID types.MarketID, userID types.UserID, side types.Side, qty string, qtyPrecision uint8) engine.PlaceMarketOrder {
	return engine.PlaceMarketOrder{
		MarketID: marketID,
		OrderID:  types.NewOrderID(),
		UserID:   userID,
		Side:     side,
		Qty:      types.MustDecimal(qty, qtyPrecision),
		TIF:      types.IOC,
	}
}

// StopOrder builds a PlaceStopOrder command that converts to a market order.
func StopOrder(marketID types.MarketID, userID types.UserID, side types.Side, triggerPrice, qty string, pricePrecision, qtyPrecision uint8) engine.PlaceStopOrder {
	return engine.PlaceStopOrder{
		MarketID:     marketID,
		OrderID:      types.NewOrderID(),
		UserID:       userID,
		Side:         side,
		TriggerPrice: types.MustDecimal(triggerPrice, pricePrecision),
		LimitPrice:   types.Zero(pricePrecision),
		Qty:          types.MustDecimal(qty, qtyPrecision),
		ConvertTo:    types.Market,
		TIF:          types.GTC,
	}
}

// DefaultConfig returns a minimal valid market config for testing.
func DefaultConfig(marketID types.MarketID) config.MarketConfig {
	return config.MarketConfig{
		MarketID:       marketID,
		PricePrecision: 2,
		QtyPrecision:   0,
		TickSize:       types.MustDecimal("0.01", 2),
		LotSize:        types.MustDecimal("1", 0),
		Features:       config.DefaultFeatures().Add(config.FeaturePostOnly).Add(config.FeatureFOK).Add(config.FeatureStopOrders),
		FeeSchedule: config.FeeSchedule{
			MakerFeeRate: types.MustDecimal("0.0000", 4),
			TakerFeeRate: types.MustDecimal("0.0000", 4),
		},
	}
}

package engine

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// Command is an instruction submitted to the engine for processing.
type Command interface {
	CommandMarketID() types.MarketID
	CommandOrderID() types.OrderID
	CommandUserID() types.UserID
}

// PlaceLimitOrder requests a new limit order be placed.
type PlaceLimitOrder struct {
	MarketID   types.MarketID
	OrderID    types.OrderID
	UserID     types.UserID
	Side       types.Side
	Price      types.Decimal
	Qty        types.Decimal
	DisplayQty types.Decimal  // iceberg visible portion; zero means not iceberg
	TIF        types.TIF
	Flags      types.OrderFlags
	ExpireAt   int64
	STPMode    config.STPMode
}

func (c PlaceLimitOrder) CommandMarketID() types.MarketID { return c.MarketID }
func (c PlaceLimitOrder) CommandOrderID() types.OrderID   { return c.OrderID }
func (c PlaceLimitOrder) CommandUserID() types.UserID     { return c.UserID }

// PlaceMarketOrder requests a new market order be placed.
type PlaceMarketOrder struct {
	MarketID types.MarketID
	OrderID  types.OrderID
	UserID   types.UserID
	Side     types.Side
	Qty      types.Decimal
	TIF      types.TIF
	Flags    types.OrderFlags
	STPMode  config.STPMode
}

func (c PlaceMarketOrder) CommandMarketID() types.MarketID { return c.MarketID }
func (c PlaceMarketOrder) CommandOrderID() types.OrderID   { return c.OrderID }
func (c PlaceMarketOrder) CommandUserID() types.UserID     { return c.UserID }

// PlaceStopOrder requests a new stop or stop-limit order.
type PlaceStopOrder struct {
	MarketID     types.MarketID
	OrderID      types.OrderID
	UserID       types.UserID
	Side         types.Side
	TriggerPrice types.Decimal
	LimitPrice   types.Decimal // zero for stop-market
	Qty          types.Decimal
	ConvertTo    types.OrderType
	TIF          types.TIF
	Flags        types.OrderFlags
	STPMode      config.STPMode
}

func (c PlaceStopOrder) CommandMarketID() types.MarketID { return c.MarketID }
func (c PlaceStopOrder) CommandOrderID() types.OrderID   { return c.OrderID }
func (c PlaceStopOrder) CommandUserID() types.UserID     { return c.UserID }

// CancelOrder requests cancellation of an existing order.
type CancelOrder struct {
	MarketID types.MarketID
	OrderID  types.OrderID
	UserID   types.UserID
}

func (c CancelOrder) CommandMarketID() types.MarketID { return c.MarketID }
func (c CancelOrder) CommandOrderID() types.OrderID   { return c.OrderID }
func (c CancelOrder) CommandUserID() types.UserID     { return c.UserID }

// AdminCreateMarket is an administrative command to instantiate a new market.
// (Processed by MultiEngine, not by CommandProcessor directly.)
type AdminCreateMarket struct {
	MarketID types.MarketID
	Config   config.MarketConfig
}

func (c AdminCreateMarket) CommandMarketID() types.MarketID { return c.MarketID }
func (c AdminCreateMarket) CommandOrderID() types.OrderID   { return "" }
func (c AdminCreateMarket) CommandUserID() types.UserID     { return "admin" }

// AdminHaltMarket is an administrative halt request.
type AdminHaltMarket struct {
	MarketID types.MarketID
	Reason   string
}

func (c AdminHaltMarket) CommandMarketID() types.MarketID { return c.MarketID }
func (c AdminHaltMarket) CommandOrderID() types.OrderID   { return "" }
func (c AdminHaltMarket) CommandUserID() types.UserID     { return "admin" }

// AdminResumeMarket is an administrative resume request.
type AdminResumeMarket struct {
	MarketID types.MarketID
}

func (c AdminResumeMarket) CommandMarketID() types.MarketID { return c.MarketID }
func (c AdminResumeMarket) CommandOrderID() types.OrderID   { return "" }
func (c AdminResumeMarket) CommandUserID() types.UserID     { return "admin" }

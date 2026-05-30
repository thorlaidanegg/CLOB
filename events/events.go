// Package events defines the output contract of the CLOB engine.
// The Event interface and all concrete types here are the public API between
// the engine and all callers. Schema must be stable after v1.0.0.
package events

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// Event is the interface implemented by all engine output events.
type Event interface {
	EventSeqNum() uint64
	EventTimestamp() int64
	EventMarketID() types.MarketID
	EventType() string
}

// Base is embedded in all concrete event types to carry the common fields.
type Base struct {
	SeqNum    uint64         `json:"seqNum"`
	Timestamp int64          `json:"timestamp"`
	MarketID  types.MarketID `json:"marketID"`
}

func (b Base) EventSeqNum() uint64           { return b.SeqNum }
func (b Base) EventTimestamp() int64         { return b.Timestamp }
func (b Base) EventMarketID() types.MarketID { return b.MarketID }

// OrderAccepted is emitted when an order passes all validation and enters the engine.
type OrderAccepted struct {
	Base
	OrderID     types.OrderID    `json:"orderID"`
	UserID      types.UserID     `json:"userID"`
	Side        types.Side       `json:"side"`
	OrderType   types.OrderType  `json:"orderType"`
	Price       types.Decimal    `json:"price"`
	StopPrice   types.Decimal    `json:"stopPrice"`
	OrigQty     types.Decimal    `json:"origQty"`
	DisplayQty  types.Decimal    `json:"displayQty"`
	TIF         types.TIF        `json:"tif"`
	Flags       types.OrderFlags `json:"flags"`
	OrderSeqNum uint64           `json:"orderSeqNum"`
}

func (e OrderAccepted) EventType() string { return TypeOrderAccepted }

// OrderRested is emitted when an order (or part of one) rests in the book.
type OrderRested struct {
	Base
	OrderID    types.OrderID `json:"orderID"`
	UserID     types.UserID  `json:"userID"`
	Side       types.Side    `json:"side"`
	Price      types.Decimal `json:"price"`
	RemainQty  types.Decimal `json:"remainQty"`
	DisplayQty types.Decimal `json:"displayQty"`
}

func (e OrderRested) EventType() string { return TypeOrderRested }

// TradeFill is emitted once per user per trade. Two TradeFill events are
// emitted per trade: one for the maker, one for the taker.
type TradeFill struct {
	Base
	FillID      types.FillID  `json:"fillID"`
	TradeID     types.TradeID `json:"tradeID"`
	OrderID     types.OrderID `json:"orderID"`
	UserID      types.UserID  `json:"userID"`
	Role        Role          `json:"role"`
	Side        types.Side    `json:"side"`
	Price       types.Decimal `json:"price"`
	FilledQty   types.Decimal `json:"filledQty"`
	RemainQty   types.Decimal `json:"remainQty"`
	Fee         types.Decimal `json:"fee"`
	FeeCurrency string        `json:"feeCurrency"`
}

func (e TradeFill) EventType() string { return TypeTradeFill }

// TradeExecuted is a summary event emitted once per trade, after both TradeFill events.
type TradeExecuted struct {
	Base
	TradeID        types.TradeID `json:"tradeID"`
	MakerOrderID   types.OrderID `json:"makerOrderID"`
	MakerUserID    types.UserID  `json:"makerUserID"`
	MakerSide      types.Side    `json:"makerSide"`
	MakerRemainQty types.Decimal `json:"makerRemainQty"`
	MakerFee       types.Decimal `json:"makerFee"`
	TakerOrderID   types.OrderID `json:"takerOrderID"`
	TakerUserID    types.UserID  `json:"takerUserID"`
	TakerRemainQty types.Decimal `json:"takerRemainQty"`
	TakerFee       types.Decimal `json:"takerFee"`
	Price          types.Decimal `json:"price"`
	Qty            types.Decimal `json:"qty"`
	FeeCurrency    string        `json:"feeCurrency"`
}

func (e TradeExecuted) EventType() string { return TypeTradeExecuted }

// OrderCanceled is emitted when an order is removed from the book.
type OrderCanceled struct {
	Base
	OrderID     types.OrderID      `json:"orderID"`
	UserID      types.UserID       `json:"userID"`
	Side        types.Side         `json:"side"`
	Price       types.Decimal      `json:"price"`
	CanceledQty types.Decimal      `json:"canceledQty"`
	FilledQty   types.Decimal      `json:"filledQty"`
	Reason      types.CancelReason `json:"reason"`
}

func (e OrderCanceled) EventType() string { return TypeOrderCanceled }

// OrderRejected is emitted when an order fails pre-trade validation.
type OrderRejected struct {
	Base
	OrderID types.OrderID         `json:"orderID"`
	UserID  types.UserID          `json:"userID"`
	Reason  types.RejectionReason `json:"reason"`
	Message string                `json:"message"`
}

func (e OrderRejected) EventType() string { return TypeOrderRejected }

// OrderExpired is emitted when a GTD order's expiry time is reached.
type OrderExpired struct {
	Base
	OrderID    types.OrderID `json:"orderID"`
	UserID     types.UserID  `json:"userID"`
	Side       types.Side    `json:"side"`
	Price      types.Decimal `json:"price"`
	ExpiredQty types.Decimal `json:"expiredQty"`
}

func (e OrderExpired) EventType() string { return TypeOrderExpired }

// StopTriggered is emitted when a stop order is triggered and converted.
type StopTriggered struct {
	Base
	StopOrderID    types.OrderID   `json:"stopOrderID"`
	UserID         types.UserID    `json:"userID"`
	TriggerPrice   types.Decimal   `json:"triggerPrice"`
	ConvertedTo    types.OrderType `json:"convertedTo"`
	ConvertedPrice types.Decimal   `json:"convertedPrice"`
}

func (e StopTriggered) EventType() string { return TypeStopTriggered }

// MarketHalted is emitted when the market transitions to Halted state.
type MarketHalted struct {
	Base
	Reason   string          `json:"reason"`
	HaltType config.HaltType `json:"haltType"`
}

func (e MarketHalted) EventType() string { return TypeMarketHalted }

// MarketResumed is emitted when the market resumes from Halted state.
type MarketResumed struct {
	Base
	ResumedBy string `json:"resumedBy"`
}

func (e MarketResumed) EventType() string { return TypeMarketResumed }

// DepthLevel is a single price level in a book snapshot or depth update.
type DepthLevel struct {
	Price      types.Decimal `json:"price"`
	TotalQty   types.Decimal `json:"totalQty"`
	DisplayQty types.Decimal `json:"displayQty"`
	OrderCount int           `json:"orderCount"`
}

// DepthUpdate is emitted for each price level affected by an order event.
type DepthUpdate struct {
	Base
	Side          types.Side      `json:"side"`
	Price         types.Decimal   `json:"price"`
	NewTotalQty   types.Decimal   `json:"newTotalQty"`
	NewDisplayQty types.Decimal   `json:"newDisplayQty"`
	NewOrderCount int             `json:"newOrderCount"`
	UpdateType    DepthUpdateType `json:"updateType"`
}

func (e DepthUpdate) EventType() string { return TypeDepthUpdate }

// BookSnapshot is a full snapshot of the order book at a point in time.
type BookSnapshot struct {
	Base
	Bids []DepthLevel `json:"bids"`
	Asks []DepthLevel `json:"asks"`
}

func (e BookSnapshot) EventType() string { return TypeBookSnapshot }

// AuctionOpened is emitted when the opening auction phase begins.
type AuctionOpened struct {
	Base
	IndicativePrice types.Decimal `json:"indicativePrice"`
	IndicativeQty   types.Decimal `json:"indicativeQty"`
}

func (e AuctionOpened) EventType() string { return TypeAuctionOpened }

// AuctionCleared is emitted when the opening auction executes.
type AuctionCleared struct {
	Base
	ClearingPrice types.Decimal `json:"clearingPrice"`
	MatchedQty    types.Decimal `json:"matchedQty"`
}

func (e AuctionCleared) EventType() string { return TypeAuctionCleared }

// NewBase constructs the shared Base for all events.
func NewBase(seqNum uint64, ts int64, marketID types.MarketID) Base {
	return Base{SeqNum: seqNum, Timestamp: ts, MarketID: marketID}
}

package events

// Event type string constants used as discriminators in JSON and Kafka messages.
const (
	TypeOrderAccepted  = "order_accepted"
	TypeOrderRested    = "order_rested"
	TypeTradeFill      = "trade_fill"
	TypeTradeExecuted  = "trade_executed"
	TypeOrderCanceled  = "order_canceled"
	TypeOrderRejected  = "order_rejected"
	TypeOrderExpired   = "order_expired"
	TypeStopTriggered  = "stop_triggered"
	TypeMarketHalted   = "market_halted"
	TypeMarketResumed  = "market_resumed"
	TypeDepthUpdate    = "depth_update"
	TypeBookSnapshot   = "book_snapshot"
	TypeAuctionOpened  = "auction_opened"
	TypeAuctionCleared = "auction_cleared"
)

// DepthUpdateType indicates whether a depth level was added, modified, or removed.
type DepthUpdateType string

const (
	DepthAdd    DepthUpdateType = "add"
	DepthModify DepthUpdateType = "modify"
	DepthDelete DepthUpdateType = "delete"
)

// Role identifies whether a user was the maker or taker in a trade.
type Role string

const (
	RoleMaker Role = "maker"
	RoleTaker Role = "taker"
)

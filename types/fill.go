package types

// Fill is the internal value type produced by the match loop for each execution.
// It is stack-allocated and never emitted directly to callers. The engine
// processor converts each Fill into events.TradeFill (maker) + events.TradeFill
// (taker) + events.TradeExecuted before writing to the event channel.
type Fill struct {
	MakerOrderID   OrderID
	TakerOrderID   OrderID
	MakerUserID    UserID
	TakerUserID    UserID
	MakerSide      Side
	Price          Decimal
	Qty            Decimal
	MakerRemainQty Decimal
	TakerRemainQty Decimal
	MakerSeqNum    uint64
	TakerSeqNum    uint64
	Timestamp      int64
}

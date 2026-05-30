package types

import "fmt"

// OrderStatus describes the lifecycle state of an order.
type OrderStatus uint8

const (
	StatusNew         OrderStatus = 1
	StatusRested      OrderStatus = 2
	StatusPartialFill OrderStatus = 3
	StatusFilled      OrderStatus = 4
	StatusCanceled    OrderStatus = 5
	StatusRejected    OrderStatus = 6
	StatusExpired     OrderStatus = 7
)

func (s OrderStatus) String() string {
	switch s {
	case StatusNew:
		return "new"
	case StatusRested:
		return "rested"
	case StatusPartialFill:
		return "partial_fill"
	case StatusFilled:
		return "filled"
	case StatusCanceled:
		return "canceled"
	case StatusRejected:
		return "rejected"
	case StatusExpired:
		return "expired"
	default:
		return fmt.Sprintf("OrderStatus(%d)", uint8(s))
	}
}

// CancelReason describes why an order was canceled.
type CancelReason uint8

const (
	CancelUserRequested CancelReason = 1
	CancelIOC           CancelReason = 2
	CancelFOK           CancelReason = 3
	CancelSTP           CancelReason = 4
	CancelExpired       CancelReason = 5
	CancelMarketClosed  CancelReason = 6
	CancelAdminCancel   CancelReason = 7
)

func (r CancelReason) String() string {
	switch r {
	case CancelUserRequested:
		return "user_requested"
	case CancelIOC:
		return "ioc"
	case CancelFOK:
		return "fok"
	case CancelSTP:
		return "stp"
	case CancelExpired:
		return "expired"
	case CancelMarketClosed:
		return "market_closed"
	case CancelAdminCancel:
		return "admin_cancel"
	default:
		return fmt.Sprintf("CancelReason(%d)", uint8(r))
	}
}

// RejectionReason describes why an order was rejected before reaching the book.
type RejectionReason uint8

const (
	RejectInvalidTick        RejectionReason = 1
	RejectInvalidLot         RejectionReason = 2
	RejectBelowMinQty        RejectionReason = 3
	RejectAboveMaxQty        RejectionReason = 4
	RejectAboveMaxValue      RejectionReason = 5
	RejectFeatureDisabled    RejectionReason = 6
	RejectMarketNotOpen      RejectionReason = 7
	RejectPostOnlyWouldCross RejectionReason = 8
	RejectFOKFailed          RejectionReason = 9
	RejectPoolExhausted      RejectionReason = 10
	RejectPreOrderHook       RejectionReason = 11
	RejectInvalidSide        RejectionReason = 12
	RejectInvalidPrice       RejectionReason = 13
	RejectDuplicateOrderID   RejectionReason = 14
	RejectSTPCancelTaker     RejectionReason = 15
	RejectMaxDepth           RejectionReason = 16
	RejectOrderNotFound      RejectionReason = 17
)

func (r RejectionReason) String() string {
	switch r {
	case RejectInvalidTick:
		return "invalid_tick"
	case RejectInvalidLot:
		return "invalid_lot"
	case RejectBelowMinQty:
		return "below_min_qty"
	case RejectAboveMaxQty:
		return "above_max_qty"
	case RejectAboveMaxValue:
		return "above_max_value"
	case RejectFeatureDisabled:
		return "feature_disabled"
	case RejectMarketNotOpen:
		return "market_not_open"
	case RejectPostOnlyWouldCross:
		return "post_only_would_cross"
	case RejectFOKFailed:
		return "fok_failed"
	case RejectPoolExhausted:
		return "pool_exhausted"
	case RejectPreOrderHook:
		return "pre_order_hook"
	case RejectInvalidSide:
		return "invalid_side"
	case RejectInvalidPrice:
		return "invalid_price"
	case RejectDuplicateOrderID:
		return "duplicate_order_id"
	case RejectSTPCancelTaker:
		return "stp_cancel_taker"
	case RejectMaxDepth:
		return "max_depth"
	case RejectOrderNotFound:
		return "order_not_found"
	default:
		return fmt.Sprintf("RejectionReason(%d)", uint8(r))
	}
}

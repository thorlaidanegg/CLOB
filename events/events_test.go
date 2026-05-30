package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func TestOrderRejected_JSONRoundTrip(t *testing.T) {
	ev := OrderRejected{
		Base:    NewBase(1, 1000000, "AAPL"),
		OrderID: types.OrderID("ord_test"),
		UserID:  types.UserID("user1"),
		Reason:  types.RejectInvalidTick,
		Message: "price not on tick",
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Prices must not appear as float64 â€” check raw JSON.
	if strings.Contains(string(b), "e+") || strings.Contains(string(b), "e-") {
		t.Errorf("JSON contains scientific notation (float64 leak): %s", b)
	}

	var got OrderRejected
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.OrderID != ev.OrderID {
		t.Errorf("OrderID: got %s, want %s", got.OrderID, ev.OrderID)
	}
	if got.Reason != ev.Reason {
		t.Errorf("Reason: got %v, want %v", got.Reason, ev.Reason)
	}
}

func TestTradeFill_JSONRoundTrip(t *testing.T) {
	ev := TradeFill{
		Base:      NewBase(2, 2000000, "AAPL"),
		FillID:    types.FillID("fil_1"),
		TradeID:   types.TradeID("trd_1"),
		OrderID:   types.OrderID("ord_1"),
		UserID:    types.UserID("user1"),
		Role:      RoleMaker,
		Side:      types.Bid,
		Price:     types.MustDecimal("101.25", 2),
		FilledQty: types.MustDecimal("10", 0),
		RemainQty: types.MustDecimal("0", 0),
		Fee:       types.MustDecimal("-0.1012", 4),
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Price must be a quoted string.
	if !strings.Contains(string(b), `"price":"101.25"`) {
		t.Errorf("price not serialized as string: %s", b)
	}
}

func TestBookSnapshot_JSONRoundTrip(t *testing.T) {
	ev := BookSnapshot{
		Base: NewBase(3, 3000000, "AAPL"),
		Bids: []DepthLevel{
			{Price: types.MustDecimal("100.00", 2), TotalQty: types.MustDecimal("50", 0), OrderCount: 2},
		},
		Asks: []DepthLevel{
			{Price: types.MustDecimal("101.00", 2), TotalQty: types.MustDecimal("30", 0), OrderCount: 1},
		},
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"100.00"`) {
		t.Errorf("bid price not serialized as string: %s", b)
	}
}

func TestEventType_Methods(t *testing.T) {
	cases := []struct {
		ev   Event
		want string
	}{
		{OrderAccepted{}, TypeOrderAccepted},
		{OrderRested{}, TypeOrderRested},
		{TradeFill{}, TypeTradeFill},
		{TradeExecuted{}, TypeTradeExecuted},
		{OrderCanceled{}, TypeOrderCanceled},
		{OrderRejected{}, TypeOrderRejected},
		{OrderExpired{}, TypeOrderExpired},
		{StopTriggered{}, TypeStopTriggered},
		{MarketHalted{}, TypeMarketHalted},
		{MarketResumed{}, TypeMarketResumed},
		{DepthUpdate{}, TypeDepthUpdate},
		{BookSnapshot{}, TypeBookSnapshot},
		{AuctionOpened{}, TypeAuctionOpened},
		{AuctionCleared{}, TypeAuctionCleared},
	}
	for _, tc := range cases {
		if got := tc.ev.Type(); got != tc.want {
			t.Errorf("%T.Type() = %q, want %q", tc.ev, got, tc.want)
		}
	}
}

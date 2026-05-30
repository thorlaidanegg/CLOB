package testutil

import (
	"testing"

	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

// AssertTrade verifies that a TradeExecuted event with the given price and qty
// is present in evts.
func AssertTrade(t *testing.T, evts []events.Event, price, qty string, pricePrecision, qtyPrecision uint8) {
	t.Helper()
	wantPrice := types.MustDecimal(price, pricePrecision)
	wantQty := types.MustDecimal(qty, qtyPrecision)

	for _, ev := range evts {
		te, ok := ev.(events.TradeExecuted)
		if !ok {
			continue
		}
		if te.Price.Equal(wantPrice) && te.Qty.Equal(wantQty) {
			return
		}
	}
	t.Errorf("AssertTrade: no TradeExecuted with price=%s qty=%s found in %d events", price, qty, len(evts))
}

// AssertRested verifies that an OrderRested event for orderID is in evts.
func AssertRested(t *testing.T, evts []events.Event, orderID types.OrderID) {
	t.Helper()
	for _, ev := range evts {
		if or, ok := ev.(events.OrderRested); ok && or.OrderID == orderID {
			return
		}
	}
	t.Errorf("AssertRested: no OrderRested for %s in %d events", orderID, len(evts))
}

// AssertRejected verifies that an OrderRejected event for orderID is in evts.
func AssertRejected(t *testing.T, evts []events.Event, orderID types.OrderID, reason types.RejectionReason) {
	t.Helper()
	for _, ev := range evts {
		if rej, ok := ev.(events.OrderRejected); ok && rej.OrderID == orderID {
			if rej.Reason == reason {
				return
			}
			t.Errorf("AssertRejected: found OrderRejected for %s but reason=%v, want %v", orderID, rej.Reason, reason)
			return
		}
	}
	t.Errorf("AssertRejected: no OrderRejected for %s in %d events", orderID, len(evts))
}

// AssertCanceled verifies that an OrderCanceled event for orderID is in evts.
func AssertCanceled(t *testing.T, evts []events.Event, orderID types.OrderID) {
	t.Helper()
	for _, ev := range evts {
		if oc, ok := ev.(events.OrderCanceled); ok && oc.OrderID == orderID {
			return
		}
	}
	t.Errorf("AssertCanceled: no OrderCanceled for %s in %d events", orderID, len(evts))
}

// CountEventType returns the number of events of the given type string.
func CountEventType(evts []events.Event, eventType string) int {
	n := 0
	for _, ev := range evts {
		if ev.Type() == eventType {
			n++
		}
	}
	return n
}

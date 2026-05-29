package engine

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func TestCommands_InterfaceSatisfaction(t *testing.T) {
	marketID := types.MarketID("BTC-USD")
	orderID := types.NewOrderID()

	cmds := []Command{
		PlaceLimitOrder{MarketID: marketID, OrderID: orderID, UserID: "user1",
			Side: types.Bid, Price: types.MustDecimal("100.00", 2), Qty: types.MustDecimal("1", 0),
			TIF: types.GTC},
		PlaceMarketOrder{MarketID: marketID, OrderID: orderID, UserID: "user1",
			Side: types.Ask, Qty: types.MustDecimal("1", 0), TIF: types.IOC},
		PlaceStopOrder{MarketID: marketID, OrderID: orderID, UserID: "user1",
			Side: types.Ask, TriggerPrice: types.MustDecimal("90.00", 2), ConvertTo: types.Market,
			Qty: types.MustDecimal("1", 0), TIF: types.GTC},
		CancelOrder{MarketID: marketID, OrderID: orderID, UserID: "user1"},
		AdminHaltMarket{MarketID: marketID, Reason: "test"},
		AdminResumeMarket{MarketID: marketID},
	}

	for _, cmd := range cmds {
		if cmd.CommandMarketID() == "" {
			t.Errorf("%T: CommandMarketID is empty", cmd)
		}
	}
}

func TestCommands_FieldRoundtrip(t *testing.T) {
	id := types.NewOrderID()
	cmd := PlaceLimitOrder{
		MarketID: "ETH-USD",
		OrderID:  id,
		UserID:   "alice",
		Side:     types.Bid,
		Price:    types.MustDecimal("2000.00", 2),
		Qty:      types.MustDecimal("5", 0),
		TIF:      types.GTC,
	}

	if cmd.CommandMarketID() != "ETH-USD" {
		t.Errorf("MarketID = %s", cmd.CommandMarketID())
	}
	if cmd.CommandOrderID() != id {
		t.Errorf("OrderID mismatch")
	}
	if cmd.CommandUserID() != "alice" {
		t.Errorf("UserID = %s", cmd.CommandUserID())
	}
}

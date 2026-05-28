package statemachine

import (
	"testing"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

func testMachine() *Machine {
	cfg := &config.MarketConfig{
		MarketID:       "TEST-USD",
		BaseAsset:      "TEST",
		QuoteAsset:     "USD",
		PricePrecision: 2,
		QtyPrecision:   0,
		TickSize:       types.MustDecimal("0.01", 2),
		LotSize:        types.MustDecimal("1", 0),
	}
	return NewMachine(cfg)
}

func TestMachine_InitialState(t *testing.T) {
	m := testMachine()
	if m.Current() != PreOpen {
		t.Errorf("initial state = %s, want PreOpen", m.Current())
	}
}

func TestMachine_ValidTransitions(t *testing.T) {
	cases := []struct {
		from, to MarketState
	}{
		{PreOpen, Auction},
		{PreOpen, Open},
		{PreOpen, Closed},
		{Auction, Open},
		{Auction, Halted},
		{Auction, Closed},
		{Open, Halted},
		{Open, Closed},
		{Halted, Open},
		{Halted, Auction},
		{Halted, Closed},
	}

	for _, tc := range cases {
		m := testMachine()
		m.state = tc.from
		if err := m.Transition(tc.to); err != nil {
			t.Errorf("%s â†’ %s: unexpected error: %v", tc.from, tc.to, err)
		}
		if m.Current() != tc.to {
			t.Errorf("after %s â†’ %s: state = %s", tc.from, tc.to, m.Current())
		}
	}
}

func TestMachine_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from, to MarketState
	}{
		{PreOpen, Halted},
		{Auction, PreOpen},
		{Open, PreOpen},
		{Open, Auction},
		{Halted, PreOpen},
		{Closed, Open},
		{Closed, Halted},
		{Closed, PreOpen},
		{Closed, Auction},
	}

	for _, tc := range cases {
		m := testMachine()
		m.state = tc.from
		if err := m.Transition(tc.to); err != ErrInvalidTransition {
			t.Errorf("%s â†’ %s: expected ErrInvalidTransition, got %v", tc.from, tc.to, err)
		}
		if m.Current() != tc.from {
			t.Errorf("%s â†’ %s: state changed on invalid transition", tc.from, tc.to)
		}
	}
}

func TestMachine_Permissions_Open(t *testing.T) {
	m := testMachine()
	m.state = Open

	if !m.CanAcceptLimitOrder() {
		t.Error("Open: CanAcceptLimitOrder should be true")
	}
	if !m.CanAcceptMarketOrder() {
		t.Error("Open: CanAcceptMarketOrder should be true")
	}
	if !m.CanAcceptStopOrder() {
		t.Error("Open: CanAcceptStopOrder should be true")
	}
	if !m.CanMatch() {
		t.Error("Open: CanMatch should be true")
	}
	if !m.CanCancel() {
		t.Error("Open: CanCancel should be true")
	}
}

func TestMachine_Permissions_Halted(t *testing.T) {
	m := testMachine()
	m.state = Halted

	if m.CanAcceptMarketOrder() {
		t.Error("Halted: CanAcceptMarketOrder should be false")
	}
	if m.CanAcceptStopOrder() {
		t.Error("Halted: CanAcceptStopOrder should be false")
	}
	if m.CanMatch() {
		t.Error("Halted: CanMatch should be false")
	}
	if !m.CanCancel() {
		t.Error("Halted: CanCancel should be true")
	}
}

func TestMachine_Permissions_Closed(t *testing.T) {
	m := testMachine()
	m.state = Closed

	if m.CanAcceptLimitOrder() {
		t.Error("Closed: CanAcceptLimitOrder should be false")
	}
	if m.CanAcceptMarketOrder() {
		t.Error("Closed: CanAcceptMarketOrder should be false")
	}
	if m.CanMatch() {
		t.Error("Closed: CanMatch should be false")
	}
	if m.CanCancel() {
		t.Error("Closed: CanCancel should be false")
	}
}

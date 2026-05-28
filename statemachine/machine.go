package statemachine

import "github.com/thorlaidanegg/clob/config"

// Machine manages the current state of a market and enforces valid transitions.
type Machine struct {
	state MarketState
	cfg   *config.MarketConfig
}

// NewMachine creates a Machine starting in PreOpen state.
func NewMachine(cfg *config.MarketConfig) *Machine {
	return &Machine{state: PreOpen, cfg: cfg}
}

// Current returns the current market state.
func (m *Machine) Current() MarketState { return m.state }

// Transition moves to dst if the transition is valid, otherwise returns ErrInvalidTransition.
func (m *Machine) Transition(to MarketState) error {
	if !CanTransition(m.state, to) {
		return ErrInvalidTransition
	}
	m.state = to
	return nil
}

// CanAcceptLimitOrder returns true when the market can accept new limit orders.
func (m *Machine) CanAcceptLimitOrder() bool {
	return m.state == Open || m.state == Auction || m.state == PreOpen
}

// CanAcceptMarketOrder returns true when the market can accept market orders.
func (m *Machine) CanAcceptMarketOrder() bool {
	return m.state == Open
}

// CanAcceptStopOrder returns true when the market can accept stop orders.
func (m *Machine) CanAcceptStopOrder() bool {
	return m.state == Open
}

// CanMatch returns true when the matching engine is allowed to execute trades.
func (m *Machine) CanMatch() bool {
	return m.state == Open
}

// CanCancel returns true when order cancellation is permitted.
func (m *Machine) CanCancel() bool {
	return m.state == Open || m.state == Halted || m.state == Auction || m.state == PreOpen
}

package statemachine

import "errors"

// MarketState represents the current operating state of a market.
type MarketState uint8

const (
	PreOpen MarketState = iota + 1
	Auction
	Open
	Halted
	Closed
)

// String returns the human-readable name of the state.
func (s MarketState) String() string {
	switch s {
	case PreOpen:
		return "PreOpen"
	case Auction:
		return "Auction"
	case Open:
		return "Open"
	case Halted:
		return "Halted"
	case Closed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// ErrInvalidTransition is returned when a state transition is not allowed.
var ErrInvalidTransition = errors.New("statemachine: invalid state transition")

// validTransitions maps each state to the set of states it may transition to.
var validTransitions = map[MarketState][]MarketState{
	PreOpen: {Auction, Open, Halted, Closed},
	Auction: {Open, Halted, Closed},
	Open:    {Halted, Closed},
	Halted:  {Open, Auction, Closed},
	Closed:  {}, // terminal
}

// CanTransition returns true if transitioning from src to dst is valid.
func CanTransition(src, dst MarketState) bool {
	for _, allowed := range validTransitions[src] {
		if allowed == dst {
			return true
		}
	}
	return false
}

// Package circuit implements the price-move circuit breaker.
//
// [CircuitBreaker] tracks a rolling window of trade prices via [RollingWindow].
// After each trade, call [CircuitBreaker.Check]: if the price has moved more
// than MaxMovePercent within the configured window, it returns shouldHalt=true
// and the engine halts the market.
package circuit

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/types"
)

// CircuitBreaker monitors trade prices and signals a market halt when price
// movement exceeds the configured threshold within the rolling window.
type CircuitBreaker struct {
	window    *RollingWindow
	cfg       config.CircuitBreakerConfig
	lastHalt  int64
}

// NewCircuitBreaker creates a CircuitBreaker using the provided config.
func NewCircuitBreaker(cfg config.CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		window: NewRollingWindow(cfg.WindowDuration),
		cfg:    cfg,
	}
}

// Check records tradePrice and returns (reason, true) if the market should halt.
// Requires at least two samples to compute a move percentage.
func (cb *CircuitBreaker) Check(tradePrice types.Decimal, now int64) (reason string, shouldHalt bool) {
	cb.window.Add(tradePrice, now)

	if cb.window.Len() < 2 {
		return "", false
	}

	oldest, ok := cb.window.OldestPrice()
	if !ok || oldest.IsZero() {
		return "", false
	}

	diff := tradePrice.Sub(oldest).Abs()
	// outPrecision=4 matches MaxMovePercent's precision â€” avoids assertSamePrecision panic.
	move := diff.Div(oldest, 4)

	if move.GreaterThan(cb.cfg.MaxMovePercent) {
		return "price move exceeded circuit breaker threshold", true
	}
	return "", false
}

// LastHalt returns the timestamp of the most recent halt this breaker triggered.
func (cb *CircuitBreaker) LastHalt() int64 { return cb.lastHalt }

// SetLastHalt records the halt timestamp (called by the processor on halt).
func (cb *CircuitBreaker) SetLastHalt(ts int64) { cb.lastHalt = ts }

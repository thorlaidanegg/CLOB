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
	window   *RollingWindow
	cfg      config.CircuitBreakerConfig
	lastHalt int64
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
// Returns false without computing if within the CooldownPeriod after the last halt.
func (cb *CircuitBreaker) Check(tradePrice types.Decimal, now int64) (reason string, shouldHalt bool) {
	cb.window.Add(tradePrice, now)

	// Suppress re-trigger during cooldown.
	if cb.cfg.CooldownPeriod > 0 && cb.lastHalt > 0 &&
		now-cb.lastHalt < int64(cb.cfg.CooldownPeriod) {
		return “”, false
	}

	if cb.window.Len() < 2 {
		return “”, false
	}

	oldest, ok := cb.window.OldestPrice()
	if !ok || oldest.IsZero() {
		return “”, false
	}

	diff := tradePrice.Sub(oldest).Abs()
	// outPrecision=4 matches MaxMovePercent's precision — avoids assertSamePrecision panic.
	move := diff.Div(oldest, 4)

	if move.GreaterThan(cb.cfg.MaxMovePercent) {
		return “price move exceeded circuit breaker threshold”, true
	}
	return “”, false
}

// LastHalt returns the timestamp of the most recent halt this breaker triggered.
func (cb *CircuitBreaker) LastHalt() int64 { return cb.lastHalt }

// SetLastHalt records the halt timestamp and resets the price window so that
// post-resume price comparisons start from a clean baseline.
func (cb *CircuitBreaker) SetLastHalt(ts int64) {
	cb.lastHalt = ts
	cb.window.Reset()
}

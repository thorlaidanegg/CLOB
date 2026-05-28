package testutil

import (
	"time"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/engine"
	"github.com/thorlaidanegg/clob/events"
)

// EngineHarness wraps an Engine for synchronous test use.
// Submit commands via Do(), then collect all buffered events via Drain().
type EngineHarness struct {
	Engine *engine.Engine
}

// NewHarness creates a started engine with the given config.
func NewHarness(cfg config.MarketConfig, opts ...engine.Option) (*EngineHarness, error) {
	e, err := engine.New(cfg, opts...)
	if err != nil {
		return nil, err
	}
	if err := e.Start(); err != nil {
		return nil, err
	}
	// Transition to Open state.
	_ = e.Submit(engine.AdminResumeMarket{MarketID: cfg.MarketID})
	// Allow the state transition to be processed.
	time.Sleep(20 * time.Millisecond)
	return &EngineHarness{Engine: e}, nil
}

// Do submits a command and pauses briefly for processing.
func (h *EngineHarness) Do(cmd engine.Command) {
	_ = h.Engine.Submit(cmd)
	time.Sleep(5 * time.Millisecond)
}

// Drain collects all events currently available on the event channel.
func (h *EngineHarness) Drain(timeout time.Duration) []events.Event {
	var collected []events.Event
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-h.Engine.Events():
			if !ok {
				return collected
			}
			collected = append(collected, ev)
		case <-deadline:
			return collected
		}
	}
}

// Close shuts down the underlying engine.
func (h *EngineHarness) Close() error {
	return h.Engine.Close()
}

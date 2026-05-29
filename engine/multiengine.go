package engine

import (
	"errors"
	"sync"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

// ErrMarketNotFound is returned when a command targets an unknown market.
var ErrMarketNotFound = errors.New("clob/engine: market not found")

// ErrMarketAlreadyExists is returned when creating a market that already exists.
var ErrMarketAlreadyExists = errors.New("clob/engine: market already exists")

// MultiEngine routes commands to per-market Engine instances.
type MultiEngine struct {
	mu      sync.RWMutex
	engines map[types.MarketID]*Engine
}

// NewMultiEngine creates an empty MultiEngine.
func NewMultiEngine() *MultiEngine {
	return &MultiEngine{
		engines: make(map[types.MarketID]*Engine),
	}
}

// CreateMarket instantiates and starts an Engine for a new market.
func (m *MultiEngine) CreateMarket(cfg config.MarketConfig, opts ...Option) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.engines[cfg.MarketID]; exists {
		return ErrMarketAlreadyExists
	}

	e, err := New(cfg, opts...)
	if err != nil {
		return err
	}
	if err := e.Start(); err != nil {
		return err
	}
	m.engines[cfg.MarketID] = e
	return nil
}

// Submit routes a command to the correct market engine.
func (m *MultiEngine) Submit(cmd Command) error {
	m.mu.RLock()
	e, ok := m.engines[cmd.CommandMarketID()]
	m.mu.RUnlock()

	if !ok {
		return ErrMarketNotFound
	}
	return e.Submit(cmd)
}

// Events returns the event channel for a specific market.
func (m *MultiEngine) Events(marketID types.MarketID) (<-chan events.Event, error) {
	m.mu.RLock()
	e, ok := m.engines[marketID]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrMarketNotFound
	}
	return e.Events(), nil
}

// AllEvents returns a merged channel carrying events from all markets.
// The caller must drain this channel; the goroutine exits when Close is called.
func (m *MultiEngine) AllEvents() <-chan events.Event {
	out := make(chan events.Event, 4096)

	m.mu.RLock()
	sources := make([]<-chan events.Event, 0, len(m.engines))
	for _, e := range m.engines {
		sources = append(sources, e.Events())
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, src := range sources {
		wg.Add(1)
		go func(ch <-chan events.Event) {
			defer wg.Done()
			for ev := range ch {
				out <- ev
			}
		}(src)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// CloseMarket stops a market's engine and removes it.
func (m *MultiEngine) CloseMarket(marketID types.MarketID) error {
	m.mu.Lock()
	e, ok := m.engines[marketID]
	if !ok {
		m.mu.Unlock()
		return ErrMarketNotFound
	}
	delete(m.engines, marketID)
	m.mu.Unlock()

	return e.Close()
}

// Close stops all market engines.
func (m *MultiEngine) Close() error {
	m.mu.Lock()
	ids := make([]types.MarketID, 0, len(m.engines))
	for id := range m.engines {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		if err := m.CloseMarket(id); err != nil {
			return err
		}
	}
	return nil
}

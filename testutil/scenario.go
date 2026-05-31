package testutil

import (
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/engine"
	"github.com/thorlaidanegg/clob/types"
)

// Scenario is a pre-built set of resting orders used to seed a book state
// before the main action under test is submitted.
type Scenario struct {
	cfg    config.MarketConfig
	orders []engine.PlaceLimitOrder
}

// EmptyBook returns a Scenario with no pre-placed orders.
func EmptyBook(cfg config.MarketConfig) *Scenario {
	return &Scenario{cfg: cfg}
}

// WithBid returns a new Scenario with an additional resting GTC bid at price/qty.
func (s *Scenario) WithBid(price, qty string) *Scenario {
	order := LimitOrder(s.cfg.MarketID, "seed", types.Bid, price, qty, s.cfg.PricePrecision, s.cfg.QtyPrecision)
	next := &Scenario{cfg: s.cfg, orders: make([]engine.PlaceLimitOrder, len(s.orders)+1)}
	copy(next.orders, s.orders)
	next.orders[len(s.orders)] = order
	return next
}

// WithAsk returns a new Scenario with an additional resting GTC ask at price/qty.
func (s *Scenario) WithAsk(price, qty string) *Scenario {
	order := LimitOrder(s.cfg.MarketID, "seed", types.Ask, price, qty, s.cfg.PricePrecision, s.cfg.QtyPrecision)
	next := &Scenario{cfg: s.cfg, orders: make([]engine.PlaceLimitOrder, len(s.orders)+1)}
	copy(next.orders, s.orders)
	next.orders[len(s.orders)] = order
	return next
}

// Apply submits all seed orders to the given harness and waits for each to be processed.
func (s *Scenario) Apply(h *EngineHarness) {
	for _, cmd := range s.orders {
		h.Do(cmd)
	}
}

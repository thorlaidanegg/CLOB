// Package engine is the top-level entry point for the CLOB matching engine.
//
// Use [New] to create a single-market engine, or [NewMultiEngine] to manage
// many markets under one roof. All commands are submitted via [Engine.Submit]
// and all results arrive on the channel returned by [Engine.Events].
//
// Example — place a resting ask and a crossing bid:
//
//	eng, _ := engine.New(cfg)
//	eng.Start()
//	defer eng.Close()
//
//	go func() {
//	    for ev := range eng.Events() {
//	        if fill, ok := ev.(events.TradeFill); ok {
//	            fmt.Println("filled", fill.FilledQty, "@", fill.Price)
//	        }
//	    }
//	}()
//
//	eng.Submit(engine.PlaceLimitOrder{Side: types.Ask, Price: p, Qty: q, ...})
//	eng.Submit(engine.PlaceLimitOrder{Side: types.Bid, Price: p, Qty: q, ...})
package engine

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/thorlaidanegg/clob/book"
	"github.com/thorlaidanegg/clob/circuit"
	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/fees"
	"github.com/thorlaidanegg/clob/hooks"
	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/sequence"
	"github.com/thorlaidanegg/clob/statemachine"
	"github.com/thorlaidanegg/clob/stopbook"
	"github.com/thorlaidanegg/clob/types"
)

// Error sentinels for engine operations.
var (
	ErrNotStarted       = errors.New("clob/engine: engine not started")
	ErrAlreadyStarted   = errors.New("clob/engine: engine already started")
	ErrCommandQueueFull = errors.New("clob/engine: command queue full")
)

const (
	defaultCmdBuffer   = 1024
	defaultEventBuffer = 4096
	defaultNodePool    = 100_000
	defaultLevelPool   = 10_000
	defaultStopPool    = 10_000
)

// EngineStats reports resource utilization for monitoring.
type EngineStats struct {
	NodePoolUsed      int
	NodePoolCapacity  int
	LevelPoolUsed     int
	LevelPoolCapacity int
	OpenOrders        int
	BidLevels         int
	AskLevels         int
}

// options holds optional engine configuration.
type options struct {
	cmdBuffer   int
	eventBuffer int
	nodePool    int
	levelPool   int
	preHook     hooks.PreOrderHook
	feeCalc     fees.FeeCalculator
}

// Option is a functional option for Engine.New.
type Option func(*options)

// WithPreOrderHook sets a pre-order validation hook.
func WithPreOrderHook(h hooks.PreOrderHook) Option {
	return func(o *options) { o.preHook = h }
}

// WithCommandBuffer sets the capacity of the command channel.
func WithCommandBuffer(n int) Option {
	return func(o *options) { o.cmdBuffer = n }
}

// WithEventBuffer sets the capacity of the event channel.
func WithEventBuffer(n int) Option {
	return func(o *options) { o.eventBuffer = n }
}

// WithNodePoolSize sets the order node pool capacity.
func WithNodePoolSize(n int) Option {
	return func(o *options) { o.nodePool = n }
}

// WithLevelPoolSize sets the price level pool capacity.
func WithLevelPoolSize(n int) Option {
	return func(o *options) { o.levelPool = n }
}

// WithFeeCalculator sets the fee calculator. Defaults to zero fees.
func WithFeeCalculator(f fees.FeeCalculator) Option {
	return func(o *options) { o.feeCalc = f }
}

// Engine drives a single market's matching loop.
// All commands are processed sequentially by a single goroutine.
type Engine struct {
	processor *CommandProcessor
	cfg       config.MarketConfig
	nodePool  *pool.Pool[book.OrderNode]
	levelPool *pool.Pool[book.PriceLevel]
	cmdChan   chan Command
	eventChan chan events.Event
	started   atomic.Bool
}

// New creates and configures an Engine. Call Start() before Submit().
func New(cfg config.MarketConfig, opts ...Option) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	o := &options{
		cmdBuffer:   defaultCmdBuffer,
		eventBuffer: defaultEventBuffer,
		nodePool:    defaultNodePool,
		levelPool:   defaultLevelPool,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.feeCalc == nil {
		o.feeCalc = &fees.ZeroFeeCalculator{}
	}

	// Shared pools and sequence counter â€” same pointers passed to both
	// CommandProcessor and OrderBook.
	nodePool := pool.New[book.OrderNode](o.nodePool)
	levelPool := pool.New[book.PriceLevel](o.levelPool)
	orderSeq := sequence.NewCounter(cfg.InitialOrderSeq)

	b := book.NewOrderBook(&cfg, nodePool, levelPool, orderSeq)

	var breaker *circuit.CircuitBreaker
	if cfg.CircuitBreaker != nil {
		breaker = circuit.NewCircuitBreaker(*cfg.CircuitBreaker)
	}

	stopNodePool := pool.New[stopbook.StopNode](defaultStopPool)
	stopLevelPool := pool.New[stopbook.StopLevel](defaultStopPool / 10)
	maxCascade := cfg.MaxCascadeDepth
	if maxCascade == 0 {
		maxCascade = 10
	}
	sb := stopbook.NewStopBook(stopNodePool, stopLevelPool, maxCascade)

	sm := statemachine.NewMachine(&cfg)

	cmdChan := make(chan Command, o.cmdBuffer)
	eventChan := make(chan events.Event, o.eventBuffer)

	p := newCommandProcessor(b, sb, breaker, sm, nodePool, orderSeq, o.feeCalc, o.preHook, &cfg, cmdChan, eventChan)

	return &Engine{
		processor: p,
		cfg:       cfg,
		nodePool:  nodePool,
		levelPool: levelPool,
		cmdChan:   cmdChan,
		eventChan: eventChan,
	}, nil
}

// Start launches the matching goroutine. Returns ErrAlreadyStarted if called twice.
func (e *Engine) Start() error {
	if !e.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	go e.processor.run()
	return nil
}

// Submit enqueues a command. Non-blocking; returns ErrCommandQueueFull if the
// channel is at capacity.
func (e *Engine) Submit(cmd Command) error {
	if !e.started.Load() {
		return ErrNotStarted
	}
	select {
	case e.cmdChan <- cmd:
		return nil
	default:
		return ErrCommandQueueFull
	}
}

// Events returns the read-only event channel.
func (e *Engine) Events() <-chan events.Event {
	return e.eventChan
}

// Snapshot returns a depth snapshot of the current order book.
func (e *Engine) Snapshot(levels int) events.BookSnapshot {
	bids, asks := e.processor.book.Snapshot(levels)
	evBids := make([]events.DepthLevel, len(bids))
	for i, l := range bids {
		evBids[i] = events.DepthLevel{
			Price:      l.Price,
			TotalQty:   l.TotalQty,
			DisplayQty: l.DisplayQty,
			OrderCount: l.OrderCount,
		}
	}
	evAsks := make([]events.DepthLevel, len(asks))
	for i, l := range asks {
		evAsks[i] = events.DepthLevel{
			Price:      l.Price,
			TotalQty:   l.TotalQty,
			DisplayQty: l.DisplayQty,
			OrderCount: l.OrderCount,
		}
	}
	return events.BookSnapshot{
		Base: events.NewBase(0, time.Now().UnixNano(), e.cfg.MarketID),
		Bids: evBids,
		Asks: evAsks,
	}
}

// BBO returns the current best bid and ask.
func (e *Engine) BBO() (bid, ask types.Decimal, hasBid, hasAsk bool) {
	return e.processor.book.BBO()
}

// Stats returns current resource utilization.
func (e *Engine) Stats() EngineStats {
	return EngineStats{
		NodePoolUsed:      e.nodePool.Len(),
		NodePoolCapacity:  e.nodePool.Capacity(),
		LevelPoolUsed:     e.levelPool.Len(),
		LevelPoolCapacity: e.levelPool.Capacity(),
		OpenOrders:        e.processor.book.OpenOrderCount(),
		BidLevels:         e.processor.book.BidLevelCount(),
		AskLevels:         e.processor.book.AskLevelCount(),
	}
}

// Close stops the engine and closes the event channel.
func (e *Engine) Close() error {
	if !e.started.Load() {
		return ErrNotStarted
	}
	close(e.processor.quit)
	<-e.processor.done
	close(e.eventChan)
	return nil
}

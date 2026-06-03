package engine

import (
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thorlaidanegg/clob/config"
	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

// These benchmarks measure the matching engine's hot path. The engine is a
// single-writer goroutine per market over a lock-free book with pooled nodes, so
// the steady-state hot path allocates nothing of its own — the only per-order
// allocation here is the test's OrderID string and the command's interface box.
//
// Where a naive engine loses: a mutex-guarded book serializes every operation and
// can't scale across markets; a float64 book mis-prices; a per-order heap
// allocation thrashes the GC. This architecture sidesteps all three.

const (
	benchPricePrec = 2
	benchQtyPrec   = 0
)

// drainCountingAccepts drains the engine's event channel and counts OrderAccepted
// events (exactly one per accepted order), so a benchmark can wait until the
// single-writer processor has actually applied every submitted command. Reading
// from the channel also establishes the happens-before edge the race detector needs.
func drainCountingAccepts(e *Engine, accepted *int64) {
	go func() {
		for ev := range e.Events() {
			if _, ok := ev.(events.OrderAccepted); ok {
				atomic.AddInt64(accepted, 1)
			}
		}
	}()
}

// submitBlocking submits cmd, spinning only while the command queue is full.
func submitBlocking(e *Engine, cmd Command) {
	for {
		err := e.Submit(cmd)
		if err == nil {
			return
		}
		if err == ErrCommandQueueFull {
			runtime.Gosched()
			continue
		}
		panic(err) // ErrNotStarted etc. — a test bug, fail loudly
	}
}

func waitAccepted(accepted *int64, want int64) {
	deadline := time.Now().Add(30 * time.Second)
	for atomic.LoadInt64(accepted) < want {
		if time.Now().After(deadline) {
			return // safety valve: never hang a benchmark on a miscount
		}
		runtime.Gosched()
	}
}

// benchEngine returns a started, Open engine with large buffers and a draining
// collector. The market is resumed to Open so orders match (in PreOpen they rest
// without matching).
func benchEngine(b *testing.B) (*Engine, *int64) {
	b.Helper()
	cfg := testConfig()
	e, err := New(cfg, WithCommandBuffer(1<<16), WithEventBuffer(1<<18))
	if err != nil {
		b.Fatal(err)
	}
	if err := e.Start(); err != nil {
		b.Fatal(err)
	}
	var accepted int64
	drainCountingAccepts(e, &accepted)
	_ = e.Submit(AdminResumeMarket{MarketID: cfg.MarketID})
	b.Cleanup(func() { e.Close() }) //nolint:errcheck
	return e, &accepted
}

func bid(id string, priceRaw int64, qty int64) PlaceLimitOrder {
	return PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.OrderID(id), UserID: "u",
		Side: types.Bid, Price: types.NewDecimal(priceRaw, benchPricePrec),
		Qty: types.NewDecimal(qty, benchQtyPrec), TIF: types.GTC,
	}
}

func ask(id string, priceRaw int64, qty int64) PlaceLimitOrder {
	return PlaceLimitOrder{
		MarketID: "BTC-USD", OrderID: types.OrderID(id), UserID: "u2",
		Side: types.Ask, Price: types.NewDecimal(priceRaw, benchPricePrec),
		Qty: types.NewDecimal(qty, benchQtyPrec), TIF: types.GTC,
	}
}

// BenchmarkEngine_PlaceCancel measures the order-churn path: place a resting limit
// then cancel it. This is the dominant workload on real venues (most orders are
// canceled, not filled) and it exercises the book's insert + O(1) cancel with the
// node pool recycling — the book stays shallow, so it measures the operation, not
// unbounded growth.
func BenchmarkEngine_PlaceCancel(b *testing.B) {
	e, accepted := benchEngine(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := "o" + strconv.Itoa(i)
		submitBlocking(e, bid(id, 10000, 1)) // rests
		submitBlocking(e, CancelOrder{MarketID: "BTC-USD", OrderID: types.OrderID(id), UserID: "u"})
	}
	waitAccepted(accepted, int64(b.N)) // one OrderAccepted per place
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "place+cancel/sec")
}

// BenchmarkEngine_MatchThroughput measures fully-matched trades: a resting bid
// immediately taken by a crossing ask. Each iteration produces exactly one trade.
func BenchmarkEngine_MatchThroughput(b *testing.B) {
	e, accepted := benchEngine(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		submitBlocking(e, bid("b"+strconv.Itoa(i), 10000, 1))
		submitBlocking(e, ask("a"+strconv.Itoa(i), 10000, 1))
	}
	waitAccepted(accepted, int64(2*b.N))
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "trades/sec")
}

// BenchmarkEngine_ParallelMarkets shows the architecture's headline scaling
// property: each market is an independent single-writer goroutine with NO shared
// lock, so throughput scales with cores as markets are added. A single mutexed
// book cannot do this — it serializes every market behind one lock.
func BenchmarkEngine_ParallelMarkets(b *testing.B) {
	const markets = 8
	multi := NewMultiEngine()
	for m := 0; m < markets; m++ {
		cfg := testConfig()
		cfg.MarketID = types.MarketID("MKT-" + strconv.Itoa(m))
		if err := multi.CreateMarket(cfg, WithCommandBuffer(1<<16), WithEventBuffer(1<<18)); err != nil {
			b.Fatal(err)
		}
		_ = multi.Submit(AdminResumeMarket{MarketID: cfg.MarketID})
	}
	b.Cleanup(func() { multi.Close() }) //nolint:errcheck

	var accepted int64
	go func() {
		for ev := range multi.AllEvents() {
			if _, ok := ev.(events.OrderAccepted); ok {
				atomic.AddInt64(&accepted, 1)
			}
		}
	}()

	var submitted int64
	var market int64
	b.ReportAllocs()
	b.ResetTimer()
	send := func(mid types.MarketID, cmd Command) {
		for {
			if err := multi.Submit(cmd); err == nil {
				return
			}
			runtime.Gosched()
		}
	}
	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets a unique id; markets cycle so several goroutines may
		// share a market (that's the point — independent single-writer per market).
		gid := int(atomic.AddInt64(&market, 1) - 1)
		mid := types.MarketID("MKT-" + strconv.Itoa(gid%markets))
		var n int
		for pb.Next() {
			n++
			pfx := strconv.Itoa(gid) + "-" + strconv.Itoa(n) // unique across goroutines
			// A resting bid immediately taken by a crossing ask → one trade, book
			// stays shallow (bounded pools).
			send(mid, PlaceLimitOrder{MarketID: mid, OrderID: types.OrderID("b" + pfx), UserID: "u",
				Side: types.Bid, Price: types.NewDecimal(10000, benchPricePrec),
				Qty: types.NewDecimal(1, benchQtyPrec), TIF: types.GTC})
			send(mid, PlaceLimitOrder{MarketID: mid, OrderID: types.OrderID("a" + pfx), UserID: "u2",
				Side: types.Ask, Price: types.NewDecimal(10000, benchPricePrec),
				Qty: types.NewDecimal(1, benchQtyPrec), TIF: types.GTC})
			atomic.AddInt64(&submitted, 2)
		}
	})
	waitAccepted(&accepted, atomic.LoadInt64(&submitted))
	b.StopTimer()
	b.ReportMetric(float64(atomic.LoadInt64(&submitted)/2)/b.Elapsed().Seconds(), "trades/sec")
}

// Ensure the bench config exercises real matching features.
var _ = config.FeatureMarketOrders

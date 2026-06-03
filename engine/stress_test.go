package engine

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thorlaidanegg/clob/events"
	"github.com/thorlaidanegg/clob/types"
)

// This file is the stress + invariants suite. It is meant to be run with the race
// detector:
//
//	go test ./engine/ -run Stress -race
//	go test ./engine/ -run Determinism
//	go test ./engine/ -run Conservation
//
// The scenarios are chosen to break a naive implementation:
//   - thousands of orders submitted concurrently across markets (a mutexed book
//     races or deadlocks; this one is a lock-free single-writer per market),
//   - exact money conservation over many trades (a float64 book drifts and leaks),
//   - deterministic output for a fixed input (the property that makes event-log
//     replay / crash recovery sound — a non-deterministic engine can't be replayed).

// collectAll drains every event from the engine into a slice. After the engine is
// Closed its channel closes, the goroutine returns, and wait() establishes a
// happens-before edge so the slice can be read race-free.
func collectAll(e *Engine) (evs *[]events.Event, count *int64, wait func()) {
	s := make([]events.Event, 0, 4096)
	var n int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range e.Events() {
			s = append(s, ev)
			atomic.AddInt64(&n, 1)
		}
	}()
	return &s, &n, func() { <-done }
}

func collectAllMulti(m *MultiEngine) (evs *[]events.Event, count *int64, wait func()) {
	s := make([]events.Event, 0, 4096)
	var n int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range m.AllEvents() {
			s = append(s, ev)
			atomic.AddInt64(&n, 1)
		}
	}()
	return &s, &n, func() { <-done }
}

// waitQuiescent blocks until the event count stops changing — i.e. the
// single-writer processors have drained every queued command.
func waitQuiescent(count *int64) {
	prev := int64(-1)
	stable := 0
	for stable < 3 {
		time.Sleep(25 * time.Millisecond)
		cur := atomic.LoadInt64(count)
		if cur == prev {
			stable++
		} else {
			stable, prev = 0, cur
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrency stress — run with -race.
// ---------------------------------------------------------------------------

// TestEngine_StressConcurrentChaos hammers a MultiEngine from many goroutines with
// a mix of limit / market / IOC / FOK orders and cancels, across several markets,
// then verifies the event stream is internally consistent. With -race this also
// proves the engine is data-race free under heavy concurrent submission — the exact
// scenario where a hand-rolled lock-based book corrupts state or deadlocks.
func TestEngine_StressConcurrentChaos(t *testing.T) {
	const (
		markets    = 4
		goroutines = 8
		perG       = 1500
	)

	multi := NewMultiEngine()
	marketIDs := make([]types.MarketID, markets)
	for i := 0; i < markets; i++ {
		cfg := testConfig()
		cfg.MarketID = types.MarketID(fmt.Sprintf("MKT-%d", i))
		if err := multi.CreateMarket(cfg, WithCommandBuffer(1<<15), WithEventBuffer(1<<17)); err != nil {
			t.Fatal(err)
		}
		_ = multi.Submit(AdminResumeMarket{MarketID: cfg.MarketID})
		marketIDs[i] = cfg.MarketID
	}

	evs, count, wait := collectAllMulti(multi)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(g) + 1))
			var mine []types.OrderID
			for i := 0; i < perG; i++ {
				mid := marketIDs[rng.Intn(markets)]
				id := types.OrderID(fmt.Sprintf("g%d-%d", g, i))
				side := types.Bid
				if rng.Intn(2) == 0 {
					side = types.Ask
				}
				// Prices overlap around 100.00 so a healthy fraction crosses.
				priceRaw := int64(9950 + rng.Intn(101)) // 99.50 .. 100.50
				qty := int64(1 + rng.Intn(5))

				var cmd Command
				switch rng.Intn(10) {
				case 0: // market IOC (sweeps the book or cancels residual)
					cmd = PlaceMarketOrder{MarketID: mid, OrderID: id, UserID: "u",
						Side: side, Qty: types.NewDecimal(qty, benchQtyPrec), TIF: types.IOC}
				case 1: // FOK limit
					cmd = PlaceLimitOrder{MarketID: mid, OrderID: id, UserID: "u",
						Side: side, Price: types.NewDecimal(priceRaw, benchPricePrec),
						Qty: types.NewDecimal(qty, benchQtyPrec), TIF: types.FOK}
				case 2: // cancel one of my own earlier orders
					if len(mine) > 0 {
						victim := mine[rng.Intn(len(mine))]
						submitBlocking2(multi, CancelOrder{MarketID: mid, OrderID: victim, UserID: "u"})
						continue
					}
					fallthrough
				default: // resting limit (GTC)
					cmd = PlaceLimitOrder{MarketID: mid, OrderID: id, UserID: "u",
						Side: side, Price: types.NewDecimal(priceRaw, benchPricePrec),
						Qty: types.NewDecimal(qty, benchQtyPrec), TIF: types.GTC}
					mine = append(mine, id)
				}
				submitBlocking2(multi, cmd)
			}
		}(g)
	}
	wg.Wait()

	waitQuiescent(count)
	multi.Close() //nolint:errcheck
	wait()

	assertEventStreamConsistent(t, *evs)
}

// submitBlocking2 is submitBlocking for a MultiEngine.
func submitBlocking2(m *MultiEngine, cmd Command) {
	for {
		err := m.Submit(cmd)
		if err == nil || err == ErrMarketNotFound {
			return
		}
		if err == ErrCommandQueueFull {
			time.Sleep(time.Millisecond)
			continue
		}
		return // any other error (e.g. not started) — ignore in chaos test
	}
}

// assertEventStreamConsistent checks the invariants that any correct matching
// engine must satisfy, derived purely from the (synchronized) event stream:
//   1. Per market, sequence numbers are strictly increasing — proves the
//      single-writer assigns a total order even under concurrent submission.
//   2. Every trade has exactly two fills of equal quantity (a maker and a taker,
//      opposite sides) — no quantity is created or destroyed in a match.
//   3. Each TradeExecuted's qty equals its fills' quantity.
//   4. Globally, total quantity bought == total quantity sold.
func assertEventStreamConsistent(t *testing.T, evs []events.Event) {
	t.Helper()

	lastSeq := map[types.MarketID]uint64{}
	fillsByTrade := map[types.TradeID][]events.TradeFill{}
	execQty := map[types.TradeID]types.Decimal{}
	var boughtRaw, soldRaw int64
	var trades int

	for _, ev := range evs {
		// (1) strictly increasing seq per market.
		mid := ev.MarketID()
		if s := ev.SeqNum(); s <= lastSeq[mid] {
			t.Fatalf("non-monotonic seq on %s: %d after %d", mid, s, lastSeq[mid])
		} else {
			lastSeq[mid] = s
		}

		switch e := ev.(type) {
		case events.TradeFill:
			fillsByTrade[e.TradeID] = append(fillsByTrade[e.TradeID], e)
			if !e.FilledQty.IsPositive() {
				t.Fatalf("non-positive fill qty: %s", e.FilledQty)
			}
			if e.Side == types.Bid {
				boughtRaw += e.FilledQty.Value()
			} else {
				soldRaw += e.FilledQty.Value()
			}
		case events.TradeExecuted:
			execQty[e.TradeID] = e.Qty
			trades++
		}
	}

	// (2) + (3) per-trade pairing and consistency.
	for tid, fills := range fillsByTrade {
		if len(fills) != 2 {
			t.Fatalf("trade %s has %d fills, want exactly 2", tid, len(fills))
		}
		if !fills[0].FilledQty.Equal(fills[1].FilledQty) {
			t.Fatalf("trade %s fills differ: %s vs %s", tid, fills[0].FilledQty, fills[1].FilledQty)
		}
		if fills[0].Side == fills[1].Side {
			t.Fatalf("trade %s both fills on side %v", tid, fills[0].Side)
		}
		if q, ok := execQty[tid]; ok && !q.Equal(fills[0].FilledQty) {
			t.Fatalf("trade %s executed qty %s != fill qty %s", tid, q, fills[0].FilledQty)
		}
	}

	// (4) global conservation: every unit bought was sold.
	if boughtRaw != soldRaw {
		t.Fatalf("quantity not conserved: bought %d != sold %d", boughtRaw, soldRaw)
	}
	if trades == 0 {
		t.Fatal("expected the chaos workload to produce trades, got none")
	}
	t.Logf("chaos: %d events, %d trades, %d units bought==sold", len(evs), trades, boughtRaw)
}

// ---------------------------------------------------------------------------
// Determinism — the property that makes event-log replay / recovery sound.
// ---------------------------------------------------------------------------

// TestEngine_DeterminismSameInputSameOutput submits an identical command sequence
// to two fresh engines and asserts the resulting event streams are identical on
// every deterministic field. Matching is a pure function of the (ordered) command
// stream — which is exactly why the book can be rebuilt by replaying the log.
func TestEngine_DeterminismSameInputSameOutput(t *testing.T) {
	run := func() []string {
		cfg := testConfig()
		e, err := New(cfg, WithCommandBuffer(1<<14), WithEventBuffer(1<<16))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Start(); err != nil {
			t.Fatal(err)
		}
		evs, count, wait := collectAll(e)
		_ = e.Submit(AdminResumeMarket{MarketID: cfg.MarketID})

		rng := rand.New(rand.NewSource(42)) // fixed seed → fixed command sequence
		for i := 0; i < 4000; i++ {
			side := types.Bid
			if rng.Intn(2) == 0 {
				side = types.Ask
			}
			submitBlocking(e, PlaceLimitOrder{
				MarketID: cfg.MarketID, OrderID: types.OrderID("o" + itoa(i)), UserID: "u",
				Side: side, Price: types.NewDecimal(int64(9950+rng.Intn(101)), benchPricePrec),
				Qty: types.NewDecimal(int64(1+rng.Intn(5)), benchQtyPrec), TIF: types.GTC,
			})
		}
		waitQuiescent(count)
		e.Close() //nolint:errcheck
		wait()

		// Fingerprint the deterministic fields (exclude random TradeID/FillID/timestamp).
		out := make([]string, 0, len(*evs))
		for _, ev := range *evs {
			switch e := ev.(type) {
			case events.OrderAccepted:
				out = append(out, fmt.Sprintf("A %d %s %v %s", e.SeqNum(), e.OrderID, e.Side, e.Price))
			case events.OrderRested:
				out = append(out, fmt.Sprintf("R %d %s %s", e.SeqNum(), e.OrderID, e.RemainQty))
			case events.TradeFill:
				out = append(out, fmt.Sprintf("F %d %s %v %s %s", e.SeqNum(), e.OrderID, e.Side, e.Price, e.FilledQty))
			case events.TradeExecuted:
				out = append(out, fmt.Sprintf("X %d %s %s", e.SeqNum(), e.Price, e.Qty))
			case events.OrderCanceled:
				out = append(out, fmt.Sprintf("C %d %s %s", e.SeqNum(), e.OrderID, e.CanceledQty))
			}
		}
		return out
	}

	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("event count differs across identical runs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic at event %d:\n  run1: %s\n  run2: %s", i, a[i], b[i])
		}
	}
	t.Logf("determinism: %d events identical across two independent runs", len(a))
}

// ---------------------------------------------------------------------------
// Exact money — no float drift over many fills.
// ---------------------------------------------------------------------------

// TestEngine_ConservationExactMoney executes many trades at a fractional price and
// asserts the total traded notional is exact to the cent. A float64 engine would
// accumulate rounding error over this many operations; fixed-point does not.
func TestEngine_ConservationExactMoney(t *testing.T) {
	cfg := testConfig()
	e, err := New(cfg, WithCommandBuffer(1<<16), WithEventBuffer(1<<18))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	evs, count, wait := collectAll(e)
	_ = e.Submit(AdminResumeMarket{MarketID: cfg.MarketID})

	const trades = 100_000
	const priceRaw = 10007 // 100.07
	for i := 0; i < trades; i++ {
		submitBlocking(e, bid("b"+itoa(i), priceRaw, 1))
		submitBlocking(e, ask("a"+itoa(i), priceRaw, 1))
	}
	waitQuiescent(count)
	e.Close() //nolint:errcheck
	wait()

	var qty, notional int64
	var executed int
	for _, ev := range *evs {
		if x, ok := ev.(events.TradeExecuted); ok {
			executed++
			qty += x.Qty.Value()
			notional += x.Price.MulQty(x.Qty).Value() // exact price × qty at price precision
		}
	}
	if executed != trades {
		t.Fatalf("expected %d trades, got %d", trades, executed)
	}
	if qty != trades { // each trade is 1 unit
		t.Fatalf("total qty = %d, want %d", qty, trades)
	}
	// trades × 1 unit × 100.07 = trades × 10007 (raw, at precision 2). Exact.
	if want := int64(trades) * priceRaw; notional != want {
		t.Fatalf("notional drifted: got %d, want %d (a float64 engine would not hit this exactly)", notional, want)
	}
	t.Logf("exact money: %d trades, total notional %d (= %d.%02d credits), zero drift",
		trades, notional, notional/100, notional%100)
}

// TestEngine_LevelPoolExhaustionRejectsNotPanics is the regression test for a bug
// the benchmarks surfaced: exhausting the price-level pool used to PANIC (crashing
// the market goroutine), unlike node-pool exhaustion which rejects gracefully. Now
// both reject. This floods more distinct price levels than the pool holds and
// asserts the surplus is rejected with RejectPoolExhausted and the engine stays
// alive (a later order at an existing level still rests).
func TestEngine_LevelPoolExhaustionRejectsNotPanics(t *testing.T) {
	cfg := testConfig()
	e, err := New(cfg, WithLevelPoolSize(3), WithCommandBuffer(256), WithEventBuffer(4096))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	evs, count, wait := collectAll(e)
	_ = e.Submit(AdminResumeMarket{MarketID: cfg.MarketID})

	// 8 non-crossing bids at distinct prices; only 3 levels fit. The old code
	// panicked on the 4th; the fixed code rejects 5 of them.
	for i := 0; i < 8; i++ {
		submitBlocking(e, bid("o"+itoa(i), 10000+int64(i), 1))
	}
	// Engine must still be alive: an order at the already-created 100.00 level rests.
	submitBlocking(e, bid("reuse", 10000, 1))

	waitQuiescent(count)
	e.Close() //nolint:errcheck
	wait()

	var rested, poolRejects int
	for _, ev := range *evs {
		switch x := ev.(type) {
		case events.OrderRested:
			rested++
		case events.OrderRejected:
			if x.Reason == types.RejectPoolExhausted {
				poolRejects++
			}
		}
	}
	if poolRejects < 5 {
		t.Fatalf("expected >=5 level-pool-exhausted rejects, got %d", poolRejects)
	}
	if rested < 4 { // 3 distinct + the reuse at an existing level
		t.Fatalf("expected the engine to stay alive and rest >=4 orders, got %d", rested)
	}
	t.Logf("level-pool guard: %d rested, %d gracefully rejected, no panic", rested, poolRejects)
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

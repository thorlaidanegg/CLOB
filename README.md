# clob

A pure Go central limit order book (CLOB) matching engine library.

`github.com/thorlaidanegg/clob`

---

## What It Is

`clob` is a self-contained, zero-infrastructure matching engine. You give it market configurations and a stream of commands; it gives you back a stream of events. No databases, no queues, no network — just a deterministic, in-memory order book.

It is designed to be **embedded**. Drop it into a trading server, a paper-trading platform, a backtester, or a research sandbox. The engine runs a single goroutine per market; all concurrency is handled for you.

```
Your code → Submit(Command) → [engine goroutine] → Events() → Your code
```

**Two external dependencies only:**

- [`github.com/tidwall/btree`](https://github.com/tidwall/btree) — B-tree for price levels
- [`github.com/oklog/ulid/v2`](https://github.com/oklog/ulid) — sortable IDs for orders and trades

---

## Package Overview

| Package | Role |
|---|---|
| `types` | Fixed-point `Decimal`, order/trade ID types, enums (`Side`, `TIF`, `OrderType`, etc.) |
| `config` | `MarketConfig` — the full description of a market |
| `events` | Event interface and all 14 concrete event types emitted by the engine |
| `engine` | `Engine` (single market) and `MultiEngine` (many markets) — the public entry point |
| `fees` | `FeeCalculator` interface, `ZeroFeeCalculator`, `FlatRateFeeCalculator`, `TieredFeeCalculator` |
| `hooks` | `PreOrderHook` (credit check / risk gate before any order enters the book) |
| `book` | Order book internals — price levels, B-tree, match loop (exported for testing) |
| `stopbook` | Stop-order book — trigger price tracking, cascade protection |
| `auction` | Call auction — equilibrium price computation and sweep |
| `statemachine` | Market lifecycle — PreOpen → Open → Halted → Closed |
| `circuit` | Circuit breaker — rolling price-move window, halt trigger |
| `pool` | Generic object pool (zero-alloc order/level reuse) |
| `sequence` | Monotonic counter for SeqNum assignment |
| `testutil` | Test builders, assertion helpers, and `EngineHarness` for integration tests |

---

## Installation

```bash
go get github.com/thorlaidanegg/clob
```

---

## Quick Start — Single Market

```go
import (
    "fmt"
    "github.com/thorlaidanegg/clob/config"
    "github.com/thorlaidanegg/clob/engine"
    "github.com/thorlaidanegg/clob/events"
    "github.com/thorlaidanegg/clob/types"
)

func main() {
    // 1. Define a market
    cfg := config.MarketConfig{
        MarketID:       "BTC-USD",
        BaseAsset:      "BTC",
        QuoteAsset:     "USD",
        PricePrecision: 2,
        QtyPrecision:   8,
        TickSize:       types.MustDecimal("0.01", 2),
        LotSize:        types.MustDecimal("0.00000001", 8),
        Features:       config.DefaultFeatures(),
    }
    if err := cfg.Validate(); err != nil {
        panic(err)
    }

    // 2. Create and start the engine
    eng, err := engine.New(cfg)
    if err != nil {
        panic(err)
    }
    eng.Start()
    defer eng.Close()

    // 3. Consume events in the background
    go func() {
        for ev := range eng.Events() {
            switch e := ev.(type) {
            case events.TradeFill:
                fmt.Printf("fill: %s %s @ %s (seq %d)\n",
                    e.FilledQty, e.Side, e.Price, e.EventSeqNum())
            case events.OrderRested:
                fmt.Printf("rested: order %s\n", e.OrderID)
            case events.OrderRejected:
                fmt.Printf("rejected: %s\n", e.Reason)
            }
        }
    }()

    // 4. Place a resting ask
    eng.Submit(engine.PlaceLimitOrder{
        MarketID: "BTC-USD",
        OrderID:  types.NewOrderID(),
        UserID:   "alice",
        Side:     types.Ask,
        Price:    types.MustDecimal("50000.00", 2),
        Qty:      types.MustDecimal("0.10000000", 8),
        TIF:      types.GTC,
    })

    // 5. Place a crossing bid — triggers a fill
    eng.Submit(engine.PlaceLimitOrder{
        MarketID: "BTC-USD",
        OrderID:  types.NewOrderID(),
        UserID:   "bob",
        Side:     types.Bid,
        Price:    types.MustDecimal("50000.00", 2),
        Qty:      types.MustDecimal("0.10000000", 8),
        TIF:      types.GTC,
    })
}
```

---

## Core Concepts

### The Decimal Type

All financial values — prices, quantities, fees — are `types.Decimal`: a fixed-point integer with an explicit precision. There is **no float64** anywhere in the matching path.

```go
// Construct
d := types.NewDecimal(10025, 2)       // 100.25 at precision 2
d := types.MustDecimal("100.25", 2)   // panics on bad input — use in tests/init
d, err := types.ParseDecimal("100.25", 2)

// Arithmetic — precision must match or panics
a := types.MustDecimal("10.00", 2)
b := types.MustDecimal("3.50", 2)
sum  := a.Add(b)           // 13.50
diff := a.Sub(b)           // 6.50
neg  := a.Neg()            // -10.00

// Division with explicit output precision (avoids precision mismatch)
move := diff.Div(a, 4)     // 0.6500 at precision 4

// Comparison
a.GreaterThan(b)           // true
a.Equal(b)                 // false
a.IsZero()                 // false

// Serialization
a.String()                 // "10.00"
json.Marshal(a)            // "\"10.00\""
```

**Precision is part of the type.** Arithmetic on two values of different precision panics. Price and quantity have different precisions — never `.Mul()` them directly; use raw `.Value()` integer math for cross-precision calculations (e.g., computing notional value in fee calculators).

### MarketConfig

`MarketConfig` is the complete, immutable description of a market. Call `Validate()` before passing to `engine.New`.

```go
cfg := config.MarketConfig{
    MarketID:       "AAPL",
    BaseAsset:      "AAPL",
    QuoteAsset:     "USD",
    PricePrecision: 2,
    QtyPrecision:   0,
    TickSize:       types.MustDecimal("0.01", 2),  // min price increment
    LotSize:        types.MustDecimal("1", 0),      // min qty increment
    MinOrderQty:    types.MustDecimal("1", 0),
    MaxOrderQty:    types.MustDecimal("10000", 0),
    MaxOrderValue:  types.MustDecimal("1000000.00", 2),
    MaxDepth:       200,                            // 0 = unlimited
    MaxDepthMode:   config.DepthRejectOrder,        // or DepthDropFarSide
    Features:       config.DefaultFeatures(),
    STPMode:        config.STPCancelBoth,           // default self-trade behavior
    FeeSchedule: config.FeeSchedule{
        Model:         config.FeeModelFlat,
        MakerFeeRate:  types.MustDecimal("-0.0002", 4), // rebate
        TakerFeeRate:  types.MustDecimal("0.0007", 4),
        Currency:      "USD",
    },
    CircuitBreaker: &config.CircuitBreakerConfig{
        WindowSeconds:  300,            // 5-minute rolling window
        MaxMovePercent: types.MustDecimal("0.0500", 4), // 5% max move
        CooldownSeconds: 60,
    },
    MaxCascadeDepth: 10,  // max stop-order chain reactions before halt
}
```

#### Feature flags

```go
// DefaultFeatures() = FeatureMarketOrders | FeatureIOC only — add more as needed:

// Customize:
cfg.Features = config.DefaultFeatures().
    Add(config.FeatureFOK).
    Add(config.FeaturePostOnly).
    Add(config.FeatureIcebergOrders).
    Add(config.FeatureStopOrders).
    Add(config.FeatureAuctions)
```

| Flag | Meaning |
|---|---|
| `FeatureMarketOrders` | Allow market orders |
| `FeatureIOC` | Allow IOC time-in-force |
| `FeatureFOK` | Allow FOK time-in-force |
| `FeaturePostOnly` | Allow PostOnly flag |
| `FeatureIcebergOrders` | Allow iceberg orders |
| `FeatureStopOrders` | Allow stop and stop-limit orders |
| `FeatureAuctions` | Enable call auction mode |
| `FeatureReduceOnly` | Allow ReduceOnly flag (enforcement via hook) |

---

## Commands

All input to the engine is a `Command`. Submit commands with `engine.Submit(cmd)`. Submission is non-blocking; returns `ErrCommandQueueFull` if the internal channel is at capacity.

### PlaceLimitOrder

```go
engine.PlaceLimitOrder{
    MarketID:   "BTC-USD",
    OrderID:    types.NewOrderID(),      // generated by caller
    UserID:     "usr_alice",
    Side:       types.Bid,               // or types.Ask
    Price:      types.MustDecimal("49500.00", 2),
    Qty:        types.MustDecimal("0.50000000", 8),
    TIF:        types.GTC,
    Flags:      types.FlagPostOnly,      // optional, combinable with |
    ExpireAt:   0,                       // unix ns; non-zero for GTD
    STPMode:    config.STPCancelBoth,    // zero = market default
}
```

**Time-in-force options:**

| TIF | Behavior |
|---|---|
| `GTC` | Good-till-canceled — rests until filled or explicitly canceled |
| `IOC` | Immediate-or-cancel — fills what it can, cancels the rest immediately |
| `FOK` | Fill-or-kill — must fill 100% immediately or the whole order is rejected; **book is not modified on failure** |
| `GTD` | Good-till-date — like GTC but expires at `ExpireAt` (unix nanoseconds) |
| `DAY` | Good for the current session (session management is outside the engine) |

**Order flags:**

| Flag | Behavior |
|---|---|
| `FlagPostOnly` | Rejected if the order would cross immediately (maker-only protection) |
| `FlagReduceOnly` | Position-reducing orders only (enforcement is in your pre-order hook) |
| `FlagIceberg` | Display only `DisplayQty`; replenish from hidden reserve when visible portion fills |

**Iceberg orders:**

```go
engine.PlaceLimitOrder{
    ...
    Flags:      types.FlagIceberg,
    Qty:        types.MustDecimal("100", 0),  // total quantity
    DisplayQty: types.MustDecimal("10", 0),   // visible per replenishment
}
```

When the visible slice fills, the order is replenished from its hidden reserve and moved to the **back of the queue** at that price level (loses time priority).

### PlaceMarketOrder

```go
engine.PlaceMarketOrder{
    MarketID: "BTC-USD",
    OrderID:  types.NewOrderID(),
    UserID:   "usr_bob",
    Side:     types.Ask,
    Qty:      types.MustDecimal("0.20000000", 8),
}
```

Market orders match at whatever price is available. If the book runs out of liquidity, the remaining quantity is **canceled** (not rested). They are rejected if `FeatureMarketOrders` is not enabled on the market.

### PlaceStopOrder

```go
// Stop-market: converts to a market order when triggered
engine.PlaceStopOrder{
    MarketID:   "BTC-USD",
    OrderID:    types.NewOrderID(),
    UserID:     "usr_carol",
    Side:       types.Ask,
    StopPrice:  types.MustDecimal("48000.00", 2),
    Qty:        types.MustDecimal("0.10000000", 8),
    ConvertTo:  types.Market,
}

// Stop-limit: converts to a limit order when triggered
engine.PlaceStopOrder{
    ...
    StopPrice:  types.MustDecimal("48000.00", 2),
    LimitPrice: types.MustDecimal("47800.00", 2),
    ConvertTo:  types.StopLimit,
}
```

Stop orders rest in a separate stop book. They are **not visible** in the regular order book depth. When the last trade price crosses a stop's trigger price, the stop converts to the specified order type and is submitted back into the matching engine.

Cascade protection: if a chain of stop triggers would exceed `MaxCascadeDepth`, the engine halts the market instead of cascading further.

### CancelOrder

```go
engine.CancelOrder{
    MarketID: "BTC-USD",
    OrderID:  existingOrderID,
    UserID:   "usr_alice",  // must match the original UserID
}
```

Ownership is enforced — canceling another user's order returns `OrderCanceled` with `ReasonOwnershipMismatch`. Admin force-cancel should use `AdminHaltMarket` then re-open, or extend the hook layer.

### Admin Commands

```go
// Create a new market (wires up engine state; config is already validated)
engine.AdminCreateMarket{MarketID: "ETH-USD", Config: ethCfg}

// Halt a running market (stops all matching; cancels are still accepted)
engine.AdminHaltMarket{MarketID: "BTC-USD", Reason: "scheduled maintenance"}

// Resume from halt
engine.AdminResumeMarket{MarketID: "BTC-USD"}
```

---

## Events

`engine.Events()` returns a `<-chan events.Event`. Drain this channel continuously — it is the only output path. The channel is closed when the engine is shut down.

Every event implements:

```go
type Event interface {
    EventSeqNum() uint64           // monotonically increasing per market
    EventTimestamp() int64         // unix nanoseconds
    EventMarketID() types.MarketID
    EventType() string             // string discriminator, e.g. "trade_fill"
}
```

### Event Reference

All event structs are **value types** (not pointers) on the channel. Use a non-pointer type assertion in switches.

| Type constant | Struct | When |
|---|---|---|
| `TypeOrderAccepted` | `events.OrderAccepted` | Any order passes validation and enters the engine |
| `TypeOrderRested` | `events.OrderRested` | A limit/stop-limit order was added to the book |
| `TypeTradeFill` | `events.TradeFill` | **One per participant per match** — maker and taker fills are separate events |
| `TypeTradeExecuted` | `events.TradeExecuted` | Summary after all fills for one incoming order |
| `TypeOrderCanceled` | `events.OrderCanceled` | IOC remainder, explicit cancel, STP removal, or cascade abort |
| `TypeOrderRejected` | `events.OrderRejected` | Validation failure (bad tick, hook rejection, etc.) |
| `TypeOrderExpired` | `events.OrderExpired` | GTD order reached its `ExpireAt` timestamp |
| `TypeStopTriggered` | `events.StopTriggered` | Stop order converted and re-submitted |
| `TypeMarketHalted` | `events.MarketHalted` | Circuit breaker or admin halt |
| `TypeMarketResumed` | `events.MarketResumed` | Market returned to Open state |
| `TypeDepthUpdate` | `events.DepthUpdate` | A price level was added, modified, or removed |
| `TypeBookSnapshot` | `events.BookSnapshot` | Full depth snapshot (from `engine.Snapshot()`) |
| `TypeAuctionOpened` | `events.AuctionOpened` | Market entered call auction mode |
| `TypeAuctionCleared` | `events.AuctionCleared` | Auction completed; unmatched GTC orders entered continuous book |

### Reading a fill

```go
case events.TradeFill:
    fmt.Printf(
        "order=%s user=%s side=%s role=%s filled=%s @ %s fee=%s\n",
        e.OrderID, e.UserID, e.Side, e.Role,
        e.FilledQty, e.Price, e.Fee,
    )
```

`TradeFill.Role` is `events.RoleMaker` or `events.RoleTaker`. For every match, the engine emits two `TradeFill` events (one for each participant) and one `TradeExecuted` summary.

---

## Options

Pass options to `engine.New` to tune resource allocation and inject dependencies:

```go
eng, err := engine.New(cfg,
    engine.WithNodePoolSize(500_000),      // pre-allocated order nodes (default 100k)
    engine.WithLevelPoolSize(20_000),      // pre-allocated price levels (default 10k)
    engine.WithCommandBuffer(50_000),      // cmd channel depth (default 10k)
    engine.WithEventBuffer(200_000),       // event channel depth (default 50k)
    engine.WithFeeCalculator(myFees),      // default: ZeroFeeCalculator
    engine.WithPreOrderHook(myHook),       // default: nil (accept all)
)
```

---

## Fee Calculators

Implement the `fees.FeeCalculator` interface and pass it with `WithFeeCalculator`:

```go
type FeeCalculator interface {
    Calculate(schedule config.FeeSchedule, fill types.Fill) FeeResult
}
```

Three built-in implementations:

### ZeroFeeCalculator (default)

Always returns zero fees. Use this for paper trading.

```go
engine.WithFeeCalculator(&fees.ZeroFeeCalculator{})
```

### FlatRateFeeCalculator

Applies `MakerFeeRate` and `TakerFeeRate` from `FeeSchedule`. Negative maker rate = rebate.

```go
engine.WithFeeCalculator(&fees.FlatRateFeeCalculator{})
```

Configure rates in `MarketConfig.FeeSchedule`:

```go
FeeSchedule: config.FeeSchedule{
    Model:        config.FeeModelFlat,
    MakerFeeRate: types.MustDecimal("-0.0002", 4),  // 2bp rebate
    TakerFeeRate: types.MustDecimal("0.0007", 4),   // 7bp fee
    Currency:     "USD",
},
```

### TieredFeeCalculator

Looks up the user's 30-day volume to select a fee tier. Requires a `VolumeProvider`:

```go
type VolumeProvider interface {
    GetVolume(userID types.UserID, marketID types.MarketID) types.Decimal
}

calc := fees.TieredFeeCalculator{Volume: myVolumeProvider}
engine.WithFeeCalculator(calc)
```

Configure tiers in `MarketConfig.FeeSchedule.Tiers` (sorted ascending by `MinVolume`).

---

## Pre-Order Hook

The `PreOrderHook` fires **before** each order enters the book. Use it for credit checks, position limits, or any risk gate:

```go
type PreOrderHook interface {
    Validate(ctx hooks.OrderContext) hooks.ValidationResult
}
```

`OrderContext` carries the full order parameters, the user ID, and a pointer to `MarketConfig`.

```go
type MyWalletHook struct{ db WalletDB }

func (h *MyWalletHook) Validate(ctx hooks.OrderContext) hooks.ValidationResult {
    if ctx.OrderType == types.Market {
        return hooks.OK() // price unknown; check after fill
    }
    // naive notional: use raw integer math to avoid precision mismatch
    cost := ctx.Price.Value() * ctx.Qty.Value() / pow10(ctx.Qty.Precision())
    balance := h.db.GetAvailable(ctx.UserID)
    if balance < cost {
        return hooks.Reject(types.RejectPreOrderHook, "insufficient credits")
    }
    h.db.Reserve(ctx.UserID, cost)
    return hooks.OK()
}

eng, _ := engine.New(cfg, engine.WithPreOrderHook(&MyWalletHook{db: db}))
```

If the hook returns a rejection, the engine emits `OrderRejected` with your reason and **never touches the book**.

---

## Market Lifecycle

Markets start in `PreOpen`. Valid transitions:

```
PreOpen → Auction → Open → Halted → Open
PreOpen → Open → Halted → Auction → Open
PreOpen → Open → Closed
Halted  → Closed
```

| State | Limit orders | Market orders | Matching | Cancels |
|---|---|---|---|---|
| PreOpen | Yes | No | No | Yes |
| Auction | Yes | No | No (batch) | Yes |
| Open | Yes | Yes | Yes | Yes |
| Halted | No | No | No | Yes |
| Closed | No | No | No | No |

The engine manages state internally. Trigger transitions with `AdminHaltMarket` / `AdminResumeMarket`, or let the circuit breaker do it automatically.

---

## Circuit Breaker

When enabled in `MarketConfig.CircuitBreaker`, the engine tracks a rolling window of trade prices. If the price moves more than `MaxMovePercent` within `WindowSeconds`, the market is automatically halted and a `MarketHalted` event is emitted.

```go
CircuitBreaker: &config.CircuitBreakerConfig{
    WindowSeconds:   300,                               // 5-minute window
    MaxMovePercent:  types.MustDecimal("0.0500", 4),   // 5% max move
    CooldownSeconds: 60,                                // min time between halts
},
```

Resume with `AdminResumeMarket`. The engine does not auto-resume.

---

## Call Auction Mode

When `FeatureAuctions` is enabled and the market is in `Auction` state, orders accumulate in an auction book rather than matching immediately. When the auction closes:

1. The engine computes the **equilibrium (clearing) price** — the price that maximizes executable volume.
2. All matched orders execute at the clearing price, regardless of their limit prices.
3. Unmatched GTC orders transfer to the continuous book.
4. IOC/FOK unmatched orders are canceled.

Use `auction.NewAuctionBook()` directly if you need the algorithm standalone (e.g., for opening/closing auctions in a custom scheduler).

---

## Multi-Market Engine

`MultiEngine` manages many markets under one roof. Commands are routed by `MarketID`:

```go
me := engine.NewMultiEngine()

// Create markets
me.CreateMarket(btcCfg, engine.WithFeeCalculator(btcFees))
me.CreateMarket(ethCfg, engine.WithFeeCalculator(ethFees))

// Submit — routed automatically by MarketID
me.Submit(engine.PlaceLimitOrder{MarketID: "BTC-USD", ...})

// Per-market event channel
btcEvents, _ := me.Events("BTC-USD")

// Merged event channel (all markets)
all := me.AllEvents()  // fan-in goroutine, single channel

// Tear down one market
me.CloseMarket("BTC-USD")

// Tear down everything
me.Close()
```

---

## Monitoring

`engine.Stats()` returns pool and order book utilization:

```go
stats := eng.Stats()
fmt.Printf("nodes: %d/%d | levels: %d/%d | open orders: %d\n",
    stats.NodePoolUsed, stats.NodePoolCapacity,
    stats.LevelPoolUsed, stats.LevelPoolCapacity,
    stats.OpenOrders,
)
```

Alert when pool utilization exceeds 80% and increase `WithNodePoolSize` / `WithLevelPoolSize` before exhaustion. Exhausting the level pool panics with a descriptive message.

---

## Best-Bid/Offer and Depth

```go
// Current best bid and ask
bid, ask, hasBid, hasAsk := eng.BBO()

// Full depth snapshot (up to n levels per side)
snapshot := eng.Snapshot(20)
for _, level := range snapshot.Bids {
    fmt.Printf("bid %s x %s (%d orders)\n", level.Price, level.TotalQty, level.OrderCount)
}
```

---

## Testing With testutil

The `testutil` package provides helpers for integration tests:

```go
import "github.com/thorlaidanegg/clob/testutil"

cfg := testutil.DefaultConfig("TEST-USD")

// NewHarness: start engine, submit commands, collect events synchronously
h, _ := testutil.NewHarness(cfg)
defer h.Close()

ask := testutil.LimitOrder("TEST-USD", "alice", types.Ask, "100.00", "10", 2, 0)
bid := testutil.LimitOrder("TEST-USD", "bob",   types.Bid, "100.00", "10", 2, 0)
h.Do(ask)
h.Do(bid)

evts := h.Drain(100 * time.Millisecond)
testutil.AssertTrade(t, evts, "100.00", "10", 2, 0)
```

**Builder helpers:**

```go
// marketID, userID, side, price, qty, pricePrecision, qtyPrecision
testutil.LimitOrder("MKT", "usr", types.Ask, "100.00", "10", 2, 0)

// marketID, userID, side, qty, qtyPrecision
testutil.MarketOrder("MKT", "usr", types.Bid, "5", 0)

// marketID, userID, side, triggerPrice, qty, pricePrecision, qtyPrecision
testutil.StopOrder("MKT", "usr", types.Ask, "98.00", "5", 2, 0)

// marketID → sensible MarketConfig for tests
testutil.DefaultConfig("MKT")
```

**Assertion helpers:**

```go
// price, qty, pricePrecision, qtyPrecision
testutil.AssertTrade(t, evts, "100.00", "10", 2, 0)

testutil.AssertRested(t, evts, orderID)
testutil.AssertRejected(t, evts, orderID, types.RejectInvalidTick)
testutil.AssertCanceled(t, evts, orderID)

// returns count of events with the given EventType() string
testutil.CountEventType(evts, events.TypeTradeFill)
```

---

## Self-Trade Prevention

Configure the default STP behavior per market, or override per order with `PlaceLimitOrder.STPMode`:

| Mode | Behavior when buyer == seller |
|---|---|
| `STPDisabled` | No protection — trades execute normally |
| `STPCancelBoth` | Both maker and incoming order are canceled |
| `STPCancelMaker` | Only the resting (maker) order is canceled; matching continues |
| `STPCancelTaker` | The incoming order is canceled; resting order stays |
| `STPDecrementCancel` | Incoming qty decremented by maker qty; smaller side canceled |

---

## Thread Safety

The engine is **not thread-safe at the API level** — that is by design. All state mutations happen inside one goroutine (started by `engine.Start()`). `Submit` is the only thread-safe method; it writes to a buffered channel. `Events`, `BBO`, `Snapshot`, and `Stats` are also safe to call from any goroutine. Do not call `Start`, `Close`, or concurrent `New` on the same instance.

---

## Performance Notes

The match loop allocates zero objects in the steady state:

- Order nodes are drawn from a pre-allocated `pool.Pool[OrderNode]` — no heap alloc per order
- Price levels are drawn from a pre-allocated `pool.Pool[PriceLevel]`
- Fill records are value types appended to a pre-allocated `[]types.Fill{0, 8}` slice

Benchmark reference (AMD Ryzen 9 5900HX, `go test -bench=. -benchmem ./book/...`):

```
BenchmarkMatchLoop_LimitCrossingAsk-16     456k ops/s   ~2.7 µs/op
BenchmarkMatchLoop_MarketOrderDrainsBook-16 184k ops/s   ~6.0 µs/op
BenchmarkMatchLoop_FOKDryRun-16            1.9M ops/s   ~0.6 µs/op
```

For sustained throughput, size pools generously (`WithNodePoolSize(500_000)`) and keep the event consumer fast — a blocked event channel stalls the match loop.

---

## Design Invariants

These are never violated:

1. No `float64` for any financial value — always `types.Decimal`
2. No locks on the order book — single goroutine owns all state via `cmdChan`
3. No heap allocation inside the match loop hot path
4. No external dependencies beyond `btree` and `ulid/v2`
5. SeqNum is the **only** time priority determinant — Timestamp is for reporting only
6. Events are the **only** output — the engine never calls back into user code
7. FOK dry-run leaves **zero state change** on failure
8. Every acquired pool node is released exactly once

---

## License

MIT

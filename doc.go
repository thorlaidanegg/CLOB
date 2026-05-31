// Package clob is a pure Go central limit order book (CLOB) matching engine.
//
// # Overview
//
// clob is a self-contained, zero-infrastructure matching engine. You give it
// market configurations and a stream of commands; it gives back a stream of
// events. No databases, no queues, no network — just a deterministic,
// in-memory order book.
//
// # Entry Points
//
// For a single market use [engine.Engine]:
//
//	eng, _ := engine.New(cfg)
//	eng.Start()
//	defer eng.Close()
//
//	eng.Submit(engine.PlaceLimitOrder{ ... })
//
//	for ev := range eng.Events() {
//	    // handle events
//	}
//
// For multiple markets use [engine.MultiEngine]:
//
//	me := engine.NewMultiEngine()
//	me.CreateMarket(btcCfg)
//	me.CreateMarket(ethCfg)
//	me.Submit(engine.PlaceLimitOrder{MarketID: "BTC-USD", ...})
//	all := me.AllEvents() // merged channel
//
// # Packages
//
//   - [github.com/thorlaidanegg/clob/engine] — Engine and MultiEngine (start here)
//   - [github.com/thorlaidanegg/clob/config] — MarketConfig and feature flags
//   - [github.com/thorlaidanegg/clob/events] — All 14 event types emitted by the engine
//   - [github.com/thorlaidanegg/clob/types]  — Decimal, Side, TIF, OrderType, flags
//   - [github.com/thorlaidanegg/clob/fees]   — FeeCalculator implementations
//   - [github.com/thorlaidanegg/clob/hooks]  — PreOrderHook for credit/risk gates
//   - [github.com/thorlaidanegg/clob/testutil] — Test helpers and EngineHarness
package clob

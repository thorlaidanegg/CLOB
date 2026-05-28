package engine

import "testing"

func TestMultiEngine_CreateAndSubmit(t *testing.T) {
	me := NewMultiEngine()
	defer me.Close() //nolint

	cfg := testConfig()
	cfg.MarketID = "ETH-USD"

	if err := me.CreateMarket(cfg); err != nil {
		t.Fatalf("CreateMarket: %v", err)
	}

	// Submit a command to the market.
	err := me.Submit(AdminResumeMarket{MarketID: "ETH-USD"})
	if err != nil {
		t.Errorf("Submit: %v", err)
	}
}

func TestMultiEngine_DuplicateMarket(t *testing.T) {
	me := NewMultiEngine()
	defer me.Close() //nolint

	cfg := testConfig()
	cfg.MarketID = "ETH-USD"

	if err := me.CreateMarket(cfg); err != nil {
		t.Fatalf("first CreateMarket: %v", err)
	}
	if err := me.CreateMarket(cfg); err != ErrMarketAlreadyExists {
		t.Errorf("expected ErrMarketAlreadyExists, got %v", err)
	}
}

func TestMultiEngine_SubmitUnknownMarket(t *testing.T) {
	me := NewMultiEngine()
	defer me.Close() //nolint

	err := me.Submit(AdminResumeMarket{MarketID: "UNKNOWN"})
	if err != ErrMarketNotFound {
		t.Errorf("expected ErrMarketNotFound, got %v", err)
	}
}

func TestMultiEngine_CloseMarket(t *testing.T) {
	me := NewMultiEngine()
	defer me.Close() //nolint

	cfg := testConfig()
	cfg.MarketID = "SOL-USD"
	if err := me.CreateMarket(cfg); err != nil {
		t.Fatalf("CreateMarket: %v", err)
	}
	if err := me.CloseMarket("SOL-USD"); err != nil {
		t.Errorf("CloseMarket: %v", err)
	}
	if err := me.CloseMarket("SOL-USD"); err != ErrMarketNotFound {
		t.Errorf("expected ErrMarketNotFound for already-closed market, got %v", err)
	}
}

func TestMultiEngine_Events(t *testing.T) {
	me := NewMultiEngine()
	defer me.Close() //nolint

	cfg := testConfig()
	cfg.MarketID = "ADA-USD"
	if err := me.CreateMarket(cfg); err != nil {
		t.Fatalf("CreateMarket: %v", err)
	}

	ch, err := me.Events("ADA-USD")
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if ch == nil {
		t.Error("Events channel should not be nil")
	}

	_, err = me.Events("NOPE")
	if err != ErrMarketNotFound {
		t.Errorf("expected ErrMarketNotFound, got %v", err)
	}
}

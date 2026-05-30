package engine

import (
	"errors"
	"time"

	"github.com/thorlaidanegg/clob/auction"
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

// CommandProcessor is the single-goroutine owner of the order book and all
// associated state. All mutation goes through the cmdChan FIFO.
type CommandProcessor struct {
	book        *book.OrderBook
	stopBook    *stopbook.StopBook
	auctionBook *auction.AuctionBook    // nil if FeatureAuctions disabled
	breaker     *circuit.CircuitBreaker // nil if circuit breaker disabled
	state       *statemachine.Machine
	nodePool    *pool.Pool[book.OrderNode] // shared with book
	orderSeq    *sequence.Counter          // shared with book
	eventSeq    *sequence.Counter          // processor-only
	feeCalc     fees.FeeCalculator
	preHook     hooks.PreOrderHook // nil if no hook
	cfg         *config.MarketConfig
	cmdChan     chan Command // bidirectional: engine submits, processor reads + re-queues stops
	eventChan   chan events.Event
	quit        chan struct{}
	done        chan struct{}
}

func newCommandProcessor(
	b *book.OrderBook,
	sb *stopbook.StopBook,
	ab *auction.AuctionBook, // nil if auctions disabled
	breaker *circuit.CircuitBreaker,
	sm *statemachine.Machine,
	nodePool *pool.Pool[book.OrderNode],
	orderSeq *sequence.Counter, // shared with book — MUST be the same instance
	feeCalc fees.FeeCalculator,
	preHook hooks.PreOrderHook,
	cfg *config.MarketConfig,
	cmdChan chan Command,
	eventChan chan events.Event,
) *CommandProcessor {
	return &CommandProcessor{
		book:        b,
		stopBook:    sb,
		auctionBook: ab,
		breaker:     breaker,
		state:       sm,
		nodePool:    nodePool,
		orderSeq:    orderSeq, // shared counter — do NOT create a new one here
		eventSeq:    sequence.NewCounter(cfg.InitialEventSeq),
		feeCalc:     feeCalc,
		preHook:     preHook,
		cfg:         cfg,
		cmdChan:     cmdChan,
		eventChan:   eventChan,
		quit:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (p *CommandProcessor) run() {
	defer close(p.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Auction timers — nil channels never fire in select, so these are no-ops
	// when auctions are disabled.
	var auctionOpenC <-chan time.Time
	var auctionClearC <-chan time.Time
	if p.auctionBook != nil && p.cfg.Auction != nil {
		now := time.Now()
		openAt := p.cfg.Auction.OpenTime.Add(-p.cfg.Auction.PreOpenDuration)
		clearAt := p.cfg.Auction.OpenTime
		if openAt.After(now) {
			auctionOpenC = time.After(time.Until(openAt))
		} else {
			// Pre-open period already started — open the auction right away.
			auctionOpenC = time.After(0)
		}
		if clearAt.After(now) {
			auctionClearC = time.After(time.Until(clearAt))
		} else {
			// OpenTime already passed — clear immediately after open fires.
			auctionClearC = time.After(time.Millisecond)
		}
	}

	for {
		select {
		case cmd := <-p.cmdChan:
			p.dispatch(cmd)
		case t := <-ticker.C:
			p.runExpiryCheck(t.UnixNano())
		case <-auctionOpenC:
			p.openAuction(time.Now().UnixNano())
			auctionOpenC = nil
		case <-auctionClearC:
			p.clearAuction(time.Now().UnixNano())
			auctionClearC = nil
		case <-p.quit:
			return
		}
	}
}

func (p *CommandProcessor) dispatch(cmd Command) {
	switch c := cmd.(type) {
	case PlaceLimitOrder:
		p.processLimitOrder(c)
	case PlaceMarketOrder:
		p.processMarketOrder(c)
	case PlaceStopOrder:
		p.processStopOrder(c)
	case CancelOrder:
		p.processCancelOrder(c)
	case AdminHaltMarket:
		p.processAdminHalt(c)
	case AdminResumeMarket:
		p.processAdminResume(c)
	}
}

// --- Limit orders --------------------------------------------------------

func (p *CommandProcessor) processLimitOrder(cmd PlaceLimitOrder) {
	now := time.Now().UnixNano()

	// 1. State check.
	if !p.state.CanAcceptLimitOrder() {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectMarketNotOpen, "market not open for limit orders", now)
		return
	}

	// 2. Feature checks.
	if cmd.TIF == types.IOC && !p.cfg.Features.Has(config.FeatureIOC) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "IOC orders disabled", now)
		return
	}
	if cmd.TIF == types.FOK && !p.cfg.Features.Has(config.FeatureFOK) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "FOK orders disabled", now)
		return
	}
	if cmd.Flags.Has(types.FlagPostOnly) && !p.cfg.Features.Has(config.FeaturePostOnly) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "PostOnly orders disabled", now)
		return
	}
	if cmd.Flags.Has(types.FlagIceberg) && !p.cfg.Features.Has(config.FeatureIcebergOrders) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "iceberg orders disabled", now)
		return
	}

	// 3. Duplicate orderID.
	if p.book.HasOrder(cmd.OrderID) || p.stopBook.Has(cmd.OrderID) ||
		(p.auctionBook != nil && p.auctionBook.Has(cmd.OrderID)) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectDuplicateOrderID, "duplicate order id", now)
		return
	}

	// 4. Validation.
	if reason, msg, ok := p.validateLimitOrder(cmd); !ok {
		p.rejectOrder(cmd.OrderID, cmd.UserID, reason, msg, now)
		return
	}

	// 4b. MaxDepth check — only for resting orders in the continuous book (skip during auction).
	if p.cfg.MaxDepth > 0 && cmd.TIF.CanRest() &&
		p.state.Current() != statemachine.Auction &&
		p.book.WouldExceedMaxDepth(cmd.Price, cmd.Side) {
		switch p.cfg.MaxDepthMode {
		case config.DepthRejectOrder:
			p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectMaxDepth, "order price exceeds book depth limit", now)
			return
		case config.DepthTreatAsIOC:
			cmd.TIF = types.IOC
		}
	}

	// 5. PostOnly pre-check (skip during auction — no immediate matching).
	if cmd.Flags.Has(types.FlagPostOnly) &&
		p.state.Current() != statemachine.Auction &&
		p.book.WouldCross(cmd.Price, cmd.Side) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectPostOnlyWouldCross, "post-only order would cross", now)
		return
	}

	// 6. Pre-order hook.
	if p.preHook != nil {
		ctx := hooks.OrderContext{
			MarketID:  p.cfg.MarketID,
			UserID:    cmd.UserID,
			OrderID:   cmd.OrderID,
			Side:      cmd.Side,
			OrderType: types.Limit,
			Price:     cmd.Price,
			Qty:       cmd.Qty,
			TIF:       cmd.TIF,
			Flags:     cmd.Flags,
			Config:    p.cfg,
		}
		result := p.preHook.Validate(ctx)
		if !result.OK {
			p.rejectOrder(cmd.OrderID, cmd.UserID, result.Reason, result.Message, now)
			return
		}
	}

	// 6b. Auction state: accumulate in auction book without matching.
	if p.state.Current() == statemachine.Auction && p.auctionBook != nil {
		seqNum := p.orderSeq.Next()
		p.emit(events.OrderAccepted{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:     cmd.OrderID,
			UserID:      cmd.UserID,
			Side:        cmd.Side,
			OrderType:   types.Limit,
			Price:       cmd.Price,
			OrigQty:     cmd.Qty,
			DisplayQty:  cmd.Qty,
			TIF:         cmd.TIF,
			Flags:       cmd.Flags,
			OrderSeqNum: seqNum,
		})
		p.auctionBook.AddOrder(auction.AuctionOrder{
			OrderID: cmd.OrderID,
			UserID:  cmd.UserID,
			Side:    cmd.Side,
			Price:   cmd.Price,
			Qty:     cmd.Qty,
			TIF:     cmd.TIF,
			SeqNum:  seqNum,
		})
		return
	}

	// 7. Acquire node, assign seqNum.
	node, idx, err := p.nodePool.Acquire()
	if err != nil {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectPoolExhausted, "node pool exhausted", now)
		return
	}
	node.PoolIndex = idx
	node.OrderID = cmd.OrderID
	node.UserID = cmd.UserID
	node.MarketID = p.cfg.MarketID
	node.Side = cmd.Side
	node.Type = types.Limit
	node.Price = cmd.Price
	node.OrigQty = cmd.Qty
	node.FilledQty = types.Zero(cmd.Qty.Precision())
	node.TIF = cmd.TIF
	node.Flags = cmd.Flags
	node.ExpireAt = cmd.ExpireAt
	node.STPMode = cmd.STPMode
	node.Timestamp = now
	node.SeqNum = p.orderSeq.Next()

	if cmd.Flags.Has(types.FlagIceberg) {
		node.DisplayQty = cmd.DisplayQty
		node.OrigDisplayQty = cmd.DisplayQty
		node.HiddenQty = cmd.Qty.Sub(cmd.DisplayQty)
		node.RemainQty = cmd.DisplayQty
	} else {
		node.RemainQty = cmd.Qty
		node.DisplayQty = cmd.Qty
		node.OrigDisplayQty = cmd.Qty
	}

	// 8. Emit OrderAccepted.
	p.emit(events.OrderAccepted{
		Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		OrderID:     cmd.OrderID,
		UserID:      cmd.UserID,
		Side:        cmd.Side,
		OrderType:   types.Limit,
		Price:       cmd.Price,
		OrigQty:     cmd.Qty,
		DisplayQty:  node.DisplayQty,
		TIF:         cmd.TIF,
		Flags:       cmd.Flags,
		OrderSeqNum: node.SeqNum,
	})

	// Halted state: rest without matching — orders queue until market resumes.
	if p.state.Current() == statemachine.Halted {
		p.book.PlaceResting(node)
		p.emitDisposition(cmd.OrderID, cmd.UserID, cmd.Side, cmd.Price, nil, book.Rested, now)
		return
	}

	// 9-10. Match, emit fills, emit disposition.
	fills, disposition := p.book.PlaceLimit(node)
	lastFillPrice := p.emitFillEvents(fills, now)
	p.emitDisposition(cmd.OrderID, cmd.UserID, cmd.Side, cmd.Price, fills, disposition, now)

	// 12â€“13. Stop triggers and circuit breaker.
	if lastFillPrice != nil {
		p.checkStopsAndBreaker(*lastFillPrice, now)
	}
}

// --- Market orders -------------------------------------------------------

func (p *CommandProcessor) processMarketOrder(cmd PlaceMarketOrder) {
	now := time.Now().UnixNano()

	if !p.state.CanAcceptMarketOrder() {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectMarketNotOpen, "market not open for market orders", now)
		return
	}
	if !p.cfg.Features.Has(config.FeatureMarketOrders) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "market orders disabled", now)
		return
	}
	if p.book.HasOrder(cmd.OrderID) || p.stopBook.Has(cmd.OrderID) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectDuplicateOrderID, "duplicate order id", now)
		return
	}
	if !cmd.Qty.IsPositive() {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectBelowMinQty, "qty must be positive", now)
		return
	}
	if !p.cfg.MinOrderQty.IsZero() && cmd.Qty.LessThan(p.cfg.MinOrderQty) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectBelowMinQty, "qty below minimum", now)
		return
	}
	if !p.cfg.MaxOrderQty.IsZero() && cmd.Qty.GreaterThan(p.cfg.MaxOrderQty) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectAboveMaxQty, "qty above maximum", now)
		return
	}

	node, idx, err := p.nodePool.Acquire()
	if err != nil {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectPoolExhausted, "node pool exhausted", now)
		return
	}
	node.PoolIndex = idx
	node.OrderID = cmd.OrderID
	node.UserID = cmd.UserID
	node.MarketID = p.cfg.MarketID
	node.Side = cmd.Side
	node.Type = types.Market
	node.Price = types.Zero(p.cfg.PricePrecision)
	node.OrigQty = cmd.Qty
	node.RemainQty = cmd.Qty
	node.FilledQty = types.Zero(cmd.Qty.Precision())
	node.DisplayQty = cmd.Qty
	node.OrigDisplayQty = cmd.Qty
	node.TIF = cmd.TIF
	node.Flags = cmd.Flags
	node.STPMode = cmd.STPMode
	node.Timestamp = now
	node.SeqNum = p.orderSeq.Next()

	p.emit(events.OrderAccepted{
		Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		OrderID:     cmd.OrderID,
		UserID:      cmd.UserID,
		Side:        cmd.Side,
		OrderType:   types.Market,
		OrigQty:     cmd.Qty,
		DisplayQty:  cmd.Qty,
		TIF:         cmd.TIF,
		Flags:       cmd.Flags,
		OrderSeqNum: node.SeqNum,
	})

	fills, disposition := p.book.PlaceMarket(node)
	lastFillPrice := p.emitFillEvents(fills, now)
	p.emitDisposition(cmd.OrderID, cmd.UserID, cmd.Side, types.Zero(p.cfg.PricePrecision), fills, disposition, now)

	if lastFillPrice != nil {
		p.checkStopsAndBreaker(*lastFillPrice, now)
	}
}

// --- Stop orders ---------------------------------------------------------

func (p *CommandProcessor) processStopOrder(cmd PlaceStopOrder) {
	now := time.Now().UnixNano()

	if !p.state.CanAcceptStopOrder() {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectMarketNotOpen, "market not open for stop orders", now)
		return
	}
	if !p.cfg.Features.Has(config.FeatureStopOrders) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectFeatureDisabled, "stop orders disabled", now)
		return
	}
	if p.book.HasOrder(cmd.OrderID) || p.stopBook.Has(cmd.OrderID) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectDuplicateOrderID, "duplicate order id", now)
		return
	}

	node, idx, err := p.stopBook.AcquireNode()
	if err != nil {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectPoolExhausted, "stop node pool exhausted", now)
		return
	}
	node.PoolIndex = idx
	node.OrderID = cmd.OrderID
	node.UserID = cmd.UserID
	node.MarketID = p.cfg.MarketID
	node.Side = cmd.Side
	node.TriggerPrice = cmd.TriggerPrice
	node.LimitPrice = cmd.LimitPrice
	node.Qty = cmd.Qty
	node.ConvertTo = cmd.ConvertTo
	node.TIF = cmd.TIF
	node.Flags = cmd.Flags
	node.STPMode = cmd.STPMode

	p.stopBook.AddStop(node)

	p.emit(events.OrderAccepted{
		Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		OrderID:     cmd.OrderID,
		UserID:      cmd.UserID,
		Side:        cmd.Side,
		OrderType:   cmd.ConvertTo,
		StopPrice:   cmd.TriggerPrice,
		Price:       cmd.LimitPrice,
		OrigQty:     cmd.Qty,
		DisplayQty:  cmd.Qty,
		TIF:         cmd.TIF,
		Flags:       cmd.Flags,
		OrderSeqNum: p.orderSeq.Next(),
	})
}

// --- Cancel --------------------------------------------------------------

func (p *CommandProcessor) processCancelOrder(cmd CancelOrder) {
	now := time.Now().UnixNano()

	// Try resting book first.
	node, err := p.book.Cancel(cmd.OrderID, cmd.UserID)
	if err == nil {
		canceledQty := node.RemainQty
		if node.Flags.Has(types.FlagIceberg) {
			canceledQty = canceledQty.Add(node.HiddenQty)
		}
		p.emit(events.OrderCanceled{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:     cmd.OrderID,
			UserID:      cmd.UserID,
			Side:        node.Side,
			Price:       node.Price,
			CanceledQty: canceledQty,
			FilledQty:   node.FilledQty,
			Reason:      types.CancelUserRequested,
		})
		p.nodePool.Release(node.PoolIndex)
		return
	} else if errors.Is(err, book.ErrOwnershipMismatch) {
		p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectOrderNotFound, "order not found", now)
		return
	}

	// Only reach here when err == book.ErrOrderNotFound; try stop book.
	stopNode, ok := p.stopBook.CancelStop(cmd.OrderID, cmd.UserID)
	if ok {
		p.emit(events.OrderCanceled{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:     cmd.OrderID,
			UserID:      cmd.UserID,
			Side:        stopNode.Side,
			Price:       stopNode.TriggerPrice,
			CanceledQty: stopNode.Qty,
			Reason:      types.CancelUserRequested,
		})
		p.stopBook.ReleaseNode(stopNode.PoolIndex)
		return
	}

	p.rejectOrder(cmd.OrderID, cmd.UserID, types.RejectOrderNotFound, "order not found", now)
}

// --- Admin ---------------------------------------------------------------

func (p *CommandProcessor) processAdminHalt(cmd AdminHaltMarket) {
	now := time.Now().UnixNano()
	if err := p.state.Transition(statemachine.Halted); err != nil {
		return
	}
	p.emit(events.MarketHalted{
		Base:     events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		Reason:   cmd.Reason,
		HaltType: config.HaltAdmin,
	})
}

func (p *CommandProcessor) processAdminResume(cmd AdminResumeMarket) {
	now := time.Now().UnixNano()
	if err := p.state.Transition(statemachine.Open); err != nil {
		return
	}
	p.emit(events.MarketResumed{
		Base:      events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		ResumedBy: "admin",
	})
}

// --- GTD expiry ----------------------------------------------------------

func (p *CommandProcessor) runExpiryCheck(now int64) {
	expired := p.book.ExpireGTD(now)
	for _, node := range expired {
		p.emit(events.OrderExpired{
			Base:       events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:    node.OrderID,
			UserID:     node.UserID,
			Side:       node.Side,
			Price:      node.Price,
			ExpiredQty: node.RemainQty,
		})
		p.nodePool.Release(node.PoolIndex)
	}
}

// --- Stop triggers + circuit breaker -------------------------------------

func (p *CommandProcessor) checkStopsAndBreaker(lastTradePrice types.Decimal, now int64) {
	triggered, err := p.stopBook.CheckTriggers(lastTradePrice, 0)
	if err == stopbook.ErrCascadeLimit {
		if tErr := p.state.Transition(statemachine.Halted); tErr == nil {
			p.emit(events.MarketHalted{
				Base:     events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
				Reason:   "stop order cascade depth limit reached",
				HaltType: config.HaltCascadeLimit,
			})
		}
		return
	}

	for _, to := range triggered {
		p.emit(events.StopTriggered{
			Base:           events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			StopOrderID:    to.OrderID,
			UserID:         to.UserID,
			TriggerPrice:   lastTradePrice,
			ConvertedTo:    to.ConvertTo,
			ConvertedPrice: to.LimitPrice,
		})

		stpMode, _ := to.STPMode.(config.STPMode)

		if to.ConvertTo == types.Market {
			p.cmdChan <- PlaceMarketOrder{
				MarketID: p.cfg.MarketID,
				OrderID:  to.OrderID,
				UserID:   to.UserID,
				Side:     to.Side,
				Qty:      to.Qty,
				TIF:      to.TIF,
				Flags:    to.Flags,
				STPMode:  stpMode,
			}
		} else {
			p.cmdChan <- PlaceLimitOrder{
				MarketID: p.cfg.MarketID,
				OrderID:  to.OrderID,
				UserID:   to.UserID,
				Side:     to.Side,
				Price:    to.LimitPrice,
				Qty:      to.Qty,
				TIF:      to.TIF,
				Flags:    to.Flags,
				STPMode:  stpMode,
			}
		}
	}

	if p.breaker != nil {
		reason, shouldHalt := p.breaker.Check(lastTradePrice, now)
		if shouldHalt {
			if err := p.state.Transition(statemachine.Halted); err == nil {
				p.breaker.SetLastHalt(now)
				p.emit(events.MarketHalted{
					Base:     events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
					Reason:   reason,
					HaltType: config.HaltCircuitBreaker,
				})
			}
		}
	}
}

// --- Event emission helpers ----------------------------------------------

func (p *CommandProcessor) emit(e events.Event) {
	p.eventChan <- e
}

func (p *CommandProcessor) nextEventSeq() uint64 { return p.eventSeq.Next() }

func (p *CommandProcessor) rejectOrder(orderID types.OrderID, userID types.UserID, reason types.RejectionReason, msg string, now int64) {
	p.emit(events.OrderRejected{
		Base:    events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		OrderID: orderID,
		UserID:  userID,
		Reason:  reason,
		Message: msg,
	})
}

// emitFillEvents emits TradeFill×2, TradeExecuted, and DepthUpdate for each
// fill in order. Returns the last fill price.
func (p *CommandProcessor) emitFillEvents(fills []types.Fill, now int64) *types.Decimal {
	if len(fills) == 0 {
		return nil
	}

	for i := range fills {
		fills[i].Timestamp = now
		f := fills[i]

		tradeID := types.NewTradeID()
		feeResult := p.feeCalc.Calculate(p.cfg.FeeSchedule, f)

		makerSide := f.MakerSide
		takerSide := makerSide.Opposite()

		p.emit(events.TradeFill{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			FillID:      types.NewFillID(),
			TradeID:     tradeID,
			OrderID:     f.MakerOrderID,
			UserID:      f.MakerUserID,
			Role:        events.RoleMaker,
			Side:        makerSide,
			Price:       f.Price,
			FilledQty:   f.Qty,
			RemainQty:   f.MakerRemainQty,
			Fee:         feeResult.MakerFee,
			FeeCurrency: feeResult.Currency,
		})
		p.emit(events.TradeFill{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			FillID:      types.NewFillID(),
			TradeID:     tradeID,
			OrderID:     f.TakerOrderID,
			UserID:      f.TakerUserID,
			Role:        events.RoleTaker,
			Side:        takerSide,
			Price:       f.Price,
			FilledQty:   f.Qty,
			RemainQty:   f.TakerRemainQty,
			Fee:         feeResult.TakerFee,
			FeeCurrency: feeResult.Currency,
		})
		p.emit(events.TradeExecuted{
			Base:           events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			TradeID:        tradeID,
			MakerOrderID:   f.MakerOrderID,
			MakerUserID:    f.MakerUserID,
			MakerSide:      makerSide,
			MakerRemainQty: f.MakerRemainQty,
			MakerFee:       feeResult.MakerFee,
			TakerOrderID:   f.TakerOrderID,
			TakerUserID:    f.TakerUserID,
			TakerRemainQty: f.TakerRemainQty,
			TakerFee:       feeResult.TakerFee,
			Price:          f.Price,
			Qty:            f.Qty,
			FeeCurrency:    feeResult.Currency,
		})

		// DepthUpdate immediately after this fill's TradeExecuted.
		var updateType events.DepthUpdateType
		if f.MakerLevelExists {
			updateType = events.DepthModify
		} else {
			updateType = events.DepthDelete
		}
		p.emit(events.DepthUpdate{
			Base:          events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			Side:          makerSide,
			Price:         f.Price,
			NewTotalQty:   f.MakerLevelTotalQty,
			NewDisplayQty: f.MakerLevelDisplayQty,
			NewOrderCount: f.MakerLevelOrderCount,
			UpdateType:    updateType,
		})
	}

	last := fills[len(fills)-1].Price
	return &last
}

func (p *CommandProcessor) emitDisposition(
	orderID types.OrderID, userID types.UserID, side types.Side, price types.Decimal,
	fills []types.Fill, disposition book.Disposition, now int64,
) {
	switch disposition {
	case book.Rested, book.PartialFill_Rested:
		totalQty, displayQty, orderCount, _ := p.book.LevelInfo(side, price)
		p.emit(events.OrderRested{
			Base:       events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:    orderID,
			UserID:     userID,
			Side:       side,
			Price:      price,
			RemainQty:  totalQty,
			DisplayQty: displayQty,
		})
		updateType := events.DepthAdd
		if disposition == book.PartialFill_Rested {
			updateType = events.DepthModify
		}
		p.emit(events.DepthUpdate{
			Base:          events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			Side:          side,
			Price:         price,
			NewTotalQty:   totalQty,
			NewDisplayQty: displayQty,
			NewOrderCount: orderCount,
			UpdateType:    updateType,
		})

	case book.FullyFilled:
		// fills already emitted; no rested/canceled event

	case book.Canceled, book.PartialFill_Canceled:
		filledQty := types.Zero(p.cfg.QtyPrecision)
		for _, f := range fills {
			filledQty = filledQty.Add(f.Qty)
		}
		p.emit(events.OrderCanceled{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:     orderID,
			UserID:      userID,
			Side:        side,
			Price:       price,
			CanceledQty: types.Zero(p.cfg.QtyPrecision),
			FilledQty:   filledQty,
			Reason:      types.CancelIOC,
		})

	case book.Rejected:
		p.emit(events.OrderRejected{
			Base:    events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID: orderID,
			UserID:  userID,
			Reason:  types.RejectFOKFailed,
			Message: "FOK order could not be fully filled",
		})
	}
}

// --- Auction -------------------------------------------------------------

func (p *CommandProcessor) openAuction(now int64) {
	if err := p.state.Transition(statemachine.Auction); err != nil {
		return // already in Auction or invalid transition
	}
	p.emit(events.AuctionOpened{
		Base:            events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		IndicativePrice: types.Zero(p.cfg.PricePrecision),
		IndicativeQty:   types.Zero(p.cfg.QtyPrecision),
	})
}

func (p *CommandProcessor) clearAuction(now int64) {
	// Ensure Auction state — may need to open first if PreOpenDuration == 0.
	if p.state.Current() == statemachine.PreOpen {
		p.openAuction(now)
	}
	if p.state.Current() != statemachine.Auction {
		return
	}

	// Pass zero refPrice — no external reference price available at this call site.
	// Operators can provide one via an admin command or config in a future iteration.
	clearingPrice, matchedQty, found := p.auctionBook.ComputeClearingPrice(types.Zero(p.cfg.PricePrecision))
	if !found {
		// No crossing orders: use zero price so Sweep drains all orders cleanly.
		clearingPrice = types.Zero(p.cfg.PricePrecision)
		matchedQty = types.Zero(p.cfg.QtyPrecision)
	}

	fills, unmatched, canceled := p.auctionBook.Sweep(clearingPrice)

	// Emit fill events (TradeFill×2 + TradeExecuted per fill).
	if len(fills) > 0 {
		p.emitFillEvents(fills, now)
	}

	// Transfer GTC unmatched orders to the continuous book.
	for _, ao := range unmatched {
		node, idx, err := p.nodePool.Acquire()
		if err != nil {
			p.rejectOrder(ao.OrderID, ao.UserID, types.RejectPoolExhausted, "node pool exhausted on auction carryover", now)
			continue
		}
		node.PoolIndex = idx
		node.OrderID = ao.OrderID
		node.UserID = ao.UserID
		node.MarketID = p.cfg.MarketID
		node.Side = ao.Side
		node.Type = types.Limit
		node.Price = ao.Price
		node.OrigQty = ao.Qty
		node.RemainQty = ao.Qty
		node.FilledQty = types.Zero(ao.Qty.Precision())
		node.DisplayQty = ao.Qty
		node.OrigDisplayQty = ao.Qty
		node.TIF = types.GTC
		node.SeqNum = ao.SeqNum

		nodeFills, disp := p.book.PlaceLimit(node)
		if len(nodeFills) > 0 {
			p.emitFillEvents(nodeFills, now)
		}
		p.emitDisposition(ao.OrderID, ao.UserID, ao.Side, ao.Price, nodeFills, disp, now)
	}

	// Emit OrderCanceled for IOC/FOK residuals.
	for _, ao := range canceled {
		p.emit(events.OrderCanceled{
			Base:        events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
			OrderID:     ao.OrderID,
			UserID:      ao.UserID,
			Side:        ao.Side,
			Price:       ao.Price,
			CanceledQty: ao.Qty,
			FilledQty:   types.Zero(ao.Qty.Precision()),
			Reason:      types.CancelIOC,
		})
	}

	// Emit AuctionCleared.
	p.emit(events.AuctionCleared{
		Base:          events.NewBase(p.nextEventSeq(), now, p.cfg.MarketID),
		ClearingPrice: clearingPrice,
		MatchedQty:    matchedQty,
	})

	// Transition to continuous trading.
	p.state.Transition(statemachine.Open) //nolint:errcheck
}

// --- Validation ----------------------------------------------------------

func (p *CommandProcessor) validateLimitOrder(cmd PlaceLimitOrder) (types.RejectionReason, string, bool) {
	if !cmd.Price.IsPositive() {
		return types.RejectInvalidPrice, "price must be positive", false
	}
	if !cmd.Qty.IsPositive() {
		return types.RejectBelowMinQty, "qty must be positive", false
	}
	if !p.cfg.MinOrderQty.IsZero() && cmd.Qty.LessThan(p.cfg.MinOrderQty) {
		return types.RejectBelowMinQty, "qty below minimum", false
	}
	if !p.cfg.MaxOrderQty.IsZero() && cmd.Qty.GreaterThan(p.cfg.MaxOrderQty) {
		return types.RejectAboveMaxQty, "qty above maximum", false
	}
	if !p.cfg.MaxOrderValue.IsZero() {
		// Compute raw notional at price precision: price_val * qty_val / 10^qty_prec.
		// Compare raw int64 values at pricePrecision vs maxOrderValue.
		scale := int64(1)
		for i := uint8(0); i < cmd.Qty.Precision(); i++ {
			scale *= 10
		}
		rawNotional := cmd.Price.Value() * cmd.Qty.Value() / scale
		if rawNotional > p.cfg.MaxOrderValue.Value() {
			return types.RejectAboveMaxValue, "order value above maximum", false
		}
	}
	if !cmd.Price.IsValidTick(p.cfg.TickSize) {
		return types.RejectInvalidTick, "price is not a valid tick", false
	}
	if !cmd.Qty.IsValidLot(p.cfg.LotSize) {
		return types.RejectInvalidLot, "qty is not a valid lot", false
	}
	if cmd.Flags.Has(types.FlagIceberg) {
		if !cmd.DisplayQty.IsPositive() {
			return types.RejectInvalidLot, "iceberg DisplayQty must be positive", false
		}
		if !cmd.DisplayQty.IsValidLot(p.cfg.LotSize) {
			return types.RejectInvalidLot, "iceberg DisplayQty is not a valid lot", false
		}
		if cmd.DisplayQty.GreaterThan(cmd.Qty) {
			return types.RejectInvalidLot, "iceberg DisplayQty exceeds total Qty", false
		}
	}
	return 0, "", true
}

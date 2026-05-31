package engine

import (
	"testing"

	"github.com/thorlaidanegg/clob/types"
)

func TestPositionTracker_InitiallyFlat(t *testing.T) {
	pt := newPositionTracker(0)
	pos := pt.get("alice")
	if !pos.IsZero() {
		t.Errorf("initial position = %s, want zero", pos)
	}
}

func TestPositionTracker_ApplyFill_BidMakerAskTaker(t *testing.T) {
	pt := newPositionTracker(0)
	f := types.Fill{
		MakerUserID: "alice",
		TakerUserID: "bob",
		MakerSide:   types.Bid, // alice is a bid maker (bought)
		Qty:         types.MustDecimal("5", 0),
	}
	pt.applyFill(f)

	// alice (bid maker) bought 5 → position +5
	if !pt.get("alice").Equal(types.MustDecimal("5", 0)) {
		t.Errorf("alice position = %s, want 5", pt.get("alice"))
	}
	// bob (ask taker) sold 5 → position -5
	if !pt.get("bob").Equal(types.MustDecimal("-5", 0)) {
		t.Errorf("bob position = %s, want -5", pt.get("bob"))
	}
}

func TestPositionTracker_ApplyFill_AskMakerBidTaker(t *testing.T) {
	pt := newPositionTracker(0)
	f := types.Fill{
		MakerUserID: "alice",
		TakerUserID: "bob",
		MakerSide:   types.Ask, // alice is an ask maker (sold)
		Qty:         types.MustDecimal("3", 0),
	}
	pt.applyFill(f)

	// alice (ask maker) sold 3 → position -3
	if !pt.get("alice").Equal(types.MustDecimal("-3", 0)) {
		t.Errorf("alice position = %s, want -3", pt.get("alice"))
	}
	// bob (bid taker) bought 3 → position +3
	if !pt.get("bob").Equal(types.MustDecimal("3", 0)) {
		t.Errorf("bob position = %s, want 3", pt.get("bob"))
	}
}

func TestPositionTracker_WouldIncrease(t *testing.T) {
	cases := []struct {
		name     string
		position string // signed decimal at precision 0
		side     types.Side
		qty      string
		want     bool
	}{
		// Flat user
		{"flat bid increases", "0", types.Bid, "1", true},
		{"flat ask increases", "0", types.Ask, "1", true},

		// Long user
		{"long bid increases", "5", types.Bid, "1", true},
		{"long ask exact reduces to flat", "5", types.Ask, "5", false},
		{"long ask over-reduces to short", "5", types.Ask, "6", true},
		{"long ask partial reduces", "5", types.Ask, "3", false},

		// Short user
		{"short ask increases", "-5", types.Ask, "1", true},
		{"short bid exact reduces to flat", "-5", types.Bid, "5", false},
		{"short bid over-reduces to long", "-5", types.Bid, "6", true},
		{"short bid partial reduces", "-5", types.Bid, "3", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pt := newPositionTracker(0)
			// Seed position by applying a synthetic fill.
			posVal := types.MustDecimal(tc.position, 0)
			if posVal.IsPositive() {
				// long: alice bought posVal
				pt.applyFill(types.Fill{
					MakerUserID: "other",
					TakerUserID: "alice",
					MakerSide:   types.Ask, // taker (alice) is bid → alice bought
					Qty:         posVal,
				})
			} else if posVal.IsNegative() {
				// short: alice sold abs(posVal)
				pt.applyFill(types.Fill{
					MakerUserID: "other",
					TakerUserID: "alice",
					MakerSide:   types.Bid, // taker (alice) is ask → alice sold
					Qty:         posVal.Abs(),
				})
			}

			got := pt.wouldIncrease("alice", tc.side, types.MustDecimal(tc.qty, 0))
			if got != tc.want {
				t.Errorf("wouldIncrease(pos=%s, side=%v, qty=%s) = %v, want %v",
					tc.position, tc.side, tc.qty, got, tc.want)
			}
		})
	}
}

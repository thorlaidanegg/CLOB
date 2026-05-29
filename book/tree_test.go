package book

import (
	"testing"

	"github.com/thorlaidanegg/clob/pool"
	"github.com/thorlaidanegg/clob/types"
)

func priceLevel(price string) *PriceLevel {
	return &PriceLevel{
		Price:      types.MustDecimal(price, 2),
		TotalQty:   types.Zero(0),
		DisplayQty: types.Zero(0),
	}
}

func TestTree_BidBest(t *testing.T) {
	tree := NewPriceLevelTree(types.Bid)
	tree.Insert(priceLevel("99.00"))
	tree.Insert(priceLevel("101.00"))
	tree.Insert(priceLevel("100.00"))

	best := tree.Best()
	if best == nil {
		t.Fatal("Best() returned nil")
	}
	if !best.Price.Equal(types.MustDecimal("101.00", 2)) {
		t.Errorf("Best bid = %s, want 101.00", best.Price)
	}
}

func TestTree_AskBest(t *testing.T) {
	tree := NewPriceLevelTree(types.Ask)
	tree.Insert(priceLevel("101.00"))
	tree.Insert(priceLevel("99.00"))
	tree.Insert(priceLevel("100.00"))

	best := tree.Best()
	if best == nil {
		t.Fatal("Best() returned nil")
	}
	if !best.Price.Equal(types.MustDecimal("99.00", 2)) {
		t.Errorf("Best ask = %s, want 99.00", best.Price)
	}
}

func TestTree_GetAndDelete(t *testing.T) {
	tree := NewPriceLevelTree(types.Ask)
	tree.Insert(priceLevel("100.00"))

	level, ok := tree.Get(types.MustDecimal("100.00", 2))
	if !ok || level == nil {
		t.Fatal("Get should find inserted level")
	}

	tree.Delete(types.MustDecimal("100.00", 2))
	if tree.Len() != 0 {
		t.Fatalf("Len = %d after delete, want 0", tree.Len())
	}
	if tree.Best() != nil {
		t.Error("Best() should be nil after deleting only level")
	}
}

func TestTree_GetOrCreate(t *testing.T) {
	tree := NewPriceLevelTree(types.Ask)
	p := pool.New[PriceLevel](10)

	level1, created := tree.GetOrCreate(types.MustDecimal("100.00", 2), p)
	if !created {
		t.Error("should have created new level")
	}
	level1.Price = types.MustDecimal("100.00", 2) // ensure set

	level2, created := tree.GetOrCreate(types.MustDecimal("100.00", 2), p)
	if created {
		t.Error("should not create duplicate level")
	}
	if level1 != level2 {
		t.Error("GetOrCreate should return same pointer on second call")
	}
}

func TestTree_Depth(t *testing.T) {
	tree := NewPriceLevelTree(types.Ask)
	for _, p := range []string{"103.00", "101.00", "102.00"} {
		tree.Insert(priceLevel(p))
	}

	depth := tree.Depth(2)
	if len(depth) != 2 {
		t.Fatalf("Depth(2) = %d levels, want 2", len(depth))
	}
	// Ask tree: ascend = 101, 102, 103. Top 2 = 101, 102.
	if !depth[0].Price.Equal(types.MustDecimal("101.00", 2)) {
		t.Errorf("depth[0] = %s, want 101.00", depth[0].Price)
	}
}

func TestTree_EmptyBest(t *testing.T) {
	tree := NewPriceLevelTree(types.Bid)
	if tree.Best() != nil {
		t.Error("Best() on empty tree should return nil")
	}
}

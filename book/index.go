package book

import "github.com/thorlaidanegg/clob/types"

// OrderIndex maps OrderID to resting OrderNode. O(1) lookup, O(n) expiry scan.
type OrderIndex struct {
	m map[types.OrderID]*OrderNode
}

// newOrderIndex creates an empty index.
func newOrderIndex() *OrderIndex {
	return &OrderIndex{m: make(map[types.OrderID]*OrderNode)}
}

// Put inserts or updates a node in the index.
func (idx *OrderIndex) Put(id types.OrderID, node *OrderNode) {
	idx.m[id] = node
}

// Get returns the node for id, or nil if not found.
func (idx *OrderIndex) Get(id types.OrderID) (*OrderNode, bool) {
	node, ok := idx.m[id]
	return node, ok
}

// Delete removes id from the index.
func (idx *OrderIndex) Delete(id types.OrderID) {
	delete(idx.m, id)
}

// Has returns true if id is in the index.
func (idx *OrderIndex) Has(id types.OrderID) bool {
	_, ok := idx.m[id]
	return ok
}

// Len returns the number of entries.
func (idx *OrderIndex) Len() int { return len(idx.m) }

// IterateExpired calls fn for every order whose ExpireAt <= now.
// O(n) over all open orders â€” acceptable for 1s expiry check interval.
func (idx *OrderIndex) IterateExpired(now int64, fn func(*OrderNode)) {
	for _, node := range idx.m {
		if node.ExpireAt > 0 && node.ExpireAt <= now {
			fn(node)
		}
	}
}

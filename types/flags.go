package types

// OrderFlags is a bitfield of per-order behavioral modifiers.
type OrderFlags uint32

const (
	FlagPostOnly   OrderFlags = 1 << 0 // reject if would take liquidity
	FlagReduceOnly OrderFlags = 1 << 1 // reject if would increase position
	FlagIceberg    OrderFlags = 1 << 2 // order has hidden quantity
)

// Has returns true if all bits in f are set in flags.
func (flags OrderFlags) Has(f OrderFlags) bool {
	return flags&f == f
}

// Set returns flags with all bits in f set.
func (flags OrderFlags) Set(f OrderFlags) OrderFlags {
	return flags | f
}

// Clear returns flags with all bits in f cleared.
func (flags OrderFlags) Clear(f OrderFlags) OrderFlags {
	return flags &^ f
}

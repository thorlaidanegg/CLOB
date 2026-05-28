package hooks

import "github.com/thorlaidanegg/clob/types"

// PostFillHook is an optional extension point called after each fill.
// The engine does not call this during the match loop â€” it is called by the
// processor after matching is complete, in the same goroutine.
// Implement only when needed; the zero value (nil) is always safe.
type PostFillHook interface {
	OnFill(fill types.Fill)
}

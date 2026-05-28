// Package pool provides a fixed-capacity, lock-free object pool.
// The pool is owned by a single goroutine exclusively — no mutex is used.
package pool

import "errors"

// ErrPoolExhausted is returned by Acquire when the pool has no available slots.
var ErrPoolExhausted = errors.New("clob/pool: pool exhausted")

// Pool is a generic fixed-capacity pool of pre-allocated objects.
// T must be a struct type. The caller is responsible for storing the slot
// index returned by Acquire and passing it back to Release.
//
// No mutex. Single-writer goroutine owns the pool exclusively.
type Pool[T any] struct {
	store    []T
	free     []int
	top      int
	capacity int
}

// New creates a Pool with the given capacity. All slots are pre-allocated
// and pushed onto the free stack in LIFO order (capacity-1 down to 0) for
// cache-friendly access.
func New[T any](capacity int) *Pool[T] {
	p := &Pool[T]{
		store:    make([]T, capacity),
		free:     make([]int, capacity),
		top:      capacity,
		capacity: capacity,
	}
	for i := 0; i < capacity; i++ {
		p.free[i] = capacity - 1 - i
	}
	return p
}

// Acquire returns a pointer to a zeroed slot and its slot index.
// The caller must store the index and pass it to Release when done.
// Returns ErrPoolExhausted if no slots are available.
func (p *Pool[T]) Acquire() (*T, int, error) {
	if p.top == 0 {
		return nil, 0, ErrPoolExhausted
	}
	p.top--
	idx := p.free[p.top]
	// Zero the slot.
	var zero T
	p.store[idx] = zero
	return &p.store[idx], idx, nil
}

// Release returns a slot to the pool by its index.
// The caller must not use the pointer after calling Release.
func (p *Pool[T]) Release(idx int) {
	p.free[p.top] = idx
	p.top++
}

// Len returns the number of currently acquired (in-use) slots.
func (p *Pool[T]) Len() int { return p.capacity - p.top }

// Capacity returns the total pool capacity.
func (p *Pool[T]) Capacity() int { return p.capacity }

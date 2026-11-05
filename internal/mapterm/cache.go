package mapterm

import (
	"container/list"
	"sync"
)

// Cache keeps decoded tiles in memory, dropping the least recently used when
// it holds more than its budget of points.
type Cache[T any] struct {
	mu     sync.Mutex
	budget int
	used   int
	order  *list.List
	items  map[TileID]*list.Element
}

type cached[T any] struct {
	id   TileID
	val  T
	cost int
}

// NewCache makes a cache with a cost budget.
func NewCache[T any](budget int) *Cache[T] {
	return &Cache[T]{budget: budget, order: list.New(), items: map[TileID]*list.Element{}}
}

// Get returns a tile and marks it recently used.
func (c *Cache[T]) Get(id TileID) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[id]
	if !ok {
		var zero T
		return zero, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*cached[T]).val, true
}

// Put stores a tile, evicting old ones over budget. The newest entry always
// stays, even if it alone exceeds the budget.
func (c *Cache[T]) Put(id TileID, val T, cost int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[id]; ok {
		c.used -= e.Value.(*cached[T]).cost
		c.order.Remove(e)
	}
	c.items[id] = c.order.PushFront(&cached[T]{id, val, cost})
	c.used += cost
	for c.used > c.budget && c.order.Len() > 1 {
		old := c.order.Back()
		v := old.Value.(*cached[T])
		c.order.Remove(old)
		delete(c.items, v.id)
		c.used -= v.cost
	}
}

// Len is the number of tiles held.
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

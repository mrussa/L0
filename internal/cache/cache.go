package cache

import (
	"sync"

	"github.com/mrussa/L0/internal/repo"
)

type OrdersCache struct {
	mu sync.RWMutex
	m  map[string]repo.Order
}

func New() *OrdersCache {
	return &OrdersCache{m: make(map[string]repo.Order, 256)}
}

func (c *OrdersCache) Get(uid string) (repo.Order, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	o, ok := c.m[uid]
	return o, ok
}

func (c *OrdersCache) Set(uid string, o repo.Order) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[uid] = o
}

func (c *OrdersCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}

func (c *OrdersCache) Delete(uid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, uid)
}

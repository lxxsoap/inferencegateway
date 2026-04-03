package router

import (
	"container/list"
	"sync"
)

// PrefixCache maps prompt prefixes to backend IDs with LRU eviction.
type PrefixCache interface {
	// Lookup finds the longest stored prefix that prompt starts with,
	// trying from len(prompt) down to minPrefixLen one byte at a time.
	// Returns found=false when no matching prefix exists.
	Lookup(prompt string) (backendID string, found bool)

	// Put records a prefix→backendID mapping.
	// Evicts the least-recently-used entry when the cache is at capacity.
	Put(prefix, backendID string)

	// Remove deletes the mapping for the given prefix.
	Remove(prefix string)
}

type lruEntry struct {
	key   string
	value string
}

type lruCache struct {
	// mu guards both items and order. Lookup also moves elements (write
	// operation on the list), so a plain Mutex is correct here — RWMutex
	// would not help because every cache hit is also a write.
	mu           sync.Mutex
	items        map[string]*list.Element
	order        *list.List
	maxSize      int
	minPrefixLen int
}

// NewPrefixCache creates a PrefixCache with the given capacity and minimum
// prefix length used during longest-prefix lookup.
func NewPrefixCache(maxSize, minPrefixLen int) PrefixCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if minPrefixLen <= 0 {
		minPrefixLen = 4
	}
	return &lruCache{
		items:        make(map[string]*list.Element, maxSize),
		order:        list.New(),
		maxSize:      maxSize,
		minPrefixLen: minPrefixLen,
	}
}

// Lookup searches for the longest prefix of prompt that has a cached mapping.
// It iterates from len(prompt) down to minPrefixLen (byte-based), and returns
// on the first match, promoting it to MRU position.
func (c *lruCache) Lookup(prompt string) (string, bool) {
	if len(prompt) < c.minPrefixLen {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for l := len(prompt); l >= c.minPrefixLen; l-- {
		if el, ok := c.items[prompt[:l]]; ok {
			c.order.MoveToFront(el)
			return el.Value.(*lruEntry).value, true
		}
	}
	return "", false
}

// Put stores or updates the prefix→backendID mapping.
// If the cache is full, the least-recently-used entry is evicted first.
func (c *lruCache) Put(prefix, backendID string) {
	if prefix == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[prefix]; ok {
		el.Value.(*lruEntry).value = backendID
		c.order.MoveToFront(el)
		return
	}
	if c.order.Len() >= c.maxSize {
		c.evictLocked()
	}
	el := c.order.PushFront(&lruEntry{key: prefix, value: backendID})
	c.items[prefix] = el
}

// Remove deletes the mapping for the given prefix.
func (c *lruCache) Remove(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[prefix]; ok {
		c.order.Remove(el)
		delete(c.items, prefix)
	}
}

// evictLocked removes the least-recently-used entry. Caller must hold mu.
func (c *lruCache) evictLocked() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	entry := oldest.Value.(*lruEntry)
	delete(c.items, entry.key)
	c.order.Remove(oldest)
}

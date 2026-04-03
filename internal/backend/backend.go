package backend

import "sync/atomic"

// Backend represents a single backend inference service instance.
// Atomic fields are safe for concurrent access without a mutex.
type Backend struct {
	ID             string
	Address        string
	MaxConcurrency int

	concurrency atomic.Int32
	healthy     atomic.Bool
}

// New creates a new Backend and marks it healthy by default.
func New(id, address string, maxConcurrency int) *Backend {
	b := &Backend{
		ID:             id,
		Address:        address,
		MaxConcurrency: maxConcurrency,
	}
	b.healthy.Store(true)
	return b
}

// IsHealthy reports whether this backend is currently healthy.
func (b *Backend) IsHealthy() bool { return b.healthy.Load() }

// SetHealthy updates the health state of this backend.
func (b *Backend) SetHealthy(v bool) { b.healthy.Store(v) }

// ActiveConcurrency returns the current number of in-flight requests.
func (b *Backend) ActiveConcurrency() int32 { return b.concurrency.Load() }

// LoadRatio returns active / max concurrency (0.0 - 1.0+).
func (b *Backend) LoadRatio() float64 {
	return float64(b.concurrency.Load()) / float64(b.MaxConcurrency)
}

// tryAcquire atomically increments the concurrency counter if the backend is
// not yet at capacity. Returns false when the backend is full.
func (b *Backend) tryAcquire() bool {
	for {
		cur := b.concurrency.Load()
		if cur >= int32(b.MaxConcurrency) {
			return false
		}
		if b.concurrency.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

// release decrements the concurrency counter by one.
func (b *Backend) release() {
	b.concurrency.Add(-1)
}

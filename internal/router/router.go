package router

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/inferencegateway/internal/backend"
	"github.com/inferencegateway/internal/config"
)

// InferRequest is the inbound request to the gateway.
// Defined here so both the server handler and router implementations share the
// same type without creating a circular import.
type InferRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
}

// ErrNoBackend is returned when no healthy, available backend can be found.
var ErrNoBackend = errors.New("no healthy backend available")

// Router selects a backend for each incoming inference request.
type Router interface {
	// Route returns the chosen backend and whether a prefix-cache hit occurred.
	Route(ctx context.Context, req *InferRequest) (b *backend.Backend, cacheHit bool, err error)
}

// RoundRobinRouter distributes requests evenly across healthy backends.
// It serves as the baseline strategy (T2.3) before smarter strategies are
// added in T3.x.
type RoundRobinRouter struct {
	mgr     backend.Manager
	counter atomic.Uint64
}

// NewRoundRobinRouter creates a RoundRobinRouter backed by the given manager.
func NewRoundRobinRouter(mgr backend.Manager) *RoundRobinRouter {
	return &RoundRobinRouter{mgr: mgr}
}

// Route picks the next healthy backend in round-robin order.
func (r *RoundRobinRouter) Route(_ context.Context, _ *InferRequest) (*backend.Backend, bool, error) {
	backends := r.mgr.HealthyBackends()
	if len(backends) == 0 {
		return nil, false, ErrNoBackend
	}
	idx := r.counter.Add(1) - 1
	return backends[idx%uint64(len(backends))], false, nil
}

// PrefixRouter routes requests with the same prompt prefix to the same backend,
// exploiting KV-cache reuse. When no cached mapping exists, or the preferred
// backend is unhealthy/full, it falls back to WLC and updates the mapping.
type PrefixRouter struct {
	mgr          backend.Manager
	cache        PrefixCache
	minPrefixLen int
}

// NewPrefixRouter creates a PrefixRouter. minPrefixLen is the minimum number of
// Unicode code points (runes) to use as a cache key.
func NewPrefixRouter(mgr backend.Manager, cache PrefixCache, minPrefixLen int) *PrefixRouter {
	if minPrefixLen <= 0 {
		minPrefixLen = 4
	}
	return &PrefixRouter{mgr: mgr, cache: cache, minPrefixLen: minPrefixLen}
}

// Route implements Router.
func (r *PrefixRouter) Route(_ context.Context, req *InferRequest) (*backend.Backend, bool, error) {
	if backendID, found := r.cache.Lookup(req.Prompt); found {
		if b := r.mgr.GetBackend(backendID); b != nil && b.LoadRatio() < 1.0 {
			// Cache hit: preferred backend is healthy and has capacity.
			return b, true, nil
		}
		// Preferred backend gone or full — remove stale mapping and fall through.
		r.cache.Remove(r.extractPrefix(req.Prompt))
	}

	// Fall back to WLC.
	b := leastLoaded(r.mgr.HealthyBackends())
	if b == nil {
		return nil, false, ErrNoBackend
	}
	r.cache.Put(r.extractPrefix(req.Prompt), b.ID)
	return b, false, nil
}

// extractPrefix returns the first r.minPrefixLen Unicode code points of s,
// or the whole string if it is shorter.
func (r *PrefixRouter) extractPrefix(s string) string {
	return extractRunePrefix(s, r.minPrefixLen)
}

// extractRunePrefix is a package-level helper used by multiple router types.
func extractRunePrefix(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s // shorter than n runes — use the whole string
}

// HybridRouter combines prefix-cache affinity with load-aware fallback.
//
// Decision logic per request:
//  1. Look up prompt's prefix in the cache.
//  2. If found and the preferred backend is healthy with load < threshold → use it (cache hit).
//  3. Otherwise fall back to WLC, update the cache mapping.
//
// The threshold (loadThresholdPercent, 0.0–1.0) controls the trade-off:
//   - higher → more cache affinity, higher hotspot risk
//   - lower  → more load spreading, lower cache reuse
type HybridRouter struct {
	mgr                  backend.Manager
	cache                PrefixCache
	minPrefixLen         int
	loadThresholdPercent float64
}

// NewHybridRouter creates a HybridRouter.
func NewHybridRouter(mgr backend.Manager, cache PrefixCache, minPrefixLen int, loadThresholdPercent float64) *HybridRouter {
	if minPrefixLen <= 0 {
		minPrefixLen = 4
	}
	if loadThresholdPercent <= 0 || loadThresholdPercent > 1.0 {
		loadThresholdPercent = 0.8
	}
	return &HybridRouter{
		mgr:                  mgr,
		cache:                cache,
		minPrefixLen:         minPrefixLen,
		loadThresholdPercent: loadThresholdPercent,
	}
}

// Route implements Router.
func (r *HybridRouter) Route(_ context.Context, req *InferRequest) (*backend.Backend, bool, error) {
	prefix := extractRunePrefix(req.Prompt, r.minPrefixLen)

	if backendID, found := r.cache.Lookup(req.Prompt); found {
		if b := r.mgr.GetBackend(backendID); b != nil && b.LoadRatio() < r.loadThresholdPercent {
			// Cache hit: preferred backend is healthy and below load threshold.
			return b, true, nil
		}
		// Preferred backend gone, unhealthy, or over threshold — evict stale entry.
		r.cache.Remove(prefix)
	}

	// Fall back to WLC and record the new mapping.
	b := leastLoaded(r.mgr.HealthyBackends())
	if b == nil {
		return nil, false, ErrNoBackend
	}
	r.cache.Put(prefix, b.ID)
	return b, false, nil
}

// New builds a Router from the given configuration.
// strategy must be one of "prefix", "load", or "hybrid".
func New(cfg config.RouterConfig, mgr backend.Manager) (Router, error) {
	switch cfg.Strategy {
	case "prefix":
		cache := NewPrefixCache(cfg.PrefixCacheMaxSize, cfg.PrefixMinLength)
		return NewPrefixRouter(mgr, cache, cfg.PrefixMinLength), nil
	case "load":
		return NewWLCRouter(mgr), nil
	case "hybrid":
		cache := NewPrefixCache(cfg.PrefixCacheMaxSize, cfg.PrefixMinLength)
		return NewHybridRouter(mgr, cache, cfg.PrefixMinLength, cfg.LoadThresholdPercent), nil
	default:
		return nil, fmt.Errorf("unknown router strategy: %q", cfg.Strategy)
	}
}

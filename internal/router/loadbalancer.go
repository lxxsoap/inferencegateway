package router

import (
	"context"
	"math"

	"github.com/inferencegateway/internal/backend"
)

// leastLoaded returns the healthy backend with the lowest load ratio
// (activeConcurrency / maxConcurrency). Fully-saturated backends are skipped.
// Returns nil if all backends are unhealthy or at capacity.
func leastLoaded(backends []*backend.Backend) *backend.Backend {
	var best *backend.Backend
	bestRatio := math.MaxFloat64
	for _, b := range backends {
		if !b.IsHealthy() {
			continue
		}
		ratio := b.LoadRatio()
		if ratio >= 1.0 {
			// At or over capacity — skip.
			continue
		}
		if ratio < bestRatio {
			bestRatio = ratio
			best = b
		}
	}
	return best
}

// WLCRouter implements Router using Weighted Least Connections — it always
// sends the next request to the healthy backend with the lowest load ratio.
type WLCRouter struct {
	mgr backend.Manager
}

// NewWLCRouter creates a WLCRouter backed by the given manager.
func NewWLCRouter(mgr backend.Manager) *WLCRouter {
	return &WLCRouter{mgr: mgr}
}

// Route selects the least-loaded healthy backend.
func (r *WLCRouter) Route(_ context.Context, _ *InferRequest) (*backend.Backend, bool, error) {
	b := leastLoaded(r.mgr.HealthyBackends())
	if b == nil {
		return nil, false, ErrNoBackend
	}
	return b, false, nil
}

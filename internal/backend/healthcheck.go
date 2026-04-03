package backend

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// HealthChecker probes each backend's /health endpoint on a fixed interval
// and updates Backend.healthy accordingly.
type HealthChecker struct {
	backends []*Backend
	interval time.Duration
	client   *http.Client
}

// NewHealthChecker creates a HealthChecker. interval is how often to probe;
// timeout is the per-probe HTTP deadline.
func NewHealthChecker(backends []*Backend, interval, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		backends: backends,
		interval: interval,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Start launches the probe loop in a goroutine. It returns immediately and
// runs until ctx is cancelled.
func (hc *HealthChecker) Start(ctx context.Context) {
	go hc.run(ctx)
}

func (hc *HealthChecker) run(ctx context.Context) {
	// Run an initial probe before the first tick so we detect down backends
	// before serving any traffic.
	hc.probeAll(ctx)

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.probeAll(ctx)
		}
	}
}

// probeAll checks every backend once. Each probe runs in its own goroutine so
// slow backends do not delay healthy ones.
func (hc *HealthChecker) probeAll(ctx context.Context) {
	for _, b := range hc.backends {
		go hc.probe(ctx, b)
	}
}

// probe performs a single GET /health against the backend and updates its
// health state.
func (hc *HealthChecker) probe(ctx context.Context, b *Backend) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.Address+"/health", nil)
	if err != nil {
		hc.markUnhealthy(b, err)
		return
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		hc.markUnhealthy(b, err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		hc.markUnhealthy(b, nil)
		slog.Warn("health check failed", "backend_id", b.ID, "status", resp.StatusCode)
		return
	}

	// Backend is healthy.
	if !b.IsHealthy() {
		slog.Info("backend recovered", "backend_id", b.ID)
	}
	b.SetHealthy(true)
}

func (hc *HealthChecker) markUnhealthy(b *Backend, err error) {
	if b.IsHealthy() {
		if err != nil {
			slog.Warn("backend unhealthy", "backend_id", b.ID, "error", err)
		} else {
			slog.Warn("backend unhealthy", "backend_id", b.ID)
		}
	}
	b.SetHealthy(false)
}

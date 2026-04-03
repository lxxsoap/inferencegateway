package backend

import (
	"context"
	"time"

	"github.com/inferencegateway/internal/config"
)

// Manager manages all backend instances and their concurrency slots.
type Manager interface {
	// GetBackend returns the backend with the given ID, or nil if it does not
	// exist or is currently unhealthy.
	GetBackend(id string) *Backend

	// HealthyBackends returns a snapshot of all currently healthy backends.
	HealthyBackends() []*Backend

	// AcquireSlot attempts to claim one concurrency slot on the named backend.
	// Returns false if the backend is unknown or already at max concurrency.
	AcquireSlot(backendID string) bool

	// ReleaseSlot returns one concurrency slot to the named backend.
	ReleaseSlot(backendID string)

	// Start begins background tasks (e.g. health checking). It returns
	// immediately; the tasks run until ctx is cancelled.
	Start(ctx context.Context)
}

type manager struct {
	backends       []*Backend
	byID           map[string]*Backend
	healthInterval time.Duration
	healthTimeout  time.Duration
}

// NewManager constructs a Manager from the provided backend configurations.
// All backends start healthy; health checking begins when Start is called.
func NewManager(cfgs []config.BackendConfig, hcCfg config.HealthCheckConfig) Manager {
	m := &manager{
		backends:       make([]*Backend, 0, len(cfgs)),
		byID:           make(map[string]*Backend, len(cfgs)),
		healthInterval: time.Duration(hcCfg.IntervalSeconds) * time.Second,
		healthTimeout:  time.Duration(hcCfg.TimeoutSeconds) * time.Second,
	}
	for _, cfg := range cfgs {
		b := New(cfg.ID, cfg.Address, cfg.MaxConcurrency)
		m.backends = append(m.backends, b)
		m.byID[cfg.ID] = b
	}
	return m
}

func (m *manager) GetBackend(id string) *Backend {
	b, ok := m.byID[id]
	if !ok || !b.IsHealthy() {
		return nil
	}
	return b
}

func (m *manager) HealthyBackends() []*Backend {
	out := make([]*Backend, 0, len(m.backends))
	for _, b := range m.backends {
		if b.IsHealthy() {
			out = append(out, b)
		}
	}
	return out
}

func (m *manager) AcquireSlot(backendID string) bool {
	b, ok := m.byID[backendID]
	if !ok {
		return false
	}
	return b.tryAcquire()
}

func (m *manager) ReleaseSlot(backendID string) {
	b, ok := m.byID[backendID]
	if !ok {
		return
	}
	b.release()
}

// Start launches the health-check loop that probes all backends on a fixed
// interval. It returns immediately; the loop runs until ctx is cancelled.
func (m *manager) Start(ctx context.Context) {
	hc := NewHealthChecker(m.backends, m.healthInterval, m.healthTimeout)
	hc.Start(ctx)
}

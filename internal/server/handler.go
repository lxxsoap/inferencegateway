package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/inferencegateway/internal/backend"
	"github.com/inferencegateway/internal/router"
)

// InferResponse is the response body from POST /infer.
type InferResponse struct {
	RequestID string `json:"request_id"`
	BackendID string `json:"backend_id"`
	Result    string `json:"result"`
	LatencyMs int64  `json:"latency_ms"`
	CacheHit  bool   `json:"cache_hit"`
}

// Handler holds the dependencies needed to serve gateway requests.
type Handler struct {
	mgr        backend.Manager
	router     router.Router
	httpClient *http.Client
}

// NewHandler creates a Handler backed by the given Manager and Router.
func NewHandler(mgr backend.Manager, r router.Router) *Handler {
	return &Handler{
		mgr:    mgr,
		router: r,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ServeHTTP routes incoming requests to the appropriate handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/infer":
		h.handleInfer(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		h.handleHealth(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleInfer(w http.ResponseWriter, r *http.Request) {
	var req router.InferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	defer r.Body.Close()

	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	b, cacheHit, err := h.router.Route(r.Context(), &req)
	if err != nil {
		if errors.Is(err, router.ErrNoBackend) {
			writeError(w, http.StatusServiceUnavailable, "no healthy backends available")
		} else {
			writeError(w, http.StatusInternalServerError, "routing error")
		}
		return
	}

	if !h.mgr.AcquireSlot(b.ID) {
		writeError(w, http.StatusServiceUnavailable, "backend at capacity")
		return
	}
	defer h.mgr.ReleaseSlot(b.ID)

	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode request")
		return
	}

	backendReq, err := http.NewRequestWithContext(
		r.Context(), http.MethodPost, b.Address+"/infer", bytes.NewReader(body),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backend request")
		return
	}
	backendReq.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(backendReq)
	if err != nil {
		slog.Error("backend request failed", "backend_id", b.ID, "error", err)
		writeError(w, http.StatusBadGateway, "backend request failed")
		return
	}
	defer resp.Body.Close()

	var inferResp InferResponse
	if err := json.NewDecoder(resp.Body).Decode(&inferResp); err != nil {
		writeError(w, http.StatusBadGateway, "failed to decode backend response")
		return
	}

	inferResp.CacheHit = cacheHit

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(inferResp); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	healthy := h.mgr.HealthyBackends()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","healthy_backends":%d}`, len(healthy))
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

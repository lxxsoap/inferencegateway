package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type inferRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
}

type inferResponse struct {
	RequestID string `json:"request_id"`
	BackendID string `json:"backend_id"`
	Result    string `json:"result"`
	LatencyMs int64  `json:"latency_ms"`
	CacheHit  bool   `json:"cache_hit"`
}

func main() {
	addr := flag.String("addr", ":8081", "listen address, e.g. :8081")
	id := flag.String("id", "backend-1", "backend id used in responses")
	minDelay := flag.Int("min-delay", 100, "minimum simulated inference delay in ms")
	maxDelay := flag.Int("max-delay", 500, "maximum simulated inference delay in ms")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(*id))
	mux.HandleFunc("POST /infer", handleInfer(*id, *minDelay, *maxDelay))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("mock backend starting", "id", *id, "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down", "id", *id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func handleHealth(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","id":%q}`, id)
	}
}

func handleInfer(id string, minDelay, maxDelay int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req inferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if req.Prompt == "" {
			http.Error(w, `{"error":"prompt is required"}`, http.StatusBadRequest)
			return
		}

		// Simulate inference latency.
		delayRange := maxDelay - minDelay
		delay := minDelay
		if delayRange > 0 {
			delay += rand.Intn(delayRange)
		}
		start := time.Now()
		time.Sleep(time.Duration(delay) * time.Millisecond)
		latency := time.Since(start).Milliseconds()

		resp := inferResponse{
			RequestID: fmt.Sprintf("%s-%d", id, time.Now().UnixNano()),
			BackendID: id,
			Result:    fmt.Sprintf("[%s] response to: %s", id, req.Prompt),
			LatencyMs: latency,
			CacheHit:  false,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}
}

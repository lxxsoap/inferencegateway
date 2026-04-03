package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/inferencegateway/internal/backend"
	"github.com/inferencegateway/internal/config"
	"github.com/inferencegateway/internal/router"
	"github.com/inferencegateway/internal/server"
)

func main() {
	configPath := flag.String("config", "configs/gateway.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("config loaded",
		"listen_addr", cfg.ListenAddr,
		"backends", len(cfg.Backends),
		"strategy", cfg.Router.Strategy,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := backend.NewManager(cfg.Backends, cfg.HealthCheck)
	mgr.Start(ctx)

	rtr, err := router.New(cfg.Router, mgr)
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}
	handler := server.NewHandler(mgr, rtr)
	srv := server.New(cfg.ListenAddr, handler)

	go func() {
		slog.Info("gateway starting", "addr", cfg.ListenAddr)
		if err := srv.Start(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gateway")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

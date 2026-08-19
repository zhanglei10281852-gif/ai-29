package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zhanglei10281852-gif/ai/internal/clock"
	"github.com/zhanglei10281852-gif/ai/internal/config"
	"github.com/zhanglei10281852-gif/ai/internal/httpapi"
	"github.com/zhanglei10281852-gif/ai/internal/service"
	"github.com/zhanglei10281852-gif/ai/internal/storage/sqlite"
	"github.com/zhanglei10281852-gif/ai/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	store, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	services := service.New(store, clock.Real{}, cfg.SessionTTL, cfg.ApprovalTaskTTL)
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.New(services, store, logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	workerCtx, stopWorker := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stopWorker()
	w := worker.New(store, clock.Real{}, cfg.WorkerInterval, cfg.WorkerSnapshotSize, logger)
	workerDone := make(chan error, 1)
	go func() { workerDone <- w.Run(workerCtx) }()
	serverErr := make(chan error, 1)
	go func() { serverErr <- httpServer.ListenAndServe() }()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-workerCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-workerDone; err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

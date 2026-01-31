package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bnema/sharm/config"
	"github.com/bnema/sharm/internal/adapter/converter/ffmpeg"
	HTTPAdapter "github.com/bnema/sharm/internal/adapter/http"
	sqlitestore "github.com/bnema/sharm/internal/adapter/storage/sqlite"
	"github.com/bnema/sharm/internal/infrastructure/logger"
	"github.com/bnema/sharm/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.Error.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	logger.Info.Printf("starting sharm on port %d, domain=%s", cfg.Port, cfg.Domain)

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		logger.Error.Printf("failed to create data directory: %v", err)
		os.Exit(1)
	}

	store, err := sqlitestore.NewStore(cfg.DataDir)
	if err != nil {
		logger.Error.Printf("failed to create store: %v", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	converter := ffmpeg.NewConverter()
	jobQueue := sqlitestore.NewJobQueue(store)
	eventBus := service.NewEventBus()

	mediaSvc := service.NewMediaService(store, converter, jobQueue, cfg.DataDir)
	authSvc := service.NewAuthService(cfg.AuthSecret)

	// Worker pool for async jobs (conversion, thumbnails)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	workerPool := service.NewWorkerPool(jobQueue, store, converter, eventBus, cfg.DataDir, 2)
	workerPool.Start(workerCtx)

	server := HTTPAdapter.NewServer(authSvc, mediaSvc, eventBus, cfg.Domain, cfg.MaxUploadSizeMB)

	// Periodic cleanup of expired media
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := mediaSvc.Cleanup(); err != nil {
					logger.Error.Printf("cleanup failed: %v", err)
				}
			case <-workerCtx.Done():
				return
			}
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logger.Info.Printf("received %s, shutting down", sig)

		// Stop accepting new requests
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error.Printf("http shutdown error: %v", err)
		}

		// Stop workers (lets in-flight jobs finish)
		workerCancel()

		logger.Info.Printf("shutdown complete")
	}()

	logger.Info.Printf("server listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error.Printf("server failed: %v", err)
		os.Exit(1)
	}
}

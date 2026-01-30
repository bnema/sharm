package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bnema/sharm/config"
	"github.com/bnema/sharm/internal/adapter/converter/ffmpeg"
	HTTPAdapter "github.com/bnema/sharm/internal/adapter/http"
	"github.com/bnema/sharm/internal/adapter/storage/jsonfile"
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

	store, err := jsonfile.NewStore(cfg.DataDir)
	if err != nil {
		logger.Error.Printf("failed to create store: %v", err)
		os.Exit(1)
	}

	converter := ffmpeg.NewConverter()

	mediaSvc := service.NewMediaService(store, converter, cfg.DataDir)

	authSvc := service.NewAuthService(cfg.AuthSecret)

	server := HTTPAdapter.NewServer(authSvc, mediaSvc, cfg.Domain, cfg.MaxUploadSizeMB)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := mediaSvc.Cleanup(); err != nil {
				logger.Error.Printf("cleanup failed: %v", err)
			}
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info.Printf("server listening on %s", addr)

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info.Printf("shutdown signal received")
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, server); err != nil {
		logger.Error.Printf("server failed: %v", err)
		os.Exit(1)
	}
}

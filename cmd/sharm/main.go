package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bnema/sharm/config"
	"github.com/bnema/sharm/internal/adapter/converter/ffmpeg"
	HTTPAdapter "github.com/bnema/sharm/internal/adapter/http"
	"github.com/bnema/sharm/internal/adapter/storage/jsonfile"
	"github.com/bnema/sharm/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	store, err := jsonfile.NewStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	converter := ffmpeg.NewConverter()

	mediaSvc := service.NewMediaService(store, converter, cfg.DataDir)

	authSvc := service.NewAuthService(cfg.AuthSecret)

	server := HTTPAdapter.NewServer(authSvc, mediaSvc, cfg.Domain, cfg.MaxUploadSizeMB)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			if err := mediaSvc.Cleanup(); err != nil {
				log.Printf("Cleanup failed: %v", err)
			}
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

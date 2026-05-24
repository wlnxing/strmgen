package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"strm/internal/config"
	"strm/internal/db"
	"strm/internal/httpapi"
	"strm/internal/scanner"
	"strm/internal/scheduler"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatal(err)
	}
	store, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	if err := store.UpsertAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatal(err)
	}

	loc, err := time.LoadLocation(cfg.CronTZ)
	if err != nil {
		log.Printf("invalid STRM_CRON_TZ %q, using local time: %v", cfg.CronTZ, err)
		loc = time.Local
	}

	scannerSvc := &scanner.Service{Store: store, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
	manager := scheduler.New(store, scannerSvc, loc)
	if err := manager.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer manager.Stop()

	api := httpapi.New(store, manager)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("strm api listening on %s", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}
}

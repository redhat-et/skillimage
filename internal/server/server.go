package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

// Config holds settings for the catalog server.
type Config struct {
	Port          int
	DBPath        string
	RegistryURL   string
	Namespace     string
	Repositories  []string
	SkipTLSVerify bool
	SyncInterval  time.Duration
}

// Run starts the catalog server and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	db, err := store.New(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer func() { _ = db.Close() }()

	syncCfg := store.SyncConfig{
		RegistryURL:   cfg.RegistryURL,
		Namespace:     cfg.Namespace,
		Repositories:  cfg.Repositories,
		SkipTLSVerify: cfg.SkipTLSVerify,
	}

	slog.Info("running initial sync", "registry", cfg.RegistryURL)
	if err := db.Sync(ctx, syncCfg); err != nil {
		slog.Error("initial sync failed", "error", err)
	}

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	triggerSync := func() {
		slog.Info("sync triggered")
		if err := db.Sync(syncCtx, syncCfg); err != nil {
			slog.Error("sync failed", "error", err)
		}
		slog.Info("sync complete")
	}

	go func() {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				triggerSync()
			}
		}
	}()

	contentCfg := handler.ContentConfig{
		RegistryURL:   cfg.RegistryURL,
		SkipTLSVerify: cfg.SkipTLSVerify,
	}
	router := NewRouter(db, triggerSync, contentCfg)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

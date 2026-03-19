// Digital Museum — Go server entry point.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on DefaultServeMux
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daveontour/digitalmuseum/internal/api/router"
	"github.com/daveontour/digitalmuseum/internal/config"
	"github.com/daveontour/digitalmuseum/internal/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// ── Structured logging ─────────────────────────────────────────────────────
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Configuration ──────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── Database ───────────────────────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := database.EnsureDatabase(ctx, cfg.DB); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}

	db, err := database.New(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migrateCancel()

	if err := database.Migrate(migrateCtx, db.Pool); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	if err := database.SeedEmailExclusionsFromJSON(migrateCtx, db.Pool, "static/data/exclusions.json"); err != nil {
		return fmt.Errorf("seed email exclusions: %w", err)
	}
	if err := database.SeedEmailMatchesFromJSON(migrateCtx, db.Pool, "static/data/email_matches.json"); err != nil {
		return fmt.Errorf("seed email matches: %w", err)
	}
	if err := database.SeedEmailClassificationsFromJSON(migrateCtx, db.Pool, "static/data/email_classifications.json"); err != nil {
		return fmt.Errorf("seed email classifications: %w", err)
	}

	// ── HTTP server ────────────────────────────────────────────────────────────
	handler := router.New(db.Pool, cfg)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // longer for SSE / streaming endpoints
		IdleTimeout:  120 * time.Second,
	}

	// ── Optional pprof debug server ───────────────────────────────────────────
	// Set ENABLE_PPROF=true to expose Go profiling endpoints on :6060/debug/pprof
	if os.Getenv("ENABLE_PPROF") == "true" {
		go func() {
			pprofAddr := ":6060"
			slog.Info("pprof server starting", "addr", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				slog.Warn("pprof server stopped", "err", err)
			}
		}()
	}

	// ── Graceful shutdown ──────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("server stopped cleanly")
	return nil
}

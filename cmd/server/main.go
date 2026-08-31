// Digital Museum — Go server entry point.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on DefaultServeMux
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/daveontour/aimuseum/internal/api/router"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/georegion"
	"github.com/daveontour/aimuseum/internal/repository"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// logSQLitePaths logs configured, absolute, cwd-relative paths and whether each file exists (before open).
func logSQLitePaths(phase string, mainPath, billingPath string) {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	execPath := ""
	if exe, err2 := os.Executable(); err2 == nil {
		execPath = exe
	}
	logOne := func(name, p string) {
		clean := filepath.Clean(p)
		abs := clean
		if a, errA := filepath.Abs(clean); errA == nil {
			abs = a
		}
		relCwd := ""
		if r, errR := filepath.Rel(wd, abs); errR == nil {
			relCwd = r
		} else {
			relCwd = "(cannot relate to go_cwd: " + errR.Error() + ")"
		}
		exists := false
		var size int64
		if st, errS := os.Stat(abs); errS == nil {
			exists = true
			size = st.Size()
		}
		slog.Info("sqlite path trace",
			"phase", phase,
			"db", name,
			"go_cwd", wd,
			"go_executable", execPath,
			"path_from_env", p,
			"path_clean", clean,
			"path_absolute", abs,
			"path_relative_to_go_cwd", relCwd,
			"file_exists_on_disk", exists,
			"file_size_bytes", size,
		)
	}
	logOne("main", mainPath)
	logOne("billing", billingPath)
}

func run() error {
	// ── Structured logging ─────────────────────────────────────────────────────
	logLevel := slog.LevelWarn
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		if err := logLevel.UnmarshalText([]byte(v)); err != nil {
			logLevel = slog.LevelWarn
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// ── Configuration ──────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// ── Database ───────────────────────────────────────────────────────────────
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	billingCfg := cfg.DB.BillingConfig()

	logSQLitePaths("before_open", cfg.DB.SQLitePath, billingCfg.BillingSQLitePath)

	billingDB, err := database.NewBilling(ctx, billingCfg)
	if err != nil {
		return fmt.Errorf("connect to billing database: %w", err)
	}
	defer func() { _ = billingDB.Close() }()

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer migrateCancel()

	if err := database.MigrateBilling(migrateCtx, billingDB.Std); err != nil {
		return fmt.Errorf("run billing migrations: %w", err)
	}

	profileRepo := repository.NewProfileRepo(billingDB.Std)
	cfg.DB.SQLitePath = repository.ResolveMainSQLiteStartupPath(migrateCtx, profileRepo)
	logSQLitePaths("after_startup_resolve", cfg.DB.SQLitePath, billingCfg.BillingSQLitePath)

	db, err := database.New(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if db != nil {
		defer func() { _ = db.Close() }()
	}

	logSQLitePaths("after_open_ping_ok", cfg.DB.SQLitePath, billingCfg.BillingSQLitePath)

	if db != nil {
		if err := database.MigrateSQLite(migrateCtx, db.Std); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	if db != nil {
		userRepo := repository.NewUserRepo(db.Std)
		if n, err := userRepo.DeleteAllSessions(migrateCtx); err != nil {
			return fmt.Errorf("clear sessions on startup: %w", err)
		} else if n > 0 {
			slog.Info("cleared auth sessions after restart", "deleted", n)
		}

		if err := database.SeedAppSystemInstructionsFromFiles(migrateCtx, db.Std, "static"); err != nil {
			return fmt.Errorf("seed app system instructions: %w", err)
		}
		if err := database.SeedRegionsFromFileIfMissing(migrateCtx, db.Std, cfg.App.RegionsConfigFile()); err != nil {
			return fmt.Errorf("seed regions: %w", err)
		}
		if err := georegion.ReloadFromDB(migrateCtx, db.Std); err != nil {
			return fmt.Errorf("load regions from db: %w", err)
		}
		if err := database.SeedSuggestionsFromFileIfMissing(migrateCtx, db.Std, cfg.App.SuggestionsConfigFile()); err != nil {
			return fmt.Errorf("seed suggestions: %w", err)
		}
		if cfg.App.GuideTopicsReloadFromFileOnStartup {
			if err := database.ReloadGuideTopicsFromFile(migrateCtx, db.Std, cfg.App.GuideTopicsConfigFile()); err != nil {
				return fmt.Errorf("reload guide topics from file: %w", err)
			}
		} else if err := database.SeedGuideTopicsFromFileIfMissing(migrateCtx, db.Std, cfg.App.GuideTopicsConfigFile()); err != nil {
			return fmt.Errorf("seed guide topics: %w", err)
		}
		if err := database.SeedAIModelsFromFileIfMissing(migrateCtx, db.Std, cfg.App.AIModelsConfigFile()); err != nil {
			return fmt.Errorf("seed ai models: %w", err)
		}
	}

	// ── HTTP server ────────────────────────────────────────────────────────────
	var mainPool *sql.DB
	if db != nil {
		mainPool = db.Std
	}
	handler, bgJobsScheduler, err := router.New(mainPool, billingDB.Std, cfg)
	if err != nil {
		return fmt.Errorf("router: %w", err)
	}

	// ── Background jobs scheduler ─────────────────────────────────────────────
	// Runs in a goroutine until the root context is cancelled at shutdown.
	bgJobsCtx, bgJobsCancel := context.WithCancel(context.Background())
	defer bgJobsCancel()
	if bgJobsScheduler != nil {
		go bgJobsScheduler.Run(bgJobsCtx)
	}

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
		tlsOn := cfg.Server.TLSCertFile != "" && cfg.Server.TLSKeyFile != ""
		slog.Info("server starting", "addr", srv.Addr, "tls", tlsOn)
		var err error
		if tlsOn {
			err = srv.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutdown signal received")

	bgJobsCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("server stopped cleanly")
	return nil
}

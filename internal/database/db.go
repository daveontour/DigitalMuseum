// Package database provides PostgreSQL connection pool management.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/daveontour/digitalmuseum/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// New creates and validates a new database connection pool.
//
// Pool settings mirror the Python SQLAlchemy configuration:
//   - pool_size=10, max_overflow=20  → MaxConns=30
//   - pool_recycle=3600              → MaxConnLifetime=1h
//   - pool_timeout=30               → AcquireTimeout=30s
//   - UTF-8 client encoding         → search_path/client_encoding via connect options
func New(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	// Match Python pool settings
	poolCfg.MaxConns = 30 // pool_size(10) + max_overflow(20)
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 10 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	// Force UTF-8 — mirrors Python connect_args={"options": "-c client_encoding=UTF8"}
	poolCfg.ConnConfig.RuntimeParams["client_encoding"] = "UTF8"

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close releases all pool connections.
func (db *DB) Close() {
	db.Pool.Close()
}

// WithTimeout returns a context with the given timeout.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

// EnsureDatabase creates the database if it does not exist.
// It connects to the postgres admin database to issue the CREATE DATABASE command.
func EnsureDatabase(ctx context.Context, cfg config.DatabaseConfig) error {
	adminDSN := cfg.AdminConnectionString()
	adminCfg, err := pgxpool.ParseConfig(adminDSN)
	if err != nil {
		return fmt.Errorf("parse admin config: %w", err)
	}
	adminCfg.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		return fmt.Errorf("connect to postgres admin: %w", err)
	}
	defer pool.Close()

	// Check if DB exists
	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.Name,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}

	if exists {
		return nil
	}

	// CREATE DATABASE does not support parameterised identifiers; escape manually.
	safeName := safeIdentifier(cfg.Name)
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, safeName))
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}

	return nil
}

// safeIdentifier escapes double-quote characters in a PostgreSQL identifier.
func safeIdentifier(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			result = append(result, '"', '"')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}

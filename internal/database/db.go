// Package database provides SQLite connection management (single-user build).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/daveontour/aimuseum/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a sql.DB pool.
type DB struct {
	Std *sql.DB
}

var sqliteVecAutoOnce sync.Once

func ensureSQLiteVecAuto() {
	sqliteVecAutoOnce.Do(func() {
		sqlite_vec.Auto()
	})
}

// EnsureSQLiteDriverLoaded registers sqlite-vec bindings. Call before opening a SQLite
// file with database/sql + mattn/go-sqlite3 outside of database.New (e.g. archive provisioning).
func EnsureSQLiteDriverLoaded() {
	ensureSQLiteVecAuto()
}

// New opens the main SQLite database file.
// Returns (nil, nil) when cfg.SQLitePath is empty — caller must nil-check.
func New(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	ensureSQLiteVecAuto()
	if cfg.SQLitePath == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	dsn := cfg.SQLiteDSN()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &DB{Std: db}, nil
}

// NewBilling opens the billing SQLite database (LLM usage).
func NewBilling(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	ensureSQLiteVecAuto()
	cfg = cfg.BillingConfig()
	path := cfg.BillingSQLitePath
	if path == "" {
		return nil, fmt.Errorf("billing sqlite path is empty (set ADMIN_SQLITE_PATH)")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create billing sqlite directory: %w", err)
	}
	dsn := config.SQLiteFileDSN(path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open billing sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping billing sqlite: %w", err)
	}
	return &DB{Std: db}, nil
}

// Close releases the database handle.
func (db *DB) Close() error {
	if db == nil || db.Std == nil {
		return nil
	}
	return db.Std.Close()
}

// WithTimeout returns a context with the given timeout.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

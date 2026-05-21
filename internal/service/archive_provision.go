package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	appcrypto "github.com/daveontour/aimuseum/internal/crypto"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

const minArchiveOwnerPasswordLen = 12

// ArchiveProvisionService creates new archive SQLite files with schema, seeds,
// admin bootstrap, first owner registration, keyring init, and subject config.
type ArchiveProvisionService struct {
	secure        bool
	adminEmail    string
	adminPassword string
	keyringPepper string
	sensitive     *SensitiveService // when non-nil, init keyring on the new archive DB (not the server main pool)
}

// NewArchiveProvisionService wires optional sensitive service (nil skips keyring init).
// keyringPepper is mixed into key derivation (same as SensitiveService); may be empty.
func NewArchiveProvisionService(
	secure bool,
	adminEmail, adminPassword, keyringPepper string,
	sensitive *SensitiveService,
) *ArchiveProvisionService {
	return &ArchiveProvisionService{
		secure:        secure,
		adminEmail:    strings.TrimSpace(adminEmail),
		adminPassword: adminPassword,
		keyringPepper: keyringPepper,
		sensitive:     sensitive,
	}
}

// CreateArchiveWithFirstUser creates a new SQLite archive at dbPath, migrates and seeds it,
// bootstraps admin from env when configured, registers the first non-admin owner, and
// initialises keyring + subject configuration when the respective services are non-nil.
func (s *ArchiveProvisionService) CreateArchiveWithFirstUser(
	ctx context.Context,
	dbPath, displayName, familyName, username, password string,
) error {
	abs := filepath.Clean(strings.TrimSpace(dbPath))
	if abs == "" || abs == "." {
		return fmt.Errorf("db path is required")
	}
	if a, err := filepath.Abs(abs); err == nil {
		abs = a
	}
	displayName = strings.TrimSpace(displayName)
	familyName = strings.TrimSpace(familyName)
	username = strings.ToLower(strings.TrimSpace(username))
	password = strings.TrimSpace(password)
	if displayName == "" {
		return fmt.Errorf("name is required")
	}
	if familyName == "" {
		return fmt.Errorf("family name is required")
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if len(password) < minArchiveOwnerPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minArchiveOwnerPasswordLen)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if st, err := os.Stat(abs); err == nil && st.Size() > 0 {
		return fmt.Errorf("database file already exists and is not empty: %s", abs)
	}
	_ = os.Remove(abs)

	database.EnsureSQLiteDriverLoaded()

	dsn := config.SQLiteFileDSN(abs)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}

	migrateCtx, cancel2 := context.WithTimeout(ctx, 90*time.Second)
	defer cancel2()
	if err := database.MigrateSQLite(migrateCtx, db); err != nil {
		return fmt.Errorf("migrate archive: %w", err)
	}
	if err := database.MigratePamBot(migrateCtx, db); err != nil {
		return fmt.Errorf("migrate pambot: %w", err)
	}
	if err := database.SeedAppSystemInstructionsFromFiles(migrateCtx, db, "static"); err != nil {
		return fmt.Errorf("seed app system instructions: %w", err)
	}

	userRepo := repository.NewUserRepo(db)
	authSvc := NewAuthService(userRepo, s.secure)
	if s.adminEmail != "" && s.adminPassword != "" {
		if err := authSvc.EnsureAdminUser(migrateCtx, s.adminEmail, s.adminPassword); err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
	}
	user, err := authSvc.Register(migrateCtx, username, password, displayName, familyName)
	if err != nil {
		return fmt.Errorf("register owner: %w", err)
	}
	userCtx := context.WithValue(ctx, appctx.ContextKeyUserID, user.ID)

	seedPassword := ""
	if s.sensitive != nil {
		// Must use the new archive connection — SensitiveService is bound to the server's main pool.
		if err := appcrypto.InitSensitiveKeyring(userCtx, db, password, s.keyringPepper); err != nil {
			return fmt.Errorf("init keyring: %w", err)
		}
		seedPassword = password
	}
	if err := SeedNewOwnerArchiveDefaults(userCtx, db, seedPassword, s.keyringPepper); err != nil {
		return err
	}
	// Subject config must be written to the new archive — not via the app-wide SubjectConfigService (main pool).
	subjectLocal := NewSubjectConfigService(repository.NewSubjectConfigRepo(db), nil, nil)
	gender := "Male"
	fn := familyName
	if _, err := subjectLocal.CreateOrUpdate(userCtx, SubjectConfigUpdateParams{
		SubjectName: displayName,
		FamilyName:  &fn,
		Gender:      &gender,
	}); err != nil {
		return fmt.Errorf("subject configuration: %w", err)
	}
	return nil
}

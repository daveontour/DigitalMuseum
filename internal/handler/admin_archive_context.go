package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
)

var errAdminArchiveNotAvailable = errors.New("no archive database is available")

type adminArchiveConn struct {
	db      *sql.DB
	owned   bool
	profile *repository.ArchiveProfile
}

func (h *AdminUsersHandler) archiveUsersUnavailable(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(r.URL.Query().Get("profile_id")) != "" && h.profileRepo != nil {
		return false
	}
	if h.userRepo != nil {
		return false
	}
	writeError(w, http.StatusServiceUnavailable, "no user archive is open — select an archive in the Archives tab")
	return true
}

func (h *AdminUsersHandler) openAdminArchive(ctx context.Context, profileID string) (adminArchiveConn, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		if h.mainPool == nil {
			return adminArchiveConn{}, errAdminArchiveNotAvailable
		}
		return adminArchiveConn{db: h.mainPool, owned: false}, nil
	}
	if h.profileRepo == nil {
		return adminArchiveConn{}, fmt.Errorf("profile store not configured")
	}
	p, err := h.profileRepo.GetByID(ctx, profileID)
	if err != nil {
		return adminArchiveConn{}, fmt.Errorf("load profile: %w", err)
	}
	if p == nil {
		return adminArchiveConn{}, fmt.Errorf("archive profile not found")
	}
	dbPath := repository.ResolveProfileDBPath(p.DBPath)
	if st, err := os.Stat(dbPath); err != nil || st.IsDir() {
		return adminArchiveConn{}, fmt.Errorf("archive database file is not accessible: %s", dbPath)
	}
	database.EnsureSQLiteDriverLoaded()
	db, err := sql.Open("sqlite3", config.SQLiteFileDSN(dbPath))
	if err != nil {
		return adminArchiveConn{}, fmt.Errorf("open archive: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return adminArchiveConn{}, fmt.Errorf("ping archive: %w", err)
	}
	return adminArchiveConn{db: db, owned: true, profile: p}, nil
}

func (c adminArchiveConn) close() {
	if c.owned && c.db != nil {
		_ = c.db.Close()
	}
}

func (h *AdminUsersHandler) listArchiveUserEmails(ctx context.Context, profileID string) ([]string, error) {
	arc, err := h.openAdminArchive(ctx, profileID)
	if err != nil {
		return nil, err
	}
	defer arc.close()
	userRepo := repository.NewUserRepo(arc.db)
	users, err := userRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(users))
	seen := make(map[string]struct{})
	for _, u := range users {
		e := strings.ToLower(strings.TrimSpace(u.Email))
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

func newDashboardServiceForDB(db *sql.DB) *service.DashboardService {
	return service.NewDashboardService(repository.NewDashboardRepo(db), repository.NewSubjectConfigRepo(db))
}

func (h *AdminUsersHandler) adminAppInstrRepo(ctx context.Context, r *http.Request) (*repository.AppSystemInstructionsRepo, func(), error) {
	profileID := strings.TrimSpace(r.URL.Query().Get("profile_id"))
	if profileID == "" {
		if h.appInstr == nil {
			return nil, nil, fmt.Errorf("system instructions store not configured")
		}
		return h.appInstr, nil, nil
	}
	arc, err := h.openAdminArchive(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	return repository.NewAppSystemInstructionsRepo(arc.db), arc.close, nil
}

func parseOptionalUserEmail(r *http.Request) *string {
	if e := strings.TrimSpace(r.URL.Query().Get("user_email")); e != "" {
		return &e
	}
	return nil
}

package keystore

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	appcrypto "github.com/daveontour/digitalmuseum/internal/crypto"
)

const (
	sessionCookieName = "dm_keyring_sid"
	// DefaultSessionTTL is how long an unlock session lasts without activity.
	DefaultSessionTTL = 24 * time.Hour
	cleanupInterval   = 15 * time.Minute
)

type sessionEntry struct {
	password  string
	expiresAt time.Time
	// isMaster is true when the session was established via owner master key unlock;
	// false when unlocked with a visitor keyring seat (same DEK material, different policy).
	isMaster bool
}

// SessionMasterStore holds per-browser master key material in RAM, keyed by an
// opaque HttpOnly session cookie.
type SessionMasterStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
	ttl      time.Duration
	secure   bool // Set-Cookie Secure flag (use behind HTTPS)
}

// NewSessionMasterStore constructs a store. secure should be true when the site is served over HTTPS.
func NewSessionMasterStore(secure bool) *SessionMasterStore {
	s := &SessionMasterStore{
		sessions: make(map[string]*sessionEntry),
		ttl:      DefaultSessionTTL,
		secure:   secure,
	}
	go s.cleanupLoop()
	return s
}

func readSessionCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c == nil || c.Value == "" {
		return ""
	}
	return c.Value
}

func randomSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *SessionMasterStore) writeSessionCookie(w http.ResponseWriter, id string, maxAgeSec int) {
	if w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secure,
	})
}

func (s *SessionMasterStore) expireSessionCookie(w http.ResponseWriter) {
	if w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secure,
	})
}

// Put validates that password is non-empty, associates it with the current or
// a new session, and sets the session cookie on w. isMaster must be true for
// owner master unlock and false for visitor seat unlock.
func (s *SessionMasterStore) Put(w http.ResponseWriter, r *http.Request, password string, isMaster bool) error {
	if s == nil {
		return nil
	}
	if password == "" {
		return nil
	}
	password = appcrypto.NormalizeKeyringPassword(password)
	if password == "" {
		return nil
	}
	now := time.Now()
	cookieID := readSessionCookie(r)

	s.mu.Lock()
	defer s.mu.Unlock()

	maxAge := int(s.ttl.Seconds())
	if maxAge < 1 {
		maxAge = 86400
	}

	if cookieID != "" {
		if e, ok := s.sessions[cookieID]; ok && now.Before(e.expiresAt) {
			e.password = password
			e.isMaster = isMaster
			e.expiresAt = now.Add(s.ttl)
			s.writeSessionCookie(w, cookieID, maxAge)
			return nil
		}
		delete(s.sessions, cookieID)
	}

	newID, err := randomSessionID()
	if err != nil {
		return err
	}
	s.sessions[newID] = &sessionEntry{
		password:  password,
		expiresAt: now.Add(s.ttl),
		isMaster:  isMaster,
	}
	s.writeSessionCookie(w, newID, maxAge)
	return nil
}

// Get returns the stored master password for this request's session cookie, if any.
// It extends the in-memory TTL on successful lookup (lazy expiry still applies).
func (s *SessionMasterStore) Get(r *http.Request) (string, bool) {
	if s == nil || r == nil {
		return "", false
	}
	id := readSessionCookie(r)
	if id == "" {
		return "", false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.sessions[id]
	if !ok || now.After(e.expiresAt) {
		if ok {
			delete(s.sessions, id)
		}
		return "", false
	}
	e.expiresAt = now.Add(s.ttl)
	return e.password, true
}

// SessionStatus returns whether the session has any unlock material and whether
// that material is from the owner master key (not a visitor seat).
func (s *SessionMasterStore) SessionStatus(r *http.Request) (unlocked bool, masterUnlocked bool) {
	if s == nil || r == nil {
		return false, false
	}
	id := readSessionCookie(r)
	if id == "" {
		return false, false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.sessions[id]
	if !ok || now.After(e.expiresAt) {
		if ok {
			delete(s.sessions, id)
		}
		return false, false
	}
	e.expiresAt = now.Add(s.ttl)
	return true, e.isMaster
}

// Clear removes the session for this request and expires the cookie.
func (s *SessionMasterStore) Clear(w http.ResponseWriter, r *http.Request) {
	if s == nil {
		return
	}
	id := readSessionCookie(r)
	s.mu.Lock()
	if id != "" {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	s.expireSessionCookie(w)
}

func (s *SessionMasterStore) cleanupLoop() {
	t := time.NewTicker(cleanupInterval)
	defer t.Stop()
	for range t.C {
		if s == nil {
			return
		}
		s.mu.Lock()
		now := time.Now()
		for id, e := range s.sessions {
			if now.After(e.expiresAt) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

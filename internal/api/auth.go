package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"sprinklergo/internal/model"
)

// Optional single-user authentication: a password login issues a session
// cookie for the browser; named API tokens (stored as SHA-256) serve
// automation via "Authorization: Bearer <token>". Everything is off by
// default — with auth disabled the API stays open like the original.

const (
	sessionCookie = "sprinklergo_session"
	sessionTTL    = 30 * 24 * time.Hour
)

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]time.Time{}}
}

func (s *sessionStore) create() string {
	buf := make([]byte, 32)
	rand.Read(buf)
	id := hex.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = time.Now().Add(sessionTTL)
	return id
}

func (s *sessionStore) valid(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[id]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.sessions, id)
		return false
	}
	return true
}

func (s *sessionStore) drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *sessionStore) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]time.Time{}
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// requireAuth gates /api/* when authentication is enabled. The SPA and its
// assets stay reachable so the login page can load; login and the auth
// status probe are always allowed.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gated := strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics"
		if !gated || !s.cfg.Snapshot().Auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/auth/login" && r.Method == http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/auth" && r.Method == http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		if s.isAuthed(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "authentication required")
	})
}

func (s *Server) isAuthed(r *http.Request) bool {
	if c, err := r.Cookie(sessionCookie); err == nil && s.sessions.valid(c.Value) {
		return true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		digest := tokenDigest(strings.TrimPrefix(h, "Bearer "))
		for _, t := range s.cfg.Snapshot().Auth.Tokens {
			if subtle.ConstantTimeCompare([]byte(t.SHA256), []byte(digest)) == 1 {
				return true
			}
		}
	}
	return false
}

func (s *Server) setSessionCookie(w http.ResponseWriter, id string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/",
		MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// GET /api/auth — status probe; token names only for authenticated callers.
func (s *Server) getAuth(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	auth := cfg.Auth
	loggedIn := !auth.Enabled || s.isAuthed(r)
	dto := map[string]any{
		"enabled":     auth.Enabled,
		"loggedIn":    loggedIn,
		"hasPassword": auth.PasswordHash != "",
		// The login page renders before any authenticated call succeeds,
		// so the language rides along on the open status probe.
		"language": cfg.Settings.Language,
	}
	if loggedIn {
		tokens := []map[string]any{}
		for _, t := range auth.Tokens {
			tokens = append(tokens, map[string]any{"name": t.Name, "createdAt": t.CreatedAt})
		}
		dto["tokens"] = tokens
	}
	writeJSON(w, http.StatusOK, dto)
}

// POST /api/auth/login {password}
func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	auth := s.cfg.Snapshot().Auth
	if auth.PasswordHash == "" ||
		bcrypt.CompareHashAndPassword([]byte(auth.PasswordHash), []byte(body.Password)) != nil {
		time.Sleep(400 * time.Millisecond) // slow down guessing
		writeErr(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/auth/logout
func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.drop(c.Value)
	}
	s.setSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/auth/password {current, new} — sets or changes the password.
// All other sessions are invalidated; the caller gets a fresh one.
func (s *Server) postPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if len(body.New) < 6 {
		writeErr(w, http.StatusBadRequest, "password must have at least 6 characters")
		return
	}
	auth := s.cfg.Snapshot().Auth
	if auth.PasswordHash != "" &&
		bcrypt.CompareHashAndPassword([]byte(auth.PasswordHash), []byte(body.Current)) != nil {
		time.Sleep(400 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "current password is wrong")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.New), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = s.cfg.Update(func(c *model.Config) error {
		c.Auth.PasswordHash = string(hash)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sessions.clear()
	s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// PUT /api/auth {enabled}
func (s *Server) putAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		if body.Enabled && c.Auth.PasswordHash == "" {
			return errors.New("set a password before enabling authentication")
		}
		c.Auth.Enabled = body.Enabled
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Enabled && !s.isAuthed(r) {
		// The caller enabling auth gets a session right away.
		s.setSessionCookie(w, s.sessions.create(), int(sessionTTL.Seconds()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/auth/tokens {name} — the plaintext token is returned exactly once.
func (s *Server) postToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	buf := make([]byte, 32)
	rand.Read(buf)
	token := hex.EncodeToString(buf)
	err := s.cfg.Update(func(c *model.Config) error {
		c.Auth.Tokens = append(c.Auth.Tokens, model.APIToken{
			Name: strings.TrimSpace(body.Name), SHA256: tokenDigest(token), CreatedAt: time.Now().Unix(),
		})
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token})
}

// DELETE /api/auth/tokens/{name}
func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	found := false
	err := s.cfg.Update(func(c *model.Config) error {
		kept := c.Auth.Tokens[:0]
		for _, t := range c.Auth.Tokens {
			if t.Name == name {
				found = true
				continue
			}
			kept = append(kept, t)
		}
		c.Auth.Tokens = kept
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

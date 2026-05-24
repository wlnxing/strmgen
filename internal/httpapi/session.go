package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const sessionCookieName = "strm_session"

type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]session
	ttl      time.Duration
}

type session struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

func newSessionManager() *sessionManager {
	return &sessionManager{
		sessions: map[string]session{},
		ttl:      24 * time.Hour,
	}
}

func (m *sessionManager) create(w http.ResponseWriter, userID int64, username string) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions[token] = session{UserID: userID, Username: username, ExpiresAt: time.Now().Add(m.ttl)}
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(m.ttl),
	})
	return nil
}

func (m *sessionManager) get(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return session{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[cookie.Value]
	if !ok || time.Now().After(sess.ExpiresAt) {
		delete(m.sessions, cookie.Value)
		return session{}, false
	}
	return sess, true
}

func (m *sessionManager) clear(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		m.mu.Lock()
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

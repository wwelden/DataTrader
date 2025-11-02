package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type contextKey string

const UserIDContextKey contextKey = "userID"

// Session represents a user session
type Session struct {
	UserID    int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore manages user sessions in memory
type SessionStore struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

var store = &SessionStore{
	sessions: make(map[string]*Session),
}

// GenerateSessionToken creates a cryptographically secure random session token
func GenerateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateSession creates a new session for a user
func CreateSession(userID int) (string, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}

	session := &Session{
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour * 7), // 7 days
	}

	store.mu.Lock()
	store.sessions[token] = session
	store.mu.Unlock()

	fmt.Printf("✓ Created session for user %d, token: %s..., expires: %s\n", userID, token[:10], session.ExpiresAt.Format("15:04:05"))
	return token, nil
}

// GetSession retrieves a session by token
func GetSession(token string) (*Session, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	session, exists := store.sessions[token]
	if !exists {
		return nil, false
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// DeleteSession removes a session
func DeleteSession(token string) {
	store.mu.Lock()
	delete(store.sessions, token)
	store.mu.Unlock()
	fmt.Printf("✓ Deleted session: %s...\n", token[:10])
}

// CleanupExpiredSessions removes expired sessions (run periodically)
func CleanupExpiredSessions() {
	store.mu.Lock()
	defer store.mu.Unlock()

	now := time.Now()
	for token, session := range store.sessions {
		if now.After(session.ExpiresAt) {
			delete(store.sessions, token)
		}
	}
}

// RequireAuth is middleware that requires authentication
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session token from cookie
		cookie, err := r.Cookie("session_token")
		if err != nil {
			fmt.Printf("✗ No session cookie found for %s: %v\n", r.URL.Path, err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session
		session, valid := GetSession(cookie.Value)
		if !valid {
			fmt.Printf("✗ Invalid session for %s, token: %s...\n", r.URL.Path, cookie.Value[:10])
			// Clear invalid cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add user ID to context
		ctx := context.WithValue(r.Context(), UserIDContextKey, session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext retrieves the user ID from the request context
func GetUserIDFromContext(r *http.Request) (int, bool) {
	userID, ok := r.Context().Value(UserIDContextKey).(int)
	return userID, ok
}

// StartSessionCleanup starts a goroutine that periodically cleans up expired sessions
func StartSessionCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			CleanupExpiredSessions()
		}
	}()
}

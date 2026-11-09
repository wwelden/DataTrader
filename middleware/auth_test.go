package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// resetStore gives each test an empty session store.
func resetStore(t *testing.T) {
	t.Helper()
	store.mu.Lock()
	store.sessions = make(map[string]*Session)
	store.mu.Unlock()
}

// assertTTL allows a little slack: CreatedAt and ExpiresAt come from separate time.Now calls.
func assertTTL(t *testing.T, s *Session, want time.Duration) {
	t.Helper()
	if ttl := s.ExpiresAt.Sub(s.CreatedAt); ttl < want || ttl > want+time.Second {
		t.Errorf("ttl = %v, want about %v", ttl, want)
	}
}

func TestGenerateSessionToken(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		tok, err := GenerateSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 44 { // base64 of 32 bytes
			t.Fatalf("token length = %d, want 44", len(tok))
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}

func TestCreateAndGetSession(t *testing.T) {
	resetStore(t)

	tok, err := CreateSession(42)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := GetSession(tok)
	if !ok {
		t.Fatal("session not found")
	}
	if s.UserID != 42 || s.IsDemo {
		t.Errorf("session = %+v, want user 42, not demo", s)
	}
	assertTTL(t, s, 7*24*time.Hour)
}

func TestCreateDemoSession(t *testing.T) {
	resetStore(t)

	tok, err := CreateDemoSession(7, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := GetSession(tok)
	if !ok {
		t.Fatal("session not found")
	}
	if s.UserID != 7 || !s.IsDemo {
		t.Errorf("session = %+v, want demo user 7", s)
	}
	assertTTL(t, s, time.Hour)
}

func TestGetSessionUnknownAndExpired(t *testing.T) {
	resetStore(t)

	if _, ok := GetSession("nope"); ok {
		t.Error("unknown token should not be valid")
	}

	tok, _ := CreateDemoSession(1, -time.Second)
	if _, ok := GetSession(tok); ok {
		t.Error("expired session should not be valid")
	}
}

func TestDeleteSession(t *testing.T) {
	resetStore(t)

	tok, _ := CreateSession(1)
	DeleteSession(tok)
	if _, ok := GetSession(tok); ok {
		t.Error("session should be gone after DeleteSession")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	resetStore(t)

	live, _ := CreateSession(1)
	dead, _ := CreateDemoSession(2, -time.Minute)

	CleanupExpiredSessions()

	store.mu.RLock()
	_, liveKept := store.sessions[live]
	_, deadKept := store.sessions[dead]
	store.mu.RUnlock()

	if !liveKept {
		t.Error("live session was removed")
	}
	if deadKept {
		t.Error("expired session was kept")
	}
}

func TestSessionStoreConcurrentAccess(t *testing.T) {
	resetStore(t)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tok, err := CreateSession(id)
			if err != nil {
				t.Error(err)
				return
			}
			if s, ok := GetSession(tok); !ok || s.UserID != id {
				t.Errorf("session for %d not readable", id)
			}
			CleanupExpiredSessions()
			DeleteSession(tok)
		}(i)
	}
	wg.Wait()
}

func requestWithCookie(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/positions.html", nil)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	}
	return r
}

func TestRequireAuthNoCookieRedirects(t *testing.T) {
	resetStore(t)

	called := false
	h := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithCookie(""))

	if called {
		t.Error("next handler should not run without a session")
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("got %d to %q, want 303 to /login", rec.Code, rec.Header().Get("Location"))
	}
}

func TestRequireAuthInvalidSessionClearsCookie(t *testing.T) {
	resetStore(t)

	h := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not run for an invalid session")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithCookie("this-token-does-not-exist"))

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("got %d to %q, want 303 to /login", rec.Code, rec.Header().Get("Location"))
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("invalid session cookie should be cleared")
	}
}

func TestRequireAuthValidSessionSetsContext(t *testing.T) {
	resetStore(t)

	tok, _ := CreateDemoSession(99, time.Hour)

	var gotID int
	var gotOK, gotDemo bool
	h := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = GetUserIDFromContext(r)
		gotDemo = IsDemo(r.Context())
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestWithCookie(tok))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, next handler did not run", rec.Code)
	}
	if !gotOK || gotID != 99 || !gotDemo {
		t.Errorf("context = (id %d, ok %v, demo %v), want (99, true, true)", gotID, gotOK, gotDemo)
	}
}

func TestSessionFromRequest(t *testing.T) {
	resetStore(t)

	if _, ok := SessionFromRequest(requestWithCookie("")); ok {
		t.Error("no cookie should mean no session")
	}
	if _, ok := SessionFromRequest(requestWithCookie("bogus-token-value")); ok {
		t.Error("unknown token should mean no session")
	}

	tok, _ := CreateSession(5)
	s, ok := SessionFromRequest(requestWithCookie(tok))
	if !ok || s.UserID != 5 {
		t.Errorf("SessionFromRequest = (%+v, %v), want user 5", s, ok)
	}
}

func TestContextHelpersWithoutValues(t *testing.T) {
	if IsDemo(context.Background()) {
		t.Error("IsDemo should be false on an empty context")
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := GetUserIDFromContext(r); ok {
		t.Error("GetUserIDFromContext should fail on an empty context")
	}
	ctx := context.WithValue(context.Background(), UserIDContextKey, "not-an-int")
	if _, ok := GetUserIDFromContext(r.WithContext(ctx)); ok {
		t.Error("GetUserIDFromContext should reject a non-int value")
	}
}

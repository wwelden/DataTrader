package handlers

import (
	"backend/middleware"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_token" {
			return c
		}
	}
	return nil
}

func signupForm(user, pass, confirm string) url.Values {
	return url.Values{"username": {user}, "password": {pass}, "confirm_password": {confirm}}
}

func TestHandleLoginAndSignupPagesRender(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{"login": HandleLogin, "signup": HandleSignup} {
		t.Run(name, func(t *testing.T) {
			rec := serve(t, h, req{})
			assertStatus(t, rec, http.StatusOK)
			assertContains(t, rec.Body.String(), `name="username"`, `name="password"`)
		})
	}
}

func TestHandleSignupPostSuccess(t *testing.T) {
	setupDB(t)

	rec := serve(t, HandleSignupPost, req{method: http.MethodPost, form: signupForm("alice", "password123", "password123")})

	assertStatus(t, rec, http.StatusSeeOther)
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("redirect = %q, want /", loc)
	}
	c := sessionCookie(rec)
	if c == nil || !c.HttpOnly {
		t.Fatalf("expected an HttpOnly session cookie, got %+v", c)
	}
	s, ok := middleware.GetSession(c.Value)
	if !ok {
		t.Fatal("cookie does not map to a live session")
	}

	var storedID int
	var hash string
	if err := db.QueryRow("SELECT id, password FROM users WHERE username = 'alice'").Scan(&storedID, &hash); err != nil {
		t.Fatal(err)
	}
	if s.UserID != storedID {
		t.Errorf("session user = %d, want %d", s.UserID, storedID)
	}
	if hash == "password123" {
		t.Fatal("password stored in plain text")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")) != nil {
		t.Error("stored hash does not match the password")
	}
}

func TestHandleSignupPostValidation(t *testing.T) {
	tests := []struct {
		name    string
		form    url.Values
		wantMsg string
	}{
		{"missing username", signupForm("", "password123", "password123"), "required"},
		{"missing password", signupForm("bob", "", ""), "required"},
		{"reserved demo prefix", signupForm("demo-abc", "password123", "password123"), "reserved"},
		{"reserved demo prefix any case", signupForm("DEMO-abc", "password123", "password123"), "reserved"},
		{"short password", signupForm("bob", "short", "short"), "at least 8"},
		{"mismatched passwords", signupForm("bob", "password123", "password124"), "do not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupDB(t)
			rec := serve(t, HandleSignupPost, req{method: http.MethodPost, form: tt.form})
			assertStatus(t, rec, http.StatusOK)
			assertContains(t, rec.Body.String(), tt.wantMsg)
			if sessionCookie(rec) != nil {
				t.Error("no session should be created on validation failure")
			}
			if n := count(t, "SELECT COUNT(*) FROM users"); n != 0 {
				t.Errorf("users = %d, want 0", n)
			}
		})
	}
}

func TestHandleSignupPostDuplicateUsername(t *testing.T) {
	setupDB(t)
	createUser(t, "alice")

	rec := serve(t, HandleSignupPost, req{method: http.MethodPost, form: signupForm("alice", "password123", "password123")})

	assertStatus(t, rec, http.StatusOK)
	assertContains(t, rec.Body.String(), "already taken")
	if n := count(t, "SELECT COUNT(*) FROM users"); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestHandleLoginPost(t *testing.T) {
	setupDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	uid := mustExec(t, "INSERT INTO users (username, password) VALUES ('carol', ?)", string(hash))

	t.Run("success", func(t *testing.T) {
		rec := serve(t, HandleLoginPost, req{method: http.MethodPost, form: url.Values{"username": {"carol"}, "password": {"correct-horse"}}})
		assertStatus(t, rec, http.StatusSeeOther)
		c := sessionCookie(rec)
		if c == nil {
			t.Fatal("no session cookie")
		}
		s, ok := middleware.GetSession(c.Value)
		if !ok || s.UserID != uid || s.IsDemo {
			t.Errorf("session = %+v, ok %v; want user %d, not demo", s, ok, uid)
		}
	})

	failures := map[string]url.Values{
		"wrong password": {"username": {"carol"}, "password": {"wrong"}},
		"unknown user":   {"username": {"nobody"}, "password": {"correct-horse"}},
	}
	for name, form := range failures {
		t.Run(name, func(t *testing.T) {
			rec := serve(t, HandleLoginPost, req{method: http.MethodPost, form: form})
			assertStatus(t, rec, http.StatusOK)
			assertContains(t, rec.Body.String(), "Invalid username or password")
			if sessionCookie(rec) != nil {
				t.Error("no session cookie expected")
			}
		})
	}

	t.Run("missing fields", func(t *testing.T) {
		rec := serve(t, HandleLoginPost, req{method: http.MethodPost, form: url.Values{"username": {"carol"}}})
		assertStatus(t, rec, http.StatusOK)
		assertContains(t, rec.Body.String(), "required")
	})
}

func TestHandleLogout(t *testing.T) {
	tok, err := middleware.CreateSession(1)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: tok})
	rec := httptest.NewRecorder()
	HandleLogout(rec, r)

	assertStatus(t, rec, http.StatusSeeOther)
	if _, ok := middleware.GetSession(tok); ok {
		t.Error("session should be deleted on logout")
	}
	if c := sessionCookie(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie should be cleared, got %+v", c)
	}
}

func TestHandleLogoutHTMX(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	r.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	HandleLogout(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("HX-Redirect"); got != "/" {
		t.Errorf("HX-Redirect = %q, want /", got)
	}
}

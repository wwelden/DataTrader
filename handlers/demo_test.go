package handlers

import (
	"backend/middleware"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHandleDemoPostCreatesSeededUser(t *testing.T) {
	setupDB(t)

	rec := serve(t, HandleDemoPost, req{method: http.MethodPost})

	assertStatus(t, rec, http.StatusSeeOther)
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("no session cookie")
	}
	if c.MaxAge != int(demoTTL.Seconds()) {
		t.Errorf("cookie MaxAge = %d, want %d", c.MaxAge, int(demoTTL.Seconds()))
	}
	s, ok := middleware.GetSession(c.Value)
	if !ok || !s.IsDemo {
		t.Fatalf("session = %+v, ok %v; want a demo session", s, ok)
	}

	var username string
	if err := db.QueryRow("SELECT username FROM users WHERE id = ?", s.UserID).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(username, demoUserPrefix) {
		t.Errorf("username = %q, want %q prefix", username, demoUserPrefix)
	}

	for table, want := range map[string]int{
		"stock_positions":  5,
		"closed_stocks":    7,
		"option_positions": 4,
		"closed_options":   7,
	} {
		if n := count(t, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", s.UserID); n != want {
			t.Errorf("%s rows = %d, want %d", table, n, want)
		}
	}
}

func TestHandleDemoPostEachCallIsSeparateUser(t *testing.T) {
	setupDB(t)

	a := sessionCookie(serve(t, HandleDemoPost, req{method: http.MethodPost}))
	b := sessionCookie(serve(t, HandleDemoPost, req{method: http.MethodPost}))
	sa, _ := middleware.GetSession(a.Value)
	sb, _ := middleware.GetSession(b.Value)
	if sa.UserID == sb.UserID {
		t.Error("two demo logins should get two different users")
	}
}

func TestHandleDemoPostCapsLiveDemos(t *testing.T) {
	setupDB(t)
	for i := range maxDemoUsers {
		createUser(t, demoUserPrefix+"seed"+strconv.Itoa(i))
	}

	rec := serve(t, HandleDemoPost, req{method: http.MethodPost})

	assertStatus(t, rec, http.StatusServiceUnavailable)
	if n := count(t, "SELECT COUNT(*) FROM users"); n != maxDemoUsers {
		t.Errorf("users = %d, want %d (no new demo user)", n, maxDemoUsers)
	}
}

func TestSeedDemoDataProfitLoss(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "seed-check")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := seedDemoData(tx, uid, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// META: bought 20 @ 468.50, sold @ 512.75.
	var pl float64
	var closeDate string
	if err := db.QueryRow("SELECT profit_loss, close_date FROM closed_stocks WHERE user_id = ? AND ticker = 'META'", uid).Scan(&pl, &closeDate); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(pl, (512.75-468.50)*20) {
		t.Errorf("META P/L = %v, want %v", pl, (512.75-468.50)*20)
	}
	if want := now.AddDate(0, 0, -120).Format("2006-01-02"); closeDate != want {
		t.Errorf("META close date = %q, want %q (relative to now)", closeDate, want)
	}

	// SOFI CSP: 5 contracts, premium 35, expired worthless; collateral = strike*100*qty.
	var collateral float64
	if err := db.QueryRow("SELECT profit_loss, collateral FROM closed_options WHERE user_id = ? AND ticker = 'SOFI'", uid).Scan(&pl, &collateral); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(pl, 35*5) || !approxEqual(collateral, 8*100*5) {
		t.Errorf("SOFI = (pl %v, collateral %v), want (175, 4000)", pl, collateral)
	}

	// PLTR long call: (sold - premium) * qty.
	if err := db.QueryRow("SELECT profit_loss FROM closed_options WHERE user_id = ? AND ticker = 'PLTR'", uid).Scan(&pl); err != nil {
		t.Fatal(err)
	}
	if !approxEqual(pl, (465-180)*2) {
		t.Errorf("PLTR P/L = %v, want %v", pl, (465-180)*2)
	}
}

func TestCleanupDemoUsers(t *testing.T) {
	setupDB(t)

	old := time.Now().Add(-demoTTL - time.Hour).UTC().Format("2006-01-02 15:04:05")
	staleDemo := mustExec(t, "INSERT INTO users (username, password, created_at) VALUES ('demo-old', 'x', ?)", old)
	freshDemo := createUser(t, "demo-new")
	oldRealUser := mustExec(t, "INSERT INTO users (username, password, created_at) VALUES ('realuser', 'x', ?)", old)

	for _, uid := range []int{staleDemo, freshDemo, oldRealUser} {
		insertStock(t, uid, "AAPL", 1, 1, "2025-01-01")
		insertOption(t, uid, "AAPL", "Call", 1, 1, 0, 1, "2025-02-01", "2025-01-01")
		insertClosedStock(t, uid, "AAPL", 1, 1, 2, "2025-01-01", "2025-01-02")
		insertClosedOption(t, uid, "AAPL", "Call", 1, 1, 2, 1, "2025-01-02")
		mustExec(t, "INSERT INTO stock_trades (id, user_id, ticker, date, code, price, amount, quantity) VALUES (?, ?, 'AAPL', 'd', 'Buy', 1, 1, 1)", "st"+strconv.Itoa(uid), uid)
		mustExec(t, "INSERT INTO option_trades (id, user_id, ticker, date, code, price, amount, quantity, strike, exp_date, option_type, premium) VALUES (?, ?, 'AAPL', 'd', 'BTO', 1, 1, 1, 1, 'e', 'Call', 1)", "ot"+strconv.Itoa(uid), uid)
	}

	if err := CleanupDemoUsers(); err != nil {
		t.Fatal(err)
	}

	if n := count(t, "SELECT COUNT(*) FROM users WHERE id = ?", staleDemo); n != 0 {
		t.Error("stale demo user should be deleted")
	}
	for _, table := range []string{"stock_trades", "option_trades", "stock_positions", "closed_stocks", "option_positions", "closed_options"} {
		if n := count(t, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", staleDemo); n != 0 {
			t.Errorf("%s still has %d rows for the stale demo user", table, n)
		}
		for _, keep := range []int{freshDemo, oldRealUser} {
			if n := count(t, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", keep); n != 1 {
				t.Errorf("%s rows for user %d = %d, want 1 (must be kept)", table, keep, n)
			}
		}
	}
	if n := count(t, "SELECT COUNT(*) FROM users"); n != 2 {
		t.Errorf("users = %d, want 2", n)
	}
}

package handlers

import (
	"backend/middleware"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// demoUserPrefix marks throwaway accounts created from the landing page.
	demoUserPrefix = "demo-"
	// demoTTL is how long a demo account (and its session) lives.
	demoTTL = 24 * time.Hour
	// maxDemoUsers caps how many live demo accounts can exist at once,
	// so a bot hammering POST /demo can't grow the database unbounded.
	maxDemoUsers = 500
)

// HandleDemoPost creates a fresh demo user filled with sample trades,
// logs them in, and drops them on the dashboard.
func HandleDemoPost(w http.ResponseWriter, r *http.Request) {
	if err := CleanupDemoUsers(); err != nil {
		slog.Error("demo cleanup failed", "err", err)
	}

	var liveDemos int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username LIKE ?", demoUserPrefix+"%").Scan(&liveDemos); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if liveDemos >= maxDemoUsers {
		http.Error(w, "The demo is busy right now. Please try again later.", http.StatusServiceUnavailable)
		return
	}

	userID, err := createDemoUser()
	if err != nil {
		slog.Error("failed to create demo user", "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sessionToken, err := middleware.CreateDemoSession(userID, demoTTL)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(demoTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// createDemoUser inserts a demo user and its sample data in one transaction.
func createDemoUser() (int, error) {
	suffix, err := randomHex(6)
	if err != nil {
		return 0, err
	}
	// Demo accounts are only reachable through their session; the password
	// is random and never shown, so nobody can log in to one directly.
	password, err := randomHex(16)
	if err != nil {
		return 0, err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO users (username, password) VALUES (?, ?)", demoUserPrefix+suffix, string(hashed))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := seedDemoData(tx, int(id), time.Now()); err != nil {
		return 0, err
	}

	return int(id), tx.Commit()
}

// seedDemoData fills a demo account with a realistic mix of open and closed
// stock and option positions. Dates are relative to now so the demo never
// looks stale.
func seedDemoData(tx *sql.Tx, userID int, now time.Time) error {
	day := func(offset int) string {
		return now.AddDate(0, 0, offset).Format("2006-01-02")
	}

	openStocks := []struct {
		ticker    string
		qty, cost float64
		opened    int
	}{
		{"AAPL", 50, 172.40, -210},
		{"MSFT", 25, 338.10, -185},
		{"NVDA", 40, 118.25, -96},
		{"AMD", 100, 142.80, -64},
		{"SPY", 15, 512.30, -30},
	}
	for _, s := range openStocks {
		if _, err := tx.Exec(`
			INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
			VALUES (?, ?, ?, ?, ?)
		`, userID, s.ticker, s.qty, s.cost, day(s.opened)); err != nil {
			return fmt.Errorf("seed stock position %s: %w", s.ticker, err)
		}
	}

	closedStocks := []struct {
		ticker           string
		qty, cost, sold  float64
		opened, closedOn int
	}{
		{"META", 20, 468.50, 512.75, -160, -120},
		{"GOOGL", 30, 152.20, 147.85, -150, -110},
		{"AMZN", 25, 178.40, 196.10, -140, -90},
		{"TSLA", 15, 245.60, 221.30, -130, -75},
		{"COST", 5, 812.00, 874.50, -120, -45},
		{"NFLX", 8, 610.25, 688.40, -100, -20},
		{"INTC", 120, 31.40, 22.85, -200, -60},
	}
	for _, s := range closedStocks {
		pl := (s.sold - s.cost) * s.qty
		if _, err := tx.Exec(`
			INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, s.ticker, day(s.opened), day(s.closedOn), s.qty, s.cost, s.sold, pl); err != nil {
			return fmt.Errorf("seed closed stock %s: %w", s.ticker, err)
		}
	}

	// Option premiums are per contract in dollars, matching the add-position form.
	openOptions := []struct {
		ticker, kind                 string
		strike, premium, qty, collat float64
		opened, expires              int
	}{
		{"AMD", "CC", 160, 210, 1, 142.80 * 100, -14, 21},
		{"TSLA", "CSP", 220, 385, 1, 220 * 100, -10, 25},
		{"NVDA", "Call", 130, 520, 2, 0, -21, 39},
		{"SPY", "Put", 500, 640, 1, 0, -7, 30},
	}
	for _, o := range openOptions {
		if _, err := tx.Exec(`
			INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, o.ticker, o.premium, o.premium, o.strike, day(o.expires), o.kind, o.collat, o.qty, day(o.opened)); err != nil {
			return fmt.Errorf("seed option position %s: %w", o.ticker, err)
		}
	}

	closedOptions := []struct {
		ticker, kind               string
		strike, premium, sold, qty float64
		opened, expires, closedOn  int
	}{
		{"AAPL", "CC", 185, 240, 0, 1, -75, -45, -45},
		{"SOFI", "CSP", 8, 35, 0, 5, -70, -42, -42},
		{"PLTR", "Call", 25, 180, 465, 2, -95, -40, -55},
		{"QQQ", "Put", 430, 520, 210, 1, -60, -30, -38},
		{"MSFT", "CC", 360, 410, 120, 1, -50, -16, -25},
		{"AMD", "CSP", 140, 290, 60, 1, -40, -12, -18},
		{"COIN", "Call", 260, 730, 0, 1, -35, -5, -5},
	}
	for _, o := range closedOptions {
		var pl float64
		var collat float64
		switch o.kind {
		case "CSP":
			pl = (o.premium - o.sold) * o.qty
			collat = o.strike * 100 * o.qty
		case "CC":
			pl = (o.premium - o.sold) * o.qty
		default:
			pl = (o.sold - o.premium) * o.qty
		}
		if _, err := tx.Exec(`
			INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, o.ticker, o.premium, o.premium, o.strike, day(o.expires), o.kind, collat, o.qty, day(o.opened), day(o.closedOn), o.sold, pl); err != nil {
			return fmt.Errorf("seed closed option %s: %w", o.ticker, err)
		}
	}

	return nil
}

// CleanupDemoUsers deletes demo accounts older than demoTTL and all their data.
// Rows are deleted explicitly because SQLite foreign keys (and so ON DELETE
// CASCADE) are off unless enabled per connection.
func CleanupDemoUsers() error {
	cutoff := time.Now().Add(-demoTTL).UTC().Format("2006-01-02 15:04:05")
	staleUsers := `SELECT id FROM users WHERE username LIKE ? AND created_at < ?`

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"stock_trades", "option_trades", "stock_positions", "closed_stocks", "option_positions", "closed_options"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE user_id IN ("+staleUsers+")", demoUserPrefix+"%", cutoff); err != nil {
			return fmt.Errorf("cleanup %s: %w", table, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM users WHERE id IN ("+staleUsers+")", demoUserPrefix+"%", cutoff); err != nil {
		return fmt.Errorf("cleanup users: %w", err)
	}

	return tx.Commit()
}

// StartDemoCleanup periodically removes expired demo accounts.
func StartDemoCleanup() {
	go func() {
		if err := CleanupDemoUsers(); err != nil {
			slog.Error("demo cleanup failed", "err", err)
		}
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := CleanupDemoUsers(); err != nil {
				slog.Error("demo cleanup failed", "err", err)
			}
		}
	}()
}

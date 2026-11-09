package handlers

import (
	"backend/middleware"
	"backend/schema"
	"context"
	"database/sql"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

func TestMain(m *testing.M) {
	// Schema migrations log a note on every fresh database; keep test output readable.
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// setupDB points the handlers at a fresh SQLite database with the real schema.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Apply(d); err != nil {
		t.Fatal(err)
	}
	prev := db
	SetDB(d)
	t.Cleanup(func() {
		SetDB(prev)
		d.Close()
	})
	return d
}

// createUser inserts a user directly (no bcrypt) and returns its id.
func createUser(t *testing.T, username string) int {
	t.Helper()
	res, err := db.Exec("INSERT INTO users (username, password) VALUES (?, 'x')", username)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func mustExec(t *testing.T, query string, args ...any) int {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func insertStock(t *testing.T, userID int, ticker string, qty, cost float64, openDate string) int {
	t.Helper()
	return mustExec(t, `INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date) VALUES (?, ?, ?, ?, ?)`,
		userID, ticker, qty, cost, openDate)
}

func insertOption(t *testing.T, userID int, ticker, kind string, strike, premium, collateral, qty float64, expDate, purchaseDate string) int {
	t.Helper()
	return mustExec(t, `INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, ticker, premium, premium, strike, expDate, kind, collateral, qty, purchaseDate)
}

func insertClosedStock(t *testing.T, userID int, ticker string, qty, cost, sold float64, openDate, closeDate string) int {
	t.Helper()
	return mustExec(t, `INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, ticker, openDate, closeDate, qty, cost, sold, (sold-cost)*qty)
}

func insertClosedOption(t *testing.T, userID int, ticker, kind string, strike, premium, sold, pl float64, closeDate string) int {
	t.Helper()
	return mustExec(t, `INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, 1, ?, ?, ?, ?)`,
		userID, ticker, premium, premium, strike, closeDate, kind, closeDate, closeDate, sold, pl)
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// req describes one request to a handler.
type req struct {
	method string
	target string
	form   url.Values
	userID int               // 0 means unauthenticated
	params map[string]string // chi URL params such as {id}
}

// serve calls h directly with the auth context and chi params that the
// router and RequireAuth would normally set up.
func serve(t *testing.T, h http.HandlerFunc, rq req) *httptest.ResponseRecorder {
	t.Helper()
	if rq.method == "" {
		rq.method = http.MethodGet
	}
	if rq.target == "" {
		rq.target = "/"
	}

	var r *http.Request
	if rq.form != nil {
		r = httptest.NewRequest(rq.method, rq.target, strings.NewReader(rq.form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(rq.method, rq.target, nil)
	}

	ctx := r.Context()
	if rq.userID != 0 {
		ctx = context.WithValue(ctx, middleware.UserIDContextKey, rq.userID)
		ctx = context.WithValue(ctx, middleware.IsDemoContextKey, false)
	}
	if len(rq.params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range rq.params {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}

	rec := httptest.NewRecorder()
	h(rec, r.WithContext(ctx))
	return rec
}

func id(n int) map[string]string { return map[string]string{"id": strconv.Itoa(n)} }

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func assertContains(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(body, s) {
			t.Errorf("body missing %q", s)
		}
	}
}

func assertNotContains(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(body, s) {
			t.Errorf("body unexpectedly contains %q", s)
		}
	}
}

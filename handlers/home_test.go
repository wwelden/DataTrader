package handlers

import (
	"backend/middleware"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestHandleRootLandingForVisitors(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleRoot(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assertStatus(t, rec, http.StatusOK)
	assertContains(t, rec.Body.String(), `action="/demo"`)
}

func TestHandleRootDashboardForSessions(t *testing.T) {
	tok, err := middleware.CreateSession(1)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: tok})
	rec := httptest.NewRecorder()
	HandleRoot(rec, r)

	assertStatus(t, rec, http.StatusOK)
	body := rec.Body.String()
	assertContains(t, body, "/api/stats")
	assertNotContains(t, body, `action="/demo"`)
}

func TestHandleStatsEmpty(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	rec := serve(t, HandleStats, req{userID: uid})
	assertStatus(t, rec, http.StatusOK)
	got := statValues(rec.Body.String())
	if len(got) != 8 {
		t.Fatalf("expected 8 stat cards, got %v", got)
	}
	if got["Total Positions"] != "0" || got["Win Rate"] != "0.0%" || got["Profit Factor"] != "0.00" {
		t.Errorf("empty stats = %v", got)
	}
}

func TestHandleStats(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")

	insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")
	insertStock(t, uid, "MSFT", 10, 100, "2025-01-01")
	insertOption(t, uid, "TSLA", "Call", 250, 500, 0, 1, "2025-02-21", "2025-01-01")

	insertClosedStock(t, uid, "META", 10, 100, 130, "2025-01-01", "2025-01-15") // +300
	insertClosedStock(t, uid, "INTC", 10, 30, 20, "2025-01-01", "2025-01-15")   // -100
	insertClosedOption(t, uid, "SOFI", "CSP", 8, 100, 0, 100, "2025-01-15")     // +100
	insertClosedOption(t, uid, "COIN", "Call", 260, 300, 0, -300, "2025-01-15") // -300

	// Another user's data must not leak into the numbers.
	insertStock(t, other, "GOOG", 1, 1, "2025-01-01")
	insertClosedStock(t, other, "GOOG", 1, 1, 1000, "2025-01-01", "2025-01-15")

	got := statValues(serve(t, HandleStats, req{userID: uid}).Body.String())
	want := map[string]string{
		"Total Positions":    "3",
		"Open Stocks":        "2",
		"Open Options":       "1",
		"Closed Trades":      "4",
		"Total P/L":          "$0.00",
		"Avg Win / Avg Loss": "$200.00/$-200.00",
		"Win Rate":           "50.0%",
		"Profit Factor":      "1.00",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestHandleStatsNoLosses(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertClosedStock(t, uid, "META", 10, 100, 130, "2025-01-01", "2025-01-15")

	got := statValues(serve(t, HandleStats, req{userID: uid}).Body.String())
	if got["Win Rate"] != "100.0%" || got["Profit Factor"] != "0.00" || got["Total P/L"] != "$300.00" {
		t.Errorf("stats = %v", got)
	}
}

// statValues maps each stat card's heading to its value with whitespace removed.
func statValues(body string) map[string]string {
	out := map[string]string{}
	re := regexp.MustCompile(`(?s)<h3>(.*?)</h3>\s*<p[^>]*>(.*?)</p>`)
	tags := regexp.MustCompile(`<[^>]+>|\s+`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		out[m[1]] = tags.ReplaceAllString(m[2], "")
	}
	return out
}

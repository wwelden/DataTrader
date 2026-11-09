package handlers

import (
	"backend/middleware"
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const importHeader = `"Activity Date","Process Date","Settle Date","Instrument","Description","Trans Code","Quantity","Price","Amount"`

func importRow(date, ticker, desc, code, qty, price string) string {
	return strings.Join([]string{
		`"` + date + `"`, `"` + date + `"`, `"` + date + `"`,
		`"` + ticker + `"`, `"` + desc + `"`, `"` + code + `"`,
		`"` + qty + `"`, `"` + price + `"`, `"$0.00"`,
	}, ",")
}

// importCSV posts csv as the csvFile upload and returns the response.
func importCSV(t *testing.T, userID int, csv string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("csvFile", "trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(csv))
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/api/import-csv", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if userID != 0 {
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDContextKey, userID))
	}
	rec := httptest.NewRecorder()
	HandleImportCSV(rec, r)
	return rec
}

func csvLines(rows ...string) string {
	return importHeader + "\n" + strings.Join(rows, "\n") + "\n"
}

func TestHandleImportCSVRequiresUploadAndUser(t *testing.T) {
	setupDB(t)

	rec := serve(t, HandleImportCSV, req{method: http.MethodPost, userID: 1})
	assertStatus(t, rec, http.StatusBadRequest)

	rec = importCSV(t, 0, csvLines(importRow("1/2/2025", "AAPL", "Apple", "Buy", "1", "$1.00")))
	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestHandleImportCSVRejectsEmptyFile(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	rec := importCSV(t, uid, importHeader+"\n")
	assertStatus(t, rec, http.StatusOK)
	assertContains(t, rec.Body.String(), "Import Failed", "header")
	if rec.Header().Get("HX-Trigger") != "" {
		t.Error("a failed import should not trigger a refresh")
	}
}

func TestHandleImportCSVStocks(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	rec := importCSV(t, uid, csvLines(
		importRow("1/2/2025", "AAPL", "Apple", "Buy", "10", "$100.00"),
		importRow("1/3/2025", "AAPL", "Apple", "Buy", "10", "$120.00"),
		importRow("1/4/2025", "AAPL", "Apple", "Sell", "5", "$130.00"),
		importRow("1/5/2025", "MSFT", "Microsoft", "Buy", "2", "$400.00"),
		importRow("1/6/2025", "MSFT", "Microsoft", "Sell", "2", "$450.00"),
	))
	assertStatus(t, rec, http.StatusOK)
	assertContains(t, rec.Body.String(), "Import Successful", "5 stock trades")
	if got := rec.Header().Get("HX-Trigger"); !strings.Contains(got, "positionAdded") || !strings.Contains(got, "historyUpdated") {
		t.Errorf("HX-Trigger = %q", got)
	}

	var qty, cost float64
	var openDate string
	if err := db.QueryRow("SELECT quantity, cost_basis, open_date FROM stock_positions WHERE user_id = ? AND ticker = 'AAPL'", uid).Scan(&qty, &cost, &openDate); err != nil {
		t.Fatal(err)
	}
	if qty != 15 || !approxEqual(cost, 110) || openDate != "01/02/25" {
		t.Errorf("AAPL = (qty %v, cost %v, %s), want (15, 110, 01/02/25)", qty, cost, openDate)
	}

	if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE user_id = ? AND ticker = 'MSFT'", uid); n != 0 {
		t.Error("fully sold MSFT should no longer be open")
	}
	var pl, sold float64
	var closeDate string
	if err := db.QueryRow("SELECT profit_loss, sell_price, close_date FROM closed_stocks WHERE user_id = ? AND ticker = 'MSFT'", uid).Scan(&pl, &sold, &closeDate); err != nil {
		t.Fatal(err)
	}
	if pl != 100 || sold != 450 || closeDate != "01/06/25" {
		t.Errorf("MSFT closed = (pl %v, sold %v, %s), want (100, 450, 01/06/25)", pl, sold, closeDate)
	}
}

func TestHandleImportCSVSellWithoutPositionIsIgnored(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	importCSV(t, uid, csvLines(importRow("1/2/2025", "AAPL", "Apple", "Sell", "10", "$100.00")))

	if n := count(t, "SELECT COUNT(*) FROM stock_positions") + count(t, "SELECT COUNT(*) FROM closed_stocks"); n != 0 {
		t.Errorf("a sell with no open position should change nothing, got %d rows", n)
	}
}

func TestHandleImportCSVOptionsLongRoundTrip(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	importCSV(t, uid, csvLines(
		importRow("1/2/2025", "TSLA", "TSLA 2/21/2025 Call $250.00", "BTO", "3", "$5.00"),
		importRow("1/9/2025", "TSLA", "TSLA 2/21/2025 Call $250.00", "STC", "1", "$7.50"),
	))

	var kind, exp string
	var qty, strike float64
	if err := db.QueryRow("SELECT type, exp_date, quantity, strike FROM option_positions WHERE user_id = ?", uid).Scan(&kind, &exp, &qty, &strike); err != nil {
		t.Fatal(err)
	}
	if kind != "Call" || exp != "02/21/25" || qty != 2 || strike != 250 {
		t.Errorf("open = (%s, %s, qty %v, strike %v), want (Call, 02/21/25, 2, 250)", kind, exp, qty, strike)
	}

	var pl, closedQty float64
	var purchased, closed string
	if err := db.QueryRow("SELECT profit_loss, quantity, purchase_date, close_date FROM closed_options WHERE user_id = ?", uid).Scan(&pl, &closedQty, &purchased, &closed); err != nil {
		t.Fatal(err)
	}
	// Brokerage prices are per share, so P/L is scaled by 100 shares per contract.
	if !approxEqual(pl, 250) || closedQty != 1 || purchased != "01/02/25" || closed != "01/09/25" {
		t.Errorf("closed = (pl %v, qty %v, %s, %s), want (250, 1, 01/02/25, 01/09/25)", pl, closedQty, purchased, closed)
	}
}

func TestHandleImportCSVOptionsShortRoundTrip(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	importCSV(t, uid, csvLines(
		importRow("1/2/2025", "AMD", "AMD 3/21/2025 Put $120.00", "STO", "2", "$3.00"),
		importRow("1/2/2025", "AMD", "AMD 3/21/2025 Call $160.00", "STO", "1", "$2.00"),
		importRow("1/9/2025", "AMD", "AMD 3/21/2025 Put $120.00", "BTC", "2", "$1.00"),
	))

	var kind string
	if err := db.QueryRow("SELECT type FROM option_positions WHERE user_id = ?", uid).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "CC" {
		t.Errorf("remaining open option = %s, want CC (sold call)", kind)
	}
	if n := count(t, "SELECT COUNT(*) FROM option_positions WHERE user_id = ?", uid); n != 1 {
		t.Errorf("open options = %d, want 1", n)
	}

	var closedKind string
	var pl float64
	if err := db.QueryRow("SELECT type, profit_loss FROM closed_options WHERE user_id = ?", uid).Scan(&closedKind, &pl); err != nil {
		t.Fatal(err)
	}
	if closedKind != "CSP" || !approxEqual(pl, (3-1)*2*100) {
		t.Errorf("closed = (%s, pl %v), want (CSP, 400)", closedKind, pl)
	}
}

func TestHandleImportCSVCloseWithoutMatchingOpenIsIgnored(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	rec := importCSV(t, uid, csvLines(
		importRow("1/9/2025", "AMD", "AMD 3/21/2025 Put $120.00", "BTC", "1", "$1.00"),
		importRow("1/9/2025", "TSLA", "TSLA 2/21/2025 Call $250.00", "STC", "1", "$1.00"),
	))
	assertContains(t, rec.Body.String(), "0 option trades")
	if n := count(t, "SELECT COUNT(*) FROM closed_options"); n != 0 {
		t.Errorf("closed options = %d, want 0", n)
	}
}

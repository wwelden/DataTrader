package handlers

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHandleGetClosedStocks(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	insertClosedStock(t, uid, "AAPL", 10, 100, 120, "2025-01-01", "2025-01-15")
	insertClosedStock(t, uid, "AMZN", 5, 100, 90, "2025-01-01", "2025-03-15")
	insertClosedStock(t, other, "GOOG", 5, 100, 90, "2025-01-01", "2025-01-15")

	body := serve(t, HandleGetClosedStocks, req{userID: uid}).Body.String()
	assertContains(t, body, "AAPL", "AMZN")
	assertNotContains(t, body, "GOOG")

	body = serve(t, HandleGetClosedStocks, req{userID: uid, target: "/?search=aap"}).Body.String()
	assertContains(t, body, "AAPL")
	assertNotContains(t, body, "AMZN")

	body = serve(t, HandleGetClosedStocks, req{userID: uid, target: "/?dateFrom=2025-02-01"}).Body.String()
	assertContains(t, body, "AMZN")
	assertNotContains(t, body, "AAPL")
}

func TestHandleGetClosedOptions(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertClosedOption(t, uid, "AAPL", "CC", 185, 240, 0, 240, "2025-01-15")
	insertClosedOption(t, uid, "TSLA", "Call", 250, 500, 700, 200, "2025-03-15")

	body := serve(t, HandleGetClosedOptions, req{userID: uid}).Body.String()
	assertContains(t, body, "AAPL", "TSLA")

	body = serve(t, HandleGetClosedOptions, req{userID: uid, target: "/?type=CC"}).Body.String()
	assertContains(t, body, "AAPL")
	assertNotContains(t, body, "TSLA")

	body = serve(t, HandleGetClosedOptions, req{userID: uid, target: "/?dateTo=2025-02-01"}).Body.String()
	assertContains(t, body, "AAPL")
	assertNotContains(t, body, "TSLA")
}

func TestHandleHistoryFilter(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertClosedStock(t, uid, "AAPL", 10, 100, 120, "2025-01-01", "2025-01-15")
	insertClosedOption(t, uid, "AAPL", "CC", 185, 240, 0, 240, "2025-01-15")
	insertClosedOption(t, uid, "TSLA", "Call", 250, 500, 700, 200, "2025-01-20")

	body := serve(t, HandleHistoryFilter, req{userID: uid}).Body.String()
	assertContains(t, body, "AAPL", "TSLA")

	// Filtering by option type drops closed stocks.
	body = serve(t, HandleHistoryFilter, req{userID: uid, target: "/?type=Call"}).Body.String()
	assertContains(t, body, "TSLA")
	assertNotContains(t, body, "AAPL")

	body = serve(t, HandleHistoryFilter, req{userID: uid, target: "/?search=zzz"}).Body.String()
	assertNotContains(t, body, "AAPL", "TSLA")
}

func TestHandleEditClosedModals(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	cs := insertClosedStock(t, uid, "AAPL", 10, 100, 120, "2025-01-01", "2025-01-15")
	co := insertClosedOption(t, uid, "TSLA", "Call", 250, 500, 700, 200, "2025-01-20")

	body := serve(t, HandleEditClosedStock, req{userID: uid, params: id(cs)}).Body.String()
	assertContains(t, body, "Edit Closed Stock: AAPL", `value="10.00"`, `value="120.00"`, `value="2025-01-15"`)

	body = serve(t, HandleEditClosedOption, req{userID: uid, params: id(co)}).Body.String()
	assertContains(t, body, "Edit Closed Option: TSLA", `value="250.00"`, `value="700.00"`)

	assertStatus(t, serve(t, HandleEditClosedStock, req{userID: other, params: id(cs)}), http.StatusNotFound)
	assertStatus(t, serve(t, HandleEditClosedOption, req{userID: other, params: id(co)}), http.StatusNotFound)
}

func TestHandleUpdateClosedStockRecomputesPL(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	cs := insertClosedStock(t, uid, "AAPL", 10, 100, 120, "2025-01-01", "2025-01-15")

	rec := serve(t, HandleUpdateClosedStock, req{method: http.MethodPost, userID: uid, params: id(cs), form: url.Values{
		"ticker": {"AAPL"}, "quantity": {"20"}, "costBasis": {"50"}, "sellPrice": {"45"},
		"openDate": {"2025-01-02"}, "closeDate": {"2025-01-20"},
	}})
	assertStatus(t, rec, http.StatusOK)
	if rec.Header().Get("HX-Trigger") != "historyUpdated" {
		t.Errorf("HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
	}

	var qty, pl float64
	var closeDate string
	db.QueryRow("SELECT quantity, profit_loss, close_date FROM closed_stocks WHERE id = ?", cs).Scan(&qty, &pl, &closeDate)
	if qty != 20 || pl != -100 || closeDate != "2025-01-20" {
		t.Errorf("row = (qty %v, pl %v, %s), want (20, -100, 2025-01-20)", qty, pl, closeDate)
	}

	serve(t, HandleUpdateClosedStock, req{method: http.MethodPost, userID: other, params: id(cs), form: url.Values{"quantity": {"1"}}})
	db.QueryRow("SELECT quantity FROM closed_stocks WHERE id = ?", cs).Scan(&qty)
	if qty != 20 {
		t.Error("another user was able to update this record")
	}
}

func TestHandleUpdateClosedOptionRecomputesPL(t *testing.T) {
	tests := []struct {
		optionType string
		wantPL     float64
	}{
		{"call", 700 - 500},
		{"put", 700 - 500},
		{"csp", 500 - 700},
		{"cc", 500 - 700},
	}
	for _, tt := range tests {
		t.Run(tt.optionType, func(t *testing.T) {
			setupDB(t)
			uid := createUser(t, "u")
			co := insertClosedOption(t, uid, "TSLA", "Call", 250, 1, 1, 0, "2025-01-20")

			serve(t, HandleUpdateClosedOption, req{method: http.MethodPost, userID: uid, params: id(co), form: url.Values{
				"ticker": {"TSLA"}, "optionType": {tt.optionType}, "strike": {"250"}, "premium": {"500"},
				"price": {"500"}, "collateral": {"0"}, "sellPrice": {"700"},
				"expDate": {"2025-02-21"}, "purchaseDate": {"2025-01-02"}, "closeDate": {"2025-01-20"},
			}})

			var pl float64
			db.QueryRow("SELECT profit_loss FROM closed_options WHERE id = ?", co).Scan(&pl)
			if pl != tt.wantPL {
				t.Errorf("P/L = %v, want %v", pl, tt.wantPL)
			}
		})
	}
}

func TestHandleDeleteClosed(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	cs := insertClosedStock(t, uid, "AAPL", 10, 100, 120, "2025-01-01", "2025-01-15")
	co := insertClosedOption(t, uid, "TSLA", "Call", 250, 500, 700, 200, "2025-01-20")

	serve(t, HandleDeleteClosedStock, req{method: http.MethodDelete, userID: other, params: id(cs)})
	serve(t, HandleDeleteClosedOption, req{method: http.MethodDelete, userID: other, params: id(co)})
	if count(t, "SELECT COUNT(*) FROM closed_stocks") != 1 || count(t, "SELECT COUNT(*) FROM closed_options") != 1 {
		t.Fatal("another user deleted history records")
	}

	rec := serve(t, HandleDeleteClosedStock, req{method: http.MethodDelete, userID: uid, params: id(cs)})
	assertStatus(t, rec, http.StatusOK)
	if rec.Header().Get("HX-Trigger") != "historyUpdated" {
		t.Errorf("HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
	}
	rec = serve(t, HandleDeleteClosedOption, req{method: http.MethodDelete, userID: uid, params: id(co)})
	assertStatus(t, rec, http.StatusOK)

	if count(t, "SELECT COUNT(*) FROM closed_stocks") != 0 || count(t, "SELECT COUNT(*) FROM closed_options") != 0 {
		t.Error("records were not deleted")
	}
}

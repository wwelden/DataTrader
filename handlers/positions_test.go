package handlers

import (
	"net/http"
	"net/url"
	"testing"
)

func stockRow(t *testing.T, id int) (ticker string, qty, cost float64, openDate string) {
	t.Helper()
	if err := db.QueryRow("SELECT ticker, quantity, cost_basis, open_date FROM stock_positions WHERE id = ?", id).Scan(&ticker, &qty, &cost, &openDate); err != nil {
		t.Fatalf("stock position %d: %v", id, err)
	}
	return
}

func TestPositionsPagesRender(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"positions page": HandlePositions,
		"add modal":      HandleModalAddPosition,
		"close modal":    HandleModalClose,
		"import modal":   HandleModalImportCSV,
		"history page":   HandleHistory,
		"home page":      HandleHome,
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(t, h, req{userID: 1})
			assertStatus(t, rec, http.StatusOK)
		})
	}
}

func TestHandleModalAddPositionFields(t *testing.T) {
	stock := serve(t, HandleModalAddPositionFields, req{target: "/?positionType=stock"}).Body.String()
	assertContains(t, stock, `name="costBasis"`)
	assertNotContains(t, stock, `name="strike"`)

	option := serve(t, HandleModalAddPositionFields, req{target: "/?positionType=option"}).Body.String()
	assertContains(t, option, `name="strike"`, `name="premium"`)

	if body := serve(t, HandleModalAddPositionFields, req{target: "/?positionType=bogus"}).Body.String(); body != "" {
		t.Errorf("unknown type should render nothing, got %q", body)
	}
}

func TestHandlersRequireUser(t *testing.T) {
	setupDB(t)
	for name, h := range map[string]http.HandlerFunc{
		"add":              HandleAddPosition,
		"stocks":           HandleGetStockPositions,
		"options":          HandleGetOptionPositions,
		"filter":           HandlePositionsFilter,
		"close":            HandleClosePosition,
		"close option":     HandleCloseOptionModal,
		"close stock post": HandleCloseStockPosition,
		"close opt post":   HandleCloseOptionPosition,
		"edit stock":       HandleEditStockPosition,
		"edit option":      HandleEditOptionPosition,
		"update stock":     HandleUpdateStockPosition,
		"update option":    HandleUpdateOptionPosition,
		"delete stock":     HandleDeleteStockPosition,
		"delete option":    HandleDeleteOptionPosition,
		"closed stocks":    HandleGetClosedStocks,
		"closed options":   HandleGetClosedOptions,
		"history filter":   HandleHistoryFilter,
		"edit closed stk":  HandleEditClosedStock,
		"edit closed opt":  HandleEditClosedOption,
		"upd closed stk":   HandleUpdateClosedStock,
		"upd closed opt":   HandleUpdateClosedOption,
		"del closed stk":   HandleDeleteClosedStock,
		"del closed opt":   HandleDeleteClosedOption,
		"stats":            HandleStats,
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(t, h, req{method: http.MethodPost, form: url.Values{}, params: id(1)})
			assertStatus(t, rec, http.StatusUnauthorized)
		})
	}
}

func TestHandleAddStockPositionNew(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	rec := serve(t, HandleAddPosition, req{method: http.MethodPost, userID: uid, form: url.Values{
		"positionType": {"stock"}, "ticker": {"aapl"}, "quantity": {"10"}, "costBasis": {"150.5"}, "openDate": {"2025-01-02"},
	}})

	assertStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("HX-Trigger"); got != "positionAdded" {
		t.Errorf("HX-Trigger = %q, want positionAdded", got)
	}
	var ticker, openDate string
	var qty, cost float64
	if err := db.QueryRow("SELECT ticker, quantity, cost_basis, open_date FROM stock_positions WHERE user_id = ?", uid).Scan(&ticker, &qty, &cost, &openDate); err != nil {
		t.Fatal(err)
	}
	if ticker != "AAPL" || qty != 10 || cost != 150.5 || openDate != "2025-01-02" {
		t.Errorf("row = (%s, %v, %v, %s), want (AAPL, 10, 150.5, 2025-01-02)", ticker, qty, cost, openDate)
	}
}

func TestHandleAddStockPositionDefaultsOpenDate(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")

	serve(t, HandleAddPosition, req{method: http.MethodPost, userID: uid, form: url.Values{
		"positionType": {"stock"}, "ticker": {"MSFT"}, "quantity": {"1"}, "costBasis": {"1"},
	}})

	var openDate string
	if err := db.QueryRow("SELECT open_date FROM stock_positions WHERE user_id = ?", uid).Scan(&openDate); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDateToTime(openDate); err != nil {
		t.Errorf("default open date %q is not a valid date", openDate)
	}
}

func TestHandleAddStockPositionAveragesCostBasis(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")

	serve(t, HandleAddPosition, req{method: http.MethodPost, userID: uid, form: url.Values{
		"positionType": {"stock"}, "ticker": {"AAPL"}, "quantity": {"30"}, "costBasis": {"200"},
	}})

	if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE user_id = ?", uid); n != 1 {
		t.Fatalf("positions = %d, want 1 (merged)", n)
	}
	_, qty, cost, openDate := stockRow(t, pid)
	if qty != 40 || !approxEqual(cost, 175) {
		t.Errorf("merged = (qty %v, cost %v), want (40, 175)", qty, cost)
	}
	if openDate != "2025-01-01" {
		t.Errorf("open date changed to %q; merging should keep the original", openDate)
	}
}

func TestHandleAddStockPositionIsolatedPerUser(t *testing.T) {
	setupDB(t)
	alice := createUser(t, "alice")
	bob := createUser(t, "bob")
	insertStock(t, alice, "AAPL", 10, 100, "2025-01-01")

	serve(t, HandleAddPosition, req{method: http.MethodPost, userID: bob, form: url.Values{
		"positionType": {"stock"}, "ticker": {"AAPL"}, "quantity": {"5"}, "costBasis": {"200"}, "openDate": {"2025-02-01"},
	}})

	if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE user_id = ?", bob); n != 1 {
		t.Errorf("bob positions = %d, want 1", n)
	}
	var aliceQty float64
	db.QueryRow("SELECT quantity FROM stock_positions WHERE user_id = ?", alice).Scan(&aliceQty)
	if aliceQty != 10 {
		t.Errorf("alice quantity = %v, want 10 (untouched)", aliceQty)
	}
}

func TestHandleAddOptionPositionCollateral(t *testing.T) {
	tests := []struct {
		name           string
		optionType     string
		sharesHeld     float64
		wantCollateral float64
	}{
		{"CSP reserves strike x 100", "CSP", 0, 50 * 100},
		{"CC with 100 shares uses stock cost", "CC", 100, 40 * 100},
		{"CC without enough shares has none", "CC", 50, 0},
		{"long call has none", "Call", 0, 0},
		{"long put has none", "Put", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupDB(t)
			uid := createUser(t, "u")
			if tt.sharesHeld > 0 {
				insertStock(t, uid, "XYZ", tt.sharesHeld, 40, "2025-01-01")
			}

			rec := serve(t, HandleAddPosition, req{method: http.MethodPost, userID: uid, form: url.Values{
				"positionType": {"option"}, "ticker": {"xyz"}, "optionType": {tt.optionType},
				"strike": {"50"}, "premium": {"120"}, "quantity": {"1"},
				"expDate": {"2025-03-21"}, "openDate": {"2025-01-10"},
			}})
			assertStatus(t, rec, http.StatusOK)

			var ticker, kind, exp, purchased string
			var price, premium, strike, collateral, qty float64
			if err := db.QueryRow(`SELECT ticker, type, exp_date, purchase_date, price, premium, strike, collateral, quantity
				FROM option_positions WHERE user_id = ?`, uid).Scan(&ticker, &kind, &exp, &purchased, &price, &premium, &strike, &collateral, &qty); err != nil {
				t.Fatal(err)
			}
			if ticker != "XYZ" || kind != tt.optionType || exp != "2025-03-21" || purchased != "2025-01-10" {
				t.Errorf("row = (%s, %s, %s, %s)", ticker, kind, exp, purchased)
			}
			if price != 120 || premium != 120 || strike != 50 || qty != 1 {
				t.Errorf("numbers = (price %v, premium %v, strike %v, qty %v)", price, premium, strike, qty)
			}
			if collateral != tt.wantCollateral {
				t.Errorf("collateral = %v, want %v", collateral, tt.wantCollateral)
			}
		})
	}
}

func TestHandleGetStockPositionsFilters(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	insertStock(t, uid, "AAPL", 10, 100, "2025-01-05")
	insertStock(t, uid, "AMZN", 5, 100, "2025-03-05")
	insertStock(t, uid, "MSFT", 0, 100, "2025-01-05") // zero quantity is hidden
	insertStock(t, other, "GOOG", 5, 100, "2025-01-05")

	body := serve(t, HandleGetStockPositions, req{userID: uid}).Body.String()
	assertContains(t, body, "AAPL", "AMZN", "01/05/25")
	assertNotContains(t, body, "MSFT", "GOOG")

	body = serve(t, HandleGetStockPositions, req{userID: uid, target: "/?search=am"}).Body.String()
	assertContains(t, body, "AMZN")
	assertNotContains(t, body, "AAPL")

	body = serve(t, HandleGetStockPositions, req{userID: uid, target: "/?dateFrom=2025-02-01&dateTo=2025-12-31"}).Body.String()
	assertContains(t, body, "AMZN")
	assertNotContains(t, body, "AAPL")
}

func TestHandleGetOptionPositionsFilters(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertOption(t, uid, "AAPL", "CSP", 150, 200, 15000, 1, "2025-02-21", "2025-01-05")
	insertOption(t, uid, "TSLA", "Call", 250, 500, 0, 2, "2025-03-21", "2025-02-05")

	body := serve(t, HandleGetOptionPositions, req{userID: uid}).Body.String()
	assertContains(t, body, "AAPL", "TSLA")

	body = serve(t, HandleGetOptionPositions, req{userID: uid, target: "/?type=Call"}).Body.String()
	assertContains(t, body, "TSLA")
	assertNotContains(t, body, "AAPL")

	body = serve(t, HandleGetOptionPositions, req{userID: uid, target: "/?search=aap"}).Body.String()
	assertContains(t, body, "AAPL")
	assertNotContains(t, body, "TSLA")

	body = serve(t, HandleGetOptionPositions, req{userID: uid, target: "/?dateTo=2025-01-31"}).Body.String()
	assertContains(t, body, "AAPL")
	assertNotContains(t, body, "TSLA")
}

func TestHandlePositionsFilter(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertStock(t, uid, "AAPL", 10, 100, "2025-01-05")
	insertOption(t, uid, "AAPL", "CC", 160, 200, 0, 1, "2025-02-21", "2025-01-06")
	insertOption(t, uid, "TSLA", "Put", 200, 300, 0, 1, "2025-02-21", "2025-01-06")

	body := serve(t, HandlePositionsFilter, req{userID: uid}).Body.String()
	assertContains(t, body, "Stock Positions", "Option Positions", "AAPL", "TSLA", "$100.00")

	// An option type filter hides stocks entirely.
	body = serve(t, HandlePositionsFilter, req{userID: uid, target: "/?type=Put"}).Body.String()
	assertContains(t, body, "No stock positions found.", "TSLA")
	assertNotContains(t, body, "AAPL")

	body = serve(t, HandlePositionsFilter, req{userID: uid, target: "/?search=zzz"}).Body.String()
	assertContains(t, body, "No stock positions found.", "No option positions found.")
}

func TestHandlePositionsFilterEscapesTicker(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertStock(t, uid, "<script>x</script>", 1, 1, "2025-01-05")

	body := serve(t, HandlePositionsFilter, req{userID: uid}).Body.String()
	assertNotContains(t, body, "<script>x</script>")
	assertContains(t, body, "&lt;script&gt;")
}

func TestHandleClosePositionModalStock(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	stock := insertStock(t, uid, "AAPL", 25, 100, "2025-01-05")
	foreign := insertStock(t, other, "GOOG", 1, 1, "2025-01-05")

	rec := serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(stock)})
	assertStatus(t, rec, http.StatusOK)
	assertContains(t, rec.Body.String(), "Close Stock Position: AAPL", "Available: 25.00", `name="sellPrice"`)
	if rec.Header().Get("HX-Retarget") != "#modal-container" {
		t.Error("stock close modal should retarget the modal container")
	}

	assertStatus(t, serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(foreign)}), http.StatusNotFound)
	assertStatus(t, serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(9999)}), http.StatusNotFound)
	assertStatus(t, serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid}), http.StatusBadRequest)
}

// HandleClosePosition falls back to option_positions when no stock matches.
// Stocks and options have separate id sequences, so these tests keep the
// stock table empty to avoid an id collision.
func TestHandleClosePositionModalOptions(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	cc := insertOption(t, uid, "AAPL", "CC", 160, 200, 0, 1, "2025-02-21", "2025-01-06")
	csp := insertOption(t, uid, "AMD", "CSP", 120, 300, 12000, 1, "2025-02-21", "2025-01-06")
	call := insertOption(t, uid, "NVDA", "Call", 130, 450, 0, 1, "2025-02-21", "2025-01-06")

	body := serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(cc)}).Body.String()
	assertContains(t, body, "AAPL CC", `value="called_away"`)
	assertNotContains(t, body, `value="assigned"`)

	body = serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(csp)}).Body.String()
	assertContains(t, body, "AMD CSP", `value="assigned"`)
	assertNotContains(t, body, `value="called_away"`)

	body = serve(t, HandleClosePosition, req{method: http.MethodPost, userID: uid, params: id(call)}).Body.String()
	assertContains(t, body, "NVDA Call", `name="outcome" value="closed"`, "450.00")
}

func TestHandleCloseOptionModal(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	csp := insertOption(t, uid, "AMD", "CSP", 120, 300, 24000, 2, "2025-02-21", "2025-01-06")
	put := insertOption(t, uid, "SPY", "Put", 500, 640, 0, 3, "2025-02-21", "2025-01-06")

	body := serve(t, HandleCloseOptionModal, req{method: http.MethodPost, userID: uid, params: id(csp)}).Body.String()
	assertContains(t, body, "AMD CSP", "Available: 2", `value="assigned"`)

	body = serve(t, HandleCloseOptionModal, req{method: http.MethodPost, userID: uid, params: id(put)}).Body.String()
	assertContains(t, body, "SPY Put", "Available: 3", `name="sellPrice"`)

	assertStatus(t, serve(t, HandleCloseOptionModal, req{method: http.MethodPost, userID: uid, params: id(999)}), http.StatusNotFound)
	assertStatus(t, serve(t, HandleCloseOptionModal, req{method: http.MethodPost, userID: uid}), http.StatusBadRequest)
}

func TestHandleOutcomeFields(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	pid := insertOption(t, uid, "AAPL", "CC", 160, 200, 0, 1, "2025-02-21", "2025-01-06")

	tests := []struct {
		outcome string
		want    []string
		absent  []string
	}{
		{"expired", []string{`name="sellPrice" value="0"`, `name="closeDate" value="2025-02-21"`}, []string{"sharePrice"}},
		{"called_away", []string{`name="sharePrice"`, "160.00", `value="2025-02-21"`}, nil},
		{"assigned", []string{`name="sellPrice" value="0"`, `value="2025-02-21"`}, []string{"sharePrice"}},
		{"closed", []string{"Buy to Close Price", "200.00"}, []string{"sharePrice"}},
	}
	for _, tt := range tests {
		t.Run(tt.outcome, func(t *testing.T) {
			body := serve(t, HandleOutcomeFields, req{userID: uid, target: "/?outcome=" + tt.outcome, params: id(pid)}).Body.String()
			assertContains(t, body, tt.want...)
			assertNotContains(t, body, tt.absent...)
		})
	}

	empties := map[string]req{
		"unknown outcome":  {userID: uid, target: "/?outcome=bogus", params: id(pid)},
		"missing position": {userID: uid, target: "/?outcome=expired", params: id(999)},
		"no id":            {userID: uid, target: "/?outcome=expired"},
		"no user":          {target: "/?outcome=expired", params: id(pid)},
	}
	for name, rq := range empties {
		t.Run(name, func(t *testing.T) {
			if body := serve(t, HandleOutcomeFields, rq).Body.String(); body != "" {
				t.Errorf("want empty body, got %q", body)
			}
		})
	}
}

func TestHandleCloseStockPosition(t *testing.T) {
	type closedRow struct {
		qty, cost, sold, pl float64
		closeDate           string
	}
	readClosed := func(t *testing.T, uid int) closedRow {
		t.Helper()
		var r closedRow
		if err := db.QueryRow("SELECT quantity, cost_basis, sell_price, profit_loss, close_date FROM closed_stocks WHERE user_id = ?", uid).
			Scan(&r.qty, &r.cost, &r.sold, &r.pl, &r.closeDate); err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("full close removes the position", func(t *testing.T) {
		setupDB(t)
		uid := createUser(t, "u")
		pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")

		rec := serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"quantity": {"10"}, "sellPrice": {"125"}, "closeDate": {"2025-02-01"},
		}})
		assertStatus(t, rec, http.StatusOK)
		if rec.Header().Get("HX-Trigger") != "positionClosed" {
			t.Errorf("HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
		}
		if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", pid); n != 0 {
			t.Error("position should be deleted")
		}
		got := readClosed(t, uid)
		if got != (closedRow{10, 100, 125, 250, "2025-02-01"}) {
			t.Errorf("closed = %+v", got)
		}
	})

	t.Run("partial close keeps the rest open", func(t *testing.T) {
		setupDB(t)
		uid := createUser(t, "u")
		pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")

		serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"quantity": {"4"}, "sellPrice": {"90"}, "closeDate": {"2025-02-01"},
		}})
		_, qty, _, _ := stockRow(t, pid)
		if qty != 6 {
			t.Errorf("remaining = %v, want 6", qty)
		}
		if got := readClosed(t, uid); got.qty != 4 || got.pl != -40 {
			t.Errorf("closed = %+v, want qty 4, pl -40", got)
		}
	})

	t.Run("second partial close merges into the same closed record", func(t *testing.T) {
		setupDB(t)
		uid := createUser(t, "u")
		pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")

		serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"quantity": {"4"}, "sellPrice": {"110"}, "closeDate": {"2025-02-01"},
		}})
		serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"quantity": {"6"}, "sellPrice": {"120"}, "closeDate": {"2025-03-01"},
		}})

		if n := count(t, "SELECT COUNT(*) FROM closed_stocks WHERE user_id = ?", uid); n != 1 {
			t.Fatalf("closed rows = %d, want 1", n)
		}
		got := readClosed(t, uid)
		if got.qty != 10 || !approxEqual(got.sold, 116) || !approxEqual(got.pl, 40+120) || got.closeDate != "2025-03-01" {
			t.Errorf("merged = %+v, want qty 10, avg sell 116, pl 160, latest close date", got)
		}
		if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", pid); n != 0 {
			t.Error("position should be fully closed")
		}
	})

	quantityCases := map[string]string{"blank": "", "zero": "0", "negative": "-3", "too many": "50"}
	for name, q := range quantityCases {
		t.Run("quantity "+name+" closes everything", func(t *testing.T) {
			setupDB(t)
			uid := createUser(t, "u")
			pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-01")

			serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
				"quantity": {q}, "sellPrice": {"100"},
			}})
			if got := readClosed(t, uid); got.qty != 10 {
				t.Errorf("closed qty = %v, want 10", got.qty)
			}
			if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", pid); n != 0 {
				t.Error("position should be gone")
			}
		})
	}

	t.Run("cannot close another user's position", func(t *testing.T) {
		setupDB(t)
		owner := createUser(t, "owner")
		intruder := createUser(t, "intruder")
		pid := insertStock(t, owner, "AAPL", 10, 100, "2025-01-01")

		rec := serve(t, HandleCloseStockPosition, req{method: http.MethodPost, userID: intruder, params: id(pid), form: url.Values{"sellPrice": {"1"}}})
		assertStatus(t, rec, http.StatusNotFound)
		if _, qty, _, _ := stockRow(t, pid); qty != 10 {
			t.Error("owner's position was modified")
		}
	})
}

func readClosedOption(t *testing.T, uid int) (kind string, qty, collateral, sold, pl float64, closeDate string) {
	t.Helper()
	if err := db.QueryRow("SELECT type, quantity, collateral, sell_price, profit_loss, close_date FROM closed_options WHERE user_id = ?", uid).
		Scan(&kind, &qty, &collateral, &sold, &pl, &closeDate); err != nil {
		t.Fatal(err)
	}
	return
}

func TestHandleCloseOptionPositionExpired(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	pid := insertOption(t, uid, "AMD", "CSP", 120, 300, 12000, 1, "2025-02-21", "2025-01-06")

	rec := serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"expired"}, "quantity": {"1"}, "sellPrice": {"99"}, "closeDate": {"2025-01-30"},
	}})
	assertStatus(t, rec, http.StatusOK)

	kind, qty, collateral, sold, pl, closeDate := readClosedOption(t, uid)
	if kind != "CSP" || qty != 1 || collateral != 12000 || sold != 0 || pl != 300 {
		t.Errorf("closed = (%s, %v, %v, %v, %v)", kind, qty, collateral, sold, pl)
	}
	if closeDate != "2025-02-21" {
		t.Errorf("close date = %q, want expiration date", closeDate)
	}
	if n := count(t, "SELECT COUNT(*) FROM option_positions WHERE id = ?", pid); n != 0 {
		t.Error("expired option should be removed")
	}
}

func TestHandleCloseOptionPositionPartialClosedCall(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	pid := insertOption(t, uid, "NVDA", "Call", 130, 400, 900, 3, "2025-02-21", "2025-01-06")

	serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"closed"}, "quantity": {"1"}, "sellPrice": {"650"}, "closeDate": {"2025-01-20"},
	}})

	kind, qty, collateral, sold, pl, closeDate := readClosedOption(t, uid)
	if kind != "Call" || qty != 1 || collateral != 300 || sold != 650 || pl != 250 || closeDate != "2025-01-20" {
		t.Errorf("closed = (%s, qty %v, coll %v, sold %v, pl %v, %s)", kind, qty, collateral, sold, pl, closeDate)
	}
	var remainingQty, remainingCollateral float64
	db.QueryRow("SELECT quantity, collateral FROM option_positions WHERE id = ?", pid).Scan(&remainingQty, &remainingCollateral)
	if remainingQty != 2 || remainingCollateral != 600 {
		t.Errorf("remaining = (qty %v, coll %v), want (2, 600)", remainingQty, remainingCollateral)
	}
}

func TestHandleCloseOptionPositionShortClosedForLoss(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	pid := insertOption(t, uid, "AMD", "CSP", 120, 300, 24000, 2, "2025-02-21", "2025-01-06")

	serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"closed"}, "quantity": {"2"}, "sellPrice": {"500"}, "closeDate": {"2025-01-20"},
	}})

	if _, _, _, _, pl, _ := readClosedOption(t, uid); pl != -400 {
		t.Errorf("P/L = %v, want (300-500)*2 = -400", pl)
	}
}

func TestHandleCloseOptionPositionCalledAway(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	stock := insertStock(t, uid, "AAPL", 150, 140, "2024-12-01")
	pid := insertOption(t, uid, "AAPL", "CC", 160, 200, 14000, 1, "2025-02-21", "2025-01-06")

	rec := serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"called_away"}, "quantity": {"1"}, "sharePrice": {"160"}, "closeDate": {"2025-02-21"},
	}})
	assertStatus(t, rec, http.StatusOK)

	var shares, cost, sold, pl float64
	var openDate string
	if err := db.QueryRow("SELECT quantity, cost_basis, sell_price, profit_loss, open_date FROM closed_stocks WHERE user_id = ?", uid).
		Scan(&shares, &cost, &sold, &pl, &openDate); err != nil {
		t.Fatal(err)
	}
	if shares != 100 || cost != 140 || sold != 160 || pl != 2000 || openDate != "2024-12-01" {
		t.Errorf("closed stock = (%v sh, cost %v, sold %v, pl %v, %s)", shares, cost, sold, pl, openDate)
	}
	if _, qty, _, _ := stockRow(t, stock); qty != 50 {
		t.Errorf("remaining shares = %v, want 50", qty)
	}
	if _, _, _, _, optPL, _ := readClosedOption(t, uid); optPL != 200 {
		t.Errorf("option P/L = %v, want full premium 200", optPL)
	}
}

func TestHandleCloseOptionPositionCalledAwayAllShares(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	stock := insertStock(t, uid, "AAPL", 100, 140, "2024-12-01")
	pid := insertOption(t, uid, "AAPL", "CC", 160, 200, 14000, 1, "2025-02-21", "2025-01-06")

	serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"called_away"}, "quantity": {"1"}, "sharePrice": {"160"}, "closeDate": {"2025-02-21"},
	}})

	if n := count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", stock); n != 0 {
		t.Error("stock position should be removed when all shares are called away")
	}
}

func TestHandleCloseOptionPositionCalledAwayWithoutShares(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	insertStock(t, uid, "AAPL", 50, 140, "2024-12-01")
	pid := insertOption(t, uid, "AAPL", "CC", 160, 200, 0, 1, "2025-02-21", "2025-01-06")

	rec := serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"outcome": {"called_away"}, "quantity": {"1"}, "sharePrice": {"160"},
	}})
	assertStatus(t, rec, http.StatusOK)

	if n := count(t, "SELECT COUNT(*) FROM closed_stocks WHERE user_id = ?", uid); n != 0 {
		t.Error("no stock should be sold when fewer than 100 shares are held")
	}
	if n := count(t, "SELECT COUNT(*) FROM closed_options WHERE user_id = ?", uid); n != 1 {
		t.Error("the option itself should still be closed")
	}
}

func TestHandleCloseOptionPositionAssigned(t *testing.T) {
	t.Run("creates a new stock position at the strike", func(t *testing.T) {
		setupDB(t)
		uid := createUser(t, "u")
		pid := insertOption(t, uid, "AMD", "CSP", 120, 300, 24000, 2, "2025-02-21", "2025-01-06")

		serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"outcome": {"assigned"}, "quantity": {"2"}, "closeDate": {"2025-02-21"},
		}})

		var qty, cost float64
		var openDate string
		if err := db.QueryRow("SELECT quantity, cost_basis, open_date FROM stock_positions WHERE user_id = ? AND ticker = 'AMD'", uid).Scan(&qty, &cost, &openDate); err != nil {
			t.Fatal(err)
		}
		if qty != 200 || cost != 120 || openDate != "2025-02-21" {
			t.Errorf("stock = (%v, %v, %s), want (200, 120, 2025-02-21)", qty, cost, openDate)
		}
		if _, _, _, sold, pl, _ := readClosedOption(t, uid); sold != 0 || pl != 600 {
			t.Errorf("option = (sold %v, pl %v), want (0, 600)", sold, pl)
		}
	})

	t.Run("averages into an existing stock position", func(t *testing.T) {
		setupDB(t)
		uid := createUser(t, "u")
		stock := insertStock(t, uid, "AMD", 100, 100, "2024-12-01")
		pid := insertOption(t, uid, "AMD", "CSP", 120, 300, 12000, 1, "2025-02-21", "2025-01-06")

		serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"outcome": {"assigned"}, "quantity": {"1"},
		}})

		_, qty, cost, _ := stockRow(t, stock)
		if qty != 200 || !approxEqual(cost, 110) {
			t.Errorf("stock = (%v, %v), want (200, 110)", qty, cost)
		}
	})
}

func TestHandleCloseOptionPositionRejectsBadInput(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	pid := insertOption(t, uid, "AMD", "CSP", 120, 300, 12000, 1, "2025-02-21", "2025-01-06")

	for _, q := range []string{"", "0", "-1", "2"} {
		rec := serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
			"outcome": {"closed"}, "quantity": {q}, "sellPrice": {"1"},
		}})
		assertStatus(t, rec, http.StatusBadRequest)
	}

	rec := serve(t, HandleCloseOptionPosition, req{method: http.MethodPost, userID: other, params: id(pid), form: url.Values{
		"outcome": {"closed"}, "quantity": {"1"},
	}})
	assertStatus(t, rec, http.StatusNotFound)

	if n := count(t, "SELECT COUNT(*) FROM closed_options"); n != 0 {
		t.Errorf("closed options = %d, want 0", n)
	}
}

func TestHandleEditPositionModals(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	stock := insertStock(t, uid, "AAPL", 12.5, 101.25, "2025-01-05")
	opt := insertOption(t, uid, "TSLA", "Put", 250, 500, 0, 1, "2025-03-21", "2025-01-06")

	body := serve(t, HandleEditStockPosition, req{userID: uid, params: id(stock)}).Body.String()
	assertContains(t, body, "Edit Stock Position: AAPL", `value="12.50"`, `value="101.25"`, `value="2025-01-05"`)

	body = serve(t, HandleEditOptionPosition, req{userID: uid, params: id(opt)}).Body.String()
	assertContains(t, body, "Edit Option Position: TSLA", `value="250.00"`, `value="500.00"`, `value="2025-03-21"`)

	assertStatus(t, serve(t, HandleEditStockPosition, req{userID: uid, params: id(999)}), http.StatusNotFound)
	assertStatus(t, serve(t, HandleEditOptionPosition, req{userID: uid, params: id(999)}), http.StatusNotFound)
}

func TestHandleUpdateStockPosition(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	pid := insertStock(t, uid, "AAPL", 10, 100, "2025-01-05")

	rec := serve(t, HandleUpdateStockPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"ticker": {"msft"}, "quantity": {"7"}, "costBasis": {"300"}, "openDate": {"2025-02-02"},
	}})
	assertStatus(t, rec, http.StatusOK)
	ticker, qty, cost, openDate := stockRow(t, pid)
	if ticker != "MSFT" || qty != 7 || cost != 300 || openDate != "2025-02-02" {
		t.Errorf("row = (%s, %v, %v, %s)", ticker, qty, cost, openDate)
	}

	serve(t, HandleUpdateStockPosition, req{method: http.MethodPost, userID: other, params: id(pid), form: url.Values{
		"ticker": {"HACK"}, "quantity": {"1"}, "costBasis": {"1"}, "openDate": {"2025-01-01"},
	}})
	if ticker, _, _, _ := stockRow(t, pid); ticker != "MSFT" {
		t.Error("another user was able to update this position")
	}
}

func TestHandleUpdateOptionPosition(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	pid := insertOption(t, uid, "TSLA", "Put", 250, 500, 0, 1, "2025-03-21", "2025-01-06")

	serve(t, HandleUpdateOptionPosition, req{method: http.MethodPost, userID: uid, params: id(pid), form: url.Values{
		"ticker": {"nvda"}, "optionType": {"Call"}, "strike": {"140"}, "premium": {"320"}, "price": {"330"},
		"expDate": {"2025-04-17"}, "purchaseDate": {"2025-01-10"},
	}})
	var ticker, kind, exp, purchased string
	var strike, premium, price float64
	db.QueryRow("SELECT ticker, type, strike, premium, price, exp_date, purchase_date FROM option_positions WHERE id = ?", pid).
		Scan(&ticker, &kind, &strike, &premium, &price, &exp, &purchased)
	if ticker != "NVDA" || kind != "Call" || strike != 140 || premium != 320 || price != 330 || exp != "2025-04-17" || purchased != "2025-01-10" {
		t.Errorf("row = (%s %s %v %v %v %s %s)", ticker, kind, strike, premium, price, exp, purchased)
	}

	serve(t, HandleUpdateOptionPosition, req{method: http.MethodPost, userID: other, params: id(pid), form: url.Values{"ticker": {"HACK"}}})
	db.QueryRow("SELECT ticker FROM option_positions WHERE id = ?", pid).Scan(&ticker)
	if ticker != "NVDA" {
		t.Error("another user was able to update this option")
	}
}

func TestHandleDeletePositions(t *testing.T) {
	setupDB(t)
	uid := createUser(t, "u")
	other := createUser(t, "other")
	stock := insertStock(t, uid, "AAPL", 10, 100, "2025-01-05")
	keep := insertStock(t, uid, "MSFT", 10, 100, "2025-01-05")
	opt := insertOption(t, uid, "TSLA", "Put", 250, 500, 0, 1, "2025-03-21", "2025-01-06")

	// Another user's delete is a no-op.
	serve(t, HandleDeleteStockPosition, req{method: http.MethodDelete, userID: other, params: id(stock)})
	serve(t, HandleDeleteOptionPosition, req{method: http.MethodDelete, userID: other, params: id(opt)})
	if count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", stock) != 1 || count(t, "SELECT COUNT(*) FROM option_positions WHERE id = ?", opt) != 1 {
		t.Fatal("another user deleted this user's positions")
	}

	rec := serve(t, HandleDeleteStockPosition, req{method: http.MethodDelete, userID: uid, params: id(stock)})
	assertStatus(t, rec, http.StatusOK)
	if rec.Header().Get("HX-Trigger") != "positionDeleted" {
		t.Errorf("HX-Trigger = %q", rec.Header().Get("HX-Trigger"))
	}
	assertContains(t, rec.Body.String(), "MSFT")
	assertNotContains(t, rec.Body.String(), "AAPL")
	if count(t, "SELECT COUNT(*) FROM stock_positions WHERE id = ?", keep) != 1 {
		t.Error("unrelated position was deleted")
	}

	rec = serve(t, HandleDeleteOptionPosition, req{method: http.MethodDelete, userID: uid, params: id(opt)})
	assertStatus(t, rec, http.StatusOK)
	if count(t, "SELECT COUNT(*) FROM option_positions WHERE id = ?", opt) != 0 {
		t.Error("option was not deleted")
	}
}

func TestJSONError(t *testing.T) {
	rec := serve(t, func(w http.ResponseWriter, r *http.Request) { jsonError(w, "nope", http.StatusTeapot) }, req{})
	assertStatus(t, rec, http.StatusTeapot)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body := rec.Body.String(); body != "{\"error\":\"nope\"}\n" {
		t.Errorf("body = %q", body)
	}
}

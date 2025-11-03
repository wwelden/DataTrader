package handlers

import (
	"backend/types"
	"backend/views/components"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func HandleHistory(w http.ResponseWriter, r *http.Request) {
	components.AppLayout("History - DATATRADER", "history", components.HistoryPage()).Render(r.Context(), w)
}

func HandleGetClosedStocks(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	query := `SELECT id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss FROM closed_stocks WHERE user_id = ?`
	args := []interface{}{userID}

	if search != "" {
		query += ` AND ticker LIKE ?`
		args = append(args, "%"+search+"%")
	}

	query += ` ORDER BY close_date DESC`

	rows, err := db.Query(query, args...)

	if err != nil {
		http.Error(w, "Failed to fetch closed stocks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var closedStocks []types.ClosedStock
	for rows.Next() {
		var cs types.ClosedStock
		if err := rows.Scan(&cs.ID, &cs.Ticker, &cs.OpenDate, &cs.CloseDate, &cs.Quantity, &cs.CostBasis, &cs.SellPrice, &cs.ProfitLoss); err != nil {
			continue
		}
		if IsDateInRange(cs.CloseDate, dateFromInput, dateToInput) {
			closedStocks = append(closedStocks, cs)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	components.ClosedStocksTable(closedStocks, FormatDate).Render(r.Context(), w)
}

func HandleHistoryFilter(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	optionType := r.URL.Query().Get("type")
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	var closedStocks []types.ClosedStock

	if optionType == "" {
		stockQuery := `SELECT id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss FROM closed_stocks WHERE user_id = ?`
		stockArgs := []interface{}{userID}

		if search != "" {
			stockQuery += ` AND ticker LIKE ?`
			stockArgs = append(stockArgs, "%"+search+"%")
		}
		stockQuery += ` ORDER BY close_date DESC`

		stockRows, err := db.Query(stockQuery, stockArgs...)
		if err != nil {
			http.Error(w, "Failed to fetch closed stocks", http.StatusInternalServerError)
			return
		}
		defer stockRows.Close()

		for stockRows.Next() {
			var cs types.ClosedStock
			if err := stockRows.Scan(&cs.ID, &cs.Ticker, &cs.OpenDate, &cs.CloseDate, &cs.Quantity, &cs.CostBasis, &cs.SellPrice, &cs.ProfitLoss); err != nil {
				continue
			}
			if IsDateInRange(cs.CloseDate, dateFromInput, dateToInput) {
				closedStocks = append(closedStocks, cs)
			}
		}
	}

	optionQuery := `SELECT id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss FROM closed_options WHERE user_id = ?`
	optionArgs := []interface{}{userID}

	if search != "" {
		optionQuery += ` AND ticker LIKE ?`
		optionArgs = append(optionArgs, "%"+search+"%")
	}
	if optionType != "" {
		optionQuery += ` AND type = ?`
		optionArgs = append(optionArgs, optionType)
	}
	optionQuery += ` ORDER BY close_date DESC`

	optionRows, err := db.Query(optionQuery, optionArgs...)
	if err != nil {
		http.Error(w, "Failed to fetch closed options", http.StatusInternalServerError)
		return
	}
	defer optionRows.Close()

	var closedOptions []types.ClosedOption
	for optionRows.Next() {
		var co types.ClosedOption
		if err := optionRows.Scan(&co.ID, &co.Ticker, &co.Price, &co.Premium, &co.Strike, &co.ExpDate, &co.Type, &co.Collateral, &co.Quantity, &co.PurchaseDate, &co.CloseDate, &co.SellPrice, &co.ProfitLoss); err != nil {
			continue
		}
		if IsDateInRange(co.CloseDate, dateFromInput, dateToInput) {
			closedOptions = append(closedOptions, co)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	components.FilteredHistory(closedStocks, closedOptions, FormatDate).Render(r.Context(), w)
}

func HandleEditClosedStock(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker, openDate, closeDate string
	var quantity, costBasis, sellPrice, profitLoss float64

	err := db.QueryRow(`
		SELECT ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &openDate, &closeDate, &quantity, &costBasis, &sellPrice, &profitLoss)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	modalHTML := fmt.Sprintf(`
		<div class="modal">
			<div class="modal-content">
				<div class="modal-header">
					<h3>Edit Closed Stock: %s</h3>
				</div>
				<form hx-post="/api/history/update-stock/%s" hx-target="#modal-container" hx-swap="innerHTML">
					<div class="form-group">
						<label>Ticker</label>
						<input type="text" name="ticker" value="%s" required />
					</div>
					<div class="form-group">
						<label>Quantity</label>
						<input type="number" name="quantity" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Cost Basis</label>
						<input type="number" name="costBasis" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Sell Price</label>
						<input type="number" name="sellPrice" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Open Date</label>
						<input type="date" name="openDate" value="%s" required />
					</div>
					<div class="form-group">
						<label>Close Date</label>
						<input type="date" name="closeDate" value="%s" required />
					</div>
					<div class="form-actions">
						<button type="submit" class="btn btn-primary">Update</button>
						<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
					</div>
				</form>
			</div>
		</div>
	`, ticker, positionID, ticker, quantity, costBasis, sellPrice, openDate, closeDate)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

func HandleUpdateClosedStock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ticker := r.FormValue("ticker")
	quantity, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costBasis, _ := strconv.ParseFloat(r.FormValue("costBasis"), 64)
	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	openDate := r.FormValue("openDate")
	closeDate := r.FormValue("closeDate")

	profitLoss := (sellPrice - costBasis) * quantity

	_, err := db.Exec(`
		UPDATE closed_stocks
		SET ticker = ?, open_date = ?, close_date = ?, quantity = ?, cost_basis = ?, sell_price = ?, profit_loss = ?
		WHERE id = ? AND user_id = ?
	`, ticker, openDate, closeDate, quantity, costBasis, sellPrice, profitLoss, positionID, userID)

	if err != nil {
		http.Error(w, "Failed to update position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "historyUpdated")
	components.ModalClose().Render(r.Context(), w)
}

func HandleDeleteClosedStock(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM closed_stocks WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "historyUpdated")
	HandleGetClosedStocks(w, r)
}

func HandleEditClosedOption(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker, expDate, purchaseDate, closeDate, optionType string
	var price, premium, strike, collateral, sellPrice, profitLoss float64

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date, close_date, sell_price, profit_loss
		FROM closed_options
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate, &closeDate, &sellPrice, &profitLoss)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	modalHTML := fmt.Sprintf(`
		<div class="modal">
			<div class="modal-content">
				<div class="modal-header">
					<h3>Edit Closed Option: %s</h3>
				</div>
				<form hx-post="/api/history/update-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
					<div class="form-group">
						<label>Ticker</label>
						<input type="text" name="ticker" value="%s" required />
					</div>
					<div class="form-group">
						<label>Type</label>
						<select name="optionType" required>
							<option value="call" %s>Call</option>
							<option value="put" %s>Put</option>
							<option value="csp" %s>CSP</option>
							<option value="cc" %s>CC</option>
						</select>
					</div>
					<div class="form-group">
						<label>Strike Price</label>
						<input type="number" name="strike" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Premium</label>
						<input type="number" name="premium" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Price</label>
						<input type="number" name="price" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Collateral</label>
						<input type="number" name="collateral" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Sell Price</label>
						<input type="number" name="sellPrice" step="0.01" value="%.2f" required />
					</div>
					<div class="form-group">
						<label>Expiration Date</label>
						<input type="date" name="expDate" value="%s" required />
					</div>
					<div class="form-group">
						<label>Purchase Date</label>
						<input type="date" name="purchaseDate" value="%s" required />
					</div>
					<div class="form-group">
						<label>Close Date</label>
						<input type="date" name="closeDate" value="%s" required />
					</div>
					<div class="form-actions">
						<button type="submit" class="btn btn-primary">Update</button>
						<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
					</div>
				</form>
			</div>
		</div>
	`, ticker, positionID, ticker,
		selected(optionType, "call"), selected(optionType, "put"),
		selected(optionType, "csp"), selected(optionType, "cc"),
		strike, premium, price, collateral, sellPrice, expDate, purchaseDate, closeDate)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

func HandleUpdateClosedOption(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ticker := r.FormValue("ticker")
	optionType := r.FormValue("optionType")
	strike, _ := strconv.ParseFloat(r.FormValue("strike"), 64)
	premium, _ := strconv.ParseFloat(r.FormValue("premium"), 64)
	price, _ := strconv.ParseFloat(r.FormValue("price"), 64)
	collateral, _ := strconv.ParseFloat(r.FormValue("collateral"), 64)
	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	expDate := r.FormValue("expDate")
	purchaseDate := r.FormValue("purchaseDate")
	closeDate := r.FormValue("closeDate")

	var profitLoss float64
	switch optionType {
	case "call", "put":
		profitLoss = sellPrice - premium
	case "csp", "cc":
		profitLoss = premium - sellPrice
	}

	_, err := db.Exec(`
		UPDATE closed_options
		SET ticker = ?, type = ?, strike = ?, premium = ?, price = ?, collateral = ?, sell_price = ?,
		    exp_date = ?, purchase_date = ?, close_date = ?, profit_loss = ?
		WHERE id = ? AND user_id = ?
	`, ticker, optionType, strike, premium, price, collateral, sellPrice, expDate, purchaseDate, closeDate, profitLoss, positionID, userID)

	if err != nil {
		http.Error(w, "Failed to update position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "historyUpdated")
	components.ModalClose().Render(r.Context(), w)
}

func HandleDeleteClosedOption(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM closed_options WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "historyUpdated")
	HandleGetClosedOptions(w, r)
}

func HandleGetClosedOptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	optionType := r.URL.Query().Get("type")
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	query := `SELECT id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss FROM closed_options WHERE user_id = ?`
	args := []interface{}{userID}

	if search != "" {
		query += ` AND ticker LIKE ?`
		args = append(args, "%"+search+"%")
	}
	if optionType != "" {
		query += ` AND type = ?`
		args = append(args, optionType)
	}

	query += ` ORDER BY close_date DESC`

	rows, err := db.Query(query, args...)

	if err != nil {
		http.Error(w, "Failed to fetch closed options", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var closedOptions []types.ClosedOption
	for rows.Next() {
		var co types.ClosedOption
		if err := rows.Scan(&co.ID, &co.Ticker, &co.Price, &co.Premium, &co.Strike, &co.ExpDate, &co.Type, &co.Collateral, &co.Quantity, &co.PurchaseDate, &co.CloseDate, &co.SellPrice, &co.ProfitLoss); err != nil {
			continue
		}
		if IsDateInRange(co.CloseDate, dateFromInput, dateToInput) {
			closedOptions = append(closedOptions, co)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	components.ClosedOptionsTable(closedOptions, FormatDate).Render(r.Context(), w)
}

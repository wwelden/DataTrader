package handlers

import (
	"backend/types"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func HandleHistory(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "history.html"))
}

func HandleGetClosedStocks(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.Query(`
		SELECT id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE user_id = ?
		ORDER BY close_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch closed stocks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ClosedStockWithID struct {
		types.ClosedStock
		ID int
	}

	var closedStocks []ClosedStockWithID
	for rows.Next() {
		var cs ClosedStockWithID
		if err := rows.Scan(&cs.ID, &cs.Ticker, &cs.OpenDate, &cs.CloseDate, &cs.Quantity, &cs.CostBasis, &cs.SellPrice, &cs.ProfitLoss); err != nil {
			continue
		}
		closedStocks = append(closedStocks, cs)
	}

	if len(closedStocks) == 0 {
		w.Write([]byte(`<div id="closed-stocks-list" hx-get="/api/history/stocks" hx-trigger="historyUpdated from:body" hx-swap="outerHTML"><p class="empty-state">No closed stock trades found.</p></div>`))
		return
	}

	htmlContent := `<div id="closed-stocks-list" hx-get="/api/history/stocks" hx-trigger="historyUpdated from:body" hx-swap="outerHTML"><table class="history-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Open Date</th>
				<th>Close Date</th>
				<th>Quantity</th>
				<th>Cost Basis</th>
				<th>Sell Price</th>
				<th>P/L</th>
				<th>P/L %</th>
				<th>ROR %</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, cs := range closedStocks {
		plClass := "positive"
		if cs.ProfitLoss < 0 {
			plClass = "negative"
		}
		plPercent := cs.PlPercent()
		rorPercent := cs.CalculateROR() * 100

		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
				<td class="%s">$%.2f</td>
				<td class="%s">%.2f%%</td>
				<td class="%s">%.2f%%</td>
				<td>
					<button class="btn btn-sm btn-primary" hx-get="/api/history/edit-stock/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
					<button class="btn btn-sm btn-danger" hx-delete="/api/history/stock/%d" hx-target="#closed-stocks-list" hx-swap="outerHTML" hx-confirm="Delete this closed position?">Delete</button>
				</td>
			</tr>`, html.EscapeString(cs.Ticker), FormatDate(cs.OpenDate), FormatDate(cs.CloseDate), cs.Quantity,
			cs.CostBasis, cs.SellPrice, plClass, cs.ProfitLoss, plClass, plPercent, plClass, rorPercent, cs.ID, cs.ID)
	}

	htmlContent += `</tbody></table></div>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
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
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
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
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
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

	rows, err := db.Query(`
		SELECT id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date, close_date, sell_price, profit_loss
		FROM closed_options
		WHERE user_id = ?
		ORDER BY close_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch closed options", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ClosedOptionWithID struct {
		types.ClosedOption
		ID int
	}

	var closedOptions []ClosedOptionWithID
	for rows.Next() {
		var co ClosedOptionWithID
		if err := rows.Scan(&co.ID, &co.Ticker, &co.Price, &co.Premium, &co.Strike, &co.ExpDate, &co.Type, &co.Collateral, &co.PurchaseDate, &co.CloseDate, &co.SellPrice, &co.ProfitLoss); err != nil {
			continue
		}
		closedOptions = append(closedOptions, co)
	}

	if len(closedOptions) == 0 {
		w.Write([]byte(`<div id="closed-options-list" hx-get="/api/history/options" hx-trigger="historyUpdated from:body" hx-swap="outerHTML"><p class="empty-state">No closed option trades found.</p></div>`))
		return
	}

	htmlContent := `<div id="closed-options-list" hx-get="/api/history/options" hx-trigger="historyUpdated from:body" hx-swap="outerHTML"><table class="history-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Type</th>
				<th>Strike</th>
				<th>Premium</th>
				<th>Exp Date</th>
				<th>Purchase Date</th>
				<th>Close Date</th>
				<th>Sell Price</th>
				<th>P/L</th>
				<th>P/L %</th>
				<th>ROR %</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, co := range closedOptions {
		plClass := "positive"
		if co.ProfitLoss < 0 {
			plClass = "negative"
		}
		plPercent := co.PlPercent()
		rorPercent := co.CalculateROR() * 100

		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>$%.2f</td>
				<td class="%s">$%.2f</td>
				<td class="%s">%.2f%%</td>
				<td class="%s">%.2f%%</td>
				<td>
					<button class="btn btn-sm btn-primary" hx-get="/api/history/edit-option/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
					<button class="btn btn-sm btn-danger" hx-delete="/api/history/option/%d" hx-target="#closed-options-list" hx-swap="outerHTML" hx-confirm="Delete this closed position?">Delete</button>
				</td>
			</tr>`, html.EscapeString(co.Ticker), co.Type, co.Strike, co.Premium,
			FormatDate(co.ExpDate), FormatDate(co.PurchaseDate), FormatDate(co.CloseDate), co.SellPrice, plClass, co.ProfitLoss, plClass, plPercent, plClass, rorPercent, co.ID, co.ID)
	}

	htmlContent += `</tbody></table></div>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

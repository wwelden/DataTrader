package handlers

import (
	"backend/types"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func HandlePositions(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "positions.html"))
}

func HandleModalAddPosition(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "modal", "add-position.html"))
}

func HandleModalAddPositionFields(w http.ResponseWriter, r *http.Request) {
	positionType := r.URL.Query().Get("positionType")

	if positionType == "option" {
		http.ServeFile(w, r, filepath.Join("views", "modal", "add-position-fields.html"))
	} else if positionType == "stock" {
		http.ServeFile(w, r, filepath.Join("views", "modal", "add-position-stock-fields.html"))
	} else {
		w.Write([]byte(""))
	}
}

func HandleModalClose(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleAddPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	positionType := r.FormValue("positionType")
	ticker := strings.ToUpper(r.FormValue("ticker"))
	quantity, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costBasis, _ := strconv.ParseFloat(r.FormValue("costBasis"), 64)
	openDate := r.FormValue("openDate")
	if openDate == "" {
		openDate = time.Now().Format("2006-01-02")
	}

	switch positionType {
	case "stock":
		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			totalQuantity := existingQuantity + quantity
			totalCost := (existingCostBasis * existingQuantity) + (costBasis * quantity)
			newCostBasis := totalCost / totalQuantity

			_, err = db.Exec(`
				UPDATE stock_positions
				SET quantity = ?, cost_basis = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, totalQuantity, newCostBasis, existingID)

			if err != nil {
				http.Error(w, "Failed to update stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			_, err = db.Exec(`
				INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
				VALUES (?, ?, ?, ?, ?)
			`, userID, ticker, quantity, costBasis, openDate)

			if err != nil {
				http.Error(w, "Failed to add stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	case "option":
		optionType := r.FormValue("optionType")
		strike, _ := strconv.ParseFloat(r.FormValue("strike"), 64)
		premium, _ := strconv.ParseFloat(r.FormValue("premium"), 64)
		expDate := r.FormValue("expDate")

		// For options, price = premium (the cost per share of the option)
		price := premium
		collateral := 0.0

		switch optionType {
		case "CSP":
			collateral = strike * 100
		case "CC":
			var stockQuantity, stockCostBasis float64
			err := db.QueryRow(`
				SELECT quantity, cost_basis
				FROM stock_positions
				WHERE user_id = ? AND ticker = ?
			`, userID, ticker).Scan(&stockQuantity, &stockCostBasis)

			if err == nil && stockQuantity >= 100 {
				collateral = stockCostBasis * 100
			}
		}

		_, err := db.Exec(`
			INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, price, premium, strike, expDate, optionType, collateral, openDate)

		if err != nil {
			http.Error(w, "Failed to add option position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("HX-Trigger", "positionAdded")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleGetStockPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.Query(`
		SELECT id, ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE user_id = ?
		ORDER BY open_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch stock positions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var positions []types.StockPos
	for rows.Next() {
		var pos types.StockPos
		if err := rows.Scan(&pos.ID, &pos.Ticker, &pos.Quantity, &pos.CostBasis, &pos.OpenDate); err != nil {
			continue
		}
		positions = append(positions, pos)
	}

	if len(positions) == 0 {
		w.Write([]byte(`<div id="stock-positions-list" hx-get="/api/positions/stocks" hx-trigger="positionAdded from:body, positionDeleted from:body" hx-swap="outerHTML"><p>No stock positions found.</p></div>`))
		return
	}

	// Render HTML table wrapped in div
	htmlContent := `<div id="stock-positions-list" hx-get="/api/positions/stocks" hx-trigger="positionAdded from:body, positionDeleted from:body" hx-swap="outerHTML"><table class="positions-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Quantity</th>
				<th>Cost Basis</th>
				<th>Open Date</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, pos := range positions {
		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>%s</td>
				<td>
					<button class="btn btn-sm btn-primary" hx-get="/api/positions/edit-stock/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
					<button class="btn btn-sm btn-danger" hx-delete="/api/positions/stock/%d" hx-target="#stock-positions-list" hx-swap="outerHTML" hx-confirm="Delete this position?">Delete</button>
					<button class="btn btn-sm btn-warning" hx-post="/api/positions/close/%d" hx-target="#modal-container" hx-swap="innerHTML">Close</button>
				</td>
			</tr>`, html.EscapeString(pos.Ticker), pos.Quantity, pos.CostBasis, FormatDate(pos.OpenDate), pos.ID, pos.ID, pos.ID)
	}

	htmlContent += `</tbody></table></div>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleGetOptionPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := db.Query(`
		SELECT id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE user_id = ?
		ORDER BY purchase_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch option positions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var positions []types.OptionPos
	for rows.Next() {
		var pos types.OptionPos
		if err := rows.Scan(&pos.ID, &pos.Ticker, &pos.Price, &pos.Premium, &pos.Strike, &pos.ExpDate, &pos.Type, &pos.Collateral, &pos.PurchaseDate); err != nil {
			continue
		}
		positions = append(positions, pos)
	}

	if len(positions) == 0 {
		w.Write([]byte(`<div id="option-positions-list" hx-get="/api/positions/options" hx-trigger="positionAdded from:body, positionDeleted from:body" hx-swap="outerHTML"><p>No option positions found.</p></div>`))
		return
	}

	// Render HTML table wrapped in div
	htmlContent := `<div id="option-positions-list" hx-get="/api/positions/options" hx-trigger="positionAdded from:body, positionDeleted from:body" hx-swap="outerHTML"><table class="positions-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Type</th>
				<th>Strike</th>
				<th>Premium</th>
				<th>Exp Date</th>
				<th>Purchase Date</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, pos := range positions {
		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
				<td>%s</td>
				<td>%s</td>
				<td>
					<button class="btn btn-sm btn-primary" hx-get="/api/positions/edit-option/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
					<button class="btn btn-sm btn-danger" hx-delete="/api/positions/option/%d" hx-target="#option-positions-list" hx-swap="outerHTML" hx-confirm="Delete this position?">Delete</button>
					<button class="btn btn-sm btn-warning" hx-post="/api/positions/close-option-modal/%d" hx-target="#modal-container" hx-swap="innerHTML">Close</button>
				</td>
			</tr>`, html.EscapeString(pos.Ticker), pos.Type, pos.Strike, pos.Premium, FormatDate(pos.ExpDate), FormatDate(pos.PurchaseDate), pos.ID, pos.ID, pos.ID)
	}

	htmlContent += `</tbody></table></div>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleClosePosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	if positionID == "" {
		http.Error(w, "Position ID required", http.StatusBadRequest)
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var quantity, costBasis float64
	var openDate string

	err := db.QueryRow(`
		SELECT ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &quantity, &costBasis, &openDate)

	if err == nil {
		html := fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Close Stock Position: %s</h3>
					</div>
					<form hx-post="/api/positions/close-stock/%s" hx-target="#modal-container" hx-swap="innerHTML">
						<div class="form-group">
							<label>Sell Price</label>
							<input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" />
						</div>
						<div class="form-group">
							<label>Close Date (defaults to today)</label>
							<input type="date" name="closeDate" />
						</div>
						<div class="form-actions">
							<button type="submit" class="btn btn-primary">Close Position</button>
							<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
						</div>
					</form>
				</div>
			</div>
		`, ticker, positionID, costBasis)

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("HX-Retarget", "#modal-container")
		w.Header().Set("HX-Reswap", "innerHTML")
		w.Write([]byte(html))
		return
	}

	var price, premium, strike, collateral float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err = db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate)

	if err == nil {
		var html string

		// For CC and CSP, show outcome selector
		if optionType == types.CC || optionType == types.CSP {
			outcomeOptions := ""
			if optionType == types.CC {
				outcomeOptions = `
					<option value="expired">Expired</option>
					<option value="called_away">Called Away</option>
					<option value="closed">Closed</option>`
			} else { // CSP
				outcomeOptions = `
					<option value="expired">Expired</option>
					<option value="assigned">Assigned</option>
					<option value="closed">Closed</option>`
			}

			html = fmt.Sprintf(`
				<div class="modal">
					<div class="modal-content">
						<div class="modal-header">
							<h3>Close Option Position: %s %s</h3>
						</div>
						<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
							<div class="form-group">
								<label>Outcome</label>
								<select id="outcome" name="outcome" required onchange="toggleOutcomeFields(this.value)">
									<option value="">Select outcome...</option>
									%s
								</select>
							</div>
							<div id="outcome-fields"></div>
							<div class="form-actions">
								<button type="submit" class="btn btn-primary">Close Position</button>
								<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
							</div>
						</form>
					</div>
				</div>
				<script>
					function toggleOutcomeFields(outcome) {
						const fieldsDiv = document.getElementById('outcome-fields');
						const optionType = '%s';
						const strike = %.2f;
						const expDate = '%s';

						if (outcome === 'expired') {
							fieldsDiv.innerHTML = '<input type="hidden" name="sellPrice" value="0"><input type="hidden" name="closeDate" value="' + expDate + '">';
						} else if (outcome === 'called_away') {
							fieldsDiv.innerHTML = '<div class="form-group"><label>Share Sale Price</label><input type="number" name="sharePrice" step="0.01" required placeholder="' + strike + '" /></div><input type="hidden" name="sellPrice" value="0"><div class="form-group"><label>Close Date (defaults to expiration)</label><input type="date" name="closeDate" value="' + expDate + '" /></div>';
						} else if (outcome === 'assigned') {
							fieldsDiv.innerHTML = '<input type="hidden" name="sellPrice" value="0"><div class="form-group"><label>Close Date (defaults to expiration)</label><input type="date" name="closeDate" value="' + expDate + '" /></div>';
						} else if (outcome === 'closed') {
							fieldsDiv.innerHTML = '<div class="form-group"><label>Buy to Close Price</label><input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" /></div><div class="form-group"><label>Close Date (defaults to today)</label><input type="date" name="closeDate" /></div>';
						}
					}
				</script>
			`, ticker, optionType, positionID, outcomeOptions, optionType, strike, expDate, premium)
		} else {
			// For regular Call/Put, show simple close form
			html = fmt.Sprintf(`
				<div class="modal">
					<div class="modal-content">
						<div class="modal-header">
							<h3>Close Option Position: %s %s</h3>
						</div>
						<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
							<input type="hidden" name="outcome" value="closed">
							<div class="form-group">
								<label>Sell Price</label>
								<input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" />
							</div>
							<div class="form-group">
								<label>Close Date (defaults to today)</label>
								<input type="date" name="closeDate" />
							</div>
							<div class="form-actions">
								<button type="submit" class="btn btn-primary">Close Position</button>
								<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
							</div>
						</form>
					</div>
				</div>
			`, ticker, optionType, positionID, premium)
		}

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("HX-Retarget", "#modal-container")
		w.Header().Set("HX-Reswap", "innerHTML")
		w.Write([]byte(html))
		return
	}

	http.Error(w, "Position not found", http.StatusNotFound)
}

func HandleCloseOptionModal(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	if positionID == "" {
		http.Error(w, "Position ID required", http.StatusBadRequest)
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var price, premium, strike, collateral float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	var html string

	// For CC and CSP, show outcome selector
	if optionType == types.CC || optionType == types.CSP {
		outcomeOptions := ""
		if optionType == types.CC {
			outcomeOptions = `
				<option value="expired">Expired</option>
				<option value="called_away">Called Away</option>
				<option value="closed">Closed</option>`
		} else { // CSP
			outcomeOptions = `
				<option value="expired">Expired</option>
				<option value="assigned">Assigned</option>
				<option value="closed">Closed</option>`
		}

		html = fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Close Option Position: %s %s</h3>
					</div>
					<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
						<div class="form-group">
							<label>Outcome</label>
							<select id="outcome" name="outcome" required onchange="toggleOutcomeFields(this.value)">
								<option value="">Select outcome...</option>
								%s
							</select>
						</div>
						<div id="outcome-fields"></div>
						<div class="form-actions">
							<button type="submit" class="btn btn-primary">Close Position</button>
							<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
						</div>
					</form>
				</div>
			</div>
			<script>
				function toggleOutcomeFields(outcome) {
					const fieldsDiv = document.getElementById('outcome-fields');
					const optionType = '%s';
					const strike = %.2f;
					const expDate = '%s';

					if (outcome === 'expired') {
						fieldsDiv.innerHTML = '<input type="hidden" name="sellPrice" value="0"><input type="hidden" name="closeDate" value="' + expDate + '">';
					} else if (outcome === 'called_away') {
						fieldsDiv.innerHTML = '<div class="form-group"><label>Share Sale Price</label><input type="number" name="sharePrice" step="0.01" required placeholder="' + strike + '" /></div><input type="hidden" name="sellPrice" value="0"><div class="form-group"><label>Close Date (defaults to expiration)</label><input type="date" name="closeDate" value="' + expDate + '" /></div>';
					} else if (outcome === 'assigned') {
						fieldsDiv.innerHTML = '<input type="hidden" name="sellPrice" value="0"><div class="form-group"><label>Close Date (defaults to expiration)</label><input type="date" name="closeDate" value="' + expDate + '" /></div>';
					} else if (outcome === 'closed') {
						fieldsDiv.innerHTML = '<div class="form-group"><label>Buy to Close Price</label><input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" /></div><div class="form-group"><label>Close Date (defaults to today)</label><input type="date" name="closeDate" /></div>';
					}
				}
			</script>
		`, ticker, optionType, positionID, outcomeOptions, optionType, strike, expDate, premium)
	} else {
		// For regular Call/Put, show simple close form
		html = fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Close Option Position: %s %s</h3>
					</div>
					<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
						<input type="hidden" name="outcome" value="closed">
						<div class="form-group">
							<label>Sell Price</label>
							<input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" />
						</div>
						<div class="form-group">
							<label>Close Date (defaults to today)</label>
							<input type="date" name="closeDate" />
						</div>
						<div class="form-actions">
							<button type="submit" class="btn btn-primary">Close Position</button>
							<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
						</div>
					</form>
				</div>
			</div>
		`, ticker, optionType, positionID, premium)
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Retarget", "#modal-container")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Write([]byte(html))
}

func HandleCloseStockPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")
	if closeDate == "" {
		closeDate = time.Now().Format("2006-01-02")
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var quantity, costBasis float64
	var openDate string

	err := db.QueryRow(`
		SELECT ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &quantity, &costBasis, &openDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	profitLoss := (sellPrice - costBasis) * quantity

	var existingID int
	var existingQuantity, existingCostBasis, existingSellPrice, existingProfitLoss float64
	err = db.QueryRow(`
		SELECT id, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE user_id = ? AND ticker = ? AND open_date = ?
	`, userID, ticker, openDate).Scan(&existingID, &existingQuantity, &existingCostBasis, &existingSellPrice, &existingProfitLoss)

	if err == nil {
		totalQuantity := existingQuantity + quantity
		totalCostBasis := ((existingCostBasis * existingQuantity) + (costBasis * quantity)) / totalQuantity
		totalSellPrice := ((existingSellPrice * existingQuantity) + (sellPrice * quantity)) / totalQuantity
		totalProfitLoss := existingProfitLoss + profitLoss

		_, err = db.Exec(`
			UPDATE closed_stocks
			SET quantity = ?, cost_basis = ?, sell_price = ?, profit_loss = ?, close_date = ?
			WHERE id = ?
		`, totalQuantity, totalCostBasis, totalSellPrice, totalProfitLoss, closeDate, existingID)

		if err != nil {
			http.Error(w, "Failed to update closed position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		_, err = db.Exec(`
			INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, openDate, closeDate, quantity, costBasis, sellPrice, profitLoss)

		if err != nil {
			http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	_, err = db.Exec(`DELETE FROM stock_positions WHERE id = ?`, positionID)
	if err != nil {
		http.Error(w, "Failed to remove position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionClosed")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleCloseOptionPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")
	outcome := r.FormValue("outcome")

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	sharePrice, _ := strconv.ParseFloat(r.FormValue("sharePrice"), 64)
	closeDate := r.FormValue("closeDate")
	if closeDate == "" {
		closeDate = time.Now().Format("2006-01-02")
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var price, premium, strike, collateral float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	// Handle different outcomes
	switch outcome {
	case "expired":
		// Expired: close at $0 on expiration date
		sellPrice = 0
		closeDate = expDate

	case "called_away":
		// Called Away (CC): Sell shares at sharePrice, close option at $0
		sellPrice = 0

		// Remove 100 shares from stock position
		var stockID int
		var stockQuantity, stockCostBasis float64
		var stockOpenDate string
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis, open_date
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&stockID, &stockQuantity, &stockCostBasis, &stockOpenDate)

		if err == nil && stockQuantity >= 100 {
			// Close 100 shares at the sharePrice
			stockProfitLoss := (sharePrice - stockCostBasis) * 100

			_, err = db.Exec(`
				INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
				VALUES (?, ?, ?, ?, 100, ?, ?, ?)
			`, userID, ticker, stockOpenDate, closeDate, stockCostBasis, sharePrice, stockProfitLoss)

			if err != nil {
				http.Error(w, "Failed to close stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Update stock position quantity
			newQuantity := stockQuantity - 100
			if newQuantity > 0 {
				_, err = db.Exec(`
					UPDATE stock_positions
					SET quantity = ?, updated_at = CURRENT_TIMESTAMP
					WHERE id = ?
				`, newQuantity, stockID)
			} else {
				_, err = db.Exec(`DELETE FROM stock_positions WHERE id = ?`, stockID)
			}

			if err != nil {
				http.Error(w, "Failed to update stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

	case "assigned":
		// Assigned (CSP): Add 100 shares at strike price, close option at $0
		sellPrice = 0

		// Check if user already has shares of this stock
		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			// Update existing position
			totalQuantity := existingQuantity + 100
			totalCost := (existingCostBasis * existingQuantity) + (strike * 100)
			newCostBasis := totalCost / totalQuantity

			_, err = db.Exec(`
				UPDATE stock_positions
				SET quantity = ?, cost_basis = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, totalQuantity, newCostBasis, existingID)
		} else {
			// Create new stock position
			_, err = db.Exec(`
				INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
				VALUES (?, ?, 100, ?, ?)
			`, userID, ticker, strike, closeDate)
		}

		if err != nil {
			http.Error(w, "Failed to add stock position: "+err.Error(), http.StatusInternalServerError)
			return
		}

	case "closed":
		// Closed normally: use the sellPrice provided
		// sellPrice already set from form
	}

	// Calculate profit/loss
	var profitLoss float64
	switch optionType {
	case types.Call, types.Put:
		profitLoss = sellPrice - premium
	case types.CSP, types.CC:
		profitLoss = premium - sellPrice
	}

	// Insert into closed_options
	_, err = db.Exec(`
		INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date, close_date, sell_price, profit_loss)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, ticker, price, premium, strike, expDate, optionType, collateral, purchaseDate, closeDate, sellPrice, profitLoss)

	if err != nil {
		http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete from option_positions
	_, err = db.Exec(`DELETE FROM option_positions WHERE id = ?`, positionID)
	if err != nil {
		http.Error(w, "Failed to remove position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionClosed")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleEditStockPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var quantity, costBasis float64
	var openDate string

	err := db.QueryRow(`
		SELECT ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &quantity, &costBasis, &openDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	modalHTML := fmt.Sprintf(`
		<div class="modal">
			<div class="modal-content">
				<div class="modal-header">
					<h3>Edit Stock Position: %s</h3>
				</div>
				<form hx-post="/api/positions/update-stock/%s" hx-target="#modal-container" hx-swap="innerHTML">
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
						<label>Open Date</label>
						<input type="date" name="openDate" value="%s" required />
					</div>
					<div class="form-actions">
						<button type="submit" class="btn btn-primary">Update</button>
						<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
					</div>
				</form>
			</div>
		</div>
	`, ticker, positionID, ticker, quantity, costBasis, openDate)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

func HandleEditOptionPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var ticker string
	var price, premium, strike, collateral float64
	var expDate, purchaseDate, optionType string

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	modalHTML := fmt.Sprintf(`
		<div class="modal">
			<div class="modal-content">
				<div class="modal-header">
					<h3>Edit Option Position: %s</h3>
				</div>
				<form hx-post="/api/positions/update-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
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
						<label>Expiration Date</label>
						<input type="date" name="expDate" value="%s" required />
					</div>
					<div class="form-group">
						<label>Purchase Date</label>
						<input type="date" name="purchaseDate" value="%s" required />
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
		strike, premium, price, expDate, purchaseDate)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

func HandleUpdateStockPosition(w http.ResponseWriter, r *http.Request) {
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

	ticker := strings.ToUpper(r.FormValue("ticker"))
	quantity, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costBasis, _ := strconv.ParseFloat(r.FormValue("costBasis"), 64)
	openDate := r.FormValue("openDate")

	_, err := db.Exec(`
		UPDATE stock_positions
		SET ticker = ?, quantity = ?, cost_basis = ?, open_date = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, ticker, quantity, costBasis, openDate, positionID, userID)

	if err != nil {
		http.Error(w, "Failed to update position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionAdded")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleUpdateOptionPosition(w http.ResponseWriter, r *http.Request) {
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

	ticker := strings.ToUpper(r.FormValue("ticker"))
	optionType := r.FormValue("optionType")
	strike, _ := strconv.ParseFloat(r.FormValue("strike"), 64)
	premium, _ := strconv.ParseFloat(r.FormValue("premium"), 64)
	price, _ := strconv.ParseFloat(r.FormValue("price"), 64)
	collateral := 0.0
	expDate := r.FormValue("expDate")
	purchaseDate := r.FormValue("purchaseDate")

	_, err := db.Exec(`
		UPDATE option_positions
		SET ticker = ?, type = ?, strike = ?, premium = ?, price = ?, collateral = ?, exp_date = ?, purchase_date = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, ticker, optionType, strike, premium, price, collateral, expDate, purchaseDate, positionID, userID)

	if err != nil {
		http.Error(w, "Failed to update position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionAdded")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func selected(current, value string) string {
	if current == value {
		return "selected"
	}
	return ""
}

func HandleDeleteStockPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM stock_positions WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionDeleted")
	HandleGetStockPositions(w, r)
}

func HandleDeleteOptionPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM option_positions WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionDeleted")
	HandleGetOptionPositions(w, r)
}

func jsonError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

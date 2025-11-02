package handlers

import (
	"backend/types"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strconv"

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
	} else {
		// Return empty for stock (no additional fields needed)
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
	ticker := r.FormValue("ticker")
	quantity, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costBasis, _ := strconv.ParseFloat(r.FormValue("costBasis"), 64)
	openDate := r.FormValue("openDate")

	if positionType == "stock" {
		// Check if position already exists for this ticker
		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			// Position exists, update it by averaging the cost basis
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
			// Position doesn't exist, insert new one
			_, err = db.Exec(`
				INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
				VALUES (?, ?, ?, ?, ?)
			`, userID, ticker, quantity, costBasis, openDate)

			if err != nil {
				http.Error(w, "Failed to add stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	} else if positionType == "option" {
		optionType := r.FormValue("optionType")
		strike, _ := strconv.ParseFloat(r.FormValue("strike"), 64)
		premium, _ := strconv.ParseFloat(r.FormValue("premium"), 64)
		expDate := r.FormValue("expDate")
		collateral, _ := strconv.ParseFloat(r.FormValue("collateral"), 64)

		_, err := db.Exec(`
			INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, costBasis, premium, strike, expDate, optionType, collateral, openDate)

		if err != nil {
			http.Error(w, "Failed to add option position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Close modal and trigger refresh
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
					<button class="btn btn-sm btn-warning" hx-post="/api/positions/close/%d" hx-target="#modal-container" hx-swap="innerHTML">Close</button>
				</td>
			</tr>`, html.EscapeString(pos.Ticker), pos.Type, pos.Strike, pos.Premium, FormatDate(pos.ExpDate), FormatDate(pos.PurchaseDate), pos.ID, pos.ID, pos.ID)
	}

	htmlContent += `</tbody></table></div>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleClosePosition(w http.ResponseWriter, r *http.Request) {
	// Extract position ID from URL path using chi
	positionID := chi.URLParam(r, "id")
	fmt.Printf("🔍 HandleClosePosition called with ID: '%s'\n", positionID)

	if positionID == "" {
		fmt.Printf("✗ No position ID provided\n")
		http.Error(w, "Position ID required", http.StatusBadRequest)
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		fmt.Printf("✗ Unauthorized user\n")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	fmt.Printf("✓ User ID: %d, Position ID: %s\n", userID, positionID)

	// First, try to find if it's a stock position
	var ticker string
	var quantity, costBasis float64
	var openDate string

	err := db.QueryRow(`
		SELECT ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &quantity, &costBasis, &openDate)

	if err == nil {
		// It's a stock position - move to closed_stocks
		// For now, we'll ask user for sell price and close date via a modal
		// But for this simple version, let's return a form to collect that info

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
							<label>Close Date</label>
							<input type="date" name="closeDate" required />
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

	// Try option position
	var price, premium, strike, collateral float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err = db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &purchaseDate)

	if err == nil {
		// It's an option position
		html := fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Close Option Position: %s %s</h3>
					</div>
					<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
						<div class="form-group">
							<label>Sell Price</label>
							<input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" />
						</div>
						<div class="form-group">
							<label>Close Date</label>
							<input type="date" name="closeDate" required />
						</div>
						<div class="form-actions">
							<button type="submit" class="btn btn-primary">Close Position</button>
							<button type="button" class="btn btn-secondary" hx-get="/modal/close" hx-target="#modal-container">Cancel</button>
						</div>
					</form>
				</div>
			</div>
		`, ticker, optionType, positionID, premium)

		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("HX-Retarget", "#modal-container")
		w.Header().Set("HX-Reswap", "innerHTML")
		w.Write([]byte(html))
		return
	}

	http.Error(w, "Position not found", http.StatusNotFound)
}

func HandleCloseStockPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the stock position details
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

	// Calculate profit/loss
	profitLoss := (sellPrice - costBasis) * quantity

	// Check if a closed position already exists for this ticker and dates
	var existingID int
	var existingQuantity, existingCostBasis, existingSellPrice, existingProfitLoss float64
	err = db.QueryRow(`
		SELECT id, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE user_id = ? AND ticker = ? AND open_date = ?
	`, userID, ticker, openDate).Scan(&existingID, &existingQuantity, &existingCostBasis, &existingSellPrice, &existingProfitLoss)

	if err == nil {
		// Position exists, update it by adding to the existing values
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
		// Position doesn't exist, insert new one
		_, err = db.Exec(`
			INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, openDate, closeDate, quantity, costBasis, sellPrice, profitLoss)

		if err != nil {
			http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete from stock_positions
	_, err = db.Exec(`DELETE FROM stock_positions WHERE id = ?`, positionID)
	if err != nil {
		http.Error(w, "Failed to remove position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Close modal and trigger refresh
	w.Header().Set("HX-Trigger", "positionClosed")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

func HandleCloseOptionPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the option position details
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

	// Calculate profit/loss (depends on option type)
	var profitLoss float64
	switch optionType {
	case types.Call, types.Put:
		// Buying options: P/L = Sell Price - Premium Paid
		profitLoss = sellPrice - premium
	case types.CSP, types.CC:
		// Selling options: P/L = Premium Collected - Buyback Price
		profitLoss = premium - sellPrice
	}

	// Check if a closed option already exists for this ticker, type, strike, and dates
	var existingID int
	var existingPrice, existingPremium, existingCollateral, existingSellPrice, existingProfitLoss float64
	err = db.QueryRow(`
		SELECT id, price, premium, collateral, sell_price, profit_loss
		FROM closed_options
		WHERE user_id = ? AND ticker = ? AND type = ? AND strike = ? AND exp_date = ? AND purchase_date = ?
	`, userID, ticker, optionType, strike, expDate, purchaseDate).Scan(&existingID, &existingPrice, &existingPremium, &existingCollateral, &existingSellPrice, &existingProfitLoss)

	if err == nil {
		// Position exists, update it by averaging prices and adding profit/loss
		avgPrice := (existingPrice + price) / 2
		avgPremium := (existingPremium + premium) / 2
		totalCollateral := existingCollateral + collateral
		avgSellPrice := (existingSellPrice + sellPrice) / 2
		totalProfitLoss := existingProfitLoss + profitLoss

		_, err = db.Exec(`
			UPDATE closed_options
			SET price = ?, premium = ?, collateral = ?, sell_price = ?, profit_loss = ?, close_date = ?
			WHERE id = ?
		`, avgPrice, avgPremium, totalCollateral, avgSellPrice, totalProfitLoss, closeDate, existingID)

		if err != nil {
			http.Error(w, "Failed to update closed position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Position doesn't exist, insert new one
		_, err = db.Exec(`
			INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date, close_date, sell_price, profit_loss)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, price, premium, strike, expDate, optionType, collateral, purchaseDate, closeDate, sellPrice, profitLoss)

		if err != nil {
			http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete from option_positions
	_, err = db.Exec(`DELETE FROM option_positions WHERE id = ?`, positionID)
	if err != nil {
		http.Error(w, "Failed to remove position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Close modal and trigger refresh
	w.Header().Set("HX-Trigger", "positionClosed")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

// HandleEditStockPosition shows edit modal for a stock position
func HandleEditStockPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the position details
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

	// Return edit form modal
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

// HandleEditOptionPosition shows edit modal for an option position
func HandleEditOptionPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the position details
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

	// Return edit form modal
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
						<label>Collateral</label>
						<input type="number" name="collateral" step="0.01" value="%.2f" required />
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
		strike, premium, price, collateral, expDate, purchaseDate)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

// HandleUpdateStockPosition updates a stock position
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

	ticker := r.FormValue("ticker")
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

	// Close modal and trigger refresh
	w.Header().Set("HX-Trigger", "positionAdded")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

// HandleUpdateOptionPosition updates an option position
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

	ticker := r.FormValue("ticker")
	optionType := r.FormValue("optionType")
	strike, _ := strconv.ParseFloat(r.FormValue("strike"), 64)
	premium, _ := strconv.ParseFloat(r.FormValue("premium"), 64)
	price, _ := strconv.ParseFloat(r.FormValue("price"), 64)
	collateral, _ := strconv.ParseFloat(r.FormValue("collateral"), 64)
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

	// Close modal and trigger refresh
	w.Header().Set("HX-Trigger", "positionAdded")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

// Helper function for select option selected attribute
func selected(current, value string) string {
	if current == value {
		return "selected"
	}
	return ""
}

// HandleDeleteStockPosition deletes a stock position
func HandleDeleteStockPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Delete the position
	_, err := db.Exec("DELETE FROM stock_positions WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	// Return the updated list
	w.Header().Set("HX-Trigger", "positionDeleted")
	HandleGetStockPositions(w, r)
}

// HandleDeleteOptionPosition deletes an option position
func HandleDeleteOptionPosition(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Delete the position
	_, err := db.Exec("DELETE FROM option_positions WHERE id = ? AND user_id = ?", positionID, userID)
	if err != nil {
		http.Error(w, "Failed to delete position", http.StatusInternalServerError)
		return
	}

	// Return the updated list
	w.Header().Set("HX-Trigger", "positionDeleted")
	HandleGetOptionPositions(w, r)
}

// Helper function to return JSON error
func jsonError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

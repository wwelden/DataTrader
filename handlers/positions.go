package handlers

import (
	"backend/types"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strconv"
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

	positionType := r.FormValue("positionType")
	ticker := r.FormValue("ticker")
	quantity, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
	costBasis, _ := strconv.ParseFloat(r.FormValue("costBasis"), 64)
	openDate := r.FormValue("openDate")

	// TODO: Get user_id from session/auth
	userID := 1

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
	// TODO: Get user_id from session/auth
	userID := 1

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
		w.Write([]byte("<p>No stock positions found.</p>"))
		return
	}

	// Render HTML table
	htmlContent := `<table class="positions-table">
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
					<button class="btn btn-sm btn-danger" hx-post="/api/positions/close/%d" hx-confirm="Close this position?">Close</button>
				</td>
			</tr>`, html.EscapeString(pos.Ticker), pos.Quantity, pos.CostBasis, pos.OpenDate, pos.ID)
	}

	htmlContent += `</tbody></table>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleGetOptionPositions(w http.ResponseWriter, r *http.Request) {
	// TODO: Get user_id from session/auth
	userID := 1

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
		w.Write([]byte("<p>No option positions found.</p>"))
		return
	}

	// Render HTML table
	htmlContent := `<table class="positions-table">
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
					<button class="btn btn-sm btn-danger" hx-post="/api/positions/close/%d" hx-confirm="Close this position?">Close</button>
				</td>
			</tr>`, html.EscapeString(pos.Ticker), pos.Type, pos.Strike, pos.Premium, pos.ExpDate, pos.PurchaseDate, pos.ID)
	}

	htmlContent += `</tbody></table>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleClosePosition(w http.ResponseWriter, r *http.Request) {
	// Extract position ID from URL path
	// Expected format: /api/positions/close/{id}
	path := r.URL.Path
	var positionID string
	fmt.Sscanf(path, "/api/positions/close/%s", &positionID)

	if positionID == "" {
		http.Error(w, "Position ID required", http.StatusBadRequest)
		return
	}

	// TODO: Get user_id from session/auth
	userID := 1

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
			<div class="modal" style="display: block">
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
			<div class="modal" style="display: block">
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

	path := r.URL.Path
	var positionID string
	fmt.Sscanf(path, "/api/positions/close-stock/%s", &positionID)

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")

	// TODO: Get user_id from session/auth
	userID := 1

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

	// Insert into closed_stocks
	_, err = db.Exec(`
		INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, ticker, openDate, closeDate, quantity, costBasis, sellPrice, profitLoss)

	if err != nil {
		http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
		return
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

	path := r.URL.Path
	var positionID string
	fmt.Sscanf(path, "/api/positions/close-option/%s", &positionID)

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")

	// TODO: Get user_id from session/auth
	userID := 1

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
		profitLoss = (sellPrice - premium) * 100 // Standard option contracts
	case types.CSP, types.CC:
		profitLoss = premium - sellPrice // Selling options
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

	// Close modal and trigger refresh
	w.Header().Set("HX-Trigger", "positionClosed")
	http.ServeFile(w, r, filepath.Join("views", "modal", "close.html"))
}

// Helper function to return JSON error
func jsonError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

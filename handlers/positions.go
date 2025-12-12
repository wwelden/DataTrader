package handlers

import (
	"backend/types"
	"backend/views/components"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func HandlePositions(w http.ResponseWriter, r *http.Request) {
	components.AppLayout("Positions - DATATRADER", "positions", components.PositionsPage()).Render(r.Context(), w)
}

func HandleModalAddPosition(w http.ResponseWriter, r *http.Request) {
	components.AddPositionModal().Render(r.Context(), w)
}

func HandleModalAddPositionFields(w http.ResponseWriter, r *http.Request) {
	positionType := r.URL.Query().Get("positionType")

	if positionType == "option" {
		components.AddPositionOptionFields().Render(r.Context(), w)
	} else if positionType == "stock" {
		components.AddPositionStockFields().Render(r.Context(), w)
	} else {
		w.Write([]byte(""))
	}
}

func HandleModalClose(w http.ResponseWriter, r *http.Request) {
	components.ModalClose().Render(r.Context(), w)
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
			INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, userID, ticker, price, premium, strike, expDate, optionType, collateral, quantity, openDate)

		if err != nil {
			http.Error(w, "Failed to add option position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("HX-Trigger", "positionAdded")
	components.ModalClose().Render(r.Context(), w)
}

func HandleGetStockPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	query := `SELECT id, ticker, quantity, cost_basis, open_date FROM stock_positions WHERE user_id = ? AND quantity > 0`
	args := []interface{}{userID}

	if search != "" {
		query += ` AND ticker LIKE ?`
		args = append(args, "%"+search+"%")
	}

	query += ` ORDER BY open_date DESC`

	rows, err := db.Query(query, args...)

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
		if IsDateInRange(pos.OpenDate, dateFromInput, dateToInput) {
			positions = append(positions, pos)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	components.StockPositionsTable(positions, FormatDate).Render(r.Context(), w)
}

func HandleGetOptionPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	optionType := r.URL.Query().Get("type")
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	query := `SELECT id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date FROM option_positions WHERE user_id = ? AND quantity > 0`
	args := []interface{}{userID}

	if search != "" {
		query += ` AND ticker LIKE ?`
		args = append(args, "%"+search+"%")
	}
	if optionType != "" {
		query += ` AND type = ?`
		args = append(args, optionType)
	}

	query += ` ORDER BY purchase_date DESC`

	rows, err := db.Query(query, args...)

	if err != nil {
		http.Error(w, "Failed to fetch option positions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var positions []types.OptionPos
	for rows.Next() {
		var pos types.OptionPos
		if err := rows.Scan(&pos.ID, &pos.Ticker, &pos.Price, &pos.Premium, &pos.Strike, &pos.ExpDate, &pos.Type, &pos.Collateral, &pos.Quantity, &pos.PurchaseDate); err != nil {
			continue
		}
		if IsDateInRange(pos.PurchaseDate, dateFromInput, dateToInput) {
			positions = append(positions, pos)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	components.OptionPositionsTable(positions, FormatDate).Render(r.Context(), w)
}

func HandlePositionsFilter(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	search := strings.ToUpper(r.URL.Query().Get("search"))
	optionType := r.URL.Query().Get("type")
	dateFromInput := r.URL.Query().Get("dateFrom")
	dateToInput := r.URL.Query().Get("dateTo")

	var stockPositions []types.StockPos

	if optionType == "" {
		stockQuery := `SELECT id, ticker, quantity, cost_basis, open_date FROM stock_positions WHERE user_id = ? AND quantity > 0`
		stockArgs := []interface{}{userID}

		if search != "" {
			stockQuery += ` AND ticker LIKE ?`
			stockArgs = append(stockArgs, "%"+search+"%")
		}
		stockQuery += ` ORDER BY open_date DESC`

		stockRows, err := db.Query(stockQuery, stockArgs...)
		if err != nil {
			http.Error(w, "Failed to fetch stock positions", http.StatusInternalServerError)
			return
		}
		defer stockRows.Close()

		for stockRows.Next() {
			var pos types.StockPos
			if err := stockRows.Scan(&pos.ID, &pos.Ticker, &pos.Quantity, &pos.CostBasis, &pos.OpenDate); err != nil {
				continue
			}
			if IsDateInRange(pos.OpenDate, dateFromInput, dateToInput) {
				stockPositions = append(stockPositions, pos)
			}
		}
	}

	optionQuery := `SELECT id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date FROM option_positions WHERE user_id = ? AND quantity > 0`
	optionArgs := []interface{}{userID}

	if search != "" {
		optionQuery += ` AND ticker LIKE ?`
		optionArgs = append(optionArgs, "%"+search+"%")
	}
	if optionType != "" {
		optionQuery += ` AND type = ?`
		optionArgs = append(optionArgs, optionType)
	}
	optionQuery += ` ORDER BY purchase_date DESC`

	optionRows, err := db.Query(optionQuery, optionArgs...)
	if err != nil {
		http.Error(w, "Failed to fetch option positions", http.StatusInternalServerError)
		return
	}
	defer optionRows.Close()

	var optionPositions []types.OptionPos
	for optionRows.Next() {
		var pos types.OptionPos
		if err := optionRows.Scan(&pos.ID, &pos.Ticker, &pos.Price, &pos.Premium, &pos.Strike, &pos.ExpDate, &pos.Type, &pos.Collateral, &pos.Quantity, &pos.PurchaseDate); err != nil {
			continue
		}
		if IsDateInRange(pos.PurchaseDate, dateFromInput, dateToInput) {
			optionPositions = append(optionPositions, pos)
		}
	}

	htmlContent := `<div class="positions-section">
		<h3>Stock Positions</h3>
		<div id="stock-positions-list" hx-get="/api/positions/stocks" hx-trigger="positionAdded from:body, positionClosed from:body" hx-swap="innerHTML">`

	if len(stockPositions) == 0 {
		htmlContent += `<p>No stock positions found.</p>`
	} else {
		htmlContent += `<table class="positions-table">
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

		for _, pos := range stockPositions {
			htmlContent += fmt.Sprintf(`
				<tr>
					<td>%s</td>
					<td>%.2f</td>
					<td>$%.2f</td>
					<td>%s</td>
					<td>
						<button class="btn btn-sm btn-primary" hx-get="/api/positions/edit/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
						<button class="btn btn-sm btn-danger" hx-delete="/api/positions/%d" hx-target="#stock-positions-list" hx-swap="outerHTML" hx-confirm="Delete this position?">Delete</button>
						<button class="btn btn-sm btn-warning" hx-post="/api/positions/close/%d" hx-target="#modal-container" hx-swap="innerHTML">Close</button>
					</td>
				</tr>`, html.EscapeString(pos.Ticker), pos.Quantity, pos.CostBasis, FormatDate(pos.OpenDate), pos.ID, pos.ID, pos.ID)
		}

		htmlContent += `</tbody></table>`
	}

	htmlContent += `</div></div>
	<div class="positions-section">
		<h3>Option Positions</h3>
		<div id="option-positions-list" hx-get="/api/positions/options" hx-trigger="positionAdded from:body, positionClosed from:body" hx-swap="innerHTML">`

	if len(optionPositions) == 0 {
		htmlContent += `<p>No option positions found.</p>`
	} else {
		htmlContent += `<table class="positions-table">
			<thead>
				<tr>
					<th>Ticker</th>
					<th>Type</th>
					<th>Contracts</th>
					<th>Strike</th>
					<th>Premium</th>
					<th>Exp Date</th>
					<th>Purchase Date</th>
					<th>Actions</th>
				</tr>
			</thead>
			<tbody>`

		for _, pos := range optionPositions {
			htmlContent += fmt.Sprintf(`
				<tr>
					<td>%s</td>
					<td>%s</td>
					<td>%.0f</td>
					<td>$%.2f</td>
					<td>$%.2f</td>
					<td>%s</td>
					<td>%s</td>
					<td>
						<button class="btn btn-sm btn-primary" hx-get="/api/positions/edit-option/%d" hx-target="#modal-container" hx-swap="innerHTML">Edit</button>
						<button class="btn btn-sm btn-danger" hx-delete="/api/positions/option/%d" hx-target="#option-positions-list" hx-swap="outerHTML" hx-confirm="Delete this position?">Delete</button>
						<button class="btn btn-sm btn-warning" hx-post="/api/positions/close-option-modal/%d" hx-target="#modal-container" hx-swap="innerHTML">Close</button>
					</td>
				</tr>`, html.EscapeString(pos.Ticker), pos.Type, pos.Quantity, pos.Strike, pos.Premium, FormatDate(pos.ExpDate), FormatDate(pos.PurchaseDate), pos.ID, pos.ID, pos.ID)
		}

		htmlContent += `</tbody></table>`
	}

	htmlContent += `</div></div>`

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
							<label>Quantity to Close (Available: %.2f)</label>
							<input type="number" name="quantity" step="0.01" required placeholder="%.2f" value="%.2f" max="%.2f" />
						</div>
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
		`, ticker, positionID, quantity, quantity, quantity, quantity, costBasis)

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
								<select id="outcome" name="outcome" required
									hx-get="/api/positions/outcome-fields/%s"
									hx-target="#outcome-fields"
									hx-swap="innerHTML"
									hx-trigger="change">
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
			`, ticker, optionType, positionID, positionID, outcomeOptions)
		} else {
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
	var price, premium, strike, collateral, quantity float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &quantity, &purchaseDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	var html string

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
							<label>Contracts to Close (Available: %.0f)</label>
							<input type="number" name="quantity" step="1" required placeholder="%.0f" value="%.0f" max="%.0f" min="1" />
						</div>
						<div class="form-group">
							<label>Outcome</label>
							<select id="outcome" name="outcome" required
								hx-get="/api/positions/outcome-fields/%s"
								hx-target="#outcome-fields"
								hx-swap="innerHTML"
								hx-trigger="change">
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
		`, ticker, optionType, positionID, quantity, quantity, quantity, quantity, positionID, outcomeOptions)
	} else {
		html = fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Close Option Position: %s %s</h3>
					</div>
					<form hx-post="/api/positions/close-option/%s" hx-target="#modal-container" hx-swap="innerHTML">
						<input type="hidden" name="outcome" value="closed">
						<div class="form-group">
							<label>Contracts to Close (Available: %.0f)</label>
							<input type="number" name="quantity" step="1" required placeholder="%.0f" value="%.0f" max="%.0f" min="1" />
						</div>
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
		`, ticker, optionType, positionID, quantity, quantity, quantity, quantity, premium)
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Retarget", "#modal-container")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.Write([]byte(html))
}

func HandleOutcomeFields(w http.ResponseWriter, r *http.Request) {
	positionID := chi.URLParam(r, "id")
	outcome := r.URL.Query().Get("outcome")

	if positionID == "" {
		w.Write([]byte(""))
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		w.Write([]byte(""))
		return
	}

	var premium, strike float64
	var expDate string

	err := db.QueryRow(`
		SELECT premium, strike, exp_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&premium, &strike, &expDate)

	if err != nil {
		w.Write([]byte(""))
		return
	}

	var html string
	switch outcome {
	case "expired":
		html = fmt.Sprintf(`<input type="hidden" name="sellPrice" value="0"><input type="hidden" name="closeDate" value="%s">`, expDate)
	case "called_away":
		html = fmt.Sprintf(`
			<div class="form-group">
				<label>Share Sale Price</label>
				<input type="number" name="sharePrice" step="0.01" required placeholder="%.2f" />
			</div>
			<input type="hidden" name="sellPrice" value="0">
			<div class="form-group">
				<label>Close Date (defaults to expiration)</label>
				<input type="date" name="closeDate" value="%s" />
			</div>`, strike, expDate)
	case "assigned":
		html = fmt.Sprintf(`
			<input type="hidden" name="sellPrice" value="0">
			<div class="form-group">
				<label>Close Date (defaults to expiration)</label>
				<input type="date" name="closeDate" value="%s" />
			</div>`, expDate)
	case "closed":
		html = fmt.Sprintf(`
			<div class="form-group">
				<label>Buy to Close Price</label>
				<input type="number" name="sellPrice" step="0.01" required placeholder="%.2f" />
			</div>
			<div class="form-group">
				<label>Close Date (defaults to today)</label>
				<input type="date" name="closeDate" />
			</div>`, premium)
	default:
		html = ""
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func HandleCloseStockPosition(w http.ResponseWriter, r *http.Request) {
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

	var ticker string
	var currentQuantity, costBasis float64
	var openDate string

	err := db.QueryRow(`
		SELECT ticker, quantity, cost_basis, open_date
		FROM stock_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &currentQuantity, &costBasis, &openDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	// Parse quantity - if not provided or invalid, default to full position
	quantityStr := r.FormValue("quantity")
	var quantityToClose float64
	if quantityStr == "" {
		quantityToClose = currentQuantity
	} else {
		quantityToClose, _ = strconv.ParseFloat(quantityStr, 64)
	}

	sellPrice, _ := strconv.ParseFloat(r.FormValue("sellPrice"), 64)
	closeDate := r.FormValue("closeDate")
	if closeDate == "" {
		closeDate = time.Now().Format("2006-01-02")
	}

	// Validate quantity
	if quantityToClose <= 0 {
		quantityToClose = currentQuantity
	}
	if quantityToClose > currentQuantity {
		quantityToClose = currentQuantity
	}

	profitLoss := (sellPrice - costBasis) * quantityToClose

	var existingID int
	var existingQuantity, existingCostBasis, existingSellPrice, existingProfitLoss float64
	err = db.QueryRow(`
		SELECT id, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE user_id = ? AND ticker = ? AND open_date = ?
	`, userID, ticker, openDate).Scan(&existingID, &existingQuantity, &existingCostBasis, &existingSellPrice, &existingProfitLoss)

	if err == nil {
		totalQuantity := existingQuantity + quantityToClose
		totalCostBasis := ((existingCostBasis * existingQuantity) + (costBasis * quantityToClose)) / totalQuantity
		totalSellPrice := ((existingSellPrice * existingQuantity) + (sellPrice * quantityToClose)) / totalQuantity
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
		`, userID, ticker, openDate, closeDate, quantityToClose, costBasis, sellPrice, profitLoss)

		if err != nil {
			http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	remainingQuantity := currentQuantity - quantityToClose
	if remainingQuantity > 0 {
		_, err = db.Exec(`
			UPDATE stock_positions
			SET quantity = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, remainingQuantity, positionID)
	} else {
		_, err = db.Exec(`DELETE FROM stock_positions WHERE id = ?`, positionID)
	}

	if err != nil {
		http.Error(w, "Failed to update position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionClosed")
	components.ModalClose().Render(r.Context(), w)
}

func HandleCloseOptionPosition(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	positionID := chi.URLParam(r, "id")
	outcome := r.FormValue("outcome")

	quantityToClose, _ := strconv.ParseFloat(r.FormValue("quantity"), 64)
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
	var price, premium, strike, collateral, currentQuantity float64
	var expDate, purchaseDate string
	var optionType types.OptionType

	err := db.QueryRow(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date
		FROM option_positions
		WHERE id = ? AND user_id = ?
	`, positionID, userID).Scan(&ticker, &price, &premium, &strike, &expDate, &optionType, &collateral, &currentQuantity, &purchaseDate)

	if err != nil {
		http.Error(w, "Position not found", http.StatusNotFound)
		return
	}

	if quantityToClose <= 0 || quantityToClose > currentQuantity {
		http.Error(w, "Invalid quantity to close", http.StatusBadRequest)
		return
	}

	switch outcome {
	case "expired":
		sellPrice = 0
		closeDate = expDate

	case "called_away":
		sellPrice = 0

		sharesToSell := quantityToClose * 100
		var stockID int
		var stockQuantity, stockCostBasis float64
		var stockOpenDate string
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis, open_date
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&stockID, &stockQuantity, &stockCostBasis, &stockOpenDate)

		if err == nil && stockQuantity >= sharesToSell {
			stockProfitLoss := (sharePrice - stockCostBasis) * sharesToSell

			_, err = db.Exec(`
				INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, userID, ticker, stockOpenDate, closeDate, sharesToSell, stockCostBasis, sharePrice, stockProfitLoss)

			if err != nil {
				http.Error(w, "Failed to close stock position: "+err.Error(), http.StatusInternalServerError)
				return
			}

			newQuantity := stockQuantity - sharesToSell
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
		sellPrice = 0

		sharesToAdd := quantityToClose * 100

		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			totalQuantity := existingQuantity + sharesToAdd
			totalCost := (existingCostBasis * existingQuantity) + (strike * sharesToAdd)
			newCostBasis := totalCost / totalQuantity

			_, err = db.Exec(`
				UPDATE stock_positions
				SET quantity = ?, cost_basis = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, totalQuantity, newCostBasis, existingID)
		} else {
			_, err = db.Exec(`
				INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
				VALUES (?, ?, ?, ?, ?)
			`, userID, ticker, sharesToAdd, strike, closeDate)
		}

		if err != nil {
			http.Error(w, "Failed to add stock position: "+err.Error(), http.StatusInternalServerError)
			return
		}

	case "closed":
	}

	var profitLoss float64
	switch optionType {
	case types.Call, types.Put:
		profitLoss = (sellPrice - premium) * quantityToClose
	case types.CSP, types.CC:
		profitLoss = (premium - sellPrice) * quantityToClose
	}

	collateralForClosed := (collateral / currentQuantity) * quantityToClose

	_, err = db.Exec(`
		INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, ticker, price, premium, strike, expDate, optionType, collateralForClosed, quantityToClose, purchaseDate, closeDate, sellPrice, profitLoss)

	if err != nil {
		http.Error(w, "Failed to close position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	remainingQuantity := currentQuantity - quantityToClose
	if remainingQuantity > 0 {
		remainingCollateral := collateral - collateralForClosed
		_, err = db.Exec(`
			UPDATE option_positions
			SET quantity = ?, collateral = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, remainingQuantity, remainingCollateral, positionID)
	} else {
		_, err = db.Exec(`DELETE FROM option_positions WHERE id = ?`, positionID)
	}

	if err != nil {
		http.Error(w, "Failed to update position: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "positionClosed")
	components.ModalClose().Render(r.Context(), w)
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
	components.ModalClose().Render(r.Context(), w)
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
	components.ModalClose().Render(r.Context(), w)
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

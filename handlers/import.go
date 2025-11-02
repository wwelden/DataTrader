package handlers

import (
	"backend/utils"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
)

func HandleModalImportCSV(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "modal", "import-csv.html"))
}

func HandleImportCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	file, _, err := r.FormFile("csvFile")
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	csvContent, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file content", http.StatusInternalServerError)
		return
	}

	trades, err := utils.ParseBrokerageCSV(string(csvContent))
	if err != nil {
		modalHTML := fmt.Sprintf(`
			<div class="modal">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Import Failed</h3>
					</div>
					<p style="color: var(--danger-color); margin: 1rem 0;">%s</p>
					<div class="form-actions">
						<button type="button" class="btn btn-primary" hx-get="/modal/close" hx-target="#modal-container">Close</button>
					</div>
				</div>
			</div>
		`, err.Error())
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(modalHTML))
		return
	}

	stockCount := 0
	optionCount := 0

	for _, trade := range trades.StockTrades {
		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, trade.Ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			if trade.Code == "Buy" {
				totalQuantity := existingQuantity + trade.Quantity
				totalCost := (existingCostBasis * existingQuantity) + (trade.Price * trade.Quantity)
				newCostBasis := totalCost / totalQuantity

				_, err = db.Exec(`
					UPDATE stock_positions
					SET quantity = ?, cost_basis = ?, updated_at = CURRENT_TIMESTAMP
					WHERE id = ?
				`, totalQuantity, newCostBasis, existingID)
			} else if trade.Code == "Sell" {
				newQuantity := existingQuantity - trade.Quantity
				if newQuantity <= 0 {
					profitLoss := (trade.Price - existingCostBasis) * existingQuantity
					_, err = db.Exec(`
						INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)
					`, userID, trade.Ticker, trade.Date, trade.Date, existingQuantity, existingCostBasis, trade.Price, profitLoss)
					_, err = db.Exec(`DELETE FROM stock_positions WHERE id = ?`, existingID)
				} else {
					_, err = db.Exec(`
						UPDATE stock_positions
						SET quantity = ?, updated_at = CURRENT_TIMESTAMP
						WHERE id = ?
					`, newQuantity, existingID)
				}
			}
		} else {
			if trade.Code == "Buy" {
				_, err = db.Exec(`
					INSERT INTO stock_positions (user_id, ticker, quantity, cost_basis, open_date)
					VALUES (?, ?, ?, ?, ?)
				`, userID, trade.Ticker, trade.Quantity, trade.Price, trade.Date)
			}
		}
		stockCount++
	}

	for _, trade := range trades.OptionTrades {
		if trade.Code == "BTO" || trade.Code == "STO" {
			_, err = db.Exec(`
				INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, purchase_date)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, userID, trade.Ticker, trade.Price, trade.Premium, trade.Strike, trade.ExpDate, trade.OptionType, 0, trade.Date)
			optionCount++
		}
	}

	modalHTML := fmt.Sprintf(`
		<div class="modal">
			<div class="modal-content">
				<div class="modal-header">
					<h3>Import Successful</h3>
				</div>
				<p style="color: var(--success-color); margin: 1rem 0;">
					Successfully imported %d stock trades and %d option trades
				</p>
				<div class="form-actions">
					<button type="button" class="btn btn-primary" hx-get="/modal/close" hx-target="#modal-container">Close</button>
				</div>
			</div>
		</div>
	`, stockCount, optionCount)

	w.Header().Set("HX-Trigger", "positionAdded, historyUpdated")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(modalHTML))
}

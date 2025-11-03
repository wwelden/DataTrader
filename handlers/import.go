package handlers

import (
	"backend/utils"
	"backend/views/components"
	"fmt"
	"io"
	"net/http"
)

func HandleModalImportCSV(w http.ResponseWriter, r *http.Request) {
	components.ImportCSVModal().Render(r.Context(), w)
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
		// Normalize date to MM/DD/YY format
		normalizedDate := NormalizeDateToMMDDYY(trade.Date)

		var existingID int
		var existingQuantity, existingCostBasis float64
		err := db.QueryRow(`
			SELECT id, quantity, cost_basis
			FROM stock_positions
			WHERE user_id = ? AND ticker = ?
		`, userID, trade.Ticker).Scan(&existingID, &existingQuantity, &existingCostBasis)

		if err == nil {
			switch trade.Code {
			case "Buy":
				totalQuantity := existingQuantity + trade.Quantity
				totalCost := (existingCostBasis * existingQuantity) + (trade.Price * trade.Quantity)
				newCostBasis := totalCost / totalQuantity

				_, err = db.Exec(`
					UPDATE stock_positions
					SET quantity = ?, cost_basis = ?, updated_at = CURRENT_TIMESTAMP
					WHERE id = ?
				`, totalQuantity, newCostBasis, existingID)
			case "Sell":
				newQuantity := existingQuantity - trade.Quantity
				if newQuantity <= 0 {
					profitLoss := (trade.Price - existingCostBasis) * existingQuantity
					_, err = db.Exec(`
						INSERT INTO closed_stocks (user_id, ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss)
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)
					`, userID, trade.Ticker, normalizedDate, normalizedDate, existingQuantity, existingCostBasis, trade.Price, profitLoss)
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
				`, userID, trade.Ticker, trade.Quantity, trade.Price, normalizedDate)
			}
		}
		stockCount++
	}

	for _, trade := range trades.OptionTrades {
		// Normalize dates to MM/DD/YY format
		normalizedDate := NormalizeDateToMMDDYY(trade.Date)
		normalizedExpDate := NormalizeDateToMMDDYY(trade.ExpDate)
		switch trade.Code {
		case "BTO", "STO":
			// Opening a new option position
			_, err = db.Exec(`
				INSERT INTO option_positions (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, userID, trade.Ticker, trade.Price, trade.Premium, trade.Strike, normalizedExpDate, trade.OptionType, 0, trade.Quantity, normalizedDate)
			if err == nil {
				optionCount++
			}

		case "STC", "BTC":
			// Closing an option position - find matching open position
			var positionID int
			var positionQuantity float64
			var purchaseDate, positionType string
			var premium, collateral float64

			err := db.QueryRow(`
				SELECT id, quantity, premium, collateral, purchase_date, type
				FROM option_positions
				WHERE user_id = ? AND ticker = ? AND strike = ? AND exp_date = ? AND type = ?
				ORDER BY purchase_date ASC
				LIMIT 1
			`, userID, trade.Ticker, trade.Strike, normalizedExpDate, trade.OptionType).Scan(
				&positionID, &positionQuantity, &premium, &collateral, &purchaseDate, &positionType)

			if err == nil {
				// Found matching position - close it
				quantityToClose := trade.Quantity
				if quantityToClose > positionQuantity {
					quantityToClose = positionQuantity // Can't close more than we have
				}

				// Calculate profit/loss
				var profitLoss float64
				sellPrice := trade.Price

				// For STO (sold to open), closing means buying back (BTC)
				// For BTO (bought to open), closing means selling (STC)
				if trade.Code == "STC" {
					// Selling to close a long position (Call/Put bought)
					profitLoss = (sellPrice - premium) * quantityToClose
				} else { // BTC
					// Buying to close a short position (CSP/CC sold)
					profitLoss = (premium - sellPrice) * quantityToClose
				}

				// Calculate proportional collateral
				collateralForClosed := (collateral / positionQuantity) * quantityToClose

				// Insert into closed_options
				_, err = db.Exec(`
					INSERT INTO closed_options (user_id, ticker, price, premium, strike, exp_date, type, collateral, quantity, purchase_date, close_date, sell_price, profit_loss)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, userID, trade.Ticker, trade.Price, premium, trade.Strike, normalizedExpDate, positionType, collateralForClosed, quantityToClose, purchaseDate, normalizedDate, sellPrice, profitLoss)

				// Update or delete the position
				remainingQuantity := positionQuantity - quantityToClose
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

				optionCount++
			}
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

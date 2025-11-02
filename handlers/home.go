package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
)

func HandleHome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "index.html"))
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetOrCreateUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Count stock positions
	var stockCount int
	db.QueryRow("SELECT COUNT(*) FROM stock_positions WHERE user_id = ?", userID).Scan(&stockCount)

	// Count option positions
	var optionCount int
	db.QueryRow("SELECT COUNT(*) FROM option_positions WHERE user_id = ?", userID).Scan(&optionCount)

	// Count closed trades
	var closedStockCount int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ?", userID).Scan(&closedStockCount)

	var closedOptionCount int
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ?", userID).Scan(&closedOptionCount)

	// Calculate total P/L from closed trades
	var totalStockPL, totalOptionPL float64
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_stocks WHERE user_id = ?", userID).Scan(&totalStockPL)
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_options WHERE user_id = ?", userID).Scan(&totalOptionPL)

	totalPL := totalStockPL + totalOptionPL
	plClass := "positive"
	if totalPL < 0 {
		plClass = "negative"
	}

	totalClosed := closedStockCount + closedOptionCount
	totalPositions := stockCount + optionCount

	html := fmt.Sprintf(`
		<div class="stat-card">
			<h3>Total Positions</h3>
			<p class="stat-value">%d</p>
		</div>
		<div class="stat-card">
			<h3>Open Stocks</h3>
			<p class="stat-value">%d</p>
		</div>
		<div class="stat-card">
			<h3>Open Options</h3>
			<p class="stat-value">%d</p>
		</div>
		<div class="stat-card">
			<h3>Closed Trades</h3>
			<p class="stat-value">%d</p>
		</div>
		<div class="stat-card">
			<h3>Total P/L</h3>
			<p class="stat-value %s">$%.2f</p>
		</div>
	`, totalPositions, stockCount, optionCount, totalClosed, plClass, totalPL)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

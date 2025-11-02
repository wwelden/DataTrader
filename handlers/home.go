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
	// TODO: Get user_id from session/auth
	userID := 1

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
	`, totalPositions, stockCount, optionCount, totalClosed)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

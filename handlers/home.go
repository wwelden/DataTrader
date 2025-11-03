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

	var stockCount int
	db.QueryRow("SELECT COUNT(*) FROM stock_positions WHERE user_id = ?", userID).Scan(&stockCount)

	var optionCount int
	db.QueryRow("SELECT COUNT(*) FROM option_positions WHERE user_id = ?", userID).Scan(&optionCount)

	var closedStockCount int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ?", userID).Scan(&closedStockCount)

	var closedOptionCount int
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ?", userID).Scan(&closedOptionCount)

	var totalStockPL, totalOptionPL float64
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_stocks WHERE user_id = ?", userID).Scan(&totalStockPL)
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_options WHERE user_id = ?", userID).Scan(&totalOptionPL)

	totalPL := totalStockPL + totalOptionPL
	plClass := "positive"
	if totalPL < 0 {
		plClass = "negative"
	}

	var winningStocks, winningOptions int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ? AND profit_loss > 0", userID).Scan(&winningStocks)
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ? AND profit_loss > 0", userID).Scan(&winningOptions)

	var losingStocks, losingOptions int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ? AND profit_loss < 0", userID).Scan(&losingStocks)
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ? AND profit_loss < 0", userID).Scan(&losingOptions)

	totalWins := winningStocks + winningOptions
	totalLosses := losingStocks + losingOptions
	totalClosed := closedStockCount + closedOptionCount

	var winRate float64
	if totalClosed > 0 {
		winRate = (float64(totalWins) / float64(totalClosed)) * 100
	}

	var stockGains, optionGains float64
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_stocks WHERE user_id = ? AND profit_loss > 0", userID).Scan(&stockGains)
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_options WHERE user_id = ? AND profit_loss > 0", userID).Scan(&optionGains)
	totalGains := stockGains + optionGains

	var stockLosses, optionLosses float64
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_stocks WHERE user_id = ? AND profit_loss < 0", userID).Scan(&stockLosses)
	db.QueryRow("SELECT COALESCE(SUM(profit_loss), 0) FROM closed_options WHERE user_id = ? AND profit_loss < 0", userID).Scan(&optionLosses)
	totalLossAmount := stockLosses + optionLosses

	// Calculate average profit and average loss
	var avgProfit, avgLoss float64
	if totalWins > 0 {
		avgProfit = totalGains / float64(totalWins)
	}
	if totalLosses > 0 {
		avgLoss = totalLossAmount / float64(totalLosses)
	}

	var profitFactor float64
	if totalLossAmount != 0 {
		profitFactor = totalGains / -totalLossAmount
	}

	totalPositions := stockCount + optionCount

	winRateClass := "positive"
	if winRate < 50 {
		winRateClass = "negative"
	}

	profitFactorClass := "positive"
	if profitFactor < 1 {
		profitFactorClass = "negative"
	}

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
		<div class="stat-card">
			<h3>Win/Loss</h3>
			<p class="stat-value"><span class="positive">$%.2f</span> / <span class="negative">$%.2f</span></p>
		</div>
		<div class="stat-card">
			<h3>Win Rate</h3>
			<p class="stat-value %s">%.1f%%</p>
		</div>
		<div class="stat-card">
			<h3>Profit Factor</h3>
			<p class="stat-value %s">%.2f</p>
		</div>
	`, totalPositions, stockCount, optionCount, totalClosed, plClass, totalPL, avgProfit, avgLoss, winRateClass, winRate, profitFactorClass, profitFactor)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

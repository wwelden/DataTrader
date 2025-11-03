package handlers

import (
	"backend/views/components"
	"net/http"
)

func HandleHome(w http.ResponseWriter, r *http.Request) {
	components.AppLayout("DATATRADER - Trading Portfolio Manager", "home", components.HomePage()).Render(r.Context(), w)
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

	var winningStocks, winningOptions int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ? AND profit_loss > 0", userID).Scan(&winningStocks)
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ? AND profit_loss > 0", userID).Scan(&winningOptions)

	var losingStocks, losingOptions int
	db.QueryRow("SELECT COUNT(*) FROM closed_stocks WHERE user_id = ? AND profit_loss < 0", userID).Scan(&losingStocks)
	db.QueryRow("SELECT COUNT(*) FROM closed_options WHERE user_id = ? AND profit_loss < 0", userID).Scan(&losingOptions)

	totalWins := winningStocks + winningOptions
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

	var profitFactor float64
	if totalLossAmount != 0 {
		profitFactor = totalGains / -totalLossAmount
	}

	totalPositions := stockCount + optionCount

	stats := components.StatsData{
		TotalPositions: totalPositions,
		StockCount:     stockCount,
		OptionCount:    optionCount,
		ClosedCount:    totalClosed,
		TotalPL:        totalPL,
		TotalGains:     totalGains,
		TotalLosses:    totalLossAmount,
		WinRate:        winRate,
		ProfitFactor:   profitFactor,
	}

	w.Header().Set("Content-Type", "text/html")
	components.StatsCards(stats).Render(r.Context(), w)
}

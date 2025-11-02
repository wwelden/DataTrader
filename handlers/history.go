package handlers

import (
	"backend/types"
	"fmt"
	"html"
	"net/http"
	"path/filepath"
)

func HandleHistory(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join("views", "history.html"))
}

func HandleGetClosedStocks(w http.ResponseWriter, r *http.Request) {
	// TODO: Get user_id from session/auth
	userID := 1

	rows, err := db.Query(`
		SELECT ticker, open_date, close_date, quantity, cost_basis, sell_price, profit_loss
		FROM closed_stocks
		WHERE user_id = ?
		ORDER BY close_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch closed stocks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var closedStocks []types.ClosedStock
	for rows.Next() {
		var cs types.ClosedStock
		if err := rows.Scan(&cs.Ticker, &cs.OpenDate, &cs.CloseDate, &cs.Quantity, &cs.CostBasis, &cs.SellPrice, &cs.ProfitLoss); err != nil {
			continue
		}
		closedStocks = append(closedStocks, cs)
	}

	if len(closedStocks) == 0 {
		w.Write([]byte("<p class=\"empty-state\">No closed stock trades found.</p>"))
		return
	}

	// Render HTML table
	htmlContent := `<table class="history-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Open Date</th>
				<th>Close Date</th>
				<th>Quantity</th>
				<th>Cost Basis</th>
				<th>Sell Price</th>
				<th>P/L</th>
				<th>P/L %</th>
			</tr>
		</thead>
		<tbody>`

	for _, cs := range closedStocks {
		plClass := "positive"
		if cs.ProfitLoss < 0 {
			plClass = "negative"
		}
		plPercent := cs.PlPercent()

		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%.2f</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
				<td class="%s">$%.2f</td>
				<td class="%s">%.2f%%</td>
			</tr>`, html.EscapeString(cs.Ticker), cs.OpenDate, cs.CloseDate, cs.Quantity,
			cs.CostBasis, cs.SellPrice, plClass, cs.ProfitLoss, plClass, plPercent)
	}

	htmlContent += `</tbody></table>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

func HandleGetClosedOptions(w http.ResponseWriter, r *http.Request) {
	// TODO: Get user_id from session/auth
	userID := 1

	rows, err := db.Query(`
		SELECT ticker, price, premium, strike, exp_date, type, collateral, purchase_date, close_date, sell_price, profit_loss
		FROM closed_options
		WHERE user_id = ?
		ORDER BY close_date DESC
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch closed options", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var closedOptions []types.ClosedOption
	for rows.Next() {
		var co types.ClosedOption
		if err := rows.Scan(&co.Ticker, &co.Price, &co.Premium, &co.Strike, &co.ExpDate, &co.Type, &co.Collateral, &co.PurchaseDate, &co.CloseDate, &co.SellPrice, &co.ProfitLoss); err != nil {
			continue
		}
		closedOptions = append(closedOptions, co)
	}

	if len(closedOptions) == 0 {
		w.Write([]byte("<p class=\"empty-state\">No closed option trades found.</p>"))
		return
	}

	// Render HTML table
	htmlContent := `<table class="history-table">
		<thead>
			<tr>
				<th>Ticker</th>
				<th>Type</th>
				<th>Strike</th>
				<th>Premium</th>
				<th>Exp Date</th>
				<th>Purchase Date</th>
				<th>Close Date</th>
				<th>Sell Price</th>
				<th>P/L</th>
				<th>P/L %</th>
			</tr>
		</thead>
		<tbody>`

	for _, co := range closedOptions {
		plClass := "positive"
		if co.ProfitLoss < 0 {
			plClass = "negative"
		}
		plPercent := co.PlPercent()

		htmlContent += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>$%.2f</td>
				<td>$%.2f</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>$%.2f</td>
				<td class="%s">$%.2f</td>
				<td class="%s">%.2f%%</td>
			</tr>`, html.EscapeString(co.Ticker), co.Type, co.Strike, co.Premium,
			co.ExpDate, co.PurchaseDate, co.CloseDate, co.SellPrice, plClass, co.ProfitLoss, plClass, plPercent)
	}

	htmlContent += `</tbody></table>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}

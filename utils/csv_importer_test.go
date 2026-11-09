package utils

import (
	"backend/types"
	"reflect"
	"strings"
	"testing"
)

const brokerageHeader = `"Activity Date","Process Date","Settle Date","Instrument","Description","Trans Code","Quantity","Price","Amount"`

// row builds one brokerage CSV line in the column order ParseBrokerageCSV expects.
func row(date, ticker, desc, code, qty, price, amount string) string {
	return strings.Join([]string{
		`"` + date + `"`, `"` + date + `"`, `"` + date + `"`,
		`"` + ticker + `"`, `"` + desc + `"`, `"` + code + `"`,
		`"` + qty + `"`, `"` + price + `"`, `"` + amount + `"`,
	}, ",")
}

func csvOf(rows ...string) string {
	return brokerageHeader + "\n" + strings.Join(rows, "\n") + "\n"
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"10", 10},
		{"-3.25", -3.25},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"1,000", 0}, // commas must be cleaned first; ParseFloat does not strip them
	}
	for _, tt := range tests {
		if got := ParseFloat(tt.in); got != tt.want {
			t.Errorf("ParseFloat(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCleanCurrencyString(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"$1,234.56", 1234.56},
		{"1234.56", 1234.56},
		{`"$12.00"`, 12},
		{"($500.00)", -500},
		{"($1,500.25)", -1500.25},
		{"-$3.00", -3},
		{"$0.00", 0},
		{"", 0},
		{"N/A", 0},
	}
	for _, tt := range tests {
		if got := CleanCurrencyString(tt.in); got != tt.want {
			t.Errorf("CleanCurrencyString(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestMakeStockTradeFromBrokerageFormat(t *testing.T) {
	data := []string{"1/15/2025", "1/15/2025", "1/16/2025", " AAPL ", "Apple Inc", "Buy", "10", "$150.25", "($1,502.50)"}
	got := MakeStockTradeFromBrokerageFormat(types.Buy, data)
	want := types.StockTrade{
		Ticker:   "AAPL",
		Date:     "1/15/2025",
		Code:     types.Buy,
		Price:    150.25,
		Amount:   -1502.50,
		Quantity: 10,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMakeOptionTradeFromBrokerageFormat(t *testing.T) {
	tests := []struct {
		name string
		data []string
		want types.OptionTrade
	}{
		{
			name: "call with full description",
			data: []string{"2/3/2025", "", "", "TSLA", "TSLA 3/21/2025 Call $250.00", "BTO", "2", "$5.10", "($1,020.00)"},
			want: types.OptionTrade{
				Ticker: "TSLA", Date: "2/3/2025", Code: types.BTO, Price: 5.10, Amount: -1020, Quantity: 2,
				Strike: 250, ExpDate: "3/21/2025", OptionType: types.Call, Premium: 5.10,
			},
		},
		{
			name: "put with strike containing comma",
			data: []string{"2/3/2025", "", "", "SPX", "SPX 12/19/2025 Put $5,000.00", "STO", "1", "$12.00", "$1,200.00"},
			want: types.OptionTrade{
				Ticker: "SPX", Date: "2/3/2025", Code: types.STO, Price: 12, Amount: 1200, Quantity: 1,
				Strike: 5000, ExpDate: "12/19/2025", OptionType: types.Put, Premium: 12,
			},
		},
		{
			name: "strike without dollar sign",
			data: []string{"2/3/2025", "", "", "AMD", "AMD 1/9/2026 Call 140", "STC", "1", "$1.00", "$100.00"},
			want: types.OptionTrade{
				Ticker: "AMD", Date: "2/3/2025", Code: types.STC, Price: 1, Amount: 100, Quantity: 1,
				Strike: 140, ExpDate: "1/9/2026", OptionType: types.Call, Premium: 1,
			},
		},
		{
			name: "unstructured description falls back to keyword match",
			data: []string{"2/3/2025", "", "", "AMD", "some weird put text", "BTC", "1", "$2.00", "($200.00)"},
			want: types.OptionTrade{
				Ticker: "AMD", Date: "2/3/2025", Code: types.BTC, Price: 2, Amount: -200, Quantity: 1,
				OptionType: types.Put, Premium: 2,
			},
		},
		{
			name: "unstructured description with call keyword",
			data: []string{"2/3/2025", "", "", "AMD", "Covered CALL thing", "STO", "1", "$2.00", "$200.00"},
			want: types.OptionTrade{
				Ticker: "AMD", Date: "2/3/2025", Code: types.STO, Price: 2, Amount: 200, Quantity: 1,
				OptionType: types.Call, Premium: 2,
			},
		},
		{
			name: "empty description sets only premium",
			data: []string{"2/3/2025", "", "", "AMD", "", "BTO", "1", "$2.00", "($200.00)"},
			want: types.OptionTrade{
				Ticker: "AMD", Date: "2/3/2025", Code: types.BTO, Price: 2, Amount: -200, Quantity: 1,
				Premium: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeOptionTradeFromBrokerageFormat(tt.want.Code, tt.data)
			if got != tt.want {
				t.Errorf("\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

func TestParseOptionDetailsDescriptionTickerOverridesInstrument(t *testing.T) {
	trade := types.OptionTrade{Ticker: "WRONG", Price: 3}
	parseOptionDetailsFromDescription(&trade, "NVDA 6/20/2025 Put $100.00")
	if trade.Ticker != "NVDA" {
		t.Errorf("Ticker = %q, want NVDA (description should win)", trade.Ticker)
	}
	if trade.Premium != 3 {
		t.Errorf("Premium = %v, want 3", trade.Premium)
	}
}

func TestParseBrokerageCSVErrors(t *testing.T) {
	if _, err := ParseBrokerageCSV(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty CSV: err = %v, want 'empty' error", err)
	}
	if _, err := ParseBrokerageCSV(brokerageHeader + "\n"); err == nil || !strings.Contains(err.Error(), "header") {
		t.Errorf("header-only CSV: err = %v, want 'header' error", err)
	}
}

func TestParseBrokerageCSV(t *testing.T) {
	csv := csvOf(
		row("1/2/2025", "AAPL", "Apple", "Buy", "10", "$150.00", "($1,500.00)"),
		row("1/3/2025", "AAPL", "Apple", "Sell", "5", "$160.00", "$800.00"),
		row("1/4/2025", "TSLA", "TSLA 2/21/2025 Call $250.00", "BTO", "1", "$5.00", "($500.00)"),
		row("1/5/2025", "TSLA", "TSLA 2/21/2025 Call $250.00", "STC", "1", "$7.00", "$700.00"),
		row("1/6/2025", "AMD", "AMD 3/21/2025 Put $120.00", "STO", "2", "$3.00", "$600.00"),
		row("1/7/2025", "AMD", "AMD 3/21/2025 Put $120.00", "BTC", "2", "$1.00", "($200.00)"),
	)

	got, err := ParseBrokerageCSV(csv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.StockTrades) != 2 {
		t.Fatalf("stock trades = %d, want 2", len(got.StockTrades))
	}
	if len(got.OptionTrades) != 4 {
		t.Fatalf("option trades = %d, want 4", len(got.OptionTrades))
	}

	wantStockCodes := []types.TradeCode{types.Buy, types.Sell}
	for i, tr := range got.StockTrades {
		if tr.Code != wantStockCodes[i] {
			t.Errorf("stock trade %d code = %s, want %s", i, tr.Code, wantStockCodes[i])
		}
	}
	wantOptionCodes := []types.TradeCode{types.BTO, types.STC, types.STO, types.BTC}
	for i, tr := range got.OptionTrades {
		if tr.Code != wantOptionCodes[i] {
			t.Errorf("option trade %d code = %s, want %s", i, tr.Code, wantOptionCodes[i])
		}
	}

	put := got.OptionTrades[2]
	if put.OptionType != types.Put || put.Strike != 120 || put.ExpDate != "3/21/2025" || put.Quantity != 2 {
		t.Errorf("STO put parsed wrong: %+v", put)
	}
}

func TestParseBrokerageCSVSkipsRows(t *testing.T) {
	csv := csvOf(
		row("1/2/2025", "", "Interest payment", "INT", "", "", "$0.12"),            // no ticker
		row("1/2/2025", "AAPL", "Apple", "CDIV", "", "", "$2.00"),                  // dividend: unsupported code
		row("1/2/2025", "AAPL", "Apple", "Buy", "0", "$150.00", "$0.00"),           // zero quantity
		row("1/2/2025", "AAPL", "Apple", "Buy", "", "$150.00", "$0.00"),            // missing quantity
		`"1/2/2025","1/2/2025","1/2/2025","AAPL","Apple","Buy","10"`,               // too few columns
		row("1/2/2025", "MSFT", "Microsoft", "Buy", "3", "$400.00", "($1,200.00)"), // valid
		"",
		`"The data provided is for informational purposes only."`, // footer line
	)

	got, err := ParseBrokerageCSV(csv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []types.StockTrade{{Ticker: "MSFT", Date: "1/2/2025", Code: types.Buy, Price: 400, Amount: -1200, Quantity: 3}}
	if !reflect.DeepEqual(got.StockTrades, want) {
		t.Errorf("stock trades = %+v, want %+v", got.StockTrades, want)
	}
	if len(got.OptionTrades) != 0 {
		t.Errorf("option trades = %+v, want none", got.OptionTrades)
	}
}

func TestParseBrokerageCSVHeaderOnlyRowsAfterFiltering(t *testing.T) {
	got, err := ParseBrokerageCSV(csvOf(row("1/2/2025", "", "", "INT", "", "", "$1.00")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StockTrades == nil || got.OptionTrades == nil {
		t.Error("result slices should be non-nil even when empty")
	}
	if len(got.StockTrades)+len(got.OptionTrades) != 0 {
		t.Errorf("expected no trades, got %+v", got)
	}
}

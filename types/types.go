package types

type TradeCode string
type OptionType string

const (
	Buy  TradeCode  = "Buy"
	Sell TradeCode  = "Sell"
	BTO  TradeCode  = "BTO"
	BTC  TradeCode  = "BTC"
	STO  TradeCode  = "STO"
	STC  TradeCode  = "STC"
	Put  OptionType = "Put"
	Call OptionType = "Call"
	CSP  OptionType = "CSP"
	CC   OptionType = "CC"
)

type StockTrade struct {
	ID       string    `json:"id"`
	Ticker   string    `json:"ticker"`
	Date     string    `json:"date"`
	Code     TradeCode `json:"code"`
	Price    float64   `json:"price"`
	Amount   float64   `json:"amount"`
	Quantity float64   `json:"quantity"`
}
type OptionTrade struct {
	ID       string    `json:"id"`
	Ticker   string    `json:"ticker"`
	Date     string    `json:"date"`
	Code     TradeCode `json:"code"`
	Price    float64   `json:"price"`
	Amount   float64   `json:"amount"`
	Quantity float64   `json:"quantity"`

	Strike     float64    `json:"strike"`
	ExpDate    string     `json:"exp_date"`
	OptionType OptionType `json:"option_type"`
	Premium    float64    `json:"premium"`
}

type StockPos struct {
	ID        int     `json:"id"`
	OpenDate  string  `json:"open_date"`
	Ticker    string  `json:"ticker"`
	Quantity  float64 `json:"quantity"`
	CostBasis float64 `json:"cost_basis"`
}

type User struct {
	Username       string               `json:"username"`
	Password       string               `json:"password,omitempty"`
	ClerkUserID    string               `json:"clerk_user_id"`
	StockTrades    []StockTrade         `json:"stock_trades"`
	OptionTrades   []OptionTrade        `json:"option_trades"`
	Positions      map[string]StockPos  `json:"positions"`
	StockHistory   []ClosedStock        `json:"stock_history"`
	Options        map[string]OptionPos `json:"options"`
	OptionsHistory []ClosedOption       `json:"options_history"`
}

type ClosedStock struct {
	ID         int     `json:"id"`
	Ticker     string  `json:"ticker"`
	OpenDate   string  `json:"open_date"`
	CloseDate  string  `json:"close_date"`
	Quantity   float64 `json:"quantity"`
	CostBasis  float64 `json:"cost_basis"`
	SellPrice  float64 `json:"sell_price"`
	ProfitLoss float64 `json:"profit_loss"`
}

type OptionPos struct {
	ID           int        `json:"id"`
	Ticker       string     `json:"ticker"`
	Price        float64    `json:"price"`
	Premium      float64    `json:"premium"`
	Strike       float64    `json:"strike"`
	ExpDate      string     `json:"exp_date"`
	Type         OptionType `json:"type"`
	Collateral   float64    `json:"collateral"`
	Quantity     float64    `json:"quantity"`
	PurchaseDate string     `json:"purchase_date"`
}

type ClosedOption struct {
	ID           int        `json:"id"`
	Ticker       string     `json:"ticker"`
	Price        float64    `json:"price"`
	Premium      float64    `json:"premium"`
	Strike       float64    `json:"strike"`
	ExpDate      string     `json:"exp_date"`
	Type         OptionType `json:"type"`
	Collateral   float64    `json:"collateral"`
	Quantity     float64    `json:"quantity"`
	PurchaseDate string     `json:"purchase_date"`
	CloseDate    string     `json:"close_date"`
	SellPrice    float64    `json:"sell_price"`
	ProfitLoss   float64    `json:"profit_loss"`
}

func (cs ClosedStock) CalculateROR() float64 {
	return cs.ProfitLoss / cs.CostBasis
}
func (co ClosedOption) CalculateROR() float64 {
	switch co.Type {
	case Call, Put:
		return co.ProfitLoss / co.Premium
	case CC, CSP:
		return co.ProfitLoss / co.Collateral
	default:
		return 0
	}
}
func (co ClosedOption) RORPercent() float64 {
	return co.CalculateROR() * 100
}
func (stock ClosedStock) PlPercent() float64 {
	return (stock.ProfitLoss / (stock.CostBasis * stock.Quantity)) * 100
}
func (option ClosedOption) PlPercent() float64 {
	return (option.ProfitLoss / (option.Premium + option.Collateral)) * 100
}

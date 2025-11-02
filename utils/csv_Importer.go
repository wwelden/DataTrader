package utils

import (
	"backend/types"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func ParseFloat(input string) float64 {
	ret, err := strconv.ParseFloat(input, 64)
	if err != nil {
		fmt.Println("Error:", err)
	}
	return ret
}

func CleanCurrencyString(s string) float64 {
	s = strings.Trim(s, "\"")
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = "-" + strings.Trim(s, "()")
	}
	return ParseFloat(s)
}

func MakeStockTradeFromBrokerageFormat(tc types.TradeCode, data []string) types.StockTrade {
	price := CleanCurrencyString(data[7])
	amount := CleanCurrencyString(data[8])
	quantity := ParseFloat(data[6])
	ticker := strings.TrimSpace(data[3])

	return types.StockTrade{
		Ticker:   ticker,
		Date:     data[0],
		Code:     tc,
		Price:    price,
		Amount:   amount,
		Quantity: quantity,
	}
}

func MakeOptionTradeFromBrokerageFormat(tc types.TradeCode, data []string) types.OptionTrade {
	price := CleanCurrencyString(data[7])
	amount := CleanCurrencyString(data[8])
	quantity := ParseFloat(data[6])
	ticker := strings.TrimSpace(data[3])
	description := ""
	if len(data) > 4 {
		description = strings.TrimSpace(data[4])
	}

	trade := types.OptionTrade{
		Ticker:   ticker,
		Date:     data[0],
		Code:     tc,
		Price:    price,
		Amount:   amount,
		Quantity: quantity,
	}

	parseOptionDetailsFromDescription(&trade, description)

	return trade
}

func parseOptionDetailsFromDescription(trade *types.OptionTrade, description string) {
	if description == "" {
		trade.Premium = trade.Price
		return
	}

	re := regexp.MustCompile(`^([A-Z]+)\s+(\d{1,2}/\d{1,2}/\d{4})\s+(Call|Put)\s+\$?([\d,.]+)`)
	matches := re.FindStringSubmatch(description)

	if len(matches) >= 5 {
		trade.Ticker = matches[1]
		trade.ExpDate = matches[2]

		optionTypeStr := strings.ToLower(matches[3])
		if optionTypeStr == "call" {
			trade.OptionType = types.Call
		} else if optionTypeStr == "put" {
			trade.OptionType = types.Put
		}

		strikeStr := strings.ReplaceAll(matches[4], ",", "")
		trade.Strike = ParseFloat(strikeStr)

		trade.Premium = trade.Price
	} else {
		trade.Premium = trade.Price

		descLower := strings.ToLower(description)
		if strings.Contains(descLower, "call") {
			trade.OptionType = types.Call
		} else if strings.Contains(descLower, "put") {
			trade.OptionType = types.Put
		}
	}
}

type ImportedTrades struct {
	StockTrades  []types.StockTrade
	OptionTrades []types.OptionTrade
}

func ParseBrokerageCSV(csvContent string) (*ImportedTrades, error) {
	rows := Parse(csvContent)

	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV is empty")
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("CSV only contains header row")
	}

	result := &ImportedTrades{
		StockTrades:  []types.StockTrade{},
		OptionTrades: []types.OptionTrade{},
	}

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 9 {
			continue
		}
		ticker := strings.Trim(row[3], "\" ")
		if ticker == "" {
			continue
		}
		transCode := strings.Trim(row[5], "\" ")
		switch transCode {
		case "Buy":
			record := MakeStockTradeFromBrokerageFormat(types.Buy, row)
			if record.Quantity > 0 {
				result.StockTrades = append(result.StockTrades, record)
			}
		case "Sell":
			record := MakeStockTradeFromBrokerageFormat(types.Sell, row)
			if record.Quantity > 0 {
				result.StockTrades = append(result.StockTrades, record)
			}
		case "BTC":
			record := MakeOptionTradeFromBrokerageFormat(types.BTC, row)
			if record.Quantity > 0 {
				result.OptionTrades = append(result.OptionTrades, record)
			}
		case "BTO":
			record := MakeOptionTradeFromBrokerageFormat(types.BTO, row)
			if record.Quantity > 0 {
				result.OptionTrades = append(result.OptionTrades, record)
			}
		case "STO":
			record := MakeOptionTradeFromBrokerageFormat(types.STO, row)
			if record.Quantity > 0 {
				result.OptionTrades = append(result.OptionTrades, record)
			}
		case "STC":
			record := MakeOptionTradeFromBrokerageFormat(types.STC, row)
			if record.Quantity > 0 {
				result.OptionTrades = append(result.OptionTrades, record)
			}
		}
	}

	return result, nil
}

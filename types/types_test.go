package types

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestClosedStockCalculateROR(t *testing.T) {
	cs := ClosedStock{ProfitLoss: 50, CostBasis: 200}
	if got := cs.CalculateROR(); !approx(got, 0.25) {
		t.Errorf("CalculateROR = %v, want 0.25", got)
	}
}

func TestClosedStockPlPercent(t *testing.T) {
	tests := []struct {
		name string
		cs   ClosedStock
		want float64
	}{
		{"gain", ClosedStock{ProfitLoss: 100, CostBasis: 50, Quantity: 10}, 20},
		{"loss", ClosedStock{ProfitLoss: -250, CostBasis: 100, Quantity: 5}, -50},
		{"flat", ClosedStock{ProfitLoss: 0, CostBasis: 100, Quantity: 5}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cs.PlPercent(); !approx(got, tt.want) {
				t.Errorf("PlPercent = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClosedOptionCalculateROR(t *testing.T) {
	tests := []struct {
		name string
		co   ClosedOption
		want float64
	}{
		{"call uses premium", ClosedOption{Type: Call, ProfitLoss: 50, Premium: 200, Collateral: 9999}, 0.25},
		{"put uses premium", ClosedOption{Type: Put, ProfitLoss: -100, Premium: 400}, -0.25},
		{"cc uses collateral", ClosedOption{Type: CC, ProfitLoss: 200, Premium: 200, Collateral: 10000}, 0.02},
		{"csp uses collateral", ClosedOption{Type: CSP, ProfitLoss: 300, Premium: 300, Collateral: 15000}, 0.02},
		{"unknown type", ClosedOption{Type: "weird", ProfitLoss: 300, Premium: 300}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.co.CalculateROR(); !approx(got, tt.want) {
				t.Errorf("CalculateROR = %v, want %v", got, tt.want)
			}
			if got := tt.co.RORPercent(); !approx(got, tt.want*100) {
				t.Errorf("RORPercent = %v, want %v", got, tt.want*100)
			}
		})
	}
}

func TestClosedOptionZeroDenominator(t *testing.T) {
	co := ClosedOption{Type: CSP, ProfitLoss: 100, Collateral: 0}
	if got := co.CalculateROR(); !math.IsInf(got, 1) {
		t.Errorf("CalculateROR with zero collateral = %v, want +Inf", got)
	}
}

func TestClosedOptionPlPercent(t *testing.T) {
	co := ClosedOption{ProfitLoss: 50, Premium: 100, Collateral: 400}
	if got := co.PlPercent(); !approx(got, 10) {
		t.Errorf("PlPercent = %v, want 10", got)
	}
}

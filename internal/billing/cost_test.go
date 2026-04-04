package billing

import (
	"testing"
)

func TestCostCalculator_KnownModel(t *testing.T) {
	calc := NewCostCalculator()

	// llama-3-8b: prompt $0.10/M, completion $0.20/M
	cost := calc.Calculate("llama-3-8b", 1000, 500)
	expectedPrompt := 1000.0 / 1_000_000 * 0.10   // 0.0001
	expectedCompletion := 500.0 / 1_000_000 * 0.20 // 0.0001
	expected := expectedPrompt + expectedCompletion

	if cost != expected {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCostCalculator_UnknownModel(t *testing.T) {
	calc := NewCostCalculator()

	// Unknown model uses default: prompt $0.50/M, completion $1.00/M
	cost := calc.Calculate("unknown-model", 1_000_000, 1_000_000)
	expected := 0.50 + 1.00

	if cost != expected {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCostCalculator_ZeroTokens(t *testing.T) {
	calc := NewCostCalculator()

	cost := calc.Calculate("llama-3-8b", 0, 0)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}

func TestCostCalculator_LargeVolume(t *testing.T) {
	calc := NewCostCalculator()

	// 10M prompt + 5M completion for llama-3-70b ($0.50 + $1.00 per M)
	cost := calc.Calculate("llama-3-70b", 10_000_000, 5_000_000)
	expected := 10.0*0.50 + 5.0*1.00 // $5 + $5 = $10

	if cost != expected {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

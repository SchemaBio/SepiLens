package score_test

import (
	"errors"
	"testing"

	"github.com/pzweuj/sepilens/internal/score"
)

func TestCNVBand_ParsingAndClassification(t *testing.T) {
	tests := []struct {
		input       string
		expectedVal int64
		expectedCat string
	}{
		{"0.99", 99, score.ClassPathogenic},
		{"+0.99", 99, score.ClassPathogenic},
		{"1.00", 100, score.ClassPathogenic},
		{"2.50", 250, score.ClassPathogenic},
		{"0.98", 98, score.ClassLikelyPathogenic},
		{"0.90", 90, score.ClassLikelyPathogenic},
		{"+0.95", 95, score.ClassLikelyPathogenic},
		{"0.89", 89, score.ClassVUS},
		{"0", 0, score.ClassVUS},
		{"0.0", 0, score.ClassVUS},
		{"0.00", 0, score.ClassVUS},
		{"-0.00", 0, score.ClassVUS},
		{"-0.89", -89, score.ClassVUS},
		{"-0.90", -90, score.ClassLikelyBenign},
		{"-0.95", -95, score.ClassLikelyBenign},
		{"-0.98", -98, score.ClassLikelyBenign},
		{"-0.99", -99, score.ClassBenign},
		{"-1.00", -100, score.ClassBenign},
		{"-1.50", -150, score.ClassBenign},
	}

	for _, tc := range tests {
		val, err := score.ParseHundredths(tc.input)
		if err != nil {
			t.Fatalf("unexpected error parsing %q: %v", tc.input, err)
		}
		if val != tc.expectedVal {
			t.Errorf("for input %q, expected %d hundredths, got %d", tc.input, tc.expectedVal, val)
		}
		cat := score.Classify(val)
		if cat != tc.expectedCat {
			t.Errorf("for input %q (val=%d), expected %q, got %q", tc.input, val, tc.expectedCat, cat)
		}
	}
}

func TestCNVBand_InvalidInputsRejected(t *testing.T) {
	invalid := []string{
		"",
		"   ",
		"0.985",
		"-0.999",
		"1e-1",
		"1E-2",
		"NaN",
		"Infinity",
		"-Infinity",
		"0.1.2",
		"abc",
		"--0.9",
		"+",
		"-",
	}

	for _, input := range invalid {
		_, err := score.ParseHundredths(input)
		if err == nil {
			t.Fatalf("expected error for invalid input %q, got nil", input)
		}
	}

	_, errDec := score.ParseHundredths("0.985")
	if !errors.Is(errDec, score.ErrTooManyDecimals) {
		t.Fatalf("expected ErrTooManyDecimals for 0.985, got %v", errDec)
	}
}

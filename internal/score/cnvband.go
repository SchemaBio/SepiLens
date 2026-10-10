package score

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidScoreFormat = errors.New("invalid score format")
	ErrTooManyDecimals    = errors.New("score precision exceeds two decimal places")
)

const (
	ClassPathogenic       = "Pathogenic"
	ClassLikelyPathogenic = "Likely pathogenic"
	ClassVUS              = "VUS"
	ClassLikelyBenign     = "Likely benign"
	ClassBenign           = "Benign"
)

// ParseHundredths parses a decimal score string like "0.99" or "-1.50" into hundredths integer (99, -150).
// Rejects scientific notation, NaN, infinity, and fractions with more than two decimal digits.
func ParseHundredths(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("%w: empty score string", ErrInvalidScoreFormat)
	}

	if strings.ContainsAny(s, "eE") {
		return 0, fmt.Errorf("%w: scientific notation is forbidden", ErrInvalidScoreFormat)
	}

	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	if s == "" {
		return 0, fmt.Errorf("%w: missing digits after sign", ErrInvalidScoreFormat)
	}

	if strings.ContainsAny(s, "+-") {
		return 0, fmt.Errorf("%w: extra sign character", ErrInvalidScoreFormat)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("%w: multiple decimal separators", ErrInvalidScoreFormat)
	}

	intPartStr := parts[0]
	if intPartStr == "" {
		intPartStr = "0"
	}
	intVal, err := strconv.ParseInt(intPartStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid integer part", ErrInvalidScoreFormat)
	}

	var decVal int64
	if len(parts) == 2 {
		decPartStr := parts[1]
		if len(decPartStr) == 0 {
			return 0, fmt.Errorf("%w: empty fractional part", ErrInvalidScoreFormat)
		}
		if len(decPartStr) > 2 {
			return 0, fmt.Errorf("%w: expected at most 2 decimal digits, got %d", ErrTooManyDecimals, len(decPartStr))
		}
		// Pad to 2 digits
		if len(decPartStr) == 1 {
			decPartStr += "0"
		}
		decVal, err = strconv.ParseInt(decPartStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid fractional digits", ErrInvalidScoreFormat)
		}
	}

	total := intVal*100 + decVal
	if negative {
		total = -total
	}
	return total, nil
}

// Classify maps hundredths score integer to the five ACMG/ClinGen 2020 CNV classification categories.
// >= 99 (+0.99)               -> Pathogenic
// 90 .. 98 (+0.90 .. +0.98)   -> Likely pathogenic
// -89 .. 89 (-0.89 .. +0.89)  -> VUS
// -98 .. -90 (-0.98 .. -0.90) -> Likely benign
// <= -99 (-0.99)              -> Benign
func Classify(scoreHundredths int64) string {
	switch {
	case scoreHundredths >= 99:
		return ClassPathogenic
	case scoreHundredths >= 90:
		return ClassLikelyPathogenic
	case scoreHundredths >= -89:
		return ClassVUS
	case scoreHundredths >= -98:
		return ClassLikelyBenign
	default:
		return ClassBenign
	}
}

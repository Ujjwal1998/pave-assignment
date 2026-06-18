package money

import (
	"fmt"

	"github.com/govalues/decimal"
	"pave-bank/domain"
)

// Convert converts amount from one ISO 4217 currency to another using static rates.
// The result is rounded to the target currency's decimal scale.
func Convert(amount decimal.Decimal, from, to string) (decimal.Decimal, error) {
	if from == to {
		return amount, nil
	}

	rate, err := conversionRate(from, to)
	if err != nil {
		return decimal.Decimal{}, err
	}

	converted, err := amount.Mul(rate)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("convert amount: %w", err)
	}

	return RoundToCurrencyScale(converted, to)
}

// SupportsConversion reports whether a static rate exists for from → to.
func SupportsConversion(from, to string) bool {
	if from == to {
		return true
	}
	toRates, ok := rates[from]
	if !ok {
		return false
	}
	_, ok = toRates[to]
	return ok
}

func conversionRate(from, to string) (decimal.Decimal, error) {
	toRates, ok := rates[from]
	if !ok {
		return decimal.Decimal{}, domain.ErrUnsupportedCurrencyPair
	}
	rateStr, ok := toRates[to]
	if !ok {
		return decimal.Decimal{}, domain.ErrUnsupportedCurrencyPair
	}
	rate, err := decimal.Parse(rateStr)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("parse rate %s→%s: %w", from, to, err)
	}
	return rate, nil
}

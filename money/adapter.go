package money

import (
	"fmt"

	"github.com/govalues/decimal"
	goMoney "github.com/govalues/money"
)

// ToAmount converts a decimal value and ISO 4217 currency code into a money.Amount.
func ToAmount(d decimal.Decimal, currencyCode string) (goMoney.Amount, error) {
	amount, err := goMoney.ParseAmount(currencyCode, d.String())
	if err != nil {
		return goMoney.Amount{}, fmt.Errorf("parse amount: %w", err)
	}
	return amount, nil
}

// FromAmount converts a money.Amount into its decimal value and currency code.
func FromAmount(a goMoney.Amount) (decimal.Decimal, string) {
	return a.Decimal(), a.Curr().Code()
}

// RoundToCurrencyScale rounds d to the number of decimal places defined by the
// currency (e.g. 2 for USD, 0 for JPY) using banker's rounding.
func RoundToCurrencyScale(d decimal.Decimal, currencyCode string) (decimal.Decimal, error) {
	amount, err := ToAmount(d, currencyCode)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("round to currency scale: %w", err)
	}
	return amount.RoundToCurr().Decimal(), nil
}

// ValidateCurrency returns an error if code is not a recognised ISO 4217 currency.
// Enforces exactly 3 uppercase ASCII letters to match the DB schema constraint.
func ValidateCurrency(code string) error {
	if len(code) != 3 {
		return fmt.Errorf("currency %q is not a supported ISO 4217 code", code)
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return fmt.Errorf("currency %q is not a supported ISO 4217 code", code)
		}
	}
	if _, err := goMoney.ParseAmount(code, "0"); err != nil {
		return fmt.Errorf("currency %q is not a supported ISO 4217 code", code)
	}
	return nil
}

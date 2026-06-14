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

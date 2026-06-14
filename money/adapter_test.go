package money

import (
	"testing"

	"github.com/govalues/decimal"
	goMoney "github.com/govalues/money"
)

func TestToAmountFromAmountRoundTrip(t *testing.T) {
	original, err := decimal.Parse("99.00")
	if err != nil {
		t.Fatalf("parse decimal: %v", err)
	}

	amount, err := ToAmount(original, "USD")
	if err != nil {
		t.Fatalf("ToAmount: %v", err)
	}

	got, currency := FromAmount(amount)
	if currency != "USD" {
		t.Fatalf("currency = %q, want USD", currency)
	}
	if !got.Equal(original) {
		t.Fatalf("decimal = %s, want %s", got, original)
	}
}

func TestAddEnforcesCurrencyHomogeneity(t *testing.T) {
	usd, err := ToAmount(decimal.MustParse("10.00"), "USD")
	if err != nil {
		t.Fatalf("ToAmount USD: %v", err)
	}
	eur, err := ToAmount(decimal.MustParse("10.00"), "EUR")
	if err != nil {
		t.Fatalf("ToAmount EUR: %v", err)
	}

	if _, err := usd.Add(eur); err == nil {
		t.Fatal("expected error when adding USD and EUR")
	}
}

func TestZeroAccumulator(t *testing.T) {
	total, err := goMoney.NewAmountFromInt64("USD", 0, 0, 0)
	if err != nil {
		t.Fatalf("NewAmountFromInt64: %v", err)
	}

	line, err := ToAmount(decimal.MustParse("5.00"), "USD")
	if err != nil {
		t.Fatalf("ToAmount: %v", err)
	}

	total, err = total.Add(line)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, currency := FromAmount(total)
	if currency != "USD" {
		t.Fatalf("currency = %q, want USD", currency)
	}
	if !got.Equal(decimal.MustParse("5.00")) {
		t.Fatalf("total = %s, want 5.00", got)
	}
}

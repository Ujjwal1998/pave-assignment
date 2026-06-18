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

func TestRoundToCurrencyScale(t *testing.T) {
	usd, err := RoundToCurrencyScale(decimal.MustParse("104.000000000"), "USD")
	if err != nil {
		t.Fatalf("USD round: %v", err)
	}
	if usd.String() != "104.00" {
		t.Fatalf("USD total = %q, want 104.00", usd.String())
	}

	jpy, err := RoundToCurrencyScale(decimal.MustParse("1050.500000000"), "JPY")
	if err != nil {
		t.Fatalf("JPY round: %v", err)
	}
	if jpy.String() != "1050" {
		t.Fatalf("JPY total = %q, want 1050", jpy.String())
	}

	gel, err := RoundToCurrencyScale(decimal.MustParse("42.567000000"), "GEL")
	if err != nil {
		t.Fatalf("GEL round: %v", err)
	}
	if gel.String() != "42.57" {
		t.Fatalf("GEL total = %q, want 42.57", gel.String())
	}
}

func TestValidateCurrency(t *testing.T) {
	if err := ValidateCurrency("USD"); err != nil {
		t.Fatalf("USD should be valid: %v", err)
	}
	if err := ValidateCurrency("EUR"); err != nil {
		t.Fatalf("EUR should be valid: %v", err)
	}
	if err := ValidateCurrency("GEL"); err != nil {
		t.Fatalf("GEL should be valid: %v", err)
	}
	if err := ValidateCurrency("ZZZ"); err == nil {
		t.Fatal("ZZZ should be rejected")
	}
	if err := ValidateCurrency("usd"); err == nil {
		t.Fatal("lowercase usd should be rejected")
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

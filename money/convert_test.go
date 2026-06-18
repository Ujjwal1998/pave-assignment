package money

import (
	"errors"
	"testing"

	"github.com/govalues/decimal"
	"pave-bank/domain"
)

func TestConvertIdentity(t *testing.T) {
	amount := decimal.MustParse("42.50")
	got, err := Convert(amount, "USD", "USD")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !got.Equal(amount) {
		t.Fatalf("got %s, want %s", got, amount)
	}
}

func TestConvertGELToUSD(t *testing.T) {
	got, err := Convert(decimal.MustParse("100"), "GEL", "USD")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !got.Equal(decimal.MustParse("37.00")) {
		t.Fatalf("got %s, want 37.00", got)
	}
}

func TestConvertUSDToGEL(t *testing.T) {
	got, err := Convert(decimal.MustParse("10"), "USD", "GEL")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !got.Equal(decimal.MustParse("27.00")) {
		t.Fatalf("got %s, want 27.00", got)
	}
}

func TestConvertUnsupportedPair(t *testing.T) {
	_, err := Convert(decimal.MustParse("1"), "EUR", "USD")
	if !errors.Is(err, domain.ErrUnsupportedCurrencyPair) {
		t.Fatalf("expected ErrUnsupportedCurrencyPair, got %v", err)
	}
}

func TestSupportsConversion(t *testing.T) {
	if !SupportsConversion("USD", "GEL") {
		t.Fatal("expected USD→GEL supported")
	}
	if SupportsConversion("EUR", "USD") {
		t.Fatal("expected EUR→USD unsupported")
	}
}

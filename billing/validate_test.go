package billing

import (
	"errors"
	"testing"
	"time"

	"github.com/govalues/decimal"
	"pave-bank/domain"
)

func TestParseCreateBillRequest(t *testing.T) {
	params, err := parseCreateBillRequest(&domain.CreateBillRequest{
		CustomerID:  "cust_001",
		PeriodStart: "2025-01-01",
		PeriodEnd:   "2025-01-31",
		Currency:    "USD",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.CustomerID != "cust_001" || params.Currency != "USD" {
		t.Fatalf("unexpected params: %+v", params)
	}
}

func TestParseCreateBillRequestInvalidPeriod(t *testing.T) {
	_, err := parseCreateBillRequest(&domain.CreateBillRequest{
		CustomerID:  "cust_001",
		PeriodStart: "2025-01-31",
		PeriodEnd:   "2025-01-01",
		Currency:    "USD",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseCreateBillRequestInvalidCurrency(t *testing.T) {
	_, err := parseCreateBillRequest(&domain.CreateBillRequest{
		CustomerID:  "cust_001",
		PeriodStart: "2025-01-01",
		PeriodEnd:   "2025-01-31",
		Currency:    "usd",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAddLineItemRequest(t *testing.T) {
	bill := domain.Bill{
		ID:          "bill-1",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	params, err := parseAddLineItemRequest(&domain.AddLineItemRequest{
		FeeType:             domain.FeeTypeSubscription,
		Description:         "Monthly plan",
		Quantity:            "1",
		UnitPrice:           "99.00",
		EffectiveDate:       "2025-01-01",
		ExternalReferenceID: "sub-jan-2025",
	}, bill)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !params.TotalAmount.Equal(decimal.MustParse("99.00")) {
		t.Fatalf("total = %s, want 99.00", params.TotalAmount)
	}
	if params.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", params.Currency)
	}
}

func TestParseAddLineItemRequestOutOfPeriod(t *testing.T) {
	bill := domain.Bill{
		ID:          "bill-1",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := parseAddLineItemRequest(&domain.AddLineItemRequest{
		FeeType:             domain.FeeTypeUsage,
		Description:         "API calls",
		Quantity:            "1",
		UnitPrice:           "5.00",
		EffectiveDate:       "2025-02-01",
		ExternalReferenceID: "usage-feb",
	}, bill)
	if !errors.Is(err, domain.ErrLineItemOutOfPeriod) {
		t.Fatalf("expected ErrLineItemOutOfPeriod, got %v", err)
	}
}

func TestParseAddLineItemRequestInvalidFeeType(t *testing.T) {
	bill := domain.Bill{
		ID:          "bill-1",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := parseAddLineItemRequest(&domain.AddLineItemRequest{
		FeeType:             domain.FeeType("test"),
		Description:         "test",
		Quantity:            "1",
		UnitPrice:           "1.00",
		EffectiveDate:       "2025-01-01",
		ExternalReferenceID: "test",
	}, bill)
	if !errors.Is(err, domain.ErrInvalidFeeType) {
		t.Fatalf("expected ErrInvalidFeeType, got %v", err)
	}
}

func TestParseAddLineItemRequestMissingQuantity(t *testing.T) {
	bill := domain.Bill{
		ID:          "bill-1",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	_, err := parseAddLineItemRequest(&domain.AddLineItemRequest{
		FeeType:             domain.FeeTypeSubscription,
		Description:         "test",
		Quantity:            "",
		UnitPrice:           "1.00",
		EffectiveDate:       "2025-01-01",
		ExternalReferenceID: "test",
	}, bill)
	if err == nil || err.Error() != "quantity is required" {
		t.Fatalf("expected quantity is required, got %v", err)
	}
}

func TestParseAddLineItemRequestDiscount(t *testing.T) {
	bill := domain.Bill{
		ID:          "bill-1",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	}

	params, err := parseAddLineItemRequest(&domain.AddLineItemRequest{
		FeeType:             domain.FeeTypeDiscount,
		Description:         "Promo",
		Quantity:            "1",
		UnitPrice:           "-10.00",
		EffectiveDate:       "2025-01-15",
		ExternalReferenceID: "promo-jan",
	}, bill)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !params.TotalAmount.Equal(decimal.MustParse("-10.00")) {
		t.Fatalf("total = %s, want -10.00", params.TotalAmount)
	}
}

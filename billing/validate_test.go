package billing

import (
	"testing"

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

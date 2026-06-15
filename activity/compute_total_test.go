package activity

import (
	"context"
	"testing"

	"github.com/govalues/decimal"
)

type mockStore struct {
	currency  string
	amounts   []LineAmount
	closedID  string
	closedAmt decimal.Decimal
}

func (m *mockStore) ListLineItemAmounts(context.Context, string) ([]LineAmount, error) {
	return m.amounts, nil
}

func (m *mockStore) GetBillCurrency(context.Context, string) (string, error) {
	return m.currency, nil
}

func (m *mockStore) MarkBillClosed(_ context.Context, billID string, total decimal.Decimal) error {
	m.closedID = billID
	m.closedAmt = total
	return nil
}

func withStore(t *testing.T, s Store) {
	prev := store
	store = s
	t.Cleanup(func() { store = prev })
}

func TestComputeTotalSumsLineItems(t *testing.T) {
	withStore(t, &mockStore{
		currency: "USD",
		amounts: []LineAmount{
			{Amount: decimal.MustParse("99.00"), Currency: "USD"},
			{Amount: decimal.MustParse("5.00"), Currency: "USD"},
		},
	})

	result, err := ComputeTotal(context.Background(), "bill-1")
	if err != nil {
		t.Fatalf("ComputeTotal: %v", err)
	}
	if !result.TotalAmount.Equal(decimal.MustParse("104.00")) {
		t.Fatalf("total = %s, want 104.00", result.TotalAmount)
	}
	if result.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", result.Currency)
	}
}

func TestComputeTotalEmptyBill(t *testing.T) {
	withStore(t, &mockStore{currency: "USD"})

	result, err := ComputeTotal(context.Background(), "bill-1")
	if err != nil {
		t.Fatalf("ComputeTotal: %v", err)
	}
	if !result.TotalAmount.IsZero() {
		t.Fatalf("total = %s, want 0", result.TotalAmount)
	}
}

func TestUpdateBillClosed(t *testing.T) {
	mock := &mockStore{}
	withStore(t, mock)

	total := decimal.MustParse("42.00")
	err := UpdateBillClosed(context.Background(), UpdateBillClosedInput{
		BillID:      "bill-1",
		TotalAmount: total,
		Currency:    "USD",
	})
	if err != nil {
		t.Fatalf("UpdateBillClosed: %v", err)
	}
	if mock.closedID != "bill-1" {
		t.Fatalf("closed bill id = %q", mock.closedID)
	}
	if !mock.closedAmt.Equal(total) {
		t.Fatalf("closed total = %s", mock.closedAmt)
	}
}

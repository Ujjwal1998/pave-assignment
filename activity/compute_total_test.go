package activity

import (
	"context"
	"testing"
	"time"

	"github.com/govalues/decimal"
)

type mockStore struct {
	currency         string
	amounts          []LineAmount
	accrualID        string
	accrualTotal     decimal.Decimal
	ensuredClosingID string
	closedID         string
	closedAmt        decimal.Decimal
}

func (m *mockStore) ListLineItemAmounts(context.Context, string) ([]LineAmount, error) {
	return m.amounts, nil
}

func (m *mockStore) GetBillCurrency(context.Context, string) (string, error) {
	return m.currency, nil
}

func (m *mockStore) UpdateAccrualTotal(_ context.Context, billID string, total decimal.Decimal) error {
	m.accrualID = billID
	m.accrualTotal = total
	return nil
}

func (m *mockStore) ActivateBill(context.Context, string) error {
	return nil
}

func (m *mockStore) PersistLineItem(_ context.Context, in PersistLineItemInput) (PersistLineItemResult, error) {
	q, err := decimal.Parse(in.Quantity)
	if err != nil {
		return PersistLineItemResult{}, err
	}
	p, err := decimal.Parse(in.UnitPrice)
	if err != nil {
		return PersistLineItemResult{}, err
	}
	total, err := q.Mul(p)
	if err != nil {
		return PersistLineItemResult{}, err
	}
	eff, err := time.Parse("2006-01-02", in.EffectiveDate)
	if err != nil {
		return PersistLineItemResult{}, err
	}
	return PersistLineItemResult{
		LineItemID:          "li-" + in.ExternalReferenceID,
		ExternalReferenceID: in.ExternalReferenceID,
		FeeType:             in.FeeType,
		Description:         in.Description,
		TotalAmount:         total.String(),
		Currency:            in.Currency,
		EffectiveDate:       eff,
		Created:             true,
	}, nil
}

func (m *mockStore) EnsureBillClosing(_ context.Context, billID string) error {
	m.ensuredClosingID = billID
	return nil
}

func (m *mockStore) FinalizeBillTotal(_ context.Context, billID string, total decimal.Decimal) error {
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

func TestComputeTotalMixedCurrency(t *testing.T) {
	withStore(t, &mockStore{
		currency: "USD",
		amounts: []LineAmount{
			{Amount: decimal.MustParse("99.00"), Currency: "USD"},
			{Amount: decimal.MustParse("100.00"), Currency: "GEL"},
		},
	})

	result, err := ComputeTotal(context.Background(), "bill-1")
	if err != nil {
		t.Fatalf("ComputeTotal: %v", err)
	}
	if !result.TotalAmount.Equal(decimal.MustParse("136.00")) {
		t.Fatalf("total = %s, want 136.00", result.TotalAmount)
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

func TestEnsureBillClosingActivity(t *testing.T) {
	mock := &mockStore{}
	withStore(t, mock)

	err := EnsureBillClosing(context.Background(), EnsureBillClosingInput{BillID: "bill-1"})
	if err != nil {
		t.Fatalf("EnsureBillClosing: %v", err)
	}
	if mock.ensuredClosingID != "bill-1" {
		t.Fatalf("ensured bill id = %q", mock.ensuredClosingID)
	}
}

func TestFinalizeBillTotalActivity(t *testing.T) {
	mock := &mockStore{}
	withStore(t, mock)

	err := FinalizeBillTotal(context.Background(), FinalizeBillTotalInput{
		BillID:      "bill-1",
		TotalAmount: "42.00",
	})
	if err != nil {
		t.Fatalf("FinalizeBillTotal: %v", err)
	}
	if mock.closedID != "bill-1" {
		t.Fatalf("closed bill id = %q", mock.closedID)
	}
	if !mock.closedAmt.Equal(decimal.MustParse("42.00")) {
		t.Fatalf("closed total = %s, want 42.00", mock.closedAmt)
	}
}

func TestUpdateAccrualTotal(t *testing.T) {
	mock := &mockStore{currency: "USD"}
	withStore(t, mock)

	err := UpdateAccrualTotal(context.Background(), UpdateAccrualTotalInput{
		BillID:       "bill-1",
		AccrualTotal: "104.00",
	})
	if err != nil {
		t.Fatalf("UpdateAccrualTotal: %v", err)
	}
	if mock.accrualID != "bill-1" {
		t.Fatalf("accrual bill id = %q", mock.accrualID)
	}
	if !mock.accrualTotal.Equal(decimal.MustParse("104.00")) {
		t.Fatalf("accrual total = %s, want 104.00", mock.accrualTotal)
	}
}

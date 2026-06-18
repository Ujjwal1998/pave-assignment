package activity

import (
	"context"

	"github.com/govalues/decimal"
)

type LineAmount struct {
	Amount   decimal.Decimal
	Currency string
}

type Store interface {
	ListLineItemAmounts(ctx context.Context, billID string) ([]LineAmount, error)
	GetBillCurrency(ctx context.Context, billID string) (string, error)
	FinalizeBillTotal(ctx context.Context, billID string, totalAmount decimal.Decimal) error
}

type ComputeTotalResult struct {
	TotalAmount decimal.Decimal
	Currency    string
}

type UpdateBillClosedInput struct {
	BillID string
}

var store Store

func Init(s Store) {
	store = s
}

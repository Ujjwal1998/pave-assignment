package billing

import (
	"context"

	"github.com/govalues/decimal"
	"pave-bank/activity"
)

type activityStore struct{}

func newActivityStore() activity.Store {
	return activityStore{}
}

func (activityStore) ListLineItemAmounts(ctx context.Context, billID string) ([]activity.LineAmount, error) {
	return listLineItemAmounts(ctx, billID)
}

func (activityStore) GetBillCurrency(ctx context.Context, billID string) (string, error) {
	bill, err := GetBillByID(ctx, billID)
	if err != nil {
		return "", err
	}
	return bill.Currency, nil
}

func (activityStore) FinalizeBillTotal(ctx context.Context, billID string, totalAmount decimal.Decimal) error {
	return FinalizeBillTotal(ctx, billID, totalAmount)
}

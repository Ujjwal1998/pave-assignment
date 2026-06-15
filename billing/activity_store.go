package billing

import (
	"context"
	"fmt"

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

func (activityStore) MarkBillClosed(ctx context.Context, billID string, totalAmount decimal.Decimal) error {
	_, err := MarkBillClosed(ctx, billID, totalAmount)
	if err != nil {
		return fmt.Errorf("close bill: %w", err)
	}
	return nil
}

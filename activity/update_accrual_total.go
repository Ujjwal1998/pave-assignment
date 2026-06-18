package activity

import (
	"context"
	"fmt"

	"github.com/govalues/decimal"

	"pave-bank/money"
)

type UpdateAccrualTotalInput struct {
	BillID       string
	AccrualTotal string
}

func UpdateAccrualTotal(ctx context.Context, input UpdateAccrualTotalInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}

	total, err := decimal.Parse(input.AccrualTotal)
	if err != nil {
		return fmt.Errorf("parse accrual total: %w", err)
	}

	currency, err := store.GetBillCurrency(ctx, input.BillID)
	if err != nil {
		return fmt.Errorf("get bill currency: %w", err)
	}

	rounded, err := money.RoundToCurrencyScale(total, currency)
	if err != nil {
		return fmt.Errorf("round accrual total: %w", err)
	}

	if err := store.UpdateAccrualTotal(ctx, input.BillID, rounded); err != nil {
		return fmt.Errorf("update accrual total: %w", err)
	}
	return nil
}

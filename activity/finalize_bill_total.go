package activity

import (
	"context"
	"fmt"

	"github.com/govalues/decimal"
)

type FinalizeBillTotalInput struct {
	BillID      string
	TotalAmount string
}

func FinalizeBillTotal(ctx context.Context, input FinalizeBillTotalInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}

	total, err := decimal.Parse(input.TotalAmount)
	if err != nil {
		return fmt.Errorf("parse total amount: %w", err)
	}

	if err := store.FinalizeBillTotal(ctx, input.BillID, total); err != nil {
		return fmt.Errorf("finalize bill total: %w", err)
	}
	return nil
}

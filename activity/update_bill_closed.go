package activity

import (
	"context"
	"fmt"
)

func UpdateBillClosed(ctx context.Context, input UpdateBillClosedInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}

	result, err := ComputeTotal(ctx, input.BillID)
	if err != nil {
		return fmt.Errorf("compute total: %w", err)
	}

	if err := store.FinalizeBillTotal(ctx, input.BillID, result.TotalAmount); err != nil {
		return fmt.Errorf("finalize bill total: %w", err)
	}
	return nil
}

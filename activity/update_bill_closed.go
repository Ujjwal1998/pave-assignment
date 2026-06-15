package activity

import (
	"context"
	"fmt"
)

func UpdateBillClosed(ctx context.Context, input UpdateBillClosedInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}
	if err := store.MarkBillClosed(ctx, input.BillID, input.TotalAmount); err != nil {
		return fmt.Errorf("mark bill closed: %w", err)
	}
	return nil
}

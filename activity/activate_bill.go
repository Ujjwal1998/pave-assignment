package activity

import (
	"context"
	"fmt"
)

type ActivateBillInput struct {
	BillID string
}

func ActivateBill(ctx context.Context, input ActivateBillInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}
	if err := store.ActivateBill(ctx, input.BillID); err != nil {
		return fmt.Errorf("activate bill: %w", err)
	}
	return nil
}

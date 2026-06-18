package activity

import (
	"context"
	"fmt"
)

type EnsureBillClosingInput struct {
	BillID string
}

func EnsureBillClosing(ctx context.Context, input EnsureBillClosingInput) error {
	if store == nil {
		return fmt.Errorf("activity store is not initialized")
	}
	if err := store.EnsureBillClosing(ctx, input.BillID); err != nil {
		return fmt.Errorf("ensure bill closing: %w", err)
	}
	return nil
}

package billing

import (
	"context"
	"fmt"
	"time"

	"pave-bank/activity"
	"pave-bank/domain"
)

func (activityStore) PersistLineItem(ctx context.Context, input activity.PersistLineItemInput) (activity.PersistLineItemResult, error) {
	params, err := parsePersistLineItemInput(ctx, input)
	if err != nil {
		return activity.PersistLineItemResult{}, err
	}

	result, err := InsertLineItem(ctx, params)
	if err != nil {
		return activity.PersistLineItemResult{}, err
	}

	return activity.PersistLineItemResult{
		LineItemID:          result.LineItem.ID,
		ExternalReferenceID: result.LineItem.ExternalReferenceID,
		FeeType:             string(result.LineItem.FeeType),
		Description:         result.LineItem.Description,
		TotalAmount:         result.LineItem.TotalAmount.String(),
		Currency:            result.LineItem.Currency,
		EffectiveDate:       result.LineItem.EffectiveDate,
		Created:             result.Created,
	}, nil
}

func parsePersistLineItemInput(ctx context.Context, input activity.PersistLineItemInput) (InsertLineItemParams, error) {
	if input.BillID == "" {
		return InsertLineItemParams{}, fmt.Errorf("bill_id is required")
	}
	if input.ExternalReferenceID == "" {
		return InsertLineItemParams{}, fmt.Errorf("external_reference_id is required")
	}
	if input.Description == "" {
		return InsertLineItemParams{}, fmt.Errorf("description is required")
	}
	if !domain.FeeType(input.FeeType).Valid() {
		return InsertLineItemParams{}, domain.ErrInvalidFeeType
	}

	bill, err := GetBillByID(ctx, input.BillID)
	if err != nil {
		return InsertLineItemParams{}, err
	}
	if bill.Status != domain.BillStatusOpen {
		switch bill.Status {
		case domain.BillStatusScheduled:
			return InsertLineItemParams{}, domain.ErrBillNotYetOpen
		default:
			return InsertLineItemParams{}, domain.ErrBillAlreadyClosed
		}
	}

	req := &domain.AddLineItemRequest{
		FeeType:             domain.FeeType(input.FeeType),
		Description:         input.Description,
		Quantity:            input.Quantity,
		UnitPrice:           input.UnitPrice,
		EffectiveDate:       input.EffectiveDate,
		ExternalReferenceID: input.ExternalReferenceID,
		Currency:            input.Currency,
	}
	return parseAddLineItemRequest(req, bill)
}

// waitForLineItem polls until the workflow persists a line item (Phase 4 read-after-signal).
func waitForLineItem(ctx context.Context, billID, externalRef string) (domain.LineItem, error) {
	const maxTries = 50
	for i := 0; i < maxTries; i++ {
		item, err := getLineItemByExternalRef(ctx, billID, externalRef)
		if err == nil {
			return item, nil
		}
		// getLineItemByExternalRef returns ErrDuplicateLineItem when the row is not found yet.
		select {
		case <-ctx.Done():
			return domain.LineItem{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return domain.LineItem{}, fmt.Errorf("line item %q not persisted in time", externalRef)
}

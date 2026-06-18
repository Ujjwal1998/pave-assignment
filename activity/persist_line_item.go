package activity

import (
	"context"
	"fmt"
	"time"
)

type PersistLineItemInput struct {
	BillID              string
	FeeType             string
	Description         string
	Quantity            string
	UnitPrice           string
	Currency            string
	EffectiveDate       string // YYYY-MM-DD
	ExternalReferenceID string
}

type PersistLineItemResult struct {
	LineItemID          string    `json:"line_item_id"`
	ExternalReferenceID string    `json:"external_reference_id"`
	FeeType             string    `json:"fee_type"`
	Description         string    `json:"description"`
	TotalAmount         string    `json:"total_amount"`
	Currency            string    `json:"currency"`
	EffectiveDate       time.Time `json:"effective_date"`
	Created             bool      `json:"created"`
}

func PersistLineItem(ctx context.Context, input PersistLineItemInput) (PersistLineItemResult, error) {
	if store == nil {
		return PersistLineItemResult{}, fmt.Errorf("activity store is not initialized")
	}
	return store.PersistLineItem(ctx, input)
}

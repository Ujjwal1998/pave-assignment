package domain

import (
	"time"

	"github.com/govalues/decimal"
)

type FeeType string

const (
	FeeTypeSubscription FeeType = "subscription"
	FeeTypeUsage        FeeType = "usage"
	FeeTypeTax          FeeType = "tax"
	FeeTypePenalty      FeeType = "penalty"
	FeeTypeDiscount     FeeType = "discount"
)

func (f FeeType) Valid() bool {
	switch f {
	case FeeTypeSubscription, FeeTypeUsage, FeeTypeTax, FeeTypePenalty, FeeTypeDiscount:
		return true
	default:
		return false
	}
}

type LineItem struct {
	ID                  string          `json:"id"`
	BillID              string          `json:"bill_id"`
	FeeType             FeeType         `json:"fee_type"`
	Description         string          `json:"description"`
	Quantity            decimal.Decimal `json:"quantity"`
	UnitPrice           decimal.Decimal `json:"unit_price"`
	TotalAmount         decimal.Decimal `json:"total_amount"`
	Currency            string          `json:"currency"`
	EffectiveDate       time.Time       `json:"effective_date"`
	ExternalReferenceID string          `json:"external_reference_id"`
	CreatedAt           time.Time       `json:"created_at"`
}

type AddLineItemRequest struct {
	FeeType             FeeType `json:"fee_type"`
	Description         string  `json:"description"`
	Quantity            string  `json:"quantity"`
	UnitPrice           string  `json:"unit_price"`
	EffectiveDate       string  `json:"effective_date"`
	ExternalReferenceID string  `json:"external_reference_id"`
	Currency            string  `json:"currency,omitempty"` // optional; must match bill currency if provided
}

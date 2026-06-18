package domain

import (
	"time"

	"github.com/govalues/decimal"
)

type BillStatus string

const (
	BillStatusScheduled BillStatus = "scheduled"
	BillStatusOpen      BillStatus = "open"
	BillStatusClosing   BillStatus = "closing"
	BillStatusClosed    BillStatus = "closed"
)

type Bill struct {
	ID            string           `json:"id"`
	CustomerID    string           `json:"customer_id"`
	PeriodStart   time.Time        `json:"period_start"`
	PeriodEnd     time.Time        `json:"period_end"`
	Currency      string           `json:"currency"`
	Status        BillStatus       `json:"status"`
	AccrualTotal  *decimal.Decimal `json:"accrual_total,omitempty"`
	TotalAmount   *decimal.Decimal `json:"total_amount,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	ClosedAt      *time.Time       `json:"closed_at,omitempty"`
	WorkflowRunID string           `json:"-"`
	LineItems     []LineItem       `json:"line_items,omitempty"`
}

type CreateBillRequest struct {
	CustomerID  string `json:"customer_id"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Currency    string `json:"currency"`
}

type CloseBillResponse struct {
	BillID        string          `json:"bill_id"`
	CustomerID    string          `json:"customer_id"`
	PeriodStart   string          `json:"period_start"`
	PeriodEnd     string          `json:"period_end"`
	Currency      string          `json:"currency"`
	TotalAmount   decimal.Decimal `json:"total_amount"`
	LineItemCount int             `json:"line_item_count"`
	ClosedAt      time.Time       `json:"closed_at"`
	LineItems     []LineItem      `json:"line_items"`
}

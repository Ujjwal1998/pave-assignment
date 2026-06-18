package workflow

import (
	"time"

	"github.com/govalues/decimal"
)

const (
	// BaseTaskQueueName is the suffix used to build the environment-specific task queue.
	// Actual queue name: "<env>-" + BaseTaskQueueName (computed in billing/service.go).
	BaseTaskQueueName   = "billing"
	CloseSignalName     = "bill.close"
	LineItemSignalName  = "bill.line_item.added"
	AccrualQueryName    = "accrual"
)

func WorkflowID(billID string) string {
	return "bill-" + billID
}

type Input struct {
	BillID      string
	CustomerID  string
	Currency    string
	PeriodStart time.Time
}

type CloseSignalPayload struct{}

// LineItemSignalPayload is sent to the workflow when a line item is persisted.
type LineItemSignalPayload struct {
	LineItemID          string    `json:"line_item_id"`
	ExternalReferenceID string    `json:"external_reference_id"`
	FeeType             string    `json:"fee_type"`
	Description         string    `json:"description"`
	TotalAmount         string    `json:"total_amount"`
	Currency            string    `json:"currency"`
	EffectiveDate       time.Time `json:"effective_date"`
}

// AccrualState is the running fee accrual tracked by the workflow.
type AccrualState struct {
	BillID        string                 `json:"bill_id"`
	Currency      string                 `json:"currency"`
	LineItemCount int                    `json:"line_item_count"`
	RunningTotal  string                 `json:"running_total"` // decimal string, sum of all line item totals
	LineItems     []LineItemSignalPayload `json:"line_items"`
}

func (s *AccrualState) addItem(item LineItemSignalPayload) {
	for _, existing := range s.LineItems {
		if existing.ExternalReferenceID == item.ExternalReferenceID {
			return
		}
	}
	s.LineItems = append(s.LineItems, item)
	s.LineItemCount = len(s.LineItems)

	current, _ := decimal.Parse(s.RunningTotal)
	added, err := decimal.Parse(item.TotalAmount)
	if err == nil {
		if sum, err := current.Add(added); err == nil {
			s.RunningTotal = sum.String()
		}
	}
}

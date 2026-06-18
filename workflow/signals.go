package workflow

import (
	"time"

	"github.com/govalues/decimal"

	"pave-bank/money"
)

const (
	// BaseTaskQueueName is the suffix used to build the environment-specific task queue.
	// Actual queue name: "<env>-" + BaseTaskQueueName (computed in billing/service.go).
	BaseTaskQueueName  = "billing"
	CloseSignalName    = "bill.close"
	LineItemSignalName = "bill.line_item.added"
	AccrualQueryName   = "accrual"
	StatusQueryName    = "status"
)

func WorkflowID(billID string) string {
	return "bill-" + billID
}

type Input struct {
	BillID      string
	CustomerID  string
	Currency    string
	PeriodStart time.Time
	PeriodEnd   time.Time
}

type CloseSignalPayload struct{}

// BillPhase is the current segment of the bill lifecycle process.
type BillPhase string

const (
	PhaseWaitingPeriodStart BillPhase = "waiting_period_start"
	PhaseAccruing           BillPhase = "accruing"
	PhaseClosing            BillPhase = "closing"
)

// ProcessStatus is returned by the Temporal "status" query for live process inspection.
type ProcessStatus struct {
	BillID        string    `json:"bill_id"`
	Phase         BillPhase `json:"phase"`
	Currency      string    `json:"currency"`
	AccrualTotal  string    `json:"accrual_total"`
	LineItemCount int       `json:"line_item_count"`
	PeriodStart   time.Time `json:"period_start"`
	PeriodEnd     time.Time `json:"period_end"`
}

// LineItemSignalPayload is the add-fee request sent to the workflow (Phase 4).
// The workflow persists via PersistLineItem before updating accrual state.
type LineItemSignalPayload struct {
	ExternalReferenceID string `json:"external_reference_id"`
	FeeType             string `json:"fee_type"`
	Description         string `json:"description"`
	Quantity            string `json:"quantity"`
	UnitPrice           string `json:"unit_price"`
	Currency            string `json:"currency"`
	EffectiveDate       string `json:"effective_date"` // YYYY-MM-DD

	// Populated in workflow state after PersistLineItem (queries / accrual mirror).
	LineItemID  string    `json:"line_item_id,omitempty"`
	TotalAmount string    `json:"total_amount,omitempty"`
	EffectiveAt time.Time `json:"effective_at,omitempty"`
}

// AccrualState is the running fee accrual tracked by the workflow.
type AccrualState struct {
	BillID        string                  `json:"bill_id"`
	Currency      string                  `json:"currency"`
	LineItemCount int                     `json:"line_item_count"`
	RunningTotal  string                  `json:"running_total"` // sum in bill currency
	LineItems     []LineItemSignalPayload `json:"line_items"`
}

func (s *AccrualState) processStatus(phase BillPhase, periodStart, periodEnd time.Time) ProcessStatus {
	return ProcessStatus{
		BillID:        s.BillID,
		Phase:         phase,
		Currency:      s.Currency,
		AccrualTotal:  s.RunningTotal,
		LineItemCount: s.LineItemCount,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
	}
}

// addItem records a new line item in workflow state. Returns true when the item was added
// (false for duplicate external_reference_id replays).
func (s *AccrualState) addItem(item LineItemSignalPayload) bool {
	for _, existing := range s.LineItems {
		if existing.ExternalReferenceID == item.ExternalReferenceID {
			return false
		}
	}
	s.LineItems = append(s.LineItems, item)
	s.LineItemCount = len(s.LineItems)

	added, err := decimal.Parse(item.TotalAmount)
	if err != nil {
		return true
	}

	converted, err := money.Convert(added, item.Currency, s.Currency)
	if err != nil {
		return true
	}

	current, _ := decimal.Parse(s.RunningTotal)
	if sum, err := current.Add(converted); err == nil {
		s.RunningTotal = sum.String()
	}
	return true
}

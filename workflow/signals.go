package workflow

const (
	TaskQueueName   = "billing"
	CloseSignalName = "bill.close"
)

func WorkflowID(billID string) string {
	return "bill-" + billID
}

type Input struct {
	BillID     string
	CustomerID string
	Currency   string
}

type CloseSignalPayload struct{}

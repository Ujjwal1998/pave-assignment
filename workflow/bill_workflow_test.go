package workflow

import (
	"testing"
	"time"

	"github.com/govalues/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"pave-bank/activity"
)

func TestBillWorkflowCloseSignalRunsActivities(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(BillWorkflow)

	total := decimal.MustParse("104.00")
	env.OnActivity(activity.ComputeTotal, mock.Anything, "bill-1").Return(activity.ComputeTotalResult{
		TotalAmount: total,
		Currency:    "USD",
	}, nil)
	env.OnActivity(activity.UpdateBillClosed, mock.Anything, mock.MatchedBy(func(in activity.UpdateBillClosedInput) bool {
		return in.BillID == "bill-1" && in.Currency == "USD" && in.TotalAmount.Equal(total)
	})).Return(nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(CloseSignalName, CloseSignalPayload{})
	}, time.Millisecond)

	env.ExecuteWorkflow(BillWorkflow, Input{
		BillID:     "bill-1",
		CustomerID: "cust_001",
		Currency:   "USD",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

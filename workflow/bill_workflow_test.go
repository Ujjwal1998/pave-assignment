package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"pave-bank/activity"
)

func TestBillWorkflowCloseSignalRunsActivities(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(BillWorkflow)

	env.OnActivity(activity.UpdateBillClosed, mock.Anything, mock.MatchedBy(func(in activity.UpdateBillClosedInput) bool {
		return in.BillID == "bill-1"
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

func TestBillWorkflowAccruesLineItemsUntilClose(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(BillWorkflow)

	env.OnActivity(activity.UpdateAccrualTotal, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(activity.UpdateBillClosed, mock.Anything, mock.Anything).Return(nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(LineItemSignalName, LineItemSignalPayload{
			LineItemID:          "li-1",
			ExternalReferenceID: "sub-apr",
			FeeType:             "subscription",
			Description:         "Monthly plan",
			TotalAmount:         "99.00",
			Currency:            "USD",
			EffectiveDate:       time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		})
		env.SignalWorkflow(LineItemSignalName, LineItemSignalPayload{
			LineItemID:          "li-2",
			ExternalReferenceID: "usage-apr",
			FeeType:             "usage",
			Description:         "API calls",
			TotalAmount:         "5.00",
			Currency:            "USD",
			EffectiveDate:       time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC),
		})
		env.SignalWorkflow(LineItemSignalName, LineItemSignalPayload{
			LineItemID:          "li-1-dup",
			ExternalReferenceID: "sub-apr",
			TotalAmount:         "99.00",
			Currency:            "USD",
		})
	}, time.Millisecond)

	env.RegisterDelayedCallback(func() {
		result, err := env.QueryWorkflow(AccrualQueryName)
		require.NoError(t, err)

		var state AccrualState
		require.NoError(t, result.Get(&state))
		require.Equal(t, 2, state.LineItemCount)
		require.Equal(t, "104.00", state.RunningTotal)

		statusResult, err := env.QueryWorkflow(StatusQueryName)
		require.NoError(t, err)

		var status ProcessStatus
		require.NoError(t, statusResult.Get(&status))
		require.Equal(t, PhaseAccruing, status.Phase)
		require.Equal(t, "104.00", status.AccrualTotal)
		require.Equal(t, 2, status.LineItemCount)

		env.SignalWorkflow(CloseSignalName, CloseSignalPayload{})
	}, 2*time.Millisecond)

	env.ExecuteWorkflow(BillWorkflow, Input{
		BillID:      "bill-1",
		CustomerID:  "cust_001",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 4, 30, 0, 0, 0, 0, time.UTC),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestBillWorkflowAccruesMixedCurrencyInBillCurrency(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(BillWorkflow)
	env.OnActivity(activity.UpdateAccrualTotal, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(activity.UpdateBillClosed, mock.Anything, mock.Anything).Return(nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(LineItemSignalName, LineItemSignalPayload{
			LineItemID:          "li-1",
			ExternalReferenceID: "sub-apr",
			FeeType:             "subscription",
			TotalAmount:         "99.00",
			Currency:            "USD",
			EffectiveDate:       time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		})
		env.SignalWorkflow(LineItemSignalName, LineItemSignalPayload{
			LineItemID:          "li-2",
			ExternalReferenceID: "usage-gel",
			FeeType:             "usage",
			TotalAmount:         "100.00",
			Currency:            "GEL",
			EffectiveDate:       time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC),
		})
	}, time.Millisecond)

	env.RegisterDelayedCallback(func() {
		result, err := env.QueryWorkflow(AccrualQueryName)
		require.NoError(t, err)

		var state AccrualState
		require.NoError(t, result.Get(&state))
		require.Equal(t, 2, state.LineItemCount)
		require.Equal(t, "136.00", state.RunningTotal)

		env.SignalWorkflow(CloseSignalName, CloseSignalPayload{})
	}, 2*time.Millisecond)

	env.ExecuteWorkflow(BillWorkflow, Input{
		BillID:     "bill-1",
		CustomerID: "cust_001",
		Currency:   "USD",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestAccrualStateAddItemReturnsFalseForDuplicate(t *testing.T) {
	state := AccrualState{BillID: "bill-1", Currency: "USD"}
	item := LineItemSignalPayload{
		ExternalReferenceID: "ref-1",
		TotalAmount:         "10.00",
		Currency:            "USD",
	}
	require.True(t, state.addItem(item))
	require.False(t, state.addItem(item))
	require.Equal(t, 1, state.LineItemCount)
}

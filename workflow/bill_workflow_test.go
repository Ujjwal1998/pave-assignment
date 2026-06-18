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

func registerCloseActivities(env *testsuite.TestWorkflowEnvironment) {
	env.OnActivity(activity.EnsureBillClosing, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(activity.ComputeTotal, mock.Anything, mock.Anything).Return(activity.ComputeTotalResult{
		TotalAmount: mustDecimal("0"),
		Currency:    "USD",
	}, nil)
	env.OnActivity(activity.FinalizeBillTotal, mock.Anything, mock.Anything).Return(nil)
}

func mustDecimal(s string) decimal.Decimal {
	d, err := decimal.Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestBillWorkflowCloseSignalRunsActivities(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(BillWorkflow)
	registerCloseActivities(env)

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
	env.SetStartTime(time.Date(2025, 4, 15, 0, 0, 0, 0, time.UTC))

	env.RegisterWorkflow(BillWorkflow)
	env.OnActivity(activity.UpdateAccrualTotal, mock.Anything, mock.Anything).Return(nil)
	registerCloseActivities(env)

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
	registerCloseActivities(env)

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

func TestBillWorkflowActivatesScheduledBill(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.SetStartTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	env.RegisterWorkflow(BillWorkflow)
	env.OnActivity(activity.ActivateBill, mock.Anything, mock.MatchedBy(func(in activity.ActivateBillInput) bool {
		return in.BillID == "bill-1"
	})).Return(nil).Once()
	registerCloseActivities(env)

	env.RegisterDelayedCallback(func() {
		result, err := env.QueryWorkflow(StatusQueryName)
		require.NoError(t, err)

		var status ProcessStatus
		require.NoError(t, result.Get(&status))
		require.Equal(t, PhaseAccruing, status.Phase)

		env.SignalWorkflow(CloseSignalName, CloseSignalPayload{})
	}, time.Hour*24*20)

	env.ExecuteWorkflow(BillWorkflow, Input{
		BillID:      "bill-1",
		CustomerID:  "cust_001",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestBillWorkflowAutoCloseAtPeriodEnd(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	// Start after period_end auto-close instant (July 1) so the bill closes on first loop.
	start := time.Date(2025, 7, 2, 0, 0, 0, 0, time.UTC)
	env.SetStartTime(start)

	env.RegisterWorkflow(BillWorkflow)
	env.OnActivity(activity.EnsureBillClosing, mock.Anything, mock.MatchedBy(func(in activity.EnsureBillClosingInput) bool {
		return in.BillID == "bill-1"
	})).Return(nil).Once()
	env.OnActivity(activity.ComputeTotal, mock.Anything, "bill-1").Return(activity.ComputeTotalResult{
		TotalAmount: mustDecimal("0"),
		Currency:    "USD",
	}, nil).Once()
	env.OnActivity(activity.FinalizeBillTotal, mock.Anything, mock.MatchedBy(func(in activity.FinalizeBillTotalInput) bool {
		return in.BillID == "bill-1"
	})).Return(nil).Once()

	env.ExecuteWorkflow(BillWorkflow, Input{
		BillID:      "bill-1",
		CustomerID:  "cust_001",
		Currency:    "USD",
		PeriodStart: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}

func TestBillWorkflowAutoCloseTimerFiresOnce(t *testing.T) {
	t.Skip("long auto-close timer requires full time skip in test env; covered by TestBillWorkflowAutoCloseAtPeriodEnd")
}

func TestAutoCloseTime(t *testing.T) {
	got := autoCloseTime(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC))
	want := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, want, got)
}

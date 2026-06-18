package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"pave-bank/activity"
)

func BillWorkflow(ctx workflow.Context, input Input) error {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 5,
			InitialInterval: time.Second,
		},
	})

	phase := PhaseAccruing
	if !input.PeriodStart.IsZero() {
		now := workflow.Now(ctx)
		if input.PeriodStart.After(now) {
			phase = PhaseWaitingPeriodStart
			if err := workflow.NewTimer(ctx, input.PeriodStart.Sub(now)).Get(ctx, nil); err != nil {
				return err
			}
			if err := workflow.ExecuteActivity(activityCtx, activity.ActivateBill, activity.ActivateBillInput{
				BillID: input.BillID,
			}).Get(activityCtx, nil); err != nil {
				return err
			}
			phase = PhaseAccruing
		}
	}

	state := AccrualState{
		BillID:   input.BillID,
		Currency: input.Currency,
	}

	if err := workflow.SetQueryHandler(ctx, AccrualQueryName, func() (AccrualState, error) {
		return state, nil
	}); err != nil {
		return err
	}

	if err := workflow.SetQueryHandler(ctx, StatusQueryName, func() (ProcessStatus, error) {
		return state.processStatus(phase, input.PeriodStart, input.PeriodEnd), nil
	}); err != nil {
		return err
	}

	lineItemCh := workflow.GetSignalChannel(ctx, LineItemSignalName)
	closeCh := workflow.GetSignalChannel(ctx, CloseSignalName)

	autoCloseAt := autoCloseTime(input.PeriodEnd)
	var autoCloseTimer workflow.Future
	autoCloseTimerRegistered := false
	autoClosePastDue := false
	if !autoCloseAt.IsZero() {
		now := workflow.Now(ctx)
		if !autoCloseAt.After(now) {
			autoClosePastDue = true
		} else {
			autoCloseTimer = workflow.NewTimer(ctx, autoCloseAt.Sub(now))
		}
	}

	for phase == PhaseAccruing {
		selector := workflow.NewSelector(ctx)

		var lineItem LineItemSignalPayload
		itemAdded := false
		selector.AddReceive(lineItemCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &lineItem)
			itemAdded = state.addItem(lineItem)
		})

		closed := false
		var closeSig CloseSignalPayload
		selector.AddReceive(closeCh, func(c workflow.ReceiveChannel, _ bool) {
			c.Receive(ctx, &closeSig)
			closed = true
		})

		if autoClosePastDue {
			closed = true
			autoClosePastDue = false
		} else if autoCloseTimer != nil && !autoCloseTimerRegistered {
			autoCloseTimerRegistered = true
			selector.AddFuture(autoCloseTimer, func(f workflow.Future) {
				_ = f.Get(ctx, nil)
				closed = true
			})
		}

		if !closed {
			selector.Select(ctx)
		}

		if itemAdded {
			if err := workflow.ExecuteActivity(activityCtx, activity.UpdateAccrualTotal, activity.UpdateAccrualTotalInput{
				BillID:       input.BillID,
				AccrualTotal: state.RunningTotal,
			}).Get(activityCtx, nil); err != nil {
				return err
			}
		}

		if closed {
			phase = PhaseClosing
			break
		}
	}

	return runBillClose(ctx, activityCtx, input.BillID)
}

// autoCloseTime returns the instant after period_end when the bill should auto-close.
func autoCloseTime(periodEnd time.Time) time.Time {
	if periodEnd.IsZero() {
		return time.Time{}
	}
	end := periodEnd.UTC()
	return time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}

package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"pave-bank/activity"
)

func BillWorkflow(ctx workflow.Context, input Input) error {
	closeCh := workflow.GetSignalChannel(ctx, CloseSignalName)
	var sig CloseSignalPayload
	closeCh.Receive(ctx, &sig)

	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 5,
			InitialInterval: time.Second,
		},
	})

	var result activity.ComputeTotalResult
	if err := workflow.ExecuteActivity(activityCtx, activity.ComputeTotal, input.BillID).Get(activityCtx, &result); err != nil {
		return err
	}

	return workflow.ExecuteActivity(activityCtx, activity.UpdateBillClosed, activity.UpdateBillClosedInput{
		BillID:      input.BillID,
		TotalAmount: result.TotalAmount,
		Currency:    result.Currency,
	}).Get(activityCtx, nil)
}

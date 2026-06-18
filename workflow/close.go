package workflow

import (
	"go.temporal.io/sdk/workflow"

	"pave-bank/activity"
)

// runBillClose executes the multi-step close segment: freeze → compute → finalize.
func runBillClose(ctx workflow.Context, activityCtx workflow.Context, billID string) error {
	if err := workflow.ExecuteActivity(activityCtx, activity.EnsureBillClosing, activity.EnsureBillClosingInput{
		BillID: billID,
	}).Get(activityCtx, nil); err != nil {
		return err
	}

	var totalResult activity.ComputeTotalResult
	if err := workflow.ExecuteActivity(activityCtx, activity.ComputeTotal, billID).Get(activityCtx, &totalResult); err != nil {
		return err
	}

	return workflow.ExecuteActivity(activityCtx, activity.FinalizeBillTotal, activity.FinalizeBillTotalInput{
		BillID:      billID,
		TotalAmount: totalResult.TotalAmount.String(),
	}).Get(activityCtx, nil)
}

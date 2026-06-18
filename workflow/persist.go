package workflow

import (
	"go.temporal.io/sdk/workflow"

	"pave-bank/activity"
)

func persistInput(billID string, sig LineItemSignalPayload) activity.PersistLineItemInput {
	return activity.PersistLineItemInput{
		BillID:              billID,
		FeeType:             sig.FeeType,
		Description:         sig.Description,
		Quantity:            sig.Quantity,
		UnitPrice:           sig.UnitPrice,
		Currency:            sig.Currency,
		EffectiveDate:       sig.EffectiveDate,
		ExternalReferenceID: sig.ExternalReferenceID,
	}
}

func accrualItemFromResult(result activity.PersistLineItemResult) LineItemSignalPayload {
	return LineItemSignalPayload{
		LineItemID:          result.LineItemID,
		ExternalReferenceID: result.ExternalReferenceID,
		FeeType:             result.FeeType,
		Description:         result.Description,
		TotalAmount:         result.TotalAmount,
		Currency:            result.Currency,
		EffectiveDate:       result.EffectiveDate.Format("2006-01-02"),
		EffectiveAt:         result.EffectiveDate,
	}
}

// persistAndAccrue runs PersistLineItem then updates in-memory accrual and DB accrual_total.
// When the bill is frozen mid-flight (close race), the persist is skipped without failing the workflow.
func persistAndAccrue(
	ctx workflow.Context,
	activityCtx workflow.Context,
	state *AccrualState,
	billID string,
	sig LineItemSignalPayload,
) error {
	var result activity.PersistLineItemResult
	if err := workflow.ExecuteActivity(activityCtx, activity.PersistLineItem, persistInput(billID, sig)).Get(activityCtx, &result); err != nil {
		if isLineItemPersistRejected(err) {
			return nil
		}
		return err
	}

	if !state.addItem(accrualItemFromResult(result)) {
		return nil
	}

	return workflow.ExecuteActivity(activityCtx, activity.UpdateAccrualTotal, activity.UpdateAccrualTotalInput{
		BillID:       billID,
		AccrualTotal: state.RunningTotal,
	}).Get(activityCtx, nil)
}

package billing

import (
	"context"

	"encore.dev/beta/errs"
	"go.temporal.io/sdk/client"

	"pave-bank/activity"
	"pave-bank/domain"
	"pave-bank/money"
	"pave-bank/workflow"
)

//encore:api public method=GET path=/bills/:id
func GetBill(ctx context.Context, id string) (*domain.Bill, error) {
	if err := validateBillID(id); err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}
	bill, err := loadBill(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	return &bill, nil
}

//encore:api public method=POST path=/bills
func (s *Service) CreateBill(ctx context.Context, req *domain.CreateBillRequest) (*domain.Bill, error) {
	params, err := parseCreateBillRequest(req)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}

	result, err := CreateBillRecord(ctx, params)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if !result.Created {
		return nil, errs.B().Code(errs.AlreadyExists).
			Msg(domain.ErrBillAlreadyExists.Error()).
			Details(BillAlreadyExistsDetails{
				BillID: result.Bill.ID,
				Bill:   result.Bill,
			}).
			Err()
	}
	run, err := s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflow.WorkflowID(result.Bill.ID),
		TaskQueue: s.taskQueue,
	}, workflow.BillWorkflow, workflow.Input{
		BillID:      result.Bill.ID,
		CustomerID:  result.Bill.CustomerID,
		Currency:    result.Bill.Currency,
		PeriodStart: result.Bill.PeriodStart,
	})
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to start bill workflow").Cause(err).Err()
	}

	if err := UpdateWorkflowRunID(ctx, result.Bill.ID, run.GetRunID()); err != nil {
		return nil, mapDomainErr(err)
	}

	result.Bill.WorkflowRunID = run.GetRunID()
	return &result.Bill, nil
}

//encore:api public method=POST path=/bills/:id/close
func (s *Service) CloseBill(ctx context.Context, id string) (*domain.CloseBillResponse, error) {
	if err := validateBillID(id); err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}

	// Atomically freeze the bill in the DB before touching the workflow.
	// This prevents concurrent AddLineItem calls from inserting new rows while
	// the workflow is computing the total.
	needsFinalization, err := MarkBillClosedImmediate(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}

	if needsFinalization {
		// Recovery path: bill was frozen by a prior close call but the workflow
		// died before writing total_amount. Re-run finalization directly.
		if err := finalizeBillTotal(ctx, id); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to finalize bill total").Cause(err).Err()
		}
	} else {
		// Normal path: bill was just frozen. Signal the workflow to compute and
		// persist the total over the now-immutable set of line items.
		if err := s.temporalClient.SignalWorkflow(ctx, workflow.WorkflowID(id), "", workflow.CloseSignalName, workflow.CloseSignalPayload{}); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to signal bill workflow").Cause(err).Err()
		}
		run := s.temporalClient.GetWorkflow(ctx, workflow.WorkflowID(id), "")
		if err := run.Get(ctx, nil); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("bill close workflow failed").Cause(err).Err()
		}
	}

	closedBill, err := loadBill(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if closedBill.TotalAmount == nil || closedBill.ClosedAt == nil {
		return nil, errs.B().Code(errs.Internal).Msg("bill was not closed").Err()
	}

	rounded, err := money.RoundToCurrencyScale(*closedBill.TotalAmount, closedBill.Currency)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to round total amount").Err()
	}

	return &domain.CloseBillResponse{
		BillID:        closedBill.ID,
		CustomerID:    closedBill.CustomerID,
		PeriodStart:   closedBill.PeriodStart.Format("2006-01-02"),
		PeriodEnd:     closedBill.PeriodEnd.Format("2006-01-02"),
		Currency:      closedBill.Currency,
		TotalAmount:   rounded,
		LineItemCount: len(closedBill.LineItems),
		ClosedAt:      *closedBill.ClosedAt,
		LineItems:     closedBill.LineItems,
	}, nil
}

// finalizeBillTotal computes the sum of line items for an already-closed bill
// and writes it to total_amount. Used for the normal close path (called by the
// workflow) and the recovery path (called directly when the workflow failed).
func finalizeBillTotal(ctx context.Context, billID string) error {
	result, err := activity.ComputeTotal(ctx, billID)
	if err != nil {
		return err
	}
	return FinalizeBillTotal(ctx, billID, result.TotalAmount)
}

//encore:api public method=POST path=/bills/:id/line-items
func (s *Service) AddLineItem(ctx context.Context, id string, req *domain.AddLineItemRequest) (*domain.LineItem, error) {
	if err := validateBillID(id); err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}
	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if bill.Status == domain.BillStatusClosed {
		return nil, mapDomainErr(domain.ErrBillAlreadyClosed)
	}

	params, err := parseAddLineItemRequest(req, bill)
	if err != nil {
		return nil, mapValidationErr(err)
	}

	result, err := InsertLineItem(ctx, params)
	if err != nil {
		return nil, mapDomainErr(err)
	}

	if err := s.temporalClient.SignalWorkflow(ctx, workflow.WorkflowID(id), "", workflow.LineItemSignalName, lineItemSignalPayload(result.LineItem)); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to signal bill workflow with line item").Cause(err).Err()
	}

	return &result.LineItem, nil
}

func lineItemSignalPayload(item domain.LineItem) workflow.LineItemSignalPayload {
	return workflow.LineItemSignalPayload{
		LineItemID:          item.ID,
		ExternalReferenceID: item.ExternalReferenceID,
		FeeType:             string(item.FeeType),
		Description:         item.Description,
		TotalAmount:         item.TotalAmount.String(),
		Currency:            item.Currency,
		EffectiveDate:       item.EffectiveDate,
	}
}

func loadBill(ctx context.Context, id string) (domain.Bill, error) {
	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return domain.Bill{}, err
	}

	items, err := ListLineItems(ctx, id)
	if err != nil {
		return domain.Bill{}, err
	}
	bill.LineItems = items
	return bill, nil
}

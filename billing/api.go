package billing

import (
	"context"

	"encore.dev/beta/errs"
	"go.temporal.io/sdk/client"

	"pave-bank/activity"
	"pave-bank/domain"
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
		PeriodEnd:   result.Bill.PeriodEnd,
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

// finalizeBillTotal computes the sum of line items for a bill in closing state
// and writes it to total_amount. Used by the workflow close segment and recovery paths.
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
	if bill.Status != domain.BillStatusOpen {
		switch bill.Status {
		case domain.BillStatusScheduled:
			return nil, mapDomainErr(domain.ErrBillNotYetOpen)
		default:
			return nil, mapDomainErr(domain.ErrBillAlreadyClosed)
		}
	}

	params, err := parseAddLineItemRequest(req, bill)
	if err != nil {
		return nil, mapValidationErr(err)
	}

	if err := s.temporalClient.SignalWorkflow(ctx, workflow.WorkflowID(id), "", workflow.LineItemSignalName, lineItemSignalPayload(params)); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to signal bill workflow with line item").Cause(err).Err()
	}

	item, err := waitForLineItem(ctx, id, params.ExternalReferenceID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("line item not persisted by workflow").Cause(err).Err()
	}

	return &item, nil
}

func lineItemSignalPayload(params InsertLineItemParams) workflow.LineItemSignalPayload {
	return workflow.LineItemSignalPayload{
		ExternalReferenceID: params.ExternalReferenceID,
		FeeType:             string(params.FeeType),
		Description:         params.Description,
		Quantity:            params.Quantity.String(),
		UnitPrice:           params.UnitPrice.String(),
		Currency:            params.Currency,
		EffectiveDate:       params.EffectiveDate.Format("2006-01-02"),
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

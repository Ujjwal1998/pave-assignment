package billing

import (
	"context"

	"encore.dev/beta/errs"
	"go.temporal.io/sdk/client"

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
		BillID:     result.Bill.ID,
		CustomerID: result.Bill.CustomerID,
		Currency:   result.Bill.Currency,
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
	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if bill.Status == domain.BillStatusClosed {
		return nil, errs.B().Code(errs.AlreadyExists).Msg("bill is already closed").Err()
	}

	if err := s.temporalClient.SignalWorkflow(ctx, workflow.WorkflowID(id), "", workflow.CloseSignalName, workflow.CloseSignalPayload{}); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to signal bill workflow").Cause(err).Err()
	}

	run := s.temporalClient.GetWorkflow(ctx, workflow.WorkflowID(id), "")
	if err := run.Get(ctx, nil); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("bill close workflow failed").Cause(err).Err()
	}

	closedBill, err := loadBill(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if closedBill.TotalAmount == nil || closedBill.ClosedAt == nil {
		return nil, errs.B().Code(errs.Internal).Msg("bill was not closed").Err()
	}

	return &domain.CloseBillResponse{
		BillID:      closedBill.ID,
		CustomerID:  closedBill.CustomerID,
		PeriodStart: closedBill.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   closedBill.PeriodEnd.Format("2006-01-02"),
		Currency:    closedBill.Currency,
		TotalAmount: *closedBill.TotalAmount,
		ClosedAt:    *closedBill.ClosedAt,
		LineItems:   closedBill.LineItems,
	}, nil
}

//encore:api public method=POST path=/bills/:id/line-items
func AddLineItem(ctx context.Context, id string, req *domain.AddLineItemRequest) (*domain.LineItem, error) {
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
	return &result.LineItem, nil
}

func loadBill(ctx context.Context, id string) (domain.Bill, error) {
	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return domain.Bill{}, err
	}

	if bill.Status != domain.BillStatusClosed {
		return bill, nil
	}

	items, err := ListLineItems(ctx, id)
	if err != nil {
		return domain.Bill{}, err
	}
	bill.LineItems = items
	return bill, nil
}

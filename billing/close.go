package billing

import (
	"context"
	"encoding/json"
	"net/http"

	"encore.dev"
	"encore.dev/beta/errs"

	"pave-bank/domain"
	"pave-bank/money"
	"pave-bank/workflow"
)

type closeOutcome struct {
	Accepted *domain.CloseBillAccepted
	Closed   *domain.CloseBillResponse
}

//encore:api public raw method=POST path=/bills/:id/close
func (s *Service) CloseBill(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := encore.CurrentRequest().PathParams.Get("id")
	if err := validateBillID(id); err != nil {
		writeAPIError(w, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err())
		return
	}

	wait := req.URL.Query().Get("wait") == "true"
	outcome, err := s.requestClose(ctx, id, wait)
	if err != nil {
		writeAPIError(w, mapDomainErr(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if outcome.Accepted != nil {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(outcome.Accepted)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(outcome.Closed)
}

//encore:api public method=POST path=/bills/:id/finalize
func (s *Service) FinalizeBill(ctx context.Context, id string) (*domain.CloseBillResponse, error) {
	if err := validateBillID(id); err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}

	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if bill.Status != domain.BillStatusClosing {
		return nil, mapDomainErr(domain.ErrBillAlreadyClosed)
	}
	if bill.TotalAmount != nil {
		return buildCloseBillResponse(ctx, id)
	}

	if err := finalizeBillTotal(ctx, id); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to finalize bill total").Cause(err).Err()
	}
	return buildCloseBillResponse(ctx, id)
}

func (s *Service) requestClose(ctx context.Context, id string, wait bool) (*closeOutcome, error) {
	needsFinalization, err := MarkBillClosing(ctx, id)
	if err != nil {
		return nil, err
	}

	if needsFinalization {
		if err := finalizeBillTotal(ctx, id); err != nil {
			return nil, err
		}
		resp, err := buildCloseBillResponse(ctx, id)
		if err != nil {
			return nil, err
		}
		return &closeOutcome{Closed: resp}, nil
	}

	if err := s.signalClose(ctx, id); err != nil {
		return nil, err
	}

	if !wait {
		return &closeOutcome{Accepted: &domain.CloseBillAccepted{
			BillID: id,
			Status: domain.BillStatusClosing,
		}}, nil
	}

	if err := s.waitForCloseWorkflow(ctx, id); err != nil {
		return nil, err
	}
	resp, err := buildCloseBillResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return &closeOutcome{Closed: resp}, nil
}

func (s *Service) signalClose(ctx context.Context, id string) error {
	if err := s.temporalClient.SignalWorkflow(ctx, workflow.WorkflowID(id), "", workflow.CloseSignalName, workflow.CloseSignalPayload{}); err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to signal bill workflow").Cause(err).Err()
	}
	return nil
}

func (s *Service) waitForCloseWorkflow(ctx context.Context, id string) error {
	run := s.temporalClient.GetWorkflow(ctx, workflow.WorkflowID(id), "")
	if err := run.Get(ctx, nil); err != nil {
		return errs.B().Code(errs.Internal).Msg("bill close workflow failed").Cause(err).Err()
	}
	return nil
}

func buildCloseBillResponse(ctx context.Context, id string) (*domain.CloseBillResponse, error) {
	closedBill, err := loadBill(ctx, id)
	if err != nil {
		return nil, err
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

func writeAPIError(w http.ResponseWriter, err error) {
	apiErr, ok := err.(*errs.Error)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(errCodeHTTPStatus(apiErr.Code))
	_ = json.NewEncoder(w).Encode(apiErr)
}

func errCodeHTTPStatus(code errs.ErrCode) int {
	switch code {
	case errs.InvalidArgument:
		return http.StatusBadRequest
	case errs.NotFound:
		return http.StatusNotFound
	case errs.AlreadyExists:
		return http.StatusConflict
	case errs.FailedPrecondition:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

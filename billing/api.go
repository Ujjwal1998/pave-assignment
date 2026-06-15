package billing

import (
	"context"
	"errors"

	"encore.dev/beta/errs"
	"pave-bank/domain"
)

//encore:api public method=GET path=/bills/:id
func GetBill(ctx context.Context, id string) (*domain.Bill, error) {
	bill, err := loadBill(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	return &bill, nil
}

//encore:api public method=POST path=/bills
func CreateBill(ctx context.Context, req *domain.CreateBillRequest) (*domain.Bill, error) {
	params, err := parseCreateBillRequest(req)
	if err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	}

	result, err := CreateBillRecord(ctx, params)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if !result.Created {
		return nil, mapDomainErr(domain.ErrBillAlreadyExists)
	}
	return &result.Bill, nil
}

//encore:api public method=POST path=/bills/:id/line-items
func AddLineItem(ctx context.Context, id string, req *domain.AddLineItemRequest) (*domain.LineItem, error) {
	bill, err := GetBillByID(ctx, id)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if bill.Status == domain.BillStatusClosed {
		return nil, mapDomainErr(domain.ErrBillAlreadyClosed)
	}

	params, err := parseAddLineItemRequest(req, bill)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFeeType) ||
			errors.Is(err, domain.ErrLineItemOutOfPeriod) ||
			errors.Is(err, domain.ErrInvalidDecimal) {
			return nil, mapDomainErr(err)
		}
		return nil, errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
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

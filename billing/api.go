package billing

import (
	"context"

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

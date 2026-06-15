package activity

import (
	"context"
	"fmt"

	goMoney "github.com/govalues/money"

	"pave-bank/money"
)

func ComputeTotal(ctx context.Context, billID string) (ComputeTotalResult, error) {
	if store == nil {
		return ComputeTotalResult{}, fmt.Errorf("activity store is not initialized")
	}

	currency, err := store.GetBillCurrency(ctx, billID)
	if err != nil {
		return ComputeTotalResult{}, fmt.Errorf("get bill currency: %w", err)
	}

	amounts, err := store.ListLineItemAmounts(ctx, billID)
	if err != nil {
		return ComputeTotalResult{}, fmt.Errorf("list line item amounts: %w", err)
	}

	total, err := goMoney.NewAmountFromInt64(currency, 0, 0, 0)
	if err != nil {
		return ComputeTotalResult{}, fmt.Errorf("create zero amount: %w", err)
	}

	for _, row := range amounts {
		lineAmt, err := money.ToAmount(row.Amount, row.Currency)
		if err != nil {
			return ComputeTotalResult{}, fmt.Errorf("convert line amount: %w", err)
		}
		total, err = total.Add(lineAmt)
		if err != nil {
			return ComputeTotalResult{}, fmt.Errorf("add line amount: %w", err)
		}
	}

	totalDecimal, totalCurrency := money.FromAmount(total)
	return ComputeTotalResult{
		TotalAmount: totalDecimal,
		Currency:    totalCurrency,
	}, nil
}

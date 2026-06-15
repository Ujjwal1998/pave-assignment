package billing

import (
	"fmt"
	"regexp"
	"time"

	"github.com/govalues/decimal"
	"pave-bank/domain"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func parseCreateBillRequest(req *domain.CreateBillRequest) (CreateBillParams, error) {
	if req == nil {
		return CreateBillParams{}, fmt.Errorf("request body is required")
	}
	if req.CustomerID == "" {
		return CreateBillParams{}, fmt.Errorf("customer_id is required")
	}

	periodStart, err := parseDate(req.PeriodStart, "period_start")
	if err != nil {
		return CreateBillParams{}, err
	}
	periodEnd, err := parseDate(req.PeriodEnd, "period_end")
	if err != nil {
		return CreateBillParams{}, err
	}
	if !periodEnd.After(periodStart) {
		return CreateBillParams{}, fmt.Errorf("period_end must be after period_start")
	}
	if !currencyPattern.MatchString(req.Currency) {
		return CreateBillParams{}, fmt.Errorf("currency must be a 3-letter ISO 4217 code")
	}

	return CreateBillParams{
		CustomerID:  req.CustomerID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Currency:    req.Currency,
	}, nil
}

func parseDate(value, field string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be formatted as YYYY-MM-DD", field)
	}
	return parsed, nil
}

func parseAddLineItemRequest(req *domain.AddLineItemRequest, bill domain.Bill) (InsertLineItemParams, error) {
	if req == nil {
		return InsertLineItemParams{}, fmt.Errorf("request body is required")
	}
	if req.Description == "" {
		return InsertLineItemParams{}, fmt.Errorf("description is required")
	}
	if req.ExternalReferenceID == "" {
		return InsertLineItemParams{}, fmt.Errorf("external_reference_id is required")
	}
	if !req.FeeType.Valid() {
		return InsertLineItemParams{}, domain.ErrInvalidFeeType
	}
	if req.Quantity == "" {
		return InsertLineItemParams{}, fmt.Errorf("quantity is required")
	}
	if req.UnitPrice == "" {
		return InsertLineItemParams{}, fmt.Errorf("unit_price is required")
	}
	if req.EffectiveDate == "" {
		return InsertLineItemParams{}, fmt.Errorf("effective_date is required")
	}

	quantity, err := parsePositiveDecimal(req.Quantity, "quantity")
	if err != nil {
		return InsertLineItemParams{}, err
	}
	unitPrice, err := parseDecimal(req.UnitPrice, "unit_price")
	if err != nil {
		return InsertLineItemParams{}, err
	}

	effectiveDate, err := parseDate(req.EffectiveDate, "effective_date")
	if err != nil {
		return InsertLineItemParams{}, err
	}
	if !dateWithinPeriod(effectiveDate, bill.PeriodStart, bill.PeriodEnd) {
		return InsertLineItemParams{}, domain.ErrLineItemOutOfPeriod
	}

	totalAmount, err := quantity.Mul(unitPrice)
	if err != nil {
		return InsertLineItemParams{}, domain.ErrInvalidDecimal
	}

	return InsertLineItemParams{
		BillID:              bill.ID,
		FeeType:             req.FeeType,
		Description:         req.Description,
		Quantity:            quantity,
		UnitPrice:           unitPrice,
		TotalAmount:         totalAmount,
		Currency:            bill.Currency,
		EffectiveDate:       effectiveDate,
		ExternalReferenceID: req.ExternalReferenceID,
	}, nil
}

func parseDecimal(value, field string) (decimal.Decimal, error) {
	amount, err := decimal.Parse(value)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%w: %s", domain.ErrInvalidDecimal, field)
	}
	return amount, nil
}

func parsePositiveDecimal(value, field string) (decimal.Decimal, error) {
	amount, err := parseDecimal(value, field)
	if err != nil {
		return decimal.Decimal{}, err
	}
	pos := amount.IsPos()
	if !pos {
		return decimal.Decimal{}, fmt.Errorf("%s must be greater than zero", field)
	}
	return amount, nil
}

func dateWithinPeriod(date, periodStart, periodEnd time.Time) bool {
	d := truncateDate(date)
	start := truncateDate(periodStart)
	end := truncateDate(periodEnd)
	return !d.Before(start) && !d.After(end)
}

func truncateDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

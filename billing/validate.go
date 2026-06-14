package billing

import (
	"fmt"
	"regexp"
	"time"

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

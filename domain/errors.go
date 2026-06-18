package domain

import "errors"

var (
	ErrBillNotFound        = errors.New("bill not found")
	ErrBillAlreadyClosed   = errors.New("bill is already closed")
	ErrBillAlreadyExists   = errors.New("bill already exists for this customer/period/currency")
	ErrDuplicateLineItem         = errors.New("line item with this external_reference_id already exists")
	ErrUnsupportedCurrencyPair = errors.New("no exchange rate for this currency pair")
	ErrLineItemOutOfPeriod = errors.New("effective_date is outside the bill period")
	ErrInvalidFeeType      = errors.New("invalid fee_type")
	ErrInvalidDecimal      = errors.New("invalid decimal value")
)

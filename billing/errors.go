package billing

import (
	"errors"

	"encore.dev/beta/errs"
	"pave-bank/domain"
)

func mapDomainErr(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrBillNotFound):
		return errs.B().Code(errs.NotFound).Msg(err.Error()).Cause(err).Err()
	case errors.Is(err, domain.ErrBillAlreadyClosed):
		return errs.B().Code(errs.FailedPrecondition).Msg(err.Error()).Cause(err).Err()
	case errors.Is(err, domain.ErrBillAlreadyExists):
		return errs.B().Code(errs.AlreadyExists).Msg(err.Error()).Cause(err).Err()
	case errors.Is(err, domain.ErrDuplicateLineItem):
		return errs.B().Code(errs.AlreadyExists).Msg(err.Error()).Cause(err).Err()
	case errors.Is(err, domain.ErrCurrencyMismatch),
		errors.Is(err, domain.ErrLineItemOutOfPeriod),
		errors.Is(err, domain.ErrInvalidDecimal):
		return errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
	case errors.Is(err, domain.ErrInvalidFeeType):
		return errs.B().Code(errs.InvalidArgument).
			Msg("fee_type must be one of: subscription, usage, tax, penalty, discount").Err()
	default:
		return errs.B().Code(errs.Internal).Msg("internal error").Cause(err).Err()
	}
}

func mapValidationErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInvalidFeeType) ||
		errors.Is(err, domain.ErrLineItemOutOfPeriod) ||
		errors.Is(err, domain.ErrInvalidDecimal) {
		return mapDomainErr(err)
	}
	return errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
}

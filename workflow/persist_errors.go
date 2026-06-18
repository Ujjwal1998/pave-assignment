package workflow

import (
	"errors"
	"strings"

	"go.temporal.io/sdk/temporal"

	"pave-bank/domain"
)

func isLineItemPersistRejected(err error) bool {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.Type() == "BillFrozen" {
		return true
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		if errors.Is(e, domain.ErrBillAlreadyClosed) || errors.Is(e, domain.ErrBillNotYetOpen) {
			return true
		}
	}
	msg := err.Error()
	return strings.Contains(msg, domain.ErrBillAlreadyClosed.Error()) ||
		strings.Contains(msg, domain.ErrBillNotYetOpen.Error())
}

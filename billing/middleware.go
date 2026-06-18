package billing

import (
	"errors"
	"net/http"

	"encore.dev/middleware"
	"pave-bank/domain"
)

// BillStateMiddleware maps bill lifecycle errors to HTTP 422 so clients can
// distinguish state violations from generic validation failures.
//
//encore:middleware target=all
func BillStateMiddleware(req middleware.Request, next middleware.Next) middleware.Response {
	resp := next(req)
	if resp.Err == nil {
		return resp
	}
	if errors.Is(resp.Err, domain.ErrBillAlreadyClosed) ||
		errors.Is(resp.Err, domain.ErrBillNotYetOpen) {
		resp.HTTPStatus = http.StatusUnprocessableEntity
	}
	return resp
}

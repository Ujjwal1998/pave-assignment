package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"encore.dev/storage/sqldb"
	"github.com/govalues/decimal"
	"github.com/jackc/pgx/v5/pgconn"
	"pave-bank/activity"
	"pave-bank/domain"
)

var billingDB = sqldb.NewDatabase("billing", sqldb.DatabaseConfig{
	Migrations: "migrations",
})

const (
	billColumns = `
		id, customer_id, period_start, period_end, currency,
		status, total_amount, created_at, closed_at, workflow_run_id`

	lineItemColumns = `
		id, bill_id, fee_type, description, quantity, unit_price,
		total_amount, currency, effective_date, external_reference_id, created_at`
)

type CreateBillParams struct {
	CustomerID  string
	PeriodStart time.Time
	PeriodEnd   time.Time
	Currency    string
}

type CreateBillResult struct {
	Bill    domain.Bill
	Created bool
}

type InsertLineItemParams struct {
	BillID              string
	FeeType             domain.FeeType
	Description         string
	Quantity            decimal.Decimal
	UnitPrice           decimal.Decimal
	TotalAmount         decimal.Decimal
	Currency            string
	EffectiveDate       time.Time
	ExternalReferenceID string
}

type InsertLineItemResult struct {
	LineItem domain.LineItem
	Created  bool
}

func CreateBillRecord(ctx context.Context, params CreateBillParams) (CreateBillResult, error) {
	const query = `
		INSERT INTO bills (customer_id, period_start, period_end, currency)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (customer_id, period_start, period_end, currency) DO NOTHING
		RETURNING` + billColumns

	row := billingDB.QueryRow(ctx, query,
		params.CustomerID,
		params.PeriodStart,
		params.PeriodEnd,
		params.Currency,
	)

	bill, err := scanBill(row)
	if err == nil {
		return CreateBillResult{Bill: bill, Created: true}, nil
	}
	if !errors.Is(err, sqldb.ErrNoRows) {
		return CreateBillResult{}, fmt.Errorf("insert bill: %w", err)
	}

	existing, err := getBillByUniqueKey(ctx, params.CustomerID, params.PeriodStart, params.PeriodEnd, params.Currency)
	if err != nil {
		return CreateBillResult{}, err
	}
	return CreateBillResult{Bill: existing, Created: false}, nil
}

func GetBillByID(ctx context.Context, billID string) (domain.Bill, error) {
	const query = `SELECT` + billColumns + ` FROM bills WHERE id = $1`

	row := billingDB.QueryRow(ctx, query, billID)
	bill, err := scanBill(row)
	if errors.Is(err, sqldb.ErrNoRows) {
		return domain.Bill{}, domain.ErrBillNotFound
	}
	if err != nil {
		return domain.Bill{}, fmt.Errorf("get bill: %w", err)
	}
	return bill, nil
}

func UpdateWorkflowRunID(ctx context.Context, billID, workflowRunID string) error {
	const query = `
		UPDATE bills
		SET workflow_run_id = $2
		WHERE id = $1`

	_, err := billingDB.Exec(ctx, query, billID, workflowRunID)
	if err != nil {
		return fmt.Errorf("update workflow run id: %w", err)
	}
	return nil
}

// MarkBillClosedImmediate atomically transitions the bill from open to closed,
// freezing line items immediately so concurrent AddLineItem calls are rejected.
// total_amount is left NULL; the workflow fills it via FinalizeBillTotal.
//
// Returns (needsFinalization=true, nil) when the bill is already closed but
// has no total yet (workflow died mid-close) — caller should re-run finalization.
// Returns (false, ErrBillAlreadyClosed) when the bill is fully closed.
func MarkBillClosedImmediate(ctx context.Context, billID string) (needsFinalization bool, err error) {
	const query = `
		UPDATE bills
		SET status = 'closed', closed_at = now()
		WHERE id = $1 AND status = 'open'
		RETURNING id`

	var id string
	scanErr := billingDB.QueryRow(ctx, query, billID).Scan(&id)
	if scanErr == nil {
		return false, nil
	}
	if !errors.Is(scanErr, sqldb.ErrNoRows) {
		return false, fmt.Errorf("mark bill closed immediate: %w", scanErr)
	}

	bill, err := GetBillByID(ctx, billID)
	if err != nil {
		return false, err
	}
	if bill.Status == domain.BillStatusClosed && bill.TotalAmount == nil {
		return true, nil
	}
	if bill.Status == domain.BillStatusClosed {
		return false, domain.ErrBillAlreadyClosed
	}
	return false, domain.ErrBillNotFound
}

// FinalizeBillTotal writes the computed total onto an already-closed bill.
// Safe to call multiple times (idempotent on workflow retry).
func FinalizeBillTotal(ctx context.Context, billID string, totalAmount decimal.Decimal) error {
	const query = `UPDATE bills SET total_amount = $2 WHERE id = $1 AND status = 'closed'`
	_, err := billingDB.Exec(ctx, query, billID, totalAmount)
	if err != nil {
		return fmt.Errorf("finalize bill total: %w", err)
	}
	return nil
}

func InsertLineItem(ctx context.Context, params InsertLineItemParams) (InsertLineItemResult, error) {
	const query = `
		INSERT INTO line_items (
			bill_id, fee_type, description, quantity, unit_price,
			total_amount, currency, effective_date, external_reference_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (bill_id, external_reference_id) DO NOTHING
		RETURNING` + lineItemColumns

	row := billingDB.QueryRow(ctx, query,
		params.BillID,
		string(params.FeeType),
		params.Description,
		params.Quantity,
		params.UnitPrice,
		params.TotalAmount,
		params.Currency,
		params.EffectiveDate,
		params.ExternalReferenceID,
	)

	item, err := scanLineItem(row)
	if err == nil {
		return InsertLineItemResult{LineItem: item, Created: true}, nil
	}
	if !errors.Is(err, sqldb.ErrNoRows) {
		return InsertLineItemResult{}, classifyLineItemInsertErr(err)
	}

	existing, err := getLineItemByExternalRef(ctx, params.BillID, params.ExternalReferenceID)
	if err != nil {
		return InsertLineItemResult{}, err
	}
	return InsertLineItemResult{LineItem: existing, Created: false}, nil
}

func ListLineItems(ctx context.Context, billID string) ([]domain.LineItem, error) {
	const query = `
		SELECT` + lineItemColumns + `
		FROM line_items
		WHERE bill_id = $1
		ORDER BY effective_date, created_at`

	rows, err := billingDB.Query(ctx, query, billID)
	if err != nil {
		return nil, fmt.Errorf("list line items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.LineItem, 0)
	for rows.Next() {
		item, err := scanLineItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan line item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate line items: %w", err)
	}
	return items, nil
}

func listLineItemAmounts(ctx context.Context, billID string) ([]activity.LineAmount, error) {
	const query = `
		SELECT total_amount, currency
		FROM line_items
		WHERE bill_id = $1
		ORDER BY effective_date, created_at`

	rows, err := billingDB.Query(ctx, query, billID)
	if err != nil {
		return nil, fmt.Errorf("list line item amounts: %w", err)
	}
	defer rows.Close()

	var amounts []activity.LineAmount
	for rows.Next() {
		var amount activity.LineAmount
		if err := rows.Scan(&amount.Amount, &amount.Currency); err != nil {
			return nil, fmt.Errorf("scan line item amount: %w", err)
		}
		amounts = append(amounts, amount)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate line item amounts: %w", err)
	}
	return amounts, nil
}

func getBillByUniqueKey(ctx context.Context, customerID string, periodStart, periodEnd time.Time, currency string) (domain.Bill, error) {
	const query = `
		SELECT` + billColumns + `
		FROM bills
		WHERE customer_id = $1
		  AND period_start = $2
		  AND period_end = $3
		  AND currency = $4`

	row := billingDB.QueryRow(ctx, query, customerID, periodStart, periodEnd, currency)
	bill, err := scanBill(row)
	if errors.Is(err, sqldb.ErrNoRows) {
		return domain.Bill{}, domain.ErrBillNotFound
	}
	if err != nil {
		return domain.Bill{}, fmt.Errorf("get bill by unique key: %w", err)
	}
	return bill, nil
}

func getLineItemByExternalRef(ctx context.Context, billID, externalReferenceID string) (domain.LineItem, error) {
	const query = `
		SELECT` + lineItemColumns + `
		FROM line_items
		WHERE bill_id = $1 AND external_reference_id = $2`

	row := billingDB.QueryRow(ctx, query, billID, externalReferenceID)
	item, err := scanLineItem(row)
	if errors.Is(err, sqldb.ErrNoRows) {
		return domain.LineItem{}, domain.ErrDuplicateLineItem
	}
	if err != nil {
		return domain.LineItem{}, fmt.Errorf("get line item by external ref: %w", err)
	}
	return item, nil
}

func classifyLineItemInsertErr(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return domain.ErrBillAlreadyClosed
	}
	return fmt.Errorf("insert line item: %w", err)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBill(row rowScanner) (domain.Bill, error) {
	var (
		bill                                              domain.Bill
		status                                            string
		totalAmount                                       sql.NullString
		closedAt                                          sql.NullTime
	)

	err := row.Scan(
		&bill.ID,
		&bill.CustomerID,
		&bill.PeriodStart,
		&bill.PeriodEnd,
		&bill.Currency,
		&status,
		&totalAmount,
		&bill.CreatedAt,
		&closedAt,
		&bill.WorkflowRunID,
	)
	if err != nil {
		return domain.Bill{}, err
	}

	bill.Status = domain.BillStatus(status)
	if totalAmount.Valid {
		amount, err := decimal.Parse(totalAmount.String)
		if err != nil {
			return domain.Bill{}, fmt.Errorf("parse bill total: %w", err)
		}
		bill.TotalAmount = &amount
	}
	if closedAt.Valid {
		bill.ClosedAt = &closedAt.Time
	}
	return bill, nil
}

func scanLineItem(row rowScanner) (domain.LineItem, error) {
	var (
		item    domain.LineItem
		feeType string
	)

	err := row.Scan(
		&item.ID,
		&item.BillID,
		&feeType,
		&item.Description,
		&item.Quantity,
		&item.UnitPrice,
		&item.TotalAmount,
		&item.Currency,
		&item.EffectiveDate,
		&item.ExternalReferenceID,
		&item.CreatedAt,
	)
	if err != nil {
		return domain.LineItem{}, err
	}

	item.FeeType = domain.FeeType(feeType)
	return item, nil
}

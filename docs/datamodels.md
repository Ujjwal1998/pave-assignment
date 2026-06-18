# Data Models

Postgres is the **read model** for the HTTP API. Temporal workflows (`bill-{id}`) orchestrate lifecycle transitions and persist accrual totals back to the database.

## Entity relationship

```
customers (logical, not a table)
    │
    └── bills (1 per customer / period / currency)
            │
            └── line_items (many; idempotent on external_reference_id)
```

---

## Bill lifecycle: DB status vs workflow phase

Two parallel status systems exist. They align during normal operation but are not identical.

| DB `bills.status` | Workflow `phase` (Temporal `status` query) | Line items allowed? |
|-------------------|--------------------------------------------|---------------------|
| `scheduled` | `waiting_period_start` | No |
| `open` | `accruing` | Yes |
| `closing` | `closing` | No |
| `closed` | *(workflow completed — query unavailable)* | No |

### State transitions

```
                    CreateBill (period_start in future)
                              │
                              ▼
                        ┌───────────┐
                        │ scheduled │
                        └─────┬─────┘
                              │ ActivateBill (at period_start)
                              ▼
         CreateBill ──► ┌───────────┐ ◄── line-item signals
         (period started)│   open    │
                        └─────┬─────┘
                              │ POST /close OR auto-close timer
                              ▼
                        ┌───────────┐
                        │  closing  │  closed_at set; total_amount NULL
                        └─────┬─────┘
                              │ FinalizeBillTotal (UpdateBillClosed activity)
                              ▼
                        ┌───────────┐
                        │  closed   │  total_amount set; accrual_total cleared
                        └───────────┘
```

**Notes**

- **`closing`** is brief: the API or auto-close freezes the bill; the workflow runs `UpdateBillClosed` (compute + finalize).
- Workflow has **no `closed` phase**. When finalization completes, the workflow is **Completed** and the `status` query no longer works — use `GET /bills/:id`.
- **`accrual_total`** is maintained while `open` and cleared when the bill becomes `closed`.
- **Auto-close** fires at **00:00 UTC on the day after `period_end`** (e.g. period ending 2026-06-30 closes at 2026-07-01T00:00:00Z).

---

## `bills`

Invoice header. One row per `(customer_id, period_start, period_end, currency)`.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `customer_id` | TEXT | Customer identifier |
| `period_start` | DATE | Billing period start (inclusive) |
| `period_end` | DATE | Billing period end (inclusive) |
| `currency` | CHAR(3) | Invoice / settlement currency (ISO 4217) |
| `status` | `bill_status` | `scheduled` \| `open` \| `closing` \| `closed` |
| `accrual_total` | NUMERIC(19,9) | Running total in bill currency while `open`; NULL otherwise |
| `total_amount` | NUMERIC(19,9) | Final invoice total when `closed`; NULL while open/closing |
| `created_at` | TIMESTAMPTZ | Row creation time |
| `closed_at` | TIMESTAMPTZ | Set when entering `closing` |
| `workflow_run_id` | TEXT | Temporal workflow run ID |

**Constraints**

- `UNIQUE (customer_id, period_start, period_end, currency)`
- `period_end > period_start`
- `currency` matches `^[A-Z]{3}$`

**Initial status on create**

- `scheduled` if `period_start` is after today (UTC date)
- `open` if `period_start` is today or in the past

**Go type:** `domain.Bill` (`domain/bill.go`)

---

## `line_items`

Individual fees on a bill. Stored in original amount and currency; converted to bill currency at close.

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `bill_id` | UUID | FK → `bills.id` |
| `fee_type` | `fee_type` | `subscription` \| `usage` \| `tax` \| `penalty` \| `discount` |
| `description` | TEXT | Human-readable label |
| `quantity` | NUMERIC(19,9) | Must be > 0 |
| `unit_price` | NUMERIC(19,9) | May be negative (discounts) |
| `total_amount` | NUMERIC(19,9) | `quantity × unit_price` |
| `currency` | CHAR(3) | Line denomination; defaults to bill currency if omitted on API |
| `effective_date` | DATE | Must fall within bill period |
| `external_reference_id` | TEXT | Idempotency key per bill |
| `created_at` | TIMESTAMPTZ | Row creation time |

**Constraints**

- `UNIQUE (bill_id, external_reference_id)` — duplicate POST returns existing row
- DB trigger rejects inserts when parent bill `status <> 'open'`

**Go type:** `domain.LineItem` (`domain/line_item.go`)

---

## Multi-currency

| Concept | Rule |
|---------|------|
| Bill `currency` | Invoice currency; `total_amount` and `accrual_total` are always in this currency |
| Line item `currency` | Optional; may differ from bill (e.g. GEL line on USD bill) |
| Conversion | Static rates in `money/rates.go` at close / accrual time |
| Supported pairs | USD ↔ GEL (extensible) |

Example: USD bill with `99 USD` + `100 GEL` at `1 GEL = 0.37 USD` → close total **`136.00 USD`**.

Line items in API responses keep their **original** `currency` and `total_amount`.

---

## Temporal workflow

| Property | Value |
|----------|-------|
| Workflow ID | `bill-{bill_uuid}` |
| Task queue | `{env}-billing` |
| Input | `BillID`, `CustomerID`, `Currency`, `PeriodStart`, `PeriodEnd` |

### Queries (while workflow is running)

| Query | Returns |
|-------|---------|
| `status` | `ProcessStatus`: `phase`, `accrual_total`, `line_item_count`, period dates |
| `accrual` | `AccrualState`: running total + full line-item signal payloads |

```bash
temporal workflow query --workflow-id bill-{id} --name status
temporal workflow query --workflow-id bill-{id} --name accrual
```

### Signals

| Signal | When sent | Effect |
|--------|-----------|--------|
| `bill.line_item.added` | After each successful `POST /line-items` | Update in-memory accrual; persist `accrual_total` |
| `bill.close` | After `POST /close` marks bill `closing` | Exit accrual loop; run `UpdateBillClosed` |

### Activities

| Activity | Purpose |
|----------|---------|
| `ActivateBill` | `scheduled` → `open` at period start |
| `UpdateAccrualTotal` | Write workflow running total to `bills.accrual_total` |
| `UpdateBillClosed` | `EnsureBillClosing` + `ComputeTotal` + `FinalizeBillTotal` |

---

## API types (summary)

### `CreateBillRequest`

`customer_id`, `period_start`, `period_end`, `currency`

### `AddLineItemRequest`

`fee_type`, `description`, `quantity`, `unit_price`, `effective_date`, `external_reference_id`, optional `currency`

### `CloseBillResponse`

Closed bill snapshot: `total_amount` (bill currency), all `line_items`, `closed_at`

---

## HTTP status codes (lifecycle-related)

| Code | When |
|------|------|
| 422 | Line item on non-open bill; close on already-closed bill; bill not yet open (`scheduled`) |
| 409 | Duplicate bill or duplicate line item (existing resource returned) |
| 400 | Validation (fee type, period, unsupported currency pair, etc.) |

---

## Source files

| Area | Path |
|------|------|
| Domain types | `domain/bill.go`, `domain/line_item.go`, `domain/errors.go` |
| Migrations | `billing/migrations/` |
| DB access | `billing/db.go` |
| Workflow | `workflow/bill_workflow.go`, `workflow/signals.go` |
| Activities | `activity/` |
| FX rates | `money/rates.go`, `money/convert.go` |

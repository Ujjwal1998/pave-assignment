# pave-assignment

Progressive accrual billing API built with [Encore](https://encore.dev) and [Temporal](https://temporal.io).

## Architecture

```
POST /bills          → DB insert + start Temporal workflow (bill-{id})
POST /bills/:id/line-items → DB insert + signal workflow (progressive accrual)
POST /close          → Signal workflow → ComputeTotal → UpdateBillClosed
GET /bills/:id       → DB read (line items included while open and when closed)
```

- **Postgres** (via Encore `sqldb`) is the source of truth for reads.
- **Temporal** tracks progressive accrual via line-item signals; query `accrual` on workflow `bill-{id}` for live state.
- **Worker** runs inside `encore run` on task queue `{env}-billing`.

### Bill lifecycle (process-oriented)

Each bill runs a long-lived Temporal workflow (`bill-{id}`) that tracks process phase:

| DB status | Workflow phase | Meaning |
|-----------|----------------|---------|
| `scheduled` | `waiting_period_start` | Bill created; waiting until `period_start` |
| `open` | `accruing` | Accepting line items; `accrual_total` updated |
| `closing` | `closing` | Close in progress; line items frozen |
| `closed` | (workflow completed) | Final `total_amount` set |

Line items are accepted only while status is **`open`**. Bills with a future `period_start` are created as **`scheduled`** and activated by the workflow at period start.

**Temporal queries** (while workflow is running):

```bash
temporal workflow query --workflow-id bill-{id} --name status
temporal workflow query --workflow-id bill-{id} --name accrual
```

`status` returns phase, `accrual_total`, line item count, and period dates.  
`accrual` returns the full in-memory accrual state including line item payloads.

### Close (Phase 3)

`POST /bills/:id/close` is **async by default** — returns **202 Accepted** with `{ bill_id, status: "closing" }` and runs the close workflow in the background. Poll `GET /bills/:id` until `status` is `closed`.

- **`?wait=true`** — blocks until finalization completes and returns **200** with the full `CloseBillResponse` (used by verify/load scripts).
- **`POST /bills/:id/finalize`** — recovery when a bill is stuck in `closing` with no `total_amount`.

Close workflow steps (visible in Temporal history):

1. `EnsureBillClosing` — freeze line items (`open` → `closing`)
2. `ComputeTotal` — sum line items with FX
3. `FinalizeBillTotal` — write `total_amount`, clear `accrual_total` (`closing` → `closed`)

**Auto-close:** at **00:00 UTC on the day after `period_end`**, the workflow closes the bill without an API call (single timer per bill).

## Prerequisites

```bash
brew install encoredev/tap/encore temporal jq
```

Docker Desktop must be running (Encore provisions local Postgres).

For local Temporal:

```bash
temporal server start-dev --namespace default --port 7233
```

Web UI: http://localhost:8233

## Run locally

Terminal 1 — Temporal:

```bash
temporal server start-dev --namespace default --port 7233
```

Terminal 2 — Encore:

```bash
encore run
```

API base URL: http://localhost:4000

## Verify end-to-end

With Temporal and Encore running:

```bash
chmod +x scripts/verify.sh
./scripts/verify.sh
```

Or manually:

```bash
./scripts/verify-discount.sh   # subscription + usage + discounts → correct close total
./scripts/verify-multi-currency.sh   # USD bill + GEL line items → FX-converted close total
./scripts/verify-accrual.sh          # accrual_total in DB matches Temporal status query
./scripts/verify-lifecycle.sh        # scheduled bills reject line items until period start
./scripts/verify-close-async.sh      # async close (202) then poll until closed
```

Or manually (full flow):

```bash
BILL_ID=$(curl -s -X POST http://localhost:4000/bills \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"cust_001","period_start":"2025-01-01","period_end":"2025-01-31","currency":"USD"}' \
  | jq -r '.id // .details.bill_id // .details.bill.id')

# On 409 conflict, id is in .details.bill_id (not top-level .id)
if [ -z "$BILL_ID" ] || [ "$BILL_ID" = "null" ]; then
  echo "error: could not get bill id" >&2
  exit 1
fi

curl -X POST "http://localhost:4000/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"subscription","description":"Monthly plan","quantity":"1","unit_price":"99.00","effective_date":"2025-01-01","external_reference_id":"sub-jan-2025"}'

curl -X POST "http://localhost:4000/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"usage","description":"API calls","quantity":"5000","unit_price":"0.001","effective_date":"2025-01-15","external_reference_id":"usage-jan-2025-api"}'

curl -X POST "http://localhost:4000/bills/$BILL_ID/close?wait=true" | jq
curl "http://localhost:4000/bills/$BILL_ID" | jq
```

Check workflow `bill-$BILL_ID` is **Completed** in the Temporal UI.

## Worker restart resilience

1. Create a bill and add line items (do not close).
2. Stop `encore run` (Ctrl+C).
3. Restart `encore run` — the worker re-attaches to the same task queue.
4. `POST /bills/:id/close` should still succeed; Temporal replays workflow history.

## HTTP status codes

| Code | When |
|------|------|
| 404 | Bill not found |
| 409 | Duplicate bill (create) or duplicate close |
| 422 | Line item on closed/scheduled bill; close on already-closed bill |
| 202 | Close accepted (`status: closing`) — poll GET until `closed` |
| 400 | Validation error (invalid fee_type, period, UUID, etc.) |

On duplicate bill create, the existing bill id is returned in `details.bill_id`.

## Multi-currency line items

- **Bill currency** is the invoice/settlement currency (e.g. `USD`). The close `total_amount` is always in this currency.
- **Line item currency** is optional on `POST /bills/:id/line-items`; it defaults to the bill currency but may differ (e.g. `GEL` on a `USD` bill).
- Line items keep their original amount and currency in storage and API responses.
- At close, each line is converted to the bill currency using static rates in `money/rates.go` (currently **USD ↔ GEL**).

Example: USD bill with `99 USD` subscription + `100 GEL` usage at `1 GEL = 0.37 USD` → close total **`136.00 USD`**.

Unsupported currency pairs (e.g. EUR on a USD bill) return **400** when adding the line item.

## Tests

```bash
ENCORERUNTIME_NOPANIC=1 go test ./...
encore test ./...
```

### Load / race-condition tests

With Temporal and Encore running:

```bash
./scripts/load/run-all.sh
```

See [scripts/load/README.md](scripts/load/README.md) for individual scenarios and Go integration tests:

```bash
go test -tags=integration ./tests/integration/ -run TestRace -v -count=1
```

## Project layout

```
billing/     Encore service (API, DB, Temporal worker)
workflow/    BillWorkflow definition
activity/    ComputeTotal, FinalizeBillTotal, EnsureBillClosing, UpdateAccrualTotal, ActivateBill
domain/      Pure types and errors
money/       decimal ↔ money adapter, FX conversion
docs/        Architecture and data model reference
```

See [docs/datamodels.md](docs/datamodels.md) for bill/line-item schemas, lifecycle states, and Temporal mapping.

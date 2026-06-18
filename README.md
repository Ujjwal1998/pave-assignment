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

| Phase | Meaning |
|-------|---------|
| `waiting_period_start` | Bill created; workflow waiting until `period_start` |
| `accruing` | Accepting line-item signals; running total updated |
| `closing` | Close signal received; finalizing total |

**Temporal queries** (while workflow is running):

```bash
temporal workflow query --workflow-id bill-{id} --name status
temporal workflow query --workflow-id bill-{id} --name accrual
```

`status` returns phase, `accrual_total`, line item count, and period dates.  
`accrual` returns the full in-memory accrual state including line item payloads.

**API read model:** open bills expose `accrual_total` on `GET /bills/:id`, persisted by the workflow after each line-item signal. Closed bills use `total_amount`; `accrual_total` is cleared on finalize.

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

curl -X POST "http://localhost:4000/bills/$BILL_ID/close" | jq
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
| 422 | Line item on closed bill |
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
activity/    ComputeTotal, UpdateBillClosed
domain/      Pure types and errors
money/       decimal ↔ money adapter
```

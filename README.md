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

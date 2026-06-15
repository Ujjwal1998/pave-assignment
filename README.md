# pave-assignment

Progressive accrual billing API built with [Encore](https://encore.dev) and [Temporal](https://temporal.io).

## Prerequisites

```bash
brew install encoredev/tap/encore temporal
```

For local Temporal, use either:

- **Temporal CLI dev server** (recommended; replaces deprecated [Temporalite](https://github.com/temporalio/temporalite-archived)):

  ```bash
  temporal server start-dev --namespace default --port 7233
  ```

- **Temporalite** (legacy single-binary dev server):

  ```bash
  go install github.com/temporalio/temporalite/cmd/temporalite@v0.3.0
  temporalite start --namespace default
  ```

Both expose the gRPC frontend on `127.0.0.1:7233` and the Web UI on `http://localhost:8233`.

## Run locally

Terminal 1 — Temporal:

```bash
temporal server start-dev --namespace default --port 7233
```

Terminal 2 — Encore app (applies DB migrations automatically):

```bash
encore run
```

Encore connects to Temporal using `billing/config.cue` (`127.0.0.1:7233` when `#Meta.Environment.Cloud == "local"`), following the [Temporal + Encore guide](https://encore.dev/docs/go/how-to/temporal).

## API

```bash
# 1. Create bill (starts Temporal workflow bill-<id>)
curl -X POST http://localhost:4000/bills \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"cust_001","period_start":"2025-01-01","period_end":"2025-01-31","currency":"USD"}'

# 2. Add line items
curl -X POST http://localhost:4000/bills/$BILL_ID/line-items \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"subscription","description":"Monthly plan","quantity":"1","unit_price":"99.00","effective_date":"2025-01-01","external_reference_id":"sub-jan-2025"}'

curl -X POST http://localhost:4000/bills/$BILL_ID/line-items \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"usage","description":"API calls","quantity":"5000","unit_price":"0.001","effective_date":"2025-01-15","external_reference_id":"usage-jan-2025-api"}'

# 3. Close bill (expected total: 104.00)
curl -X POST http://localhost:4000/bills/$BILL_ID/close

# 4. Get closed bill with line items
curl http://localhost:4000/bills/$BILL_ID
```

Temporal Web UI: http://localhost:8233 — workflow `bill-$BILL_ID` should show **Completed** after close.

## Tests

```bash
go test ./...
encore test ./...
```

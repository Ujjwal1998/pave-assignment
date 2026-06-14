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

## API (implemented so far)

```bash
# Create bill
curl -X POST http://localhost:4000/bills \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"cust_001","period_start":"2025-01-01","period_end":"2025-01-31","currency":"USD"}'

# Get bill
curl http://localhost:4000/bills/$BILL_ID
```

## Tests

```bash
go test ./...
encore test ./...
```

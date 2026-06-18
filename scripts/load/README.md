# Load and race-condition tests

Stress and concurrency tests for the billing API. Requires **Temporal** and **Encore** running locally.

## Prerequisites

```bash
# Terminal 1
temporal server start-dev --namespace default --port 7233

# Terminal 2
encore run

# Tools
brew install jq   # curl is built-in on macOS
```

## Quick start

```bash
chmod +x scripts/load/*.sh
./scripts/load/run-all.sh
```

Run a single scenario:

```bash
./scripts/load/race-duplicate-line-item.sh
CONCURRENCY=200 ./scripts/load/race-concurrent-line-items.sh
```

## Scenarios

| Script | Race IDs | What it tests |
|--------|----------|---------------|
| `race-duplicate-bill.sh` | R1, R2 | N concurrent `POST /bills` with same body → 1 bill |
| `race-concurrent-line-items.sh` | R5 | N unique `external_reference_id`s → N rows, correct sum |
| `race-duplicate-line-item.sh` | R4, R13 | N identical line-item POSTs → 1 row, same `id` |
| `race-add-after-close.sh` | R6 | N adds after close → all 422, count unchanged |
| `race-duplicate-close.sh` | R8 | N concurrent closes → 1 success, correct total |
| `race-close-vs-add.sh` | R6, R7, R10 | Close while adds in flight → closed total = DB sum |

## Audit helpers

After any scenario:

```bash
./scripts/load/audit.sh <bill_id> [expected_count] [expected_total]
```

SQL audit (requires `encore db shell billing`):

```sql
\set bill_id '''your-bill-uuid'''
\i scripts/load/audit.sql
```

## Go integration tests (barrier-synchronized)

Precise races with a `sync.WaitGroup` barrier:

```bash
# encore + temporal must be running
go test -tags=integration ./tests/integration/ -run TestRace -v -count=1
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:4000` | API base URL |
| `CONCURRENCY` | varies per script | Parallel workers |
| `SKIP_SLOW=1` | — | Skip `race-close-vs-add.sh` in `run-all.sh` |
| `SKIP_WORKFLOW_CLEANUP=1` | — | Leave workflows running (not recommended) |
| `CUSTOMER_ID` | auto-generated | Override customer id |

## Workflow cleanup

Each scenario registers bills it creates. On exit (pass or fail), scripts call `POST /close` for any still-open bill so Temporal workflows move to **Completed**.

```bash
# Auto cleanup is on by default; disable with:
SKIP_WORKFLOW_CLEANUP=1 ./scripts/load/race-concurrent-line-items.sh

# Manually close orphans (from Temporal UI workflow id bill-{uuid}):
./scripts/load/cleanup-workflows.sh 7f616f56-93ad-487f-9a12-29adb6d70130
./scripts/load/cleanup-workflows.sh bill-7f616f56-93ad-487f-9a12-29adb6d70130
```

## Interpreting failures

- **Duplicate bill: multiple UUIDs** — unique constraint or 409 handling broken
- **Duplicate ref: count > 1** — idempotency race
- **Add after close: 2xx** — closed-bill guard or DB trigger missing
- **Close total ≠ sum** — close-time consistency bug
- **D1: ok_adds ≠ db_count** — line item inserted but not counted, or ghost rejection

See the stress test plan in project docs for full race inventory (R1–R13).

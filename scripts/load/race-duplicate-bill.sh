#!/usr/bin/env bash
# Scenario A: concurrent duplicate bill create (R1, R2)
# Expect exactly one bill; rest return 409 with same bill_id.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CONCURRENCY="${CONCURRENCY:-50}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-bill-$(date +%s)}"
PERIOD_START="${PERIOD_START:-2025-05-01}"
PERIOD_END="${PERIOD_END:-2025-05-31}"
CURRENCY="${CURRENCY:-USD}"

BODY=$(jq -n \
  --arg c "$CUSTOMER_ID" \
  --arg ps "$PERIOD_START" \
  --arg pe "$PERIOD_END" \
  --arg cur "$CURRENCY" \
  '{customer_id:$c, period_start:$ps, period_end:$pe, currency:$cur}')

echo "==> Scenario A: duplicate bill create (concurrency=$CONCURRENCY)"
echo "    customer_id=$CUSTOMER_ID"

_outfile() { echo "$LOAD_TMPDIR/resp-$1.json"; }

_race_create() {
  local i="$1"
  curl -s -X POST "$BASE_URL/bills" \
    -H 'Content-Type: application/json' \
    -d "$BODY" > "$(_outfile "$i")"
}

for ((i = 0; i < CONCURRENCY; i++)); do
  _race_create "$i" &
done
wait

CREATED=0
CONFLICT=0

for ((i = 0; i < CONCURRENCY; i++)); do
  f="$(_outfile "$i")"
  code=$(jq -r '.code // "ok"' < "$f")
  id=$(jq -r '.id // .details.bill_id // .details.bill.id // empty' < "$f")
  if [[ "$code" == "already_exists" ]]; then
    ((CONFLICT++)) || true
  elif [[ -n "$id" ]]; then
    ((CREATED++)) || true
  else
    echo "unexpected response in $f:" >&2
    cat "$f" >&2
    load_fail "unexpected create response"
  fi
done

UNIQUE_BILLS=$(jq -r '.id // .details.bill_id // .details.bill.id // empty' "$LOAD_TMPDIR"/resp-*.json | sort -u | grep -c . || true)
echo "    created_responses=$CREATED conflict_responses=$CONFLICT unique_bill_ids=$UNIQUE_BILLS"

if [[ "$UNIQUE_BILLS" -ne 1 ]]; then
  load_fail "expected exactly 1 unique bill id, got $UNIQUE_BILLS"
fi

if [[ "$CONFLICT" -lt 1 && "$CONCURRENCY" -gt 1 ]]; then
  echo "WARN: expected at least one 409 conflict (got $CONFLICT); race may not have overlapped" >&2
fi

BILL_ID=$(jq -r '.id // .details.bill_id // .details.bill.id // empty' "$LOAD_TMPDIR"/resp-0.json)
echo "    bill_id=$BILL_ID"
load_track_bill "$BILL_ID"
load_pass "Scenario A"

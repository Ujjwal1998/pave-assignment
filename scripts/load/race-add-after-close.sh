#!/usr/bin/env bash
# Scenario D2: add line items after bill is closed (R6)
# Expect all adds to return 422/400 and zero new rows.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CONCURRENCY="${CONCURRENCY:-50}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-after-close-$(date +%s)}"

echo "==> Scenario D2: add after close (concurrency=$CONCURRENCY)"
load_log_responses_dir

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "2025-05-01" "2025-05-31" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

SETUP_ADD=$(load_add_line_item "$BILL_ID" "only-item" "10.00" "2025-05-01")
load_log_json_response "setup add" "$SETUP_ADD"
SETUP_CLOSE=$(load_close_bill "$BILL_ID")
load_log_json_response "setup close" "$SETUP_CLOSE"

COUNT_BEFORE=$(load_bill_line_item_count "$BILL_ID")
echo "    line_items_before=$COUNT_BEFORE"

_reject_add() {
  local i="$1"
  local body
  body=$(load_http_json_with_status POST "/bills/$BILL_ID/line-items" "$(jq -n \
    --arg ref "after-close-$i" \
    --arg date "2025-05-05" \
    '{
      fee_type: "usage",
      description: "load test",
      quantity: "1",
      unit_price: "99.00",
      effective_date: $date,
      external_reference_id: $ref
    }')")
  echo "$(load_last_http_code)" > "$LOAD_TMPDIR/status-$i"
  echo "$body" > "$LOAD_TMPDIR/body-$i.json"
}

for ((i = 0; i < CONCURRENCY; i++)); do
  _reject_add "$i" &
done
wait

REJECTED=0
for ((i = 0; i < CONCURRENCY; i++)); do
  code=$(cat "$LOAD_TMPDIR/status-$i")
  if [[ "${LOAD_VERBOSE:-}" == "1" || "$i" -eq 0 || "$i" -eq $((CONCURRENCY - 1)) ]]; then
    echo "    [reject add req-$i] http=$code"
    load_log_json_response "reject add req-$i" "$(cat "$LOAD_TMPDIR/body-$i.json")"
  fi
  if [[ "$code" == "400" || "$code" == "422" ]]; then
    ((REJECTED++)) || true
  else
    load_fail "expected 422 for add on closed bill, got $code"
  fi
done

if [[ "${LOAD_VERBOSE:-}" != "1" && "$CONCURRENCY" -gt 2 ]]; then
  echo "    ... ($((CONCURRENCY - 2)) more reject responses in $LOAD_TMPDIR/body-*.json)"
fi

COUNT_AFTER=$(load_bill_line_item_count "$BILL_ID")
echo "    rejected=$REJECTED line_items_after=$COUNT_AFTER"

load_assert_eq "$REJECTED" "$CONCURRENCY" "not all adds were rejected"
load_assert_eq "$COUNT_AFTER" "$COUNT_BEFORE" "line item count changed after close"

load_pass "Scenario D2"

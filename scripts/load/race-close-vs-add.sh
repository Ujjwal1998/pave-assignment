#!/usr/bin/env bash
# Scenario D1: close vs concurrent line item adds (R6, R7, R10)
# Fire N line-item writers and 1 close concurrently.
# Invariants: bill ends closed; closed total matches sum of persisted line items.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

ADDS="${ADDS:-40}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-close-add-$(date +%s)}"
RUN_ID="${RUN_ID:-$(date +%s)}"

echo "==> Scenario D1: close vs concurrent adds (adds=$ADDS)"
load_log_responses_dir

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "2025-05-01" "2025-05-31" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

_race_add() {
  local i="$1"
  local ref="race-${RUN_ID}-${i}"
  local body id
  body=$(load_add_line_item "$BILL_ID" "$ref" "2.50" "2025-05-08" 2>/dev/null || echo '{}')
  echo "$body" > "$LOAD_TMPDIR/add-body-$i.json"
  id=$(echo "$body" | jq -r '.id // empty')
  if [[ -n "$id" ]]; then
    echo "ok $ref $id" > "$LOAD_TMPDIR/add-$i"
  else
    echo "fail $ref" > "$LOAD_TMPDIR/add-$i"
  fi
}

_race_close() {
  local body
  body=$(load_http_json_with_status POST "/bills/$BILL_ID/close?wait=true")
  echo "$(load_last_http_code)" > "$LOAD_TMPDIR/close-status"
  echo "$body" > "$LOAD_TMPDIR/close-body.json"
}

for ((i = 0; i < ADDS; i++)); do
  _race_add "$i" &
done
_race_close &
wait

CLOSE_CODE=$(cat "$LOAD_TMPDIR/close-status")
echo "    close_http=$CLOSE_CODE"
load_log_json_response "close response" "$(cat "$LOAD_TMPDIR/close-body.json")"

if [[ "$CLOSE_CODE" != "200" ]]; then
  load_fail "close did not succeed (code=$CLOSE_CODE)"
fi

OK_ADDS=0
REJECTED_ADDS=0
for ((i = 0; i < ADDS; i++)); do
  line=$(cat "$LOAD_TMPDIR/add-$i")
  if [[ "${LOAD_VERBOSE:-}" == "1" || "$i" -eq 0 || "$i" -eq 1 || "$i" -eq $((ADDS - 1)) ]]; then
    if [[ "$line" == ok* ]]; then
      load_log_json_response "add req-$i" "$(cat "$LOAD_TMPDIR/add-body-$i.json")"
    else
      load_log_json_response "rejected add req-$i" "$(cat "$LOAD_TMPDIR/add-body-$i.json")"
    fi
  fi
  if [[ "$line" == ok* ]]; then
    ((OK_ADDS++)) || true
  else
    ((REJECTED_ADDS++)) || true
  fi
done

if [[ "${LOAD_VERBOSE:-}" != "1" && "$ADDS" -gt 3 ]]; then
  echo "    ... ($((ADDS - 3)) more add responses in $LOAD_TMPDIR/add-body-*.json)"
fi

BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "final bill" "$BILL"
STATUS=$(echo "$BILL" | jq -r '.status')
COUNT=$(echo "$BILL" | jq '.line_items | length')
SUM=$(echo "$BILL" | jq '[.line_items[].total_amount | tonumber] | add // 0')
CLOSED_TOTAL=$(echo "$BILL" | jq -r '.total_amount')

echo "    status=$STATUS ok_adds=$OK_ADDS rejected_adds=$REJECTED_ADDS db_count=$COUNT db_sum=$SUM closed_total=$CLOSED_TOTAL"
echo "    note: rejected_adds > 0 means the immediate DB freeze blocked concurrent inserts (expected)"

load_assert_eq "$STATUS" "closed" "bill must be closed"

MATCH=$(jq -n --arg closed "$CLOSED_TOTAL" --argjson sum "$SUM" '
  ($closed | tostring | gsub("[^0-9.-]";"") | tonumber) == ($sum | tostring | gsub("[^0-9.-]";"") | tonumber)
')
if [[ "$MATCH" != "true" ]]; then
  load_fail "closed total ($CLOSED_TOTAL) != sum of line items ($SUM)"
fi

load_assert_eq "$COUNT" "$OK_ADDS" "DB count must match successful adds (no ghost or missing rows)"

"$SCRIPT_DIR/audit.sh" "$BILL_ID"
load_pass "Scenario D1"

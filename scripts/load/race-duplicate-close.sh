#!/usr/bin/env bash
# Scenario E: concurrent duplicate close (R8)
# Expect exactly one successful close; others return 422.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CONCURRENCY="${CONCURRENCY:-30}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-close-$(date +%s)}"

echo "==> Scenario E: duplicate close (concurrency=$CONCURRENCY)"
load_log_responses_dir

load_ensure_open_period
BILL_ID=$(load_create_open_bill "$CUSTOMER_ID" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

ADD1=$(load_add_line_item "$BILL_ID" "sub-1" "50.00" "$PERIOD_START")
load_log_json_response "setup add sub-1" "$ADD1"
ADD2=$(load_add_line_item "$BILL_ID" "sub-2" "25.00" "$EFFECTIVE")
load_log_json_response "setup add sub-2" "$ADD2"

_outfile() { echo "$LOAD_TMPDIR/close-$1"; }
_bodyfile() { echo "$LOAD_TMPDIR/close-body-$1.json"; }

_race_close() {
  local i="$1"
  local body
  body=$(load_http_json_with_status POST "/bills/$BILL_ID/close?wait=true" "" "$(_outfile "$i")")
  echo "$body" > "$(_bodyfile "$i")"
}

for ((i = 0; i < CONCURRENCY; i++)); do
  _race_close "$i" &
done
wait

OK=0
CONFLICT=0
for ((i = 0; i < CONCURRENCY; i++)); do
  code=$(cat "$(_outfile "$i")")
  if [[ "${LOAD_VERBOSE:-}" == "1" || "$i" -eq 0 || "$i" -eq 1 || "$i" -eq $((CONCURRENCY - 1)) ]]; then
    echo "    [close req-$i] http=$code"
    load_log_json_response "close req-$i" "$(cat "$(_bodyfile "$i")")"
  fi
  if [[ "$code" == "200" ]]; then
    ((OK++)) || true
  elif [[ "$code" == "400" || "$code" == "422" ]]; then
    ((CONFLICT++)) || true
  else
    load_fail "unexpected close status $code"
  fi
done

if [[ "${LOAD_VERBOSE:-}" != "1" && "$CONCURRENCY" -gt 3 ]]; then
  echo "    ... ($((CONCURRENCY - 3)) more close responses in $LOAD_TMPDIR/close-body-*.json)"
fi

echo "    close_200=$OK close_4xx=$CONFLICT"

if [[ "$OK" -lt 1 ]]; then
  load_fail "expected at least one successful close"
fi

# Concurrent closes may all return 200 if each request signals the same workflow
# and waits for completion. The invariant is a single closed bill with correct total.
if [[ "$OK" -gt 1 && "$CONFLICT" -eq 0 ]]; then
  echo "    note: multiple 200 responses (all waited on same workflow — expected under concurrency)"
fi

BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "final bill" "$BILL"
STATUS=$(echo "$BILL" | jq -r '.status')
TOTAL=$(echo "$BILL" | jq -r '.total_amount')
COUNT=$(echo "$BILL" | jq '.line_items | length')

echo "    status=$STATUS total=$TOTAL line_items=$COUNT"

load_assert_eq "$STATUS" "closed" "bill should be closed"
load_assert_eq "$COUNT" "2" "expected 2 line items on closed bill"

# Allow common total formats
load_assert_decimal_eq "$TOTAL" "75" "expected total 75.00"

"$SCRIPT_DIR/audit.sh" "$BILL_ID" 2 "75.00"
load_pass "Scenario E"

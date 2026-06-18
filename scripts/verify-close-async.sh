#!/usr/bin/env bash
# Verify async close (HTTP 202) and poll until the bill is closed.
#
# Usage (Temporal + encore run required):
#   ./scripts/verify-close-async.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CUSTOMER_ID="${CUSTOMER_ID:-cust-close-async-$(date +%s)}"
if [[ -z "${PERIOD_START:-}" ]]; then
  load_set_open_period
fi
PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"

echo "==> Async close test for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"

SUB_RESP=$(load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE")
load_log_json_response "add subscription" "$SUB_RESP"

USAGE_RESP=$(load_add_fee "$BILL_ID" usage "API calls" "usage-jun" "5000" "0.001" "$EFFECTIVE")
load_log_json_response "add usage" "$USAGE_RESP"

echo "==> POST /close (async, expect 202)"
ACCEPT_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/close")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [async close] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "async close accepted" "$ACCEPT_BODY"
load_assert_eq "$LOAD_LAST_HTTP_CODE" "202" "async close should return 202 Accepted"

ACCEPT_STATUS=$(echo "$ACCEPT_BODY" | jq -r '.status')
load_assert_eq "$ACCEPT_STATUS" "closing" "accepted response should show closing"

echo "==> Poll until closed"
load_wait_for_closed "$BILL_ID" || load_fail "bill did not reach closed status"

FINAL_BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "get bill (closed)" "$FINAL_BILL"
FINAL_STATUS=$(echo "$FINAL_BILL" | jq -r '.status')
TOTAL=$(echo "$FINAL_BILL" | jq -r '.total_amount')

load_assert_eq "$FINAL_STATUS" "closed" "bill should be closed after workflow completes"
load_assert_decimal_eq "$TOTAL" "104" "closed total should be 104"

echo "==> Verify closing state visible before poll (short-period bill)"
SHORT_ID=$(load_create_bill "${CUSTOMER_ID}-short" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$SHORT_ID"
load_add_fee "$SHORT_ID" subscription "Plan" "sub-short" "1" "50.00" "$EFFECTIVE" >/dev/null

SHORT_ACCEPT=$(load_http_json_with_status POST "/bills/$SHORT_ID/close")
SHORT_CODE=$(load_last_http_code)
load_assert_eq "$SHORT_CODE" "202" "second async close should return 202"

CLOSING_BILL=$(load_get_bill "$SHORT_ID")
CLOSING_STATUS=$(echo "$CLOSING_BILL" | jq -r '.status')
echo "    [immediate GET after 202] status=$CLOSING_STATUS"
if [[ "$CLOSING_STATUS" != "closing" && "$CLOSING_STATUS" != "closed" ]]; then
  load_fail "expected closing or closed immediately after async close, got $CLOSING_STATUS"
fi

load_wait_for_closed "$SHORT_ID" || load_fail "short bill did not close"
load_pass "async close"

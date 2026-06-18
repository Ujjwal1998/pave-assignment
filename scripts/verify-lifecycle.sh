#!/usr/bin/env bash
# Verify scheduled bills reject accrual until the billing period starts.
#
# Usage (Temporal + encore run required):
#   ./scripts/verify-lifecycle.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CUSTOMER_ID="${CUSTOMER_ID:-cust-lifecycle-$(date +%s)}"
FUTURE_START="${FUTURE_START:-2099-01-01}"
FUTURE_END="${FUTURE_END:-2099-01-31}"
EFFECTIVE="${EFFECTIVE:-2099-01-15}"

echo "==> Lifecycle test for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$FUTURE_START" "$FUTURE_END" "USD")
load_track_bill "$BILL_ID"

BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "get bill (scheduled)" "$BILL"
STATUS=$(echo "$BILL" | jq -r '.status')
load_assert_eq "$STATUS" "scheduled" "future bill should start as scheduled"

echo "==> Reject line item on scheduled bill"
ADD_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/line-items" "$(jq -n \
  --arg date "$EFFECTIVE" \
  '{
    fee_type: "subscription",
    description: "Future plan",
    quantity: "1",
    unit_price: "99.00",
    effective_date: $date,
    external_reference_id: "sub-future"
  }')")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [add on scheduled bill] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "reject line item" "$ADD_BODY"
load_assert_eq "$LOAD_LAST_HTTP_CODE" "422" "scheduled bill should reject line items"

echo "==> Reject close on scheduled bill"
CLOSE_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/close")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [close scheduled bill] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "reject close" "$CLOSE_BODY"
load_assert_eq "$LOAD_LAST_HTTP_CODE" "422" "scheduled bill should reject close"

if command -v temporal >/dev/null 2>&1; then
  echo "==> Query Temporal status (scheduled / waiting)"
  STATUS_JSON=$(temporal workflow query \
    --namespace "${TEMPORAL_NAMESPACE:-default}" \
    --workflow-id "bill-$BILL_ID" \
    --name status \
    --output json 2>/dev/null || true)
  if [[ -n "$STATUS_JSON" ]]; then
    QUERY_PHASE=$(echo "$STATUS_JSON" | jq -r '
      if .queryResult then
        (if (.queryResult | type) == "array" then .queryResult[0].phase else .queryResult.phase end)
      else .phase end // empty
    ')
    echo "    [temporal status] workflow_phase=$QUERY_PHASE"
    load_assert_eq "$QUERY_PHASE" "waiting_period_start" "workflow should wait for period start"
  fi
fi

load_pass "bill lifecycle (scheduled)"

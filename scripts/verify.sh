#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools

BASE_URL="${BASE_URL:-http://localhost:4000}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-verify-$(date +%s)}"

if [[ -z "${PERIOD_START:-}" ]]; then
  load_set_open_period
fi
PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"

echo "==> Creating bill for $CUSTOMER_ID (period $PERIOD_START .. $PERIOD_END)"
CREATE_RESP=$(curl -s -X POST "$BASE_URL/bills" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n \
    --arg c "$CUSTOMER_ID" \
    --arg ps "$PERIOD_START" \
    --arg pe "$PERIOD_END" \
    '{customer_id:$c, period_start:$ps, period_end:$pe, currency:"USD"}')")
load_log_json_response "create bill" "$CREATE_RESP"

BILL_ID=$(echo "$CREATE_RESP" | jq -r '.id // .details.bill_id // .details.bill.id')
if [ -z "$BILL_ID" ] || [ "$BILL_ID" = "null" ]; then
  echo "failed to create bill:" >&2
  echo "$CREATE_RESP" | jq . >&2
  exit 1
fi

echo "==> Adding line items"
SUB_RESP=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n \
    --arg date "$PERIOD_START" \
    '{fee_type:"subscription",description:"Monthly plan",quantity:"1",unit_price:"99.00",effective_date:$date,external_reference_id:"sub-verify"}')")
load_log_json_response "add subscription" "$SUB_RESP"

USAGE_RESP=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n \
    --arg date "$EFFECTIVE" \
    '{fee_type:"usage",description:"API calls",quantity:"5000",unit_price:"0.001",effective_date:$date,external_reference_id:"usage-verify"}')")
load_log_json_response "add usage" "$USAGE_RESP"

echo "==> Idempotency check (resend first line item)"
DUP_RESP=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n \
    --arg date "$PERIOD_START" \
    '{fee_type:"subscription",description:"Monthly plan",quantity:"1",unit_price:"99.00",effective_date:$date,external_reference_id:"sub-verify"}')")
load_log_json_response "idempotent resend" "$DUP_RESP"

echo "==> Closing bill"
CLOSE_RESP=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/close")
load_log_json_response "close bill" "$CLOSE_RESP"
TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')

if [ "$TOTAL" != "104" ] && [ "$TOTAL" != "104.00" ] && [ "$TOTAL" != "104.000000000" ]; then
  echo "unexpected total: $TOTAL (want 104.00)" >&2
  echo "$CLOSE_RESP" | jq . >&2
  exit 1
fi

echo "==> Verify closed bill rejects new line items"
REJECT_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/line-items" "$(jq -n \
  --arg date "$EFFECTIVE" \
  '{fee_type:"tax",description:"Late fee",quantity:"1",unit_price:"1.00",effective_date:$date,external_reference_id:"tax-verify"}')")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [reject line item] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "reject line item" "$REJECT_BODY"
if [ "$LOAD_LAST_HTTP_CODE" != "400" ] && [ "$LOAD_LAST_HTTP_CODE" != "422" ]; then
  echo "expected 422 for line item on closed bill, got $LOAD_LAST_HTTP_CODE" >&2
  exit 1
fi

echo "==> Verify duplicate close returns 422"
DUP_CLOSE_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/close")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [duplicate close] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "duplicate close" "$DUP_CLOSE_BODY"
if [ "$LOAD_LAST_HTTP_CODE" != "422" ]; then
  echo "expected 422 for duplicate close, got $LOAD_LAST_HTTP_CODE" >&2
  exit 1
fi

echo "==> All checks passed"

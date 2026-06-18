#!/usr/bin/env bash
# End-to-end multi-currency line items on a single bill.
#
# A USD bill can include line items in GEL; close total is in bill currency
# using static FX rates (100 GEL * 0.37 = 37 USD).
#
# Usage (Temporal + encore run required):
#   ./scripts/verify-multi-currency.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CUSTOMER_ID="${CUSTOMER_ID:-cust-mc-$(date +%s)}"
if [[ -z "${PERIOD_START:-}" ]]; then
  load_set_open_period
fi
PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"

echo "==> Multi-currency line items for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"

echo "==> Add USD subscription line"
USD_RESP=$(load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE" "USD")
load_log_json_response "add USD line" "$USD_RESP"

echo "==> Add GEL usage line on USD bill"
GEL_RESP=$(load_add_fee "$BILL_ID" usage "Local usage" "usage-gel" "1" "100.00" "$EFFECTIVE" "GEL")
load_log_json_response "add GEL line" "$GEL_RESP"
GEL_CURRENCY=$(echo "$GEL_RESP" | jq -r '.currency')
GEL_TOTAL=$(echo "$GEL_RESP" | jq -r '.total_amount')

load_assert_eq "$GEL_CURRENCY" "GEL" "line item should keep original GEL currency"
load_assert_decimal_eq "$GEL_TOTAL" "100" "GEL line total should be 100.00"

echo "==> Reject unsupported currency pair (EUR on USD bill)"
EUR_BODY=$(load_http_json_with_status POST "/bills/$BILL_ID/line-items" "$(jq -n \
  --arg date "$EFFECTIVE" \
  '{
    fee_type: "usage",
    description: "EUR fee",
    quantity: "1",
    unit_price: "10.00",
    currency: "EUR",
    effective_date: $date,
    external_reference_id: "usage-eur"
  }')")
LOAD_LAST_HTTP_CODE=$(load_last_http_code)
echo "    [reject EUR line] http=$LOAD_LAST_HTTP_CODE"
load_log_json_response "reject EUR line" "$EUR_BODY"
load_assert_eq "$LOAD_LAST_HTTP_CODE" "400" "EUR line on USD bill should be rejected"

echo "==> Close bill"
CLOSE_RESP=$(load_close_bill "$BILL_ID")
load_log_json_response "close bill" "$CLOSE_RESP"
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
CLOSE_CURRENCY=$(echo "$CLOSE_RESP" | jq -r '.currency')
LINE_COUNT=$(echo "$CLOSE_RESP" | jq '.line_items | length')

load_assert_eq "$CLOSE_CURRENCY" "USD" "close total currency should be bill currency"
load_assert_eq "$LINE_COUNT" "2" "expected 2 line items"
load_assert_decimal_eq "$CLOSE_TOTAL" "136" "99 USD + 100 GEL*0.37 = 136 USD"

load_pass "multi-currency line items"

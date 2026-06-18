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
PERIOD_START="${PERIOD_START:-2025-06-01}"
PERIOD_END="${PERIOD_END:-2025-06-30}"
EFFECTIVE="${EFFECTIVE:-2025-06-15}"

echo "==> Multi-currency line items for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

echo "==> Add USD subscription line"
load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE" "USD" >/dev/null

echo "==> Add GEL usage line on USD bill"
GEL_RESP=$(load_add_fee "$BILL_ID" usage "Local usage" "usage-gel" "1" "100.00" "$EFFECTIVE" "GEL")
GEL_CURRENCY=$(echo "$GEL_RESP" | jq -r '.currency')
GEL_TOTAL=$(echo "$GEL_RESP" | jq -r '.total_amount')

echo "    gel line currency=$GEL_CURRENCY total_amount=$GEL_TOTAL"
load_assert_eq "$GEL_CURRENCY" "GEL" "line item should keep original GEL currency"
load_assert_decimal_eq "$GEL_TOTAL" "100" "GEL line total should be 100.00"

echo "==> Reject unsupported currency pair (EUR on USD bill)"
EUR_CODE=$(load_http_code POST "/bills/$BILL_ID/line-items" "$(jq -n \
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
load_assert_eq "$EUR_CODE" "400" "EUR line on USD bill should be rejected"

echo "==> Close bill"
CLOSE_RESP=$(load_close_bill "$BILL_ID")
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
CLOSE_CURRENCY=$(echo "$CLOSE_RESP" | jq -r '.currency')
LINE_COUNT=$(echo "$CLOSE_RESP" | jq '.line_items | length')

echo "    total_amount=$CLOSE_TOTAL currency=$CLOSE_CURRENCY line_items=$LINE_COUNT"
load_assert_eq "$CLOSE_CURRENCY" "USD" "close total currency should be bill currency"
load_assert_eq "$LINE_COUNT" "2" "expected 2 line items"
load_assert_decimal_eq "$CLOSE_TOTAL" "136" "99 USD + 100 GEL*0.37 = 136 USD"

load_pass "multi-currency line items"

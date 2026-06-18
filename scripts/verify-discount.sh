#!/usr/bin/env bash
# End-to-end discount logic verification.
#
# Discounts use fee_type=discount with a negative unit_price (quantity stays > 0).
# The close total should be the algebraic sum of all line items.
#
# Usage (Temporal + encore run required):
#   ./scripts/verify-discount.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CUSTOMER_ID="${CUSTOMER_ID:-cust-discount-$(date +%s)}"
if [[ -z "${PERIOD_START:-}" ]]; then
  load_set_open_period
fi
PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"

echo "==> Discount logic test for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"

echo "==> Add charges"
SUB_RESP=$(load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE")
load_log_json_response "add subscription" "$SUB_RESP"

USAGE_RESP=$(load_add_fee "$BILL_ID" usage "API calls" "usage-jun" "5000" "0.001" "$EFFECTIVE")
load_log_json_response "add usage" "$USAGE_RESP"

echo "==> Add discount (fee_type=discount, unit_price=-10.00)"
DISC_RESP=$(load_add_fee "$BILL_ID" discount "Promo code SAVE10" "promo-jun" "1" "-10.00" "$EFFECTIVE")
load_log_json_response "add discount" "$DISC_RESP"
DISC_TOTAL=$(echo "$DISC_RESP" | jq -r '.total_amount')
DISC_TYPE=$(echo "$DISC_RESP" | jq -r '.fee_type')

if [[ "$DISC_TYPE" != "discount" ]]; then
  load_fail "expected fee_type discount, got $DISC_TYPE"
fi
load_assert_decimal_eq "$DISC_TOTAL" "-10" "discount line item total should be -10"

echo "==> Add second discount (quantity x unit_price = 2 x -5 = -10)"
DISC2_RESP=$(load_add_fee "$BILL_ID" discount "Loyalty credit" "loyalty-jun" "2" "-5.00" "$EFFECTIVE")
load_log_json_response "add second discount" "$DISC2_RESP"

OPEN_BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "get bill (open)" "$OPEN_BILL"
OPEN_SUM=$(echo "$OPEN_BILL" | jq '[.line_items[].total_amount | tonumber] | add // 0')
echo "    open bill line_items sum=$OPEN_SUM (expect 99+5-10-10=84)"
load_assert_decimal_eq "$OPEN_SUM" "84" "open bill sum before close"

echo "==> Close bill"
CLOSE_RESP=$(load_close_bill "$BILL_ID")
load_log_json_response "close bill" "$CLOSE_RESP"
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
LINE_COUNT=$(echo "$CLOSE_RESP" | jq '.line_items | length')
DISCOUNT_COUNT=$(echo "$CLOSE_RESP" | jq '[.line_items[] | select(.fee_type == "discount")] | length')

load_assert_eq "$LINE_COUNT" "4" "expected 4 line items on close"
load_assert_eq "$DISCOUNT_COUNT" "2" "expected 2 discount line items"
load_assert_decimal_eq "$CLOSE_TOTAL" "84" "closed total should be 84.00"

echo "==> Verify discount line items on closed bill"
NEG_TOTALS=$(echo "$CLOSE_RESP" | jq '[.line_items[] | select(.fee_type == "discount") | .total_amount | tonumber] | add')
load_assert_decimal_eq "$NEG_TOTALS" "-20" "discount line items should sum to -20"

echo "==> GEL bill with discount"
GEL_BILL=$(load_create_bill "${CUSTOMER_ID}-gel" "$PERIOD_START" "$PERIOD_END" "GEL")
load_track_bill "$GEL_BILL"
GEL_SUB=$(load_add_fee "$GEL_BILL" subscription "GEL plan" "gel-sub" "1" "50.00" "$EFFECTIVE")
load_log_json_response "add GEL subscription" "$GEL_SUB"
GEL_DISC=$(load_add_fee "$GEL_BILL" discount "GEL promo" "gel-promo" "1" "-15.00" "$EFFECTIVE")
load_log_json_response "add GEL discount" "$GEL_DISC"
GEL_CLOSE=$(load_close_bill "$GEL_BILL")
load_log_json_response "close GEL bill" "$GEL_CLOSE"
GEL_TOTAL=$(echo "$GEL_CLOSE" | jq -r '.total_amount')
load_assert_decimal_eq "$GEL_TOTAL" "35" "GEL closed total should be 35.00"

load_pass "discount logic"

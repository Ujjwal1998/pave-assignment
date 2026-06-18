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
PERIOD_START="${PERIOD_START:-2025-06-01}"
PERIOD_END="${PERIOD_END:-2025-06-30}"
EFFECTIVE="${EFFECTIVE:-2025-06-15}"

echo "==> Discount logic test for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

echo "==> Add charges"
load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE" >/dev/null
load_add_fee "$BILL_ID" usage "API calls" "usage-jun" "5000" "0.001" "$EFFECTIVE" >/dev/null

echo "==> Add discount (fee_type=discount, unit_price=-10.00)"
DISC_RESP=$(load_add_fee "$BILL_ID" discount "Promo code SAVE10" "promo-jun" "1" "-10.00" "$EFFECTIVE")
DISC_TOTAL=$(echo "$DISC_RESP" | jq -r '.total_amount')
DISC_TYPE=$(echo "$DISC_RESP" | jq -r '.fee_type')

echo "    discount fee_type=$DISC_TYPE total_amount=$DISC_TOTAL"

if [[ "$DISC_TYPE" != "discount" ]]; then
  load_fail "expected fee_type discount, got $DISC_TYPE"
fi
load_assert_decimal_eq "$DISC_TOTAL" "-10" "discount line item total should be -10"

echo "==> Add second discount (quantity x unit_price = 2 x -5 = -10)"
load_add_fee "$BILL_ID" discount "Loyalty credit" "loyalty-jun" "2" "-5.00" "$EFFECTIVE" >/dev/null

OPEN_SUM=$(load_bill_sum_total "$BILL_ID")
echo "    open bill line_items sum=$OPEN_SUM (expect 99+5-10-10=84)"
load_assert_decimal_eq "$OPEN_SUM" "84" "open bill sum before close"

echo "==> Close bill"
CLOSE_RESP=$(load_close_bill "$BILL_ID")
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
LINE_COUNT=$(echo "$CLOSE_RESP" | jq '.line_items | length')
DISCOUNT_COUNT=$(echo "$CLOSE_RESP" | jq '[.line_items[] | select(.fee_type == "discount")] | length')

echo "    total_amount=$CLOSE_TOTAL line_item_count=$LINE_COUNT discount_items=$DISCOUNT_COUNT"

load_assert_eq "$LINE_COUNT" "4" "expected 4 line items on close"
load_assert_eq "$DISCOUNT_COUNT" "2" "expected 2 discount line items"
load_assert_decimal_eq "$CLOSE_TOTAL" "84" "closed total should be 84.00"

echo "==> Verify discount line items on closed bill"
NEG_TOTALS=$(echo "$CLOSE_RESP" | jq '[.line_items[] | select(.fee_type == "discount") | .total_amount | tonumber] | add')
load_assert_decimal_eq "$NEG_TOTALS" "-20" "discount line items should sum to -20"

echo "==> GEL bill with discount"
GEL_BILL=$(load_create_bill "${CUSTOMER_ID}-gel" "$PERIOD_START" "$PERIOD_END" "GEL")
load_track_bill "$GEL_BILL"
load_add_fee "$GEL_BILL" subscription "GEL plan" "gel-sub" "1" "50.00" "$EFFECTIVE" >/dev/null
load_add_fee "$GEL_BILL" discount "GEL promo" "gel-promo" "1" "-15.00" "$EFFECTIVE" >/dev/null
GEL_CLOSE=$(load_close_bill "$GEL_BILL")
GEL_TOTAL=$(echo "$GEL_CLOSE" | jq -r '.total_amount')
echo "    gel bill_id=$GEL_BILL total_amount=$GEL_TOTAL"
load_assert_decimal_eq "$GEL_TOTAL" "35" "GEL closed total should be 35.00"

load_pass "discount logic"

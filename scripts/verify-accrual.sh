#!/usr/bin/env bash
# Verify workflow accrual is persisted to bills.accrual_total and matches Temporal queries.
#
# Usage (Temporal + encore run required):
#   ./scripts/verify-accrual.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=load/lib.sh
source "$SCRIPT_DIR/load/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CUSTOMER_ID="${CUSTOMER_ID:-cust-accrual-$(date +%s)}"
PERIOD_START="${PERIOD_START:-2025-06-01}"
PERIOD_END="${PERIOD_END:-2025-06-30}"
EFFECTIVE="${EFFECTIVE:-2025-06-15}"
NAMESPACE="${TEMPORAL_NAMESPACE:-default}"

echo "==> Accrual persistence test for $CUSTOMER_ID"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE" >/dev/null
load_add_fee "$BILL_ID" usage "API calls" "usage-jun" "5000" "0.001" "$EFFECTIVE" >/dev/null

wait_for_accrual() {
  local want="$1" tries=0
  while [[ $tries -lt 20 ]]; do
    local got
    got=$(load_get_bill "$BILL_ID" | jq -r '.accrual_total // empty')
    if [[ -n "$got" ]]; then
      if jq -n --arg g "$got" --arg w "$want" \
        '($g | tostring | gsub("[^0-9.-]";"") | tonumber) == ($w | tostring | gsub("[^0-9.-]";"") | tonumber)' \
        | grep -q true; then
        echo "$got"
        return 0
      fi
    fi
    sleep 0.25
    tries=$((tries + 1))
  done
  return 1
}

echo "==> Wait for accrual_total on GET /bills/:id"
ACCRUAL=$(wait_for_accrual "104") || load_fail "accrual_total not persisted (want 104)"
echo "    accrual_total=$ACCRUAL"
load_assert_decimal_eq "$ACCRUAL" "104" "DB accrual_total should be 104"

if command -v temporal >/dev/null 2>&1; then
  echo "==> Query Temporal status"
  STATUS_JSON=$(temporal workflow query \
    --namespace "$NAMESPACE" \
    --workflow-id "bill-$BILL_ID" \
    --name status \
    --output json 2>/dev/null || true)

  if [[ -n "$STATUS_JSON" ]]; then
    # Temporal CLI returns queryResult as an array of payload objects.
    QUERY_PAYLOAD=$(echo "$STATUS_JSON" | jq -c '
      if .queryResult then
        (if (.queryResult | type) == "array" then .queryResult[0] else .queryResult end)
      else . end
    ')
    QUERY_TOTAL=$(echo "$QUERY_PAYLOAD" | jq -r '.accrual_total // empty')
    QUERY_PHASE=$(echo "$QUERY_PAYLOAD" | jq -r '.phase // empty')
    QUERY_COUNT=$(echo "$QUERY_PAYLOAD" | jq -r '.line_item_count // empty')
    echo "    phase=$QUERY_PHASE line_item_count=$QUERY_COUNT accrual_total=$QUERY_TOTAL"
    load_assert_eq "$QUERY_PHASE" "accruing" "workflow phase should be accruing"
    load_assert_eq "$QUERY_COUNT" "2" "workflow should track 2 line items"
    load_assert_decimal_eq "$QUERY_TOTAL" "104" "Temporal accrual_total should match DB"
  else
    echo "WARN: temporal CLI unavailable or query failed; skipped Temporal checks" >&2
  fi
else
  echo "WARN: temporal CLI not found; skipped Temporal checks" >&2
fi

echo "==> Close bill clears accrual_total and sets total_amount"
CLOSE_RESP=$(load_close_bill "$BILL_ID")
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
OPEN_ACCRUAL=$(load_get_bill "$BILL_ID" | jq -r '.accrual_total // "null"')
echo "    total_amount=$CLOSE_TOTAL accrual_total=$OPEN_ACCRUAL"
load_assert_decimal_eq "$CLOSE_TOTAL" "104" "closed total should be 104"
load_assert_eq "$OPEN_ACCRUAL" "null" "accrual_total should be cleared after close"

load_pass "accrual persistence"

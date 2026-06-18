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
if [[ -z "${PERIOD_START:-}" ]]; then
  load_set_open_period
fi
PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"
NAMESPACE="${TEMPORAL_NAMESPACE:-default}"

echo "==> Accrual persistence test for $CUSTOMER_ID"
echo "    period=$PERIOD_START .. $PERIOD_END effective=$EFFECTIVE"

BILL_ID=$(load_create_bill "$CUSTOMER_ID" "$PERIOD_START" "$PERIOD_END" "USD")
load_track_bill "$BILL_ID"

SUB_RESP=$(load_add_fee "$BILL_ID" subscription "Monthly plan" "sub-jun" "1" "99.00" "$EFFECTIVE")
load_log_json_response "add subscription" "$SUB_RESP"

USAGE_RESP=$(load_add_fee "$BILL_ID" usage "API calls" "usage-jun" "5000" "0.001" "$EFFECTIVE")
load_log_json_response "add usage" "$USAGE_RESP"

wait_for_accrual() {
  local want="$1" tries=0
  while [[ $tries -lt 20 ]]; do
    local got bill_json
    bill_json=$(load_get_bill "$BILL_ID")
    got=$(echo "$bill_json" | jq -r '.accrual_total // empty')
    if [[ -n "$got" ]]; then
      if jq -n --arg g "$got" --arg w "$want" \
        '($g | tostring | gsub("[^0-9.-]";"") | tonumber) == ($w | tostring | gsub("[^0-9.-]";"") | tonumber)' \
        | grep -q true; then
        load_log_json_response "get bill (accrual ready)" "$bill_json"
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
load_assert_decimal_eq "$ACCRUAL" "104" "DB accrual_total should be 104"

if command -v temporal >/dev/null 2>&1; then
  echo "==> Query Temporal status"
  STATUS_JSON=$(temporal workflow query \
    --namespace "$NAMESPACE" \
    --workflow-id "bill-$BILL_ID" \
    --name status \
    --output json 2>/dev/null || true)

  if [[ -n "$STATUS_JSON" ]]; then
    QUERY_PAYLOAD=$(echo "$STATUS_JSON" | jq -c '
      if .queryResult then
        (if (.queryResult | type) == "array" then .queryResult[0] else .queryResult end)
      else . end
    ')
    QUERY_TOTAL=$(echo "$QUERY_PAYLOAD" | jq -r '.accrual_total // empty')
    QUERY_PHASE=$(echo "$QUERY_PAYLOAD" | jq -r '.phase // empty')
    QUERY_COUNT=$(echo "$QUERY_PAYLOAD" | jq -r '.line_item_count // empty')
    echo "    [temporal status] phase=$QUERY_PHASE line_item_count=$QUERY_COUNT accrual_total=$QUERY_TOTAL"
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
load_log_json_response "close bill" "$CLOSE_RESP"
CLOSE_TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')

FINAL_BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "get bill (closed)" "$FINAL_BILL"
OPEN_ACCRUAL=$(echo "$FINAL_BILL" | jq -r '.accrual_total // "null"')

load_assert_decimal_eq "$CLOSE_TOTAL" "104" "closed total should be 104"
load_assert_eq "$OPEN_ACCRUAL" "null" "accrual_total should be cleared after close"

load_pass "accrual persistence"

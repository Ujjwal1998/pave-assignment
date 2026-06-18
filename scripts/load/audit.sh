#!/usr/bin/env bash
# Post-run invariant checks for a bill via the HTTP API.
#
# Usage:
#   ./scripts/load/audit.sh <bill_id> [expected_line_item_count] [expected_total]
#
# Examples:
#   ./scripts/load/audit.sh "$BILL_ID"
#   ./scripts/load/audit.sh "$BILL_ID" 100 100.00

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools

BILL_ID="${1:?usage: audit.sh <bill_id> [expected_count] [expected_total]}"
EXPECTED_COUNT="${2:-}"
EXPECTED_TOTAL="${3:-}"

echo "==> Auditing bill $BILL_ID"

BILL=$(load_get_bill "$BILL_ID")
load_log_json_response "audit get bill" "$BILL"
STATUS=$(echo "$BILL" | jq -r '.status')
COUNT=$(echo "$BILL" | jq '.line_items | length')
DISTINCT=$(echo "$BILL" | jq '[.line_items[].external_reference_id] | unique | length')
SUM=$(echo "$BILL" | jq '[.line_items[].total_amount | tonumber] | add // 0')

echo "    status=$STATUS"
echo "    line_item_count=$COUNT"
echo "    distinct_external_refs=$DISTINCT"
echo "    sum_line_totals=$SUM"

if [[ "$COUNT" != "$DISTINCT" ]]; then
  load_fail "duplicate external_reference_id detected (count=$COUNT distinct=$DISTINCT)"
fi

if [[ -n "$EXPECTED_COUNT" ]]; then
  load_assert_eq "$COUNT" "$EXPECTED_COUNT" "line item count mismatch"
fi

if [[ "$STATUS" == "closed" ]]; then
  CLOSED_TOTAL=$(echo "$BILL" | jq -r '.total_amount // empty')
  CLOSE_COUNT=$(echo "$BILL" | jq '.line_items | length')
  echo "    closed_total_amount=$CLOSED_TOTAL"
  echo "    close_line_item_count=$CLOSE_COUNT"

  if [[ "$CLOSE_COUNT" != "$COUNT" ]]; then
    load_fail "close response line count differs from GET"
  fi

  if [[ -n "$EXPECTED_TOTAL" ]]; then
    MATCH=$(jq -n \
      --arg closed "$CLOSED_TOTAL" \
      --argjson sum "$SUM" \
      --arg expected "$EXPECTED_TOTAL" \
      '($closed | tostring | gsub("[^0-9.-]";"") | tonumber) == ($sum | tostring | gsub("[^0-9.-]";"") | tonumber)
       and ($closed | tostring | gsub("[^0-9.-]";"") | tonumber) == ($expected | tostring | gsub("[^0-9.-]";"") | tonumber)')
    if [[ "$MATCH" != "true" ]]; then
      load_fail "closed total mismatch (closed=$CLOSED_TOTAL sum=$SUM expected=$EXPECTED_TOTAL)"
    fi
  fi
fi

load_pass "audit OK for bill $BILL_ID"

#!/usr/bin/env bash
# Scenario C: concurrent duplicate external_reference_id (R4, R13)
# Expect exactly one line item row; all successful responses share the same id.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CONCURRENCY="${CONCURRENCY:-100}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-dup-ref-$(date +%s)}"
EXTERNAL_REF="${EXTERNAL_REF:-dup-ref-$(date +%s)}"

echo "==> Scenario C: duplicate external_reference_id (concurrency=$CONCURRENCY)"
load_log_responses_dir

load_ensure_open_period
BILL_ID=$(load_create_open_bill "$CUSTOMER_ID" "USD")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"
echo "    external_reference_id=$EXTERNAL_REF"

_outfile() { echo "$LOAD_TMPDIR/resp-$1.json"; }

_race_add() {
  local i="$1"
  load_add_line_item "$BILL_ID" "$EXTERNAL_REF" "42.00" "$PERIOD_START" > "$(_outfile "$i")"
}

for ((i = 0; i < CONCURRENCY; i++)); do
  _race_add "$i" &
done
wait

SUCCESS=0
for ((i = 0; i < CONCURRENCY; i++)); do
  f="$(_outfile "$i")"
  id=$(jq -r '.id // empty' < "$f")
  if [[ "${LOAD_VERBOSE:-}" == "1" || "$CONCURRENCY" -le 5 || "$i" -eq 0 || "$i" -eq 1 || "$i" -eq $((CONCURRENCY - 1)) ]]; then
    load_log_saved_response "req-$i" "$f"
  fi
  if [[ -n "$id" ]]; then
    ((SUCCESS++)) || true
  else
    echo "unexpected response in $f:" >&2
    cat "$f" >&2
    load_fail "line item add failed"
  fi
done

if [[ "${LOAD_VERBOSE:-}" != "1" && "$CONCURRENCY" -gt 5 ]]; then
  echo "    ... ($((CONCURRENCY - 3)) more responses in $LOAD_TMPDIR/resp-*.json)"
fi

UNIQUE_LINES=$(jq -r '.id // empty' "$LOAD_TMPDIR"/resp-*.json | sort -u | grep -c . || true)
COUNT=$(load_bill_line_item_count "$BILL_ID")
SUM=$(load_bill_sum_total "$BILL_ID")

echo "    successful_responses=$SUCCESS unique_line_item_ids=$UNIQUE_LINES db_count=$COUNT db_sum=$SUM"

load_assert_eq "$UNIQUE_LINES" "1" "expected one unique line item id across all responses"
load_assert_eq "$COUNT" "1" "expected one line item in DB"
load_assert_decimal_eq "$SUM" "42" "expected total 42"

"$SCRIPT_DIR/audit.sh" "$BILL_ID" 1
load_pass "Scenario C"

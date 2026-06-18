#!/usr/bin/env bash
# Scenario B: concurrent unique line items (R5)
# Expect every unique external_reference_id to persist.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools
load_init
trap load_cleanup EXIT

CONCURRENCY="${CONCURRENCY:-100}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-race-items-$(date +%s)}"
RUN_ID="${RUN_ID:-$(date +%s)}"

echo "==> Scenario B: concurrent unique line items (concurrency=$CONCURRENCY)"
load_log_responses_dir

load_ensure_open_period
BILL_ID=$(load_create_open_bill "$CUSTOMER_ID" "GEL")
load_track_bill "$BILL_ID"
echo "    bill_id=$BILL_ID"

_outfile() { echo "$LOAD_TMPDIR/resp-$1.json"; }

_race_add() {
  local i="$1"
  local ref="usage-${RUN_ID}-${i}"
  load_add_line_item "$BILL_ID" "$ref" "1.00" "$EFFECTIVE" > "$(_outfile "$i")"
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
    echo "failed response $f:" >&2
    cat "$f" >&2
    load_fail "line item add failed"
  fi
done

if [[ "${LOAD_VERBOSE:-}" != "1" && "$CONCURRENCY" -gt 5 ]]; then
  echo "    ... ($((CONCURRENCY - 3)) more responses in $LOAD_TMPDIR/resp-*.json)"
fi

COUNT=$(load_bill_line_item_count "$BILL_ID")
DISTINCT=$(load_bill_distinct_refs "$BILL_ID")
SUM=$(load_bill_sum_total "$BILL_ID")

echo "    successful=$SUCCESS db_count=$COUNT distinct_refs=$DISTINCT db_sum=$SUM"

load_assert_eq "$SUCCESS" "$CONCURRENCY" "not all concurrent adds succeeded"
load_assert_eq "$COUNT" "$CONCURRENCY" "DB line item count mismatch"
load_assert_eq "$DISTINCT" "$CONCURRENCY" "duplicate refs detected"
load_assert_eq "$SUM" "$CONCURRENCY" "expected sum to equal item count (1.00 each)"

"$SCRIPT_DIR/audit.sh" "$BILL_ID" "$CONCURRENCY"
load_pass "Scenario B"

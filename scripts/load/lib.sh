#!/usr/bin/env bash
# Shared helpers for load / race-condition tests.
# Source from other scripts: source "$(dirname "$0")/lib.sh"

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:4000}"
LOAD_TMPDIR="${LOAD_TMPDIR:-/tmp/pave-bank-load-$$}"
LOAD_BILL_REGISTRY="${LOAD_BILL_REGISTRY:-$LOAD_TMPDIR/bills-to-close}"

load_init() {
  mkdir -p "$LOAD_TMPDIR"
  : >"$LOAD_BILL_REGISTRY"
}

load_cleanup() {
  load_close_tracked_bills
  rm -rf "$LOAD_TMPDIR"
}

# load_track_bill registers a bill for automatic close on script exit.
# Set SKIP_WORKFLOW_CLEANUP=1 to leave workflows running (not recommended).
load_track_bill() {
  local bill_id="$1"
  [[ -n "$bill_id" ]] || return 0
  echo "$bill_id" >>"$LOAD_BILL_REGISTRY"
}

load_bill_status() {
  local bill_id="$1"
  load_get_bill "$bill_id" | jq -r '.status'
}

# Closes the bill if still open so the Temporal workflow can complete.
load_close_bill_if_open() {
  local bill_id="$1"
  local status code
  [[ -n "$bill_id" ]] || return 0
  status=$(load_bill_status "$bill_id" 2>/dev/null || echo unknown)
  if [[ "$status" == "open" ]]; then
    code=$(load_close_bill_status "$bill_id")
    if [[ "$code" == "200" ]]; then
      echo "    closed bill $bill_id (workflow bill-$bill_id should complete)"
    else
      echo "WARN: failed to close bill $bill_id (http $code)" >&2
    fi
  fi
}

load_close_tracked_bills() {
  if [[ "${SKIP_WORKFLOW_CLEANUP:-}" == "1" ]]; then
    return 0
  fi
  [[ -f "$LOAD_BILL_REGISTRY" ]] || return 0
  local bill_id
  sort -u "$LOAD_BILL_REGISTRY" | while read -r bill_id; do
    [[ -n "$bill_id" ]] || continue
    load_close_bill_if_open "$bill_id"
  done
}

load_require_tools() {
  command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
  command -v jq   >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
}

load_http_code() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -s -o /dev/null -w '%{http_code}' -X "$method" "$BASE_URL$path" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -o /dev/null -w '%{http_code}' -X "$method" "$BASE_URL$path"
  fi
}

load_http_json() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -s -X "$method" "$BASE_URL$path" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -X "$method" "$BASE_URL$path"
  fi
}

load_create_bill() {
  local customer_id="$1" period_start="$2" period_end="$3" currency="$4"
  local body resp bill_id
  body=$(jq -n \
    --arg c "$customer_id" \
    --arg ps "$period_start" \
    --arg pe "$period_end" \
    --arg cur "$currency" \
    '{customer_id:$c, period_start:$ps, period_end:$pe, currency:$cur}')
  resp=$(load_http_json POST /bills "$body")
  bill_id=$(echo "$resp" | jq -r '.id // .details.bill_id // .details.bill.id // empty')
  if [[ -z "$bill_id" ]]; then
    echo "failed to create bill: $resp" >&2
    return 1
  fi
  echo "$bill_id"
}

load_add_line_item() {
  local bill_id="$1" external_ref="$2" unit_price="${3:-1.00}" effective_date="${4:-2025-04-01}"
  local body
  body=$(jq -n \
    --arg ref "$external_ref" \
    --arg price "$unit_price" \
    --arg date "$effective_date" \
    '{
      fee_type: "usage",
      description: "load test",
      quantity: "1",
      unit_price: $price,
      effective_date: $date,
      external_reference_id: $ref
    }')
  load_http_json POST "/bills/$bill_id/line-items" "$body"
}

load_add_line_item_status() {
  local bill_id="$1" external_ref="$2" unit_price="${3:-1.00}" effective_date="${4:-2025-04-01}"
  local body
  body=$(jq -n \
    --arg ref "$external_ref" \
    --arg price "$unit_price" \
    --arg date "$effective_date" \
    '{
      fee_type: "usage",
      description: "load test",
      quantity: "1",
      unit_price: $price,
      effective_date: $date,
      external_reference_id: $ref
    }')
  load_http_code POST "/bills/$bill_id/line-items" "$body"
}

load_close_bill() {
  local bill_id="$1"
  load_http_json POST "/bills/$bill_id/close"
}

load_close_bill_status() {
  local bill_id="$1"
  load_http_code POST "/bills/$bill_id/close"
}

load_get_bill() {
  local bill_id="$1"
  load_http_json GET "/bills/$bill_id"
}

load_bill_line_item_count() {
  local bill_id="$1"
  load_get_bill "$bill_id" | jq '.line_items | length'
}

load_bill_distinct_refs() {
  local bill_id="$1"
  load_get_bill "$bill_id" | jq '[.line_items[].external_reference_id] | unique | length'
}

load_bill_sum_total() {
  local bill_id="$1"
  # Sum line item totals using jq (string decimals)
  load_get_bill "$bill_id" | jq '
    [.line_items[].total_amount | tonumber] | add // 0
  '
}

load_pass() { echo "PASS: $*"; }
load_fail() { echo "FAIL: $*" >&2; exit 1; }

load_assert_eq() {
  local got="$1" want="$2" msg="$3"
  if [[ "$got" != "$want" ]]; then
    load_fail "$msg (got=$got want=$want)"
  fi
}

load_assert_decimal_eq() {
  local got="$1" want="$2" msg="$3"
  local match
  match=$(jq -n --arg g "$got" --arg w "$want" '
    ($g | tostring | gsub("[^0-9.-]";"") | tonumber) == ($w | tostring | gsub("[^0-9.-]";"") | tonumber)
  ')
  if [[ "$match" != "true" ]]; then
    load_fail "$msg (got=$got want=$want)"
  fi
}

load_run_concurrent() {
  # Usage: load_run_concurrent N callback_name arg1 arg2 ...
  local n="$1"; shift
  local fn="$1"; shift
  local i
  for ((i = 0; i < n; i++)); do
    "$fn" "$i" "$@" &
  done
  wait
}

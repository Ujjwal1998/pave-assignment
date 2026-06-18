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
  echo "000" >"$LOAD_TMPDIR/load-last-http-code"
}

load_last_http_code() {
  cat "${LOAD_TMPDIR}/load-last-http-code"
}

load_cleanup() {
  load_close_tracked_bills
  if [[ "${KEEP_LOAD_ARTIFACTS:-}" == "1" ]]; then
    echo "    kept response artifacts in $LOAD_TMPDIR"
    return 0
  fi
  rm -rf "$LOAD_TMPDIR"
}

# Print where per-request JSON responses are written.
load_log_responses_dir() {
  echo "    responses_dir=$LOAD_TMPDIR" >&2
}

# Log one saved API response: path, HTTP-style code field, and key ids.
load_log_saved_response() {
  local label="$1" file="$2"
  echo "    [$label] file=$file" >&2
  load_log_json_response "$label" "$(cat "$file")"
}

# Print key fields from an API JSON response (bill, line item, close, or error).
# Logs go to stderr so callers can safely use $(load_create_bill ...) etc.
load_log_json_response() {
  local label="$1" json="$2"
  [[ -n "$json" ]] || return 0

  local code message id bill_id status currency total accrual ref fee_type line_count

  code=$(echo "$json" | jq -r '.code // empty')
  message=$(echo "$json" | jq -r '.message // empty')

  if [[ -n "$code" && "$code" != "null" ]]; then
    bill_id=$(echo "$json" | jq -r '.details.bill_id // .details.bill.id // empty')
    echo "    [$label] error code=$code message=${message:-<none>} bill_id=${bill_id:-<none>}" >&2
    if [[ "${LOAD_VERBOSE:-}" == "1" ]]; then
      echo "$json" | jq -c . | sed 's/^/        /' >&2
    fi
    return 0
  fi

  id=$(echo "$json" | jq -r '.id // .bill_id // empty')
  status=$(echo "$json" | jq -r '.status // empty')
  currency=$(echo "$json" | jq -r '.currency // empty')
  total=$(echo "$json" | jq -r '.total_amount // empty')
  accrual=$(echo "$json" | jq -r '.accrual_total // empty')
  ref=$(echo "$json" | jq -r '.external_reference_id // empty')
  fee_type=$(echo "$json" | jq -r '.fee_type // empty')
  line_count=$(echo "$json" | jq -r 'if .line_item_count != null then .line_item_count elif .line_items != null then (.line_items | length) else empty end')

  if [[ -n "$fee_type" && "$fee_type" != "null" ]]; then
    echo "    [$label] line_item_id=${id:-<none>} fee_type=$fee_type ref=${ref:-<none>} total=${total:-<none>} currency=${currency:-<none>}" >&2
    return 0
  fi

  local parts=()
  [[ -n "$id" && "$id" != "null" ]] && parts+=("id=$id")
  [[ -n "$status" && "$status" != "null" ]] && parts+=("status=$status")
  [[ -n "$currency" && "$currency" != "null" ]] && parts+=("currency=$currency")
  [[ -n "$accrual" && "$accrual" != "null" ]] && parts+=("accrual_total=$accrual")
  [[ -n "$total" && "$total" != "null" ]] && parts+=("total_amount=$total")
  [[ -n "$line_count" && "$line_count" != "null" ]] && parts+=("line_items=$line_count")

  if [[ ${#parts[@]} -gt 0 ]]; then
    echo "    [$label] ${parts[*]}" >&2
  else
    echo "    [$label] $(echo "$json" | jq -c 'with_entries(select(.value != null and .value != ""))' 2>/dev/null || echo "$json")" >&2
  fi

  if [[ "${LOAD_VERBOSE:-}" == "1" ]]; then
    echo "$json" | jq -c . | sed 's/^/        /' >&2
  fi
}

# HTTP request that captures status code and prints the body.
# Status is persisted to LOAD_TMPDIR/load-last-http-code so it survives $(...) subshells.
load_http_json_with_status() {
  local method="$1" path="$2" body="${3:-}" code_out="${4:-}"
  local tmp code_file
  mkdir -p "${LOAD_TMPDIR:-/tmp}"
  code_file="${LOAD_TMPDIR:-/tmp}/load-last-http-code"
  tmp="${LOAD_TMPDIR:-/tmp}/curl-$$-$(date +%s%N).json"
  if [[ -n "$body" ]]; then
    LOAD_LAST_HTTP_CODE=$(curl -s -o "$tmp" -w '%{http_code}' -X "$method" "$BASE_URL$path" \
      -H 'Content-Type: application/json' -d "$body")
  else
    LOAD_LAST_HTTP_CODE=$(curl -s -o "$tmp" -w '%{http_code}' -X "$method" "$BASE_URL$path")
  fi
  if [[ -n "$code_out" ]]; then
    echo "$LOAD_LAST_HTTP_CODE" >"$code_out"
  else
    echo "$LOAD_LAST_HTTP_CODE" >"$code_file"
  fi
  cat "$tmp"
  rm -f "$tmp"
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
    local close_resp
    close_resp=$(load_close_bill "$bill_id")
    if echo "$close_resp" | jq -e '.total_amount // .bill_id' >/dev/null 2>&1; then
      load_log_json_response "cleanup close" "$close_resp"
      echo "    closed bill $bill_id (workflow bill-$bill_id should complete)"
    else
      echo "WARN: failed to close bill $bill_id" >&2
      load_log_json_response "cleanup close failed" "$close_resp"
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

# Set PERIOD_START, PERIOD_END, and EFFECTIVE to the current UTC month.
# Use for verify scripts so bills stay open (not scheduled) and are not
# auto-closed immediately by past period_end (Phase 2).
load_set_open_period() {
  read -r PERIOD_START PERIOD_END EFFECTIVE <<EOF
$(python3 - <<'PY'
from datetime import date, timedelta
today = date.today()
start = date(today.year, today.month, 1)
if today.month == 12:
    next_month = date(today.year + 1, 1, 1)
else:
    next_month = date(today.year, today.month + 1, 1)
end = next_month - timedelta(days=1)
effective = start + timedelta(days=14)
print(start.isoformat(), end.isoformat(), effective.isoformat())
PY
)
EOF
}

# Default load-test period to the current UTC month unless PERIOD_START is set.
load_ensure_open_period() {
  if [[ -n "${PERIOD_START:-}" ]]; then
    PERIOD_END="${PERIOD_END:-$(load_period_end_from_start "$PERIOD_START")}"
    EFFECTIVE="${EFFECTIVE:-$(load_effective_from_start "$PERIOD_START")}"
    return 0
  fi
  load_set_open_period
}

load_create_open_bill() {
  local customer_id="$1" currency="${2:-USD}"
  load_ensure_open_period
  load_create_bill "$customer_id" "$PERIOD_START" "$PERIOD_END" "$currency"
}

load_period_end_from_start() {
  python3 - "$1" <<'PY'
from datetime import date, timedelta
import sys
y, m, d = map(int, sys.argv[1].split("-"))
start = date(y, m, d)
if start.month == 12:
    next_month = date(start.year + 1, 1, 1)
else:
    next_month = date(start.year, start.month + 1, 1)
print((next_month - timedelta(days=1)).isoformat())
PY
}

load_effective_from_start() {
  python3 - "$1" <<'PY'
from datetime import date, timedelta
import sys
y, m, d = map(int, sys.argv[1].split("-"))
start = date(y, m, d)
print((start + timedelta(days=14)).isoformat())
PY
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
  load_log_json_response "create bill" "$resp"
  bill_id=$(echo "$resp" | jq -r '.id // .details.bill_id // .details.bill.id // empty')
  if [[ -z "$bill_id" ]]; then
    echo "failed to create bill: $resp" >&2
    return 1
  fi
  echo "$bill_id"
}

load_add_line_item() {
  local bill_id="$1" external_ref="$2" unit_price="${3:-1.00}" effective_date="${4:-}"
  if [[ -z "$effective_date" ]]; then
    load_ensure_open_period
    effective_date="$EFFECTIVE"
  fi
  load_add_fee "$bill_id" "usage" "load test" "$external_ref" "1" "$unit_price" "$effective_date"
}

# Add a line item with explicit fee_type (subscription, usage, tax, penalty, discount).
# Optional 8th argument: line item currency (defaults to bill currency on the server).
load_add_fee() {
  local bill_id="$1" fee_type="$2" description="$3" external_ref="$4" quantity="$5" unit_price="$6" effective_date="$7"
  local currency="${8:-}"
  local body
  if [[ -n "$currency" ]]; then
    body=$(jq -n \
      --arg ft "$fee_type" \
      --arg desc "$description" \
      --arg ref "$external_ref" \
      --arg qty "$quantity" \
      --arg price "$unit_price" \
      --arg date "$effective_date" \
      --arg cur "$currency" \
      '{
        fee_type: $ft,
        description: $desc,
        quantity: $qty,
        unit_price: $price,
        effective_date: $date,
        external_reference_id: $ref,
        currency: $cur
      }')
  else
    body=$(jq -n \
      --arg ft "$fee_type" \
      --arg desc "$description" \
      --arg ref "$external_ref" \
      --arg qty "$quantity" \
      --arg price "$unit_price" \
      --arg date "$effective_date" \
      '{
        fee_type: $ft,
        description: $desc,
        quantity: $qty,
        unit_price: $price,
        effective_date: $date,
        external_reference_id: $ref
      }')
  fi
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
  local wait="${2:-true}"
  if [[ "$wait" == "true" ]]; then
    load_http_json POST "/bills/$bill_id/close?wait=true"
  else
    load_http_json POST "/bills/$bill_id/close"
  fi
}

load_close_bill_status() {
  local bill_id="$1"
  local wait="${2:-true}"
  if [[ "$wait" == "true" ]]; then
    load_http_code POST "/bills/$bill_id/close?wait=true"
  else
    load_http_code POST "/bills/$bill_id/close"
  fi
}

# Poll GET /bills/:id until status=closed or timeout.
load_wait_for_closed() {
  local bill_id="$1" tries="${2:-40}" delay="${3:-0.25}"
  local i=0 status
  while [[ $i -lt $tries ]]; do
    status=$(load_get_bill "$bill_id" | jq -r '.status')
    if [[ "$status" == "closed" ]]; then
      echo "$status"
      return 0
    fi
    sleep "$delay"
    i=$((i + 1))
  done
  return 1
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

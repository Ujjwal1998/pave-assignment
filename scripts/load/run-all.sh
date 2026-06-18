#!/usr/bin/env bash
# Run all load / race-condition scenarios.
#
# Prerequisites:
#   temporal server start-dev --namespace default --port 7233
#   encore run
#
# Usage:
#   chmod +x scripts/load/*.sh
#   ./scripts/load/run-all.sh
#
# Environment:
#   BASE_URL=http://localhost:4000
#   SKIP_SLOW=1          skip D1 (close race)
#   CONCURRENCY=50       override default concurrency for applicable tests

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools

echo "==> pave-bank load / race test suite"
echo "    BASE_URL=$BASE_URL"
echo "    (open bills are auto-closed on exit unless SKIP_WORKFLOW_CLEANUP=1)"
echo

run() {
  local name="$1" script="$2"
  echo "────────────────────────────────────────"
  echo "Running $name"
  if bash "$SCRIPT_DIR/$script"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    FAILED+=("$name")
  fi
  echo
}

PASS=0
FAIL=0
FAILED=()

run "A duplicate bill create"      "race-duplicate-bill.sh"
run "B concurrent unique line items" "race-concurrent-line-items.sh"
run "C duplicate external_reference_id" "race-duplicate-line-item.sh"
run "D2 add after close"           "race-add-after-close.sh"
run "E duplicate close"            "race-duplicate-close.sh"

if [[ "${SKIP_SLOW:-}" != "1" ]]; then
  run "D1 close vs concurrent adds" "race-close-vs-add.sh"
else
  echo "Skipping D1 (SKIP_SLOW=1)"
fi

echo "════════════════════════════════════════"
echo "Results: $PASS passed, $FAIL failed"
if [[ "$FAIL" -gt 0 ]]; then
  echo "Failed:"
  printf '  - %s\n' "${FAILED[@]}"
  exit 1
fi
echo "All scenarios passed."

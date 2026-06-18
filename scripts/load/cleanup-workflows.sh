#!/usr/bin/env bash
# Close open bills so their Temporal workflows (bill-{id}) can complete.
#
# Usage:
#   ./scripts/load/cleanup-workflows.sh <bill-uuid> [bill-uuid...]
#   ./scripts/load/cleanup-workflows.sh bill-7f616f56-93ad-487f-9a12-29adb6d70130
#
# Workflow IDs in Temporal are prefixed with "bill-"; this script accepts either
# the raw UUID or the full workflow id.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

load_require_tools

if [[ $# -lt 1 ]]; then
  echo "usage: cleanup-workflows.sh <bill-id|bill-{uuid}> ..." >&2
  echo "Close open bills left behind by load tests." >&2
  exit 1
fi

normalize_bill_id() {
  local id="$1"
  id="${id#bill-}"
  echo "$id"
}

echo "==> Closing open bills (SKIP_WORKFLOW_CLEANUP ignored)"
for raw in "$@"; do
  bill_id=$(normalize_bill_id "$raw")
  echo "    bill-$bill_id"
  status=$(load_bill_status "$bill_id" 2>/dev/null || echo unknown)
  echo "    [before close] status=$status"
  load_close_bill_if_open "$bill_id"
done
echo "==> Done"

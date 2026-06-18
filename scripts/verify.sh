#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:4000}"
CUSTOMER_ID="${CUSTOMER_ID:-cust-verify-$(date +%s)}"

echo "==> Creating bill for $CUSTOMER_ID"
CREATE_RESP=$(curl -s -X POST "$BASE_URL/bills" \
  -H 'Content-Type: application/json' \
  -d "{\"customer_id\":\"$CUSTOMER_ID\",\"period_start\":\"2025-04-01\",\"period_end\":\"2025-04-30\",\"currency\":\"USD\"}")

BILL_ID=$(echo "$CREATE_RESP" | jq -r '.id // .details.bill_id // .details.bill.id')
if [ -z "$BILL_ID" ] || [ "$BILL_ID" = "null" ]; then
  echo "failed to create bill:" >&2
  echo "$CREATE_RESP" | jq . >&2
  exit 1
fi
echo "    bill_id=$BILL_ID"

echo "==> Adding line items"
curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"subscription","description":"Monthly plan","quantity":"1","unit_price":"99.00","effective_date":"2025-04-01","external_reference_id":"sub-apr-2025"}' \
  | jq -r '.id' >/dev/null

curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"usage","description":"API calls","quantity":"5000","unit_price":"0.001","effective_date":"2025-04-15","external_reference_id":"usage-apr-2025"}' \
  | jq -r '.id' >/dev/null

echo "==> Idempotency check (resend first line item)"
FIRST_ID=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"subscription","description":"Monthly plan","quantity":"1","unit_price":"99.00","effective_date":"2025-04-01","external_reference_id":"sub-apr-2025"}' \
  | jq -r '.id')
echo "    line_item_id=$FIRST_ID"

echo "==> Closing bill"
CLOSE_RESP=$(curl -sf -X POST "$BASE_URL/bills/$BILL_ID/close")
TOTAL=$(echo "$CLOSE_RESP" | jq -r '.total_amount')
echo "    total_amount=$TOTAL"

if [ "$TOTAL" != "104" ] && [ "$TOTAL" != "104.00" ] && [ "$TOTAL" != "104.000000000" ]; then
  echo "unexpected total: $TOTAL (want 104.00)" >&2
  echo "$CLOSE_RESP" | jq . >&2
  exit 1
fi

echo "==> Verify closed bill rejects new line items"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/bills/$BILL_ID/line-items" \
  -H 'Content-Type: application/json' \
  -d '{"fee_type":"tax","description":"Late fee","quantity":"1","unit_price":"1.00","effective_date":"2025-04-20","external_reference_id":"tax-apr"}')
if [ "$STATUS" != "400" ] && [ "$STATUS" != "422" ]; then
  echo "expected 422 for line item on closed bill, got $STATUS" >&2
  exit 1
fi

echo "==> Verify duplicate close returns 422"
STATUS=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/bills/$BILL_ID/close")
if [ "$STATUS" != "422" ]; then
  echo "expected 422 for duplicate close, got $STATUS" >&2
  exit 1
fi

echo "==> All checks passed"

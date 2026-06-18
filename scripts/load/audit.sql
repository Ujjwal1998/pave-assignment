-- Post-run SQL audit for billing invariants.
-- Run against the billing database:
--
--   encore db shell billing
--   \i scripts/load/audit.sql
--
-- Or pass a specific bill:
--   psql "$CONN" -v bill_id="'YOUR-UUID'" -f scripts/load/audit.sql

\if :{?bill_id}
\else
  \set bill_id '00000000-0000-0000-0000-000000000000'
\endif

\echo '=== Duplicate external_reference_id per bill (expect 0 rows) ==='
SELECT bill_id, external_reference_id, COUNT(*) AS cnt
FROM line_items
WHERE (:bill_id = '00000000-0000-0000-0000-000000000000' OR bill_id = :bill_id::uuid)
GROUP BY bill_id, external_reference_id
HAVING COUNT(*) > 1;

\echo '=== Closed bills where total != sum(line items) (expect 0 rows) ==='
SELECT
    b.id,
    b.currency,
    b.total_amount,
    COALESCE(SUM(li.total_amount), 0) AS computed_sum
FROM bills b
LEFT JOIN line_items li ON li.bill_id = b.id
WHERE b.status = 'closed'
  AND (:bill_id = '00000000-0000-0000-0000-000000000000' OR b.id = :bill_id::uuid)
GROUP BY b.id, b.currency, b.total_amount
HAVING b.total_amount IS DISTINCT FROM COALESCE(SUM(li.total_amount), 0);

\echo '=== Line items on closed bills (expect 0 rows) ==='
SELECT li.id, li.bill_id, li.external_reference_id, li.created_at, b.closed_at
FROM line_items li
JOIN bills b ON b.id = li.bill_id
WHERE b.status = 'closed'
  AND (:bill_id = '00000000-0000-0000-0000-000000000000' OR b.id = :bill_id::uuid)
  AND li.created_at > b.closed_at;

\echo '=== Open bills with empty workflow_run_id (informational) ==='
SELECT id, customer_id, created_at
FROM bills
WHERE status = 'open'
  AND workflow_run_id = ''
  AND (:bill_id = '00000000-0000-0000-0000-000000000000' OR id = :bill_id::uuid);

\echo '=== Bill summary ==='
SELECT
    b.id,
    b.status,
    b.currency,
    b.total_amount,
    COUNT(li.id) AS line_item_count,
    COALESCE(SUM(li.total_amount), 0) AS line_items_sum
FROM bills b
LEFT JOIN line_items li ON li.bill_id = b.id
WHERE :bill_id = '00000000-0000-0000-0000-000000000000' OR b.id = :bill_id::uuid
GROUP BY b.id, b.status, b.currency, b.total_amount;

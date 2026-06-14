CREATE TYPE fee_type AS ENUM ('subscription', 'usage', 'tax', 'penalty', 'discount');

CREATE TABLE line_items (
    id                    UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id               UUID          NOT NULL REFERENCES bills(id) ON DELETE RESTRICT,
    fee_type              fee_type      NOT NULL,
    description           TEXT          NOT NULL,
    quantity              NUMERIC(19,9) NOT NULL,
    unit_price            NUMERIC(19,9) NOT NULL,
    total_amount          NUMERIC(19,9) NOT NULL,
    currency              CHAR(3)       NOT NULL,
    effective_date        DATE          NOT NULL,
    external_reference_id TEXT          NOT NULL,
    created_at            TIMESTAMPTZ   NOT NULL DEFAULT now(),

    CONSTRAINT line_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT line_items_currency_check    CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT line_items_dedup             UNIQUE (bill_id, external_reference_id)
);

CREATE INDEX idx_line_items_bill_id  ON line_items (bill_id);
CREATE INDEX idx_line_items_fee_type ON line_items (bill_id, fee_type);

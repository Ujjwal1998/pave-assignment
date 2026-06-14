CREATE TYPE bill_status AS ENUM ('open', 'closed');

CREATE TABLE bills (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id      TEXT          NOT NULL,
    period_start     DATE          NOT NULL,
    period_end       DATE          NOT NULL,
    currency         CHAR(3)       NOT NULL,
    status           bill_status   NOT NULL DEFAULT 'open',
    total_amount     NUMERIC(19,9),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    closed_at        TIMESTAMPTZ,
    workflow_run_id  TEXT          NOT NULL DEFAULT '',

    CONSTRAINT bills_period_check   CHECK (period_end > period_start),
    CONSTRAINT bills_currency_check CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT bills_unique_bill    UNIQUE (customer_id, period_start, period_end, currency)
);

CREATE INDEX idx_bills_customer_id ON bills (customer_id);
CREATE INDEX idx_bills_status      ON bills (status);

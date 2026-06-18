ALTER TYPE bill_status ADD VALUE IF NOT EXISTS 'scheduled' BEFORE 'open';
ALTER TYPE bill_status ADD VALUE IF NOT EXISTS 'closing' AFTER 'open';

CREATE OR REPLACE FUNCTION reject_line_item_on_closed_bill()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM bills WHERE id = NEW.bill_id AND status <> 'open'
    ) THEN
        RAISE EXCEPTION 'bill is not open for line items'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

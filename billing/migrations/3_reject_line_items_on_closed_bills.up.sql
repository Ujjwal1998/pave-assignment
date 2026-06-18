CREATE OR REPLACE FUNCTION reject_line_item_on_closed_bill()
RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM bills WHERE id = NEW.bill_id AND status = 'closed'
    ) THEN
        RAISE EXCEPTION 'bill is already closed'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER line_items_closed_bill_check
    BEFORE INSERT ON line_items
    FOR EACH ROW
    EXECUTE FUNCTION reject_line_item_on_closed_bill();

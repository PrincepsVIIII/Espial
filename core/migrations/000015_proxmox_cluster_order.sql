ALTER TABLE integrations
    ADD COLUMN sort_order integer NOT NULL DEFAULT 0;

CREATE INDEX integrations_adapter_sort_order_idx
    ON integrations (adapter_id, sort_order, display_name, id);

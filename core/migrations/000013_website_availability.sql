-- A website monitor remains a typed integration row. These indexes support its
-- projection and availability history without introducing a second authority.
CREATE INDEX integrations_webcheck_updated_idx
    ON integrations (updated_at DESC, id DESC)
    WHERE adapter_id = 'org.ubnetdef.espial.webcheck';

CREATE INDEX observations_website_availability_idx
    ON observations (resource_id, observed_at DESC, received_at DESC, id DESC)
    WHERE check_type = 'website.availability';

-- The aggregate availability rule wins over the global rule by specificity.
INSERT INTO incident_rules (
    id, name, enabled, priority, resource_kind, check_type, recovery_state,
    recovery_min_occurrences, recovery_for_seconds
) VALUES (
    '20000000-0000-4000-8000-000000000002',
    'Website availability', true, 200, 'webpage', 'website.availability',
    'healthy', 2, 0
);

INSERT INTO incident_rule_conditions (
    rule_id, state, severity, min_occurrences, for_seconds
) VALUES
    ('20000000-0000-4000-8000-000000000002', 'critical', 'critical', 1, 0),
    ('20000000-0000-4000-8000-000000000002', 'warning', 'warning', 2, 0),
    ('20000000-0000-4000-8000-000000000002', 'unknown', 'warning', 1, 0);

ALTER TABLE administrative_mutation_idempotency
    DROP CONSTRAINT administrative_mutation_idempotency_target_type_check,
    DROP CONSTRAINT administrative_mutation_idempotency_operation_check;
ALTER TABLE administrative_mutation_idempotency
    ADD CONSTRAINT administrative_mutation_idempotency_target_type_check
        CHECK (target_type IN (
            'incident_rule', 'maintenance_window', 'silence',
            'notification_destination', 'website_monitor'
        )),
    ADD CONSTRAINT administrative_mutation_idempotency_operation_check
        CHECK (operation IN ('create', 'replace', 'revoke', 'test', 'check'));

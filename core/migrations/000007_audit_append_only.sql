CREATE OR REPLACE FUNCTION reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only' USING ERRCODE = '42501';
END;
$$;

CREATE TRIGGER audit_events_reject_update
BEFORE UPDATE ON audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_event_mutation();

CREATE TRIGGER audit_events_reject_delete
BEFORE DELETE ON audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_event_mutation();

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espial_app') THEN
        REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM espial_app;
        GRANT SELECT, INSERT ON audit_events TO espial_app;
    END IF;
END;
$$;

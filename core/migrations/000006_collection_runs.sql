CREATE TABLE integration_collection_runs (
    id uuid PRIMARY KEY,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,
    duration_ms bigint NOT NULL CHECK (duration_ms >= 0),
    result text NOT NULL CHECK (result IN ('succeeded', 'rejected', 'failed', 'skipped')),
    error_code text CHECK (
        error_code IS NULL OR error_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    resource_count integer NOT NULL DEFAULT 0 CHECK (resource_count >= 0),
    observation_count integer NOT NULL DEFAULT 0 CHECK (observation_count >= 0),
    observations_inserted integer NOT NULL DEFAULT 0 CHECK (observations_inserted >= 0),
    duplicate_observations integer NOT NULL DEFAULT 0 CHECK (duplicate_observations >= 0),
    correlation_id text NOT NULL CHECK (length(correlation_id) BETWEEN 1 AND 128),
    CHECK (completed_at >= started_at),
    CHECK (
        (result = 'succeeded' AND error_code IS NULL) OR
        (result <> 'succeeded' AND error_code IS NOT NULL)
    )
);

CREATE INDEX integration_collection_runs_integration_time_idx
    ON integration_collection_runs (integration_id, completed_at DESC, id DESC);

CREATE INDEX integration_collection_runs_result_time_idx
    ON integration_collection_runs (result, completed_at DESC);

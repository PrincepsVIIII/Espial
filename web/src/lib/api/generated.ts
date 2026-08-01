// Generated from api/schemas/v1. Do not edit by hand.

export type AdapterProtocolEnvelope = {
  [k: string]: unknown;
} & {
  protocol_version: string;
  kind: "request" | "response" | "notification";
  operation: "manifest" | "validate_config" | "collect" | "health" | "shutdown" | "ready" | "event" | "log";
  request_id?: string;
  sent_at: string;
  deadline?: string;
  payload?: {
    [k: string]: unknown;
  };
  error?: {
    code: string;
    message: string;
    retryable: boolean;
  };
};

export interface AdapterManifest {
  adapter_id: string;
  display_name: string;
  adapter_version: string;
  /**
   * @minItems 1
   */
  protocol_versions: string[];
  integration_category: string;
  resource_types?: string[];
  check_types?: string[];
  /**
   * @minItems 1
   */
  capabilities: ("collect" | "events" | "notifications" | "actions")[];
  read_only?: boolean;
  config_schema: {
    [k: string]: unknown;
  };
  secret_fields?: string[];
}

export interface APIErrorResponse {
  error: {
    code: string;
    message: string;
    request_id: string;
    /**
     * @maxItems 64
     */
    fields?: {
      field: string;
      code: string;
    }[];
  };
}

export interface RedactedAuditEventView {
  id: string;
  actor_user_id?: string;
  actor_username?: string;
  action: string;
  target_type: string;
  target_id?: string;
  result: "succeeded" | "failed" | "denied";
  source_address?: {
    [k: string]: unknown;
  } & string;
  correlation_id: string;
  before_summary?: {
    [k: string]: unknown;
  };
  after_summary?: {
    [k: string]: unknown;
  };
  occurred_at: string;
}

export interface EspialEvent {
  schema_version: "1.0";
  event_id: string;
  event_type: string;
  source: string;
  occurred_at: string;
  emitted_at?: string;
  correlation_id?: string;
  resource_id?: string;
  integration_id?: string;
  payload: {
    [k: string]: unknown;
  };
}

export type IncidentAPIDetail = IncidentAPISummary & {
  fingerprint: string;
  [k: string]: unknown;
};

export interface IncidentAPISummary {
  id: string;
  rule_id: string;
  rule_name: string;
  integration_id: string;
  integration_name: string;
  resource_id: string;
  resource_name: string;
  check_type: string;
  title: string;
  summary: string;
  severity: "warning" | "critical";
  status: "open" | "acknowledged" | "investigating" | "recovered" | "resolved";
  owner_user_id?: string;
  owner_name?: string;
  detected_at: string;
  latest_signal_at: string;
  acknowledged_at?: string;
  recovered_at?: string;
  resolved_at?: string;
  version: number;
  updated_at: string;
  fingerprint?: string;
}

export interface IncidentRuleAPIView {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  integration_id?: string;
  resource_id?: string;
  resource_kind?: string;
  check_type?: string;
  reason_code?: string;
  /**
   * @minItems 1
   * @maxItems 4
   */
  conditions: {
    state: "warning" | "critical" | "unknown" | "stale";
    severity: "warning" | "critical";
    min_occurrences: number;
    for_seconds: number;
  }[];
  recovery_state: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
  recovery_min_occurrences: number;
  recovery_for_seconds: number;
  created_at: string;
  updated_at: string;
}

export interface IncidentAPISummary {
  id: string;
  rule_id: string;
  rule_name: string;
  integration_id: string;
  integration_name: string;
  resource_id: string;
  resource_name: string;
  check_type: string;
  title: string;
  summary: string;
  severity: "warning" | "critical";
  status: "open" | "acknowledged" | "investigating" | "recovered" | "resolved";
  owner_user_id?: string;
  owner_name?: string;
  detected_at: string;
  latest_signal_at: string;
  acknowledged_at?: string;
  recovered_at?: string;
  resolved_at?: string;
  version: number;
  updated_at: string;
  fingerprint?: string;
}

export interface IncidentTimelineEvent {
  id: string;
  incident_id: string;
  signal_id?: string;
  actor_user_id?: string;
  actor_name?: string;
  kind:
    | "detected"
    | "severity_changed"
    | "recurrence"
    | "recovered"
    | "acknowledged"
    | "investigating"
    | "assigned"
    | "note"
    | "resolved"
    | "notification";
  from_status?: "open" | "acknowledged" | "investigating" | "recovered" | "resolved";
  to_status?: "open" | "acknowledged" | "investigating" | "recovered" | "resolved";
  from_severity?: "warning" | "critical";
  to_severity?: "warning" | "critical";
  summary: string;
  occurred_at: string;
}

export interface IntegrationAPIView {
  id: string;
  adapter_id: string;
  display_name: string;
  enabled: boolean;
  interval_seconds: number;
  /**
   * @maxItems 128
   */
  config_keys: string[];
  /**
   * @maxItems 128
   */
  secret_reference_keys: string[];
  runtime_state: "starting" | "healthy" | "unhealthy" | "stopped" | "disabled" | "not_started";
  resource_count: number;
  stale_count: number;
  unknown_count: number;
  instance?: {
    adapter_version: string;
    protocol_version?: string;
    state: "starting" | "healthy" | "unhealthy" | "stopped";
    last_started_at?: string;
    last_healthy_at?: string;
    last_stopped_at?: string;
    last_error_at?: string;
    last_error_code?: string;
    consecutive_failures: number;
    next_restart_at?: string;
    updated_at: string;
  };
  last_collection?: {
    started_at: string;
    completed_at: string;
    duration_ms: number;
    result: "succeeded" | "rejected" | "failed" | "skipped";
    error_code?: string;
    resource_count: number;
    observation_count: number;
    observations_inserted: number;
    duplicate_observations: number;
  };
  created_at: string;
  updated_at: string;
}

export type IntegrationWriteRequest = CreateIntegrationRequest | ReplaceIntegrationConfigurationRequest;

export interface CreateIntegrationRequest {
  adapter_id: string;
  display_name: string;
  enabled: boolean;
  interval_seconds: number;
  config_nonsecret: {
    [k: string]: unknown;
  };
  secret_references: {
    [k: string]: string;
  };
}
export interface ReplaceIntegrationConfigurationRequest {
  enabled: boolean;
  interval_seconds: number;
  config_nonsecret: {
    [k: string]: unknown;
  };
  secret_references: {
    [k: string]: string;
  };
}

export interface LiveInvalidationData {
  schema_version: 1;
  resource_id?: string;
  integration_id?: string;
  incident_id?: string;
  state?: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
  result?: string;
  changed_at: string;
}

export interface ManagedUser {
  id: string;
  username: string;
  display_name: string;
  email?: string;
  identity_provider: string;
  enabled: boolean;
  roles: string[];
  active_sessions: number;
  password_changed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface DurableMonitoringSignal {
  id: string;
  kind: "observation" | "freshness";
  integration_id: string;
  resource_id: string;
  observation_id?: string;
  check_type: string;
  state: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
  reason: string;
  reason_code?: string;
  occurred_at: string;
}

export interface NormalizedObservation {
  id?: string;
  external_resource_id: string;
  check_type: string;
  state: "healthy" | "warning" | "critical" | "unknown" | "maintenance" | "disabled";
  summary: string;
  observed_at: string;
  expected_refresh_seconds: number;
  measurements?: {
    [k: string]: number | string | boolean;
  };
  metadata?: {
    [k: string]: unknown;
  };
}

export interface MonitoringOverview {
  generated_at: string;
  /**
   * @maxItems 7
   */
  resource_counts: {
    state: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
    count: number;
  }[];
  /**
   * @maxItems 6
   */
  integration_counts: {
    state: "starting" | "healthy" | "unhealthy" | "stopped" | "disabled" | "not_started";
    count: number;
  }[];
  stale_count: number;
  unknown_count: number;
  /**
   * @maxItems 2
   */
  active_incident_counts?: {
    severity: "warning" | "critical";
    count: number;
  }[];
  /**
   * @maxItems 5
   */
  active_incidents?: {
    id: string;
    title: string;
    severity: "warning" | "critical";
    status: "open" | "acknowledged" | "investigating";
    integration_name: string;
    resource_name: string;
    detected_at: string;
    updated_at: string;
  }[];
  /**
   * @maxItems 10
   */
  recent_state_changes: {
    resource_id: string;
    integration_id: string;
    display_name: string;
    state: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
    reason: string;
    changed_at: string;
  }[];
}

export interface ResourceAPIView {
  id: string;
  integration_id: string;
  integration_name: string;
  external_id: string;
  kind: string;
  display_name: string;
  attributes: {
    [k: string]: unknown;
  };
  source_url?: string;
  first_seen_at: string;
  last_seen_at: string;
  health: {
    state: "healthy" | "warning" | "critical" | "unknown" | "stale" | "maintenance" | "disabled";
    reason: string;
    observation_id?: string;
    observed_at?: string;
    last_success_at?: string;
    stale_at?: string;
    unknown_at?: string;
    updated_at: string;
  };
  latest_observation?: {
    id: string;
    check_type: string;
    state: "healthy" | "warning" | "critical" | "unknown" | "maintenance" | "disabled";
    summary: string;
    observed_at: string;
    received_at: string;
    expected_refresh_seconds: number;
    measurements: {
      [k: string]: unknown;
    };
    metadata: {
      [k: string]: unknown;
    };
  };
}

export interface NormalizedResource {
  id?: string;
  external_id: string;
  kind: string;
  display_name: string;
  observed_at: string;
  attributes?: {
    [k: string]: unknown;
  };
  source_url?: string;
}

export interface RoleView {
  name: string;
  permissions: string[];
}

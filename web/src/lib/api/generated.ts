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
  protocol_versions: [string, ...string[]];
  integration_category: string;
  resource_types?: string[];
  check_types?: string[];
  /**
   * @minItems 1
   */
  capabilities: [
    "collect" | "events" | "notifications" | "actions",
    ...("collect" | "events" | "notifications" | "actions")[]
  ];
  read_only?: boolean;
  config_schema: {
    [k: string]: unknown;
  };
  secret_fields?: string[];
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

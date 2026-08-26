import type { ClientProblem } from '$lib/api/client';

export type ClusterFormFeedback = {
  title: string;
  details: string[];
  requestID?: string;
};

const operationalMessages: Record<string, string> = {
  target_not_approved:
    'This host or port is outside the Proxmox targets allowed by this Espial deployment.',
  unauthorized:
    'Proxmox rejected the token. Check the full token ID and token secret.',
  forbidden:
    'The token is valid but does not have permission to read every required Proxmox endpoint.',
  throttled:
    'Proxmox rate-limited the validation request. Wait briefly, then try again.',
  timeout:
    'Proxmox did not finish responding before the configured request timeout.',
  temporary_upstream_failure:
    'Proxmox returned a temporary server error. Check the cluster and try again.',
  malformed_data:
    'Proxmox responded, but its data could not be read as a supported API response.',
  oversized_response: 'The Proxmox response exceeded Espial’s safety limit.',
  tls_certificate_untrusted:
    'The HTTPS certificate is not trusted. If this is the expected self-signed certificate, enable “Trust this cluster’s self-signed certificate.”',
  tls_hostname_mismatch:
    'The HTTPS certificate does not match the host in the Base URL.',
  tls_certificate_invalid:
    'The HTTPS certificate is expired or not currently valid. Replace it, correct the cluster clock, or use a valid certificate.',
  tls_certificate_changed:
    'The HTTPS certificate no longer matches the certificate pinned when this cluster was added.',
  tls_certificate_missing:
    'The Proxmox endpoint did not present a certificate that Espial could pin.',
  tls_handshake_failed:
    'The endpoint did not complete a valid TLS handshake. Check the URL scheme, port, and Proxmox HTTPS listener.',
  dns_failed:
    'The host in the Base URL could not be resolved from Espial Core.',
  connection_failed:
    'Espial Core could not connect to this host and port. Check the Base URL, firewall, and Proxmox API listener.',
  proxmox_api_not_found:
    'The server was reached, but the expected Proxmox API endpoint was not found. Check that the Base URL points to Proxmox VE.',
  upstream_request_rejected:
    'Proxmox rejected a required API request. Check the token scope and Proxmox version.',
  proxmox_validation_failed:
    'Espial could not validate this target as a supported Proxmox cluster.',
  secret_store_unavailable:
    'The cluster was validated, but Espial could not safely store the token secret.',
  adapter_not_registered:
    'The Proxmox adapter is not available in this Espial deployment.',
  invalid_integration_config:
    'The cluster was validated, but Espial could not save the integration configuration.',
  correlation_id_failed:
    'Espial could not create the audit correlation for this cluster. Try again.',
};

const fieldMessages: Record<string, Record<string, string>> = {
  body: {
    required_fields: 'Complete every required connection and credential field.',
  },
  approve_target: {
    required: 'Approve this exact target before Espial connects to it.',
  },
  base_url: {
    invalid:
      'Enter only the Proxmox origin, such as https://proxmox.example:8006, with no path, query, or credentials.',
    invalid_scheme:
      'Use an https:// URL. HTTP requires the separate insecure HTTP approval.',
    requires_https: 'Use an https:// URL or explicitly allow insecure HTTP.',
  },
  trust_self_signed_certificate: {
    requires_https:
      'Self-signed certificate trust applies only to an https:// Base URL.',
  },
  deployment_id: {
    invalid:
      'Deployment ID must be 2–63 lowercase letters, numbers, or hyphens, beginning with a letter.',
  },
  token_id: {
    invalid:
      'Enter the complete Proxmox token ID, for example espial@pve!monitoring.',
  },
  token_secret: {
    invalid: 'Enter a token secret without line breaks.',
  },
  expected_refresh_seconds: {
    out_of_range: 'Expected freshness must be between 1 and 86,400 seconds.',
  },
  request_timeout_seconds: {
    out_of_range: 'Request timeout must be between 1 and 60 seconds.',
  },
  storage_thresholds: {
    invalid_range:
      'Storage warning must be lower than critical, and both must be from 0% to 100%.',
  },
  swap_thresholds: {
    invalid_range:
      'Swap warning must be lower than critical, and both must be from 0% to 100%.',
  },
  io_wait_thresholds: {
    invalid_range:
      'IO wait warning must be lower than critical, and both must be from 0% to 100%.',
  },
  expected_running_guests: {
    invalid:
      'Expected-running guests must use qemu:<id> or lxc:<id>, separated by commas.',
  },
};

export function clusterFormFeedback(
  problem: ClientProblem,
): ClusterFormFeedback {
  const details = (problem.fields ?? []).map(({ field, code }) => {
    if (field === 'proxmox' && operationalMessages[code]) {
      return operationalMessages[code];
    }
    return (
      fieldMessages[field]?.[code] ??
      `Check ${field.replaceAll('_', ' ')} (${code.replaceAll('_', ' ')}).`
    );
  });
  if (details.length === 0) {
    details.push(
      operationalMessages[problem.code] ??
        problem.message ??
        'The cluster could not be added.',
    );
  }
  return {
    title:
      problem.status === 0
        ? 'Espial Core could not be reached.'
        : 'Cluster validation failed.',
    details: [...new Set(details)],
    ...(problem.request_id ? { requestID: problem.request_id } : {}),
  };
}

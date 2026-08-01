import http from 'node:http';

const port = Number.parseInt(process.argv[2] ?? '18081', 10);
let eventMode = 'quiet';
let apiMode = 'ok';
let sessionMode = 'ok';
let streamConnections = 0;
let monitoringReads = 0;
let eventID = 0;
let incidentStatus = 'open';
let incidentVersion = 1;

const now = '2026-07-31T12:00:00Z';
const integrationID = '60000000-0000-4000-8000-000000000001';
const incidentID = '80000000-0000-4000-8000-000000000021';
let incidentTimeline = initialIncidentTimeline();
let users = initialUsers();
let auditEvents = initialAuditEvents();
const resources = [
  resource(
    '61000000-0000-4000-8000-000000000001',
    'core-switch-1',
    'Core switch 1',
    'network_device',
    'healthy',
    'All checks passed.',
  ),
  resource(
    '61000000-0000-4000-8000-000000000002',
    'compute-node-2',
    'Compute node 2',
    'server',
    'warning',
    'Packet loss exceeds the warning threshold.',
  ),
  resource(
    '61000000-0000-4000-8000-000000000003',
    'archive-node-3',
    'Archive node 3',
    'server',
    'stale',
    'Expected refresh window was missed.',
  ),
  resource(
    '61000000-0000-4000-8000-000000000004',
    'unknown-node-4',
    'Unknown node 4',
    'server',
    'unknown',
    'No trustworthy current observation.',
  ),
];

const server = http.createServer((request, response) => {
  const url = new URL(request.url ?? '/', `http://127.0.0.1:${port}`);
  if (url.pathname === '/__test/control') {
    eventMode = url.searchParams.get('events') ?? eventMode;
    apiMode = url.searchParams.get('api') ?? apiMode;
    sessionMode = url.searchParams.get('session') ?? sessionMode;
    if (url.searchParams.get('reset') === 'true') {
      eventMode = 'quiet';
      apiMode = 'ok';
      sessionMode = 'ok';
      streamConnections = 0;
      monitoringReads = 0;
      eventID = 0;
      incidentStatus = 'open';
      incidentVersion = 1;
      incidentTimeline = initialIncidentTimeline();
      users = initialUsers();
      auditEvents = initialAuditEvents();
    }
    return json(response, 200, state());
  }
  if (url.pathname === '/__test/state') return json(response, 200, state());
  if (url.pathname === '/api/v1/auth/session') {
    if (sessionMode === 'unauthorized')
      return apiError(response, 401, 'unauthenticated', 'Sign in is required.');
    if (sessionMode === 'unavailable')
      return apiError(response, 503, 'unavailable', 'Core is unavailable.');
    return json(response, 200, {
      user: {
        id: '70000000-0000-4000-8000-000000000001',
        username: 'admin',
        display_name: 'NOC Administrator',
        roles: ['administrator'],
        permissions: [
          'overview:read',
          'resources:read',
          'integrations:read',
          'incidents:read',
          ...(sessionMode === 'viewer'
            ? []
            : ['incidents:operate', 'audit:read', 'users:manage']),
        ],
      },
      expires_at: '2026-07-31T18:00:00Z',
      capabilities: { local: true, sso: false },
    });
  }
  if (url.pathname === '/api/v1/auth/capabilities')
    return json(response, 200, { local: true, sso: false });
  if (url.pathname === '/api/v1/auth/logout') return empty(response, 204);
  if (url.pathname === '/api/v1/events/stream')
    return stream(request, response);
  if (
    url.pathname.startsWith('/api/v1/overview') ||
    url.pathname.startsWith('/api/v1/resources') ||
    url.pathname.startsWith('/api/v1/integrations') ||
    url.pathname.startsWith('/api/v1/incidents')
  ) {
    monitoringReads += 1;
    if (apiMode === 'forbidden')
      return apiError(
        response,
        403,
        'forbidden',
        'Your account does not have permission to view this data.',
      );
    if (apiMode === 'unavailable')
      return apiError(
        response,
        503,
        'unavailable',
        'Espial Core is temporarily unavailable.',
      );
  }
  if (url.pathname === '/api/v1/overview') {
    return json(response, 200, {
      generated_at: now,
      resource_counts: [
        { state: 'healthy', count: 1 },
        { state: 'warning', count: 1 },
        { state: 'stale', count: 1 },
        { state: 'unknown', count: 1 },
      ],
      integration_counts: [{ state: 'healthy', count: 1 }],
      stale_count: 1,
      unknown_count: 1,
      active_incident_counts: [
        { severity: 'critical', count: 1 },
        { severity: 'warning', count: 0 },
      ],
      active_incidents: [
        {
          id: incidentID,
          title: 'Compute node 2: availability',
          severity: 'critical',
          status: 'open',
          integration_name: 'Sample infrastructure',
          resource_name: 'Compute node 2',
          detected_at: '2026-07-31T11:55:00Z',
          updated_at: now,
        },
      ],
      recent_state_changes: [],
    });
  }
  if (url.pathname === '/api/v1/resources') {
    let items = [...resources];
    const stateFilter = url.searchParams.get('state');
    const kind = url.searchParams.get('kind');
    const stale = url.searchParams.get('stale');
    if (stateFilter)
      items = items.filter((item) => item.health.state === stateFilter);
    if (kind) items = items.filter((item) => item.kind === kind);
    if (stale === 'true')
      items = items.filter((item) => item.health.state === 'stale');
    if (stale === 'false')
      items = items.filter((item) => item.health.state !== 'stale');
    return json(response, 200, { items });
  }
  if (url.pathname === '/api/v1/integrations') {
    return json(response, 200, {
      items: [
        {
          id: integrationID,
          adapter_id: 'org.ubnetdef.espial.sample',
          display_name: 'Sample infrastructure',
          enabled: true,
          interval_seconds: 60,
          config_keys: ['scenario'],
          secret_reference_keys: [],
          runtime_state: 'healthy',
          resource_count: 4,
          stale_count: 1,
          unknown_count: 1,
          last_collection: {
            started_at: '2026-07-31T11:59:59Z',
            completed_at: now,
            duration_ms: 850,
            result: 'succeeded',
            resource_count: 4,
            observation_count: 4,
            observations_inserted: 4,
            duplicate_observations: 0,
          },
          created_at: '2026-07-31T10:00:00Z',
          updated_at: now,
        },
      ],
    });
  }
  if (url.pathname === '/api/v1/incidents' && request.method === 'GET') {
    const active = url.searchParams.get('active') !== 'false';
    const items = active ? [incidentSummary()] : [recoveredIncidentSummary()];
    const severity = url.searchParams.get('severity');
    const status = url.searchParams.get('status');
    return json(response, 200, {
      items: items.filter(
        (item) =>
          (!severity || item.severity === severity) &&
          (!status || item.status === status),
      ),
    });
  }
  if (url.pathname === '/api/v1/incident-assignees') {
    if (sessionMode === 'viewer')
      return apiError(
        response,
        403,
        'forbidden',
        'You do not have permission to perform this action.',
      );
    return json(response, 200, {
      items: [
        {
          id: '70000000-0000-4000-8000-000000000001',
          display_name: 'NOC Administrator',
        },
      ],
    });
  }
  if (url.pathname === `/api/v1/incidents/${incidentID}`) {
    return json(
      response,
      200,
      {
        ...incidentSummary(),
        fingerprint: 'default:compute-node-2:availability',
      },
      'browser-incident-detail',
      `"v${incidentVersion.toString(16)}"`,
    );
  }
  if (url.pathname === `/api/v1/incidents/${incidentID}/timeline`) {
    return json(response, 200, {
      items: incidentTimeline,
    });
  }
  const incidentAction = url.pathname.match(
    new RegExp(
      `^/api/v1/incidents/${incidentID}/(acknowledge|investigate|notes|resolve)$`,
    ),
  );
  const ownerAction =
    url.pathname === `/api/v1/incidents/${incidentID}/owner` &&
    request.method === 'PUT';
  if (incidentAction || ownerAction) {
    if (sessionMode === 'viewer')
      return apiError(
        response,
        403,
        'forbidden',
        'You do not have permission to perform this action.',
      );
    if (apiMode === 'conflict-once') {
      apiMode = 'ok';
      return apiError(
        response,
        412,
        'precondition_failed',
        'The incident changed; fetch it and review the current state before retrying.',
      );
    }
    return readJSON(request).then((body) => {
      const requestID =
        request.headers['x-request-id'] ?? 'browser-incident-action';
      const action = ownerAction ? 'owner' : incidentAction[1];
      if (action === 'acknowledge') incidentStatus = 'acknowledged';
      if (action === 'investigate') incidentStatus = 'investigating';
      if (action === 'resolve') incidentStatus = 'resolved';
      incidentVersion += 1;
      const kind =
        action === 'acknowledge'
          ? 'acknowledged'
          : action === 'investigate'
            ? 'investigating'
            : action === 'owner'
              ? 'assigned'
              : action === 'notes'
                ? 'note'
                : 'resolved';
      const timelineID = `81000000-0000-4000-8000-${String(incidentTimeline.length + 22).padStart(12, '0')}`;
      incidentTimeline.unshift({
        id: timelineID,
        incident_id: incidentID,
        actor_user_id: '70000000-0000-4000-8000-000000000001',
        actor_name: 'NOC Administrator',
        kind,
        summary:
          body.note ??
          (action === 'owner'
            ? 'Assigned to NOC Administrator.'
            : `${kind.charAt(0).toUpperCase()}${kind.slice(1)} by NOC Administrator.`),
        occurred_at: now,
      });
      auditEvents.unshift(incidentAudit(requestID, action));
      return json(
        response,
        200,
        {
          incident_id: incidentID,
          status: incidentStatus,
          version: incidentVersion,
          timeline_event_id: timelineID,
          request_id: requestID,
          replayed: false,
          audit_url: `/audit?correlation_id=${encodeURIComponent(requestID)}`,
        },
        requestID,
        `"v${incidentVersion.toString(16)}"`,
      );
    });
  }
  if (url.pathname === '/api/v1/audit' && request.method === 'GET') {
    const correlationID = url.searchParams.get('correlation_id');
    return json(response, 200, {
      items: correlationID
        ? auditEvents.filter((event) => event.correlation_id === correlationID)
        : auditEvents,
      from: '2026-07-30T12:00:00Z',
      to: now,
    });
  }
  if (url.pathname === '/api/v1/roles' && request.method === 'GET') {
    return json(response, 200, {
      items: [
        { name: 'administrator', permissions: ['users:manage', 'audit:read'] },
        { name: 'operator', permissions: ['resources:read'] },
        { name: 'viewer', permissions: ['resources:read'] },
      ],
    });
  }
  if (url.pathname === '/api/v1/users' && request.method === 'GET') {
    return json(response, 200, { items: users });
  }
  if (url.pathname === '/api/v1/users' && request.method === 'POST') {
    return readJSON(request).then((body) => {
      const requestID =
        request.headers['x-request-id'] ?? 'browser-user-create';
      const created = {
        id: `70000000-0000-4000-8000-${String(users.length + 1).padStart(12, '0')}`,
        username: body.username,
        display_name: body.display_name,
        ...(body.email ? { email: body.email } : {}),
        identity_provider: 'local',
        enabled: true,
        roles: [body.role],
        active_sessions: 0,
        created_at: now,
        updated_at: now,
      };
      users.push(created);
      auditEvents.unshift(
        userAudit(requestID, 'auth.local.user.created', created),
      );
      return json(response, 201, created, requestID, `"${now}"`);
    });
  }
  const userMatch = url.pathname.match(
    /^\/api\/v1\/users\/([0-9a-f-]+)(\/password)?$/,
  );
  if (userMatch && request.method === 'PUT' && !userMatch[2]) {
    return readJSON(request).then((body) => {
      const user = users.find((candidate) => candidate.id === userMatch[1]);
      if (!user)
        return apiError(response, 404, 'not_found', 'The user was not found.');
      Object.assign(user, {
        display_name: body.display_name,
        ...(body.email ? { email: body.email } : {}),
        enabled: body.enabled,
        roles: [body.role],
        updated_at: '2026-07-31T12:01:00Z',
      });
      if (!body.email) delete user.email;
      const requestID =
        request.headers['x-request-id'] ?? 'browser-user-update';
      auditEvents.unshift(userAudit(requestID, 'auth.user.updated', user));
      return json(response, 200, user, requestID, `"${user.updated_at}"`);
    });
  }
  if (userMatch && request.method === 'POST' && userMatch[2]) {
    const user = users.find((candidate) => candidate.id === userMatch[1]);
    if (!user)
      return apiError(response, 404, 'not_found', 'The user was not found.');
    const requestID =
      request.headers['x-request-id'] ?? 'browser-password-reset';
    auditEvents.unshift(userAudit(requestID, 'auth.password.reset', user));
    return empty(response, 204, requestID);
  }
  apiError(response, 404, 'not_found', 'The requested record was not found.');
});

server.listen(port, '127.0.0.1', () => {
  process.stdout.write(`mock Core listening on ${port}\n`);
});

function stream(_request, response) {
  streamConnections += 1;
  if (eventMode === 'unauthorized')
    return apiError(response, 401, 'unauthenticated', 'Sign in is required.');
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  response.write(': connected\n\n');
  if (eventMode === 'disconnect-once' && streamConnections === 1) {
    return setTimeout(() => response.destroy(), 100);
  }
  if (eventMode === 'resync' && streamConnections === 1) {
    eventID += 1;
    response.write(
      `id: ${eventID}\nevent: resync_required\ndata: ${JSON.stringify({ schema_version: 1, changed_at: now })}\n\n`,
    );
    return response.end();
  }
  const eventTimer =
    eventMode === 'quiet'
      ? null
      : setTimeout(() => {
          if (response.destroyed) return;
          eventID += 1;
          response.write(
            `id: ${eventID}\nevent: integration_changed\ndata: ${JSON.stringify({ schema_version: 1, integration_id: integrationID, result: 'collected', changed_at: now })}\n\n`,
          );
        }, 250);
  const heartbeat = setInterval(() => {
    if (!response.destroyed) response.write(': heartbeat\n\n');
  }, 500);
  response.on('close', () => {
    if (eventTimer) clearTimeout(eventTimer);
    clearInterval(heartbeat);
  });
}

function resource(id, externalID, displayName, kind, state, reason) {
  return {
    id,
    integration_id: integrationID,
    integration_name: 'Sample infrastructure',
    external_id: externalID,
    kind,
    display_name: displayName,
    attributes: {},
    first_seen_at: '2026-07-31T10:00:00Z',
    last_seen_at: now,
    health: { state, reason, observed_at: now, updated_at: now },
  };
}

function incidentSummary() {
  return {
    id: incidentID,
    rule_id: '20000000-0000-4000-8000-000000000001',
    rule_name: 'Default resource health',
    integration_id: integrationID,
    integration_name: 'Sample infrastructure',
    resource_id: '61000000-0000-4000-8000-000000000002',
    resource_name: 'Compute node 2',
    check_type: 'availability',
    title: 'Compute node 2: availability',
    summary: 'Host unreachable.',
    severity: 'critical',
    status: incidentStatus,
    detected_at: '2026-07-31T11:55:00Z',
    latest_signal_at: now,
    version: incidentVersion,
    updated_at: now,
  };
}

function recoveredIncidentSummary() {
  return {
    ...incidentSummary(),
    id: '80000000-0000-4000-8000-000000000022',
    title: 'Archive node 3: availability',
    resource_id: '61000000-0000-4000-8000-000000000003',
    resource_name: 'Archive node 3',
    summary: 'Condition recovered: reachable.',
    status: 'recovered',
    recovered_at: now,
  };
}

function state() {
  return {
    eventMode,
    apiMode,
    sessionMode,
    streamConnections,
    monitoringReads,
    eventID,
    incidentStatus,
    incidentVersion,
    incidentAuditCount: auditEvents.filter(
      (event) => event.target_type === 'incident',
    ).length,
  };
}

function json(
  response,
  status,
  body,
  requestID = 'browser-test-request',
  etag,
) {
  response.writeHead(status, {
    'Content-Type': 'application/json',
    'X-Request-ID': requestID,
    ...(etag ? { ETag: etag } : {}),
  });
  response.end(JSON.stringify(body));
}

function apiError(response, status, code, message) {
  json(response, status, {
    error: { code, message, request_id: 'browser-test-request' },
  });
}

function empty(response, status, requestID = 'browser-test-request') {
  response.writeHead(status, { 'X-Request-ID': requestID });
  response.end();
}

function readJSON(request) {
  return new Promise((resolve, reject) => {
    let raw = '';
    request.setEncoding('utf8');
    request.on('data', (chunk) => (raw += chunk));
    request.on('end', () => {
      try {
        resolve(JSON.parse(raw || '{}'));
      } catch (error) {
        reject(error);
      }
    });
    request.on('error', reject);
  });
}

function initialUsers() {
  return [
    {
      id: '70000000-0000-4000-8000-000000000001',
      username: 'admin',
      display_name: 'NOC Administrator',
      email: 'admin@example.test',
      identity_provider: 'local',
      enabled: true,
      roles: ['administrator'],
      active_sessions: 1,
      password_changed_at: now,
      created_at: '2026-07-30T12:00:00Z',
      updated_at: now,
    },
  ];
}

function initialAuditEvents() {
  return [
    {
      id: '80000000-0000-4000-8000-000000000001',
      actor_user_id: '70000000-0000-4000-8000-000000000001',
      actor_username: 'admin',
      action: 'auth.local.bootstrap',
      target_type: 'user',
      target_id: '70000000-0000-4000-8000-000000000001',
      result: 'succeeded',
      correlation_id: 'bootstrap-browser-test',
      occurred_at: '2026-07-30T12:00:00Z',
    },
  ];
}

function initialIncidentTimeline() {
  return [
    {
      id: '81000000-0000-4000-8000-000000000021',
      incident_id: incidentID,
      kind: 'detected',
      to_status: 'open',
      to_severity: 'critical',
      summary: 'Incident detected: host unreachable',
      occurred_at: '2026-07-31T11:55:00Z',
    },
  ];
}

function incidentAudit(correlationID, action) {
  return {
    id: `80000000-0000-4000-8000-${String(auditEvents.length + 1).padStart(12, '0')}`,
    actor_user_id: '70000000-0000-4000-8000-000000000001',
    actor_username: 'admin',
    action: `incident.${action}`,
    target_type: 'incident',
    target_id: incidentID,
    result: 'succeeded',
    correlation_id: correlationID,
    after_summary: { status: incidentStatus, version: incidentVersion },
    occurred_at: now,
  };
}

function userAudit(correlationID, action, user) {
  return {
    id: `80000000-0000-4000-8000-${String(auditEvents.length + 1).padStart(12, '0')}`,
    actor_user_id: '70000000-0000-4000-8000-000000000001',
    actor_username: 'admin',
    action,
    target_type: 'user',
    target_id: user.id,
    result: 'succeeded',
    correlation_id: correlationID,
    after_summary: {
      username: user.username,
      role: user.roles[0],
      enabled: user.enabled,
    },
    occurred_at: now,
  };
}

const shutdown = () => server.close(() => process.exit(0));
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

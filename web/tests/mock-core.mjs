import http from 'node:http';

const port = Number.parseInt(process.argv[2] ?? '18081', 10);
let eventMode = 'quiet';
let apiMode = 'ok';
let sessionMode = 'ok';
let streamConnections = 0;
let monitoringReads = 0;
let eventID = 0;

const now = '2026-07-31T12:00:00Z';
const integrationID = '60000000-0000-4000-8000-000000000001';
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
        username: 'operator',
        display_name: 'NOC Operator',
        roles: ['operator'],
        permissions: ['overview:read', 'resources:read', 'integrations:read'],
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
    url.pathname.startsWith('/api/v1/integrations')
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

function state() {
  return {
    eventMode,
    apiMode,
    sessionMode,
    streamConnections,
    monitoringReads,
    eventID,
  };
}

function json(response, status, body) {
  response.writeHead(status, {
    'Content-Type': 'application/json',
    'X-Request-ID': 'browser-test-request',
  });
  response.end(JSON.stringify(body));
}

function apiError(response, status, code, message) {
  json(response, status, {
    error: { code, message, request_id: 'browser-test-request' },
  });
}

function empty(response, status) {
  response.writeHead(status);
  response.end();
}

const shutdown = () => server.close(() => process.exit(0));
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

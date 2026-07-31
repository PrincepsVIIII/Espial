import { randomBytes } from "node:crypto";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const temporaryRoot = mkdtempSync(path.join(os.tmpdir(), "espial-phase1-"));
const suffix = randomBytes(4).toString("hex");
const projects = [];
const adminPassword = `Phase1 administrator ${randomBytes(18).toString("base64url")}`;
const viewerPassword = `Phase1 viewer ${randomBytes(18).toString("base64url")}`;

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function availablePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close((error) => {
        if (error) reject(error);
        else resolve(address.port);
      });
    });
  });
}

function createStack(name) {
  const directory = path.join(temporaryRoot, name);
  const password = randomBytes(24).toString("base64url");
  const environmentPath = path.join(temporaryRoot, `${name}.env`);
  const dsnPath = path.join(temporaryRoot, `${name}.dsn`);
  return Promise.all([availablePort(), availablePort(), availablePort()]).then(
    ([webPort, corePort, postgresPort]) => {
      const project = `espial-phase1-${suffix}-${name}`;
      writeFileSync(
        environmentPath,
        [
          "ESPIAL_ENV=development",
          `ESPIAL_PUBLIC_URL=http://localhost:${webPort}`,
          "ESPIAL_AUTH_MODE=local",
          "ESPIAL_AUTH_SESSION_IDLE=30m",
          "ESPIAL_AUTH_SESSION_ABSOLUTE=12h",
          "ESPIAL_AUTH_FAILURE_LIMIT=5",
          "ESPIAL_AUTH_LOCKOUT_DURATION=15m",
          "ESPIAL_AUTH_LOGIN_RATE_LIMIT=100",
          "ESPIAL_AUTH_LOGIN_RATE_WINDOW=1m",
          `ESPIAL_DATABASE_DSN_SECRET_FILE=${dsnPath}`,
          "ESPIAL_DATABASE_MIGRATE_ON_START=true",
          "ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS=20",
          "ESPIAL_SAMPLE_ADAPTER_EXECUTABLE=/espial-sample-adapter",
          "ESPIAL_ADAPTER_GLOBAL_CONCURRENCY=4",
          "ESPIAL_ADAPTER_RECONCILE_INTERVAL=500ms",
          "ESPIAL_FRESHNESS_INTERVAL=500ms",
          "ESPIAL_FRESHNESS_BATCH_SIZE=100",
          "ESPIAL_EVENT_REPLAY_SIZE=1024",
          "ESPIAL_SSE_HEARTBEAT=1s",
          "ESPIAL_SSE_MAX_CLIENTS=100",
          "POSTGRES_DB=espial",
          "POSTGRES_USER=espial",
          `POSTGRES_PASSWORD=${password}`,
          `POSTGRES_PORT=${postgresPort}`,
          `ESPIAL_CORE_PORT=${corePort}`,
          `ESPIAL_WEB_PORT=${webPort}`,
          "",
        ].join("\n"),
        { mode: 0o600 },
      );
      writeFileSync(
        dsnPath,
        `postgres://espial:${encodeURIComponent(password)}@postgres:5432/espial?sslmode=disable\n`,
        { mode: 0o600 },
      );
      const stack = {
        directory,
        project,
        environmentPath,
        webPort,
        corePort,
        webURL: `http://localhost:${webPort}`,
        coreURL: `http://127.0.0.1:${corePort}`,
      };
      projects.push(stack);
      return stack;
    },
  );
}

function compose(stack, args, options = {}) {
  assert(
    stack.project.startsWith("espial-phase1-"),
    "refusing unsafe Compose project name",
  );
  const command = [
    "compose",
    "--project-name",
    stack.project,
    "--env-file",
    stack.environmentPath,
    "-f",
    "deployments/local/compose.yml",
    ...args,
  ];
  const result = spawnSync("docker", command, {
    cwd: root,
    input: options.input,
    encoding: options.capture || options.input ? "utf8" : undefined,
    stdio: options.capture
      ? [options.input ? "pipe" : "ignore", "pipe", "pipe"]
      : options.input
        ? ["pipe", "inherit", "inherit"]
        : "inherit",
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const diagnostic = options.capture ? `\n${result.stderr}` : "";
    throw new Error(
      `docker ${command.join(" ")} failed with ${result.status}${diagnostic}`,
    );
  }
  return result.stdout ?? "";
}

async function waitFor(description, operation, timeout = 30_000) {
  const deadline = Date.now() + timeout;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const value = await operation();
      if (value) return value;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(
    `timed out waiting for ${description}${lastError ? `: ${lastError.message}` : ""}`,
  );
}

async function waitHTTP(url) {
  return waitFor(
    url,
    async () => {
      const response = await fetch(url);
      return response.ok;
    },
    90_000,
  );
}

function cookieJar(response) {
  const cookies = new Map();
  for (const value of response.headers.getSetCookie()) {
    const [pair] = value.split(";", 1);
    const separator = pair.indexOf("=");
    cookies.set(pair.slice(0, separator), pair.slice(separator + 1));
  }
  return cookies;
}

function cookieHeader(cookies) {
  return [...cookies].map(([name, value]) => `${name}=${value}`).join("; ");
}

async function login(stack, username, password) {
  const response = await fetch(`${stack.webURL}/api/v1/auth/local/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Origin: stack.webURL,
    },
    body: JSON.stringify({ username, password }),
  });
  assert(
    response.status === 200,
    `login for ${username} returned ${response.status}`,
  );
  return cookieJar(response);
}

async function api(stack, cookies, pathname, options = {}) {
  const headers = new Headers(options.headers);
  headers.set("Cookie", cookieHeader(cookies));
  const response = await fetch(`${stack.webURL}${pathname}`, {
    ...options,
    headers,
  });
  return response;
}

function mutationHeaders(stack, cookies, additional = {}) {
  return {
    "Content-Type": "application/json",
    Origin: stack.webURL,
    "X-CSRF-Token": cookies.get("espial_csrf"),
    ...additional,
  };
}

async function createIntegration(stack, cookies, displayName, scenario) {
  const response = await api(stack, cookies, "/api/v1/integrations", {
    method: "POST",
    headers: mutationHeaders(stack, cookies),
    body: JSON.stringify({
      adapter_id: "org.ubnetdef.espial.sample",
      display_name: displayName,
      enabled: true,
      interval_seconds: 1,
      config_nonsecret: {
        scenario,
        count: 1,
        fault_mode: "none",
        expected_refresh_seconds: 20,
      },
      secret_references: {},
    }),
  });
  const body = await response.json().catch(() => ({}));
  assert(
    response.status === 201,
    `integration creation returned ${response.status}: ${JSON.stringify(body)}`,
  );
  return body.id;
}

async function integration(stack, cookies, id) {
  const response = await api(stack, cookies, `/api/v1/integrations/${id}`);
  assert(response.ok, `integration read returned ${response.status}`);
  return { value: await response.json(), etag: response.headers.get("etag") };
}

async function updateIntegration(stack, cookies, id, enabled, config) {
  const current = await integration(stack, cookies, id);
  const response = await api(
    stack,
    cookies,
    `/api/v1/integrations/${id}/configuration`,
    {
      method: "PUT",
      headers: mutationHeaders(stack, cookies, { "If-Match": current.etag }),
      body: JSON.stringify({
        enabled,
        interval_seconds: 1,
        config_nonsecret: config,
        secret_references: {},
      }),
    },
  );
  assert(
    response.status === 204,
    `integration update returned ${response.status}: ${await response.text()}`,
  );
}

async function resourceStates(stack, cookies, integrationID) {
  const response = await api(
    stack,
    cookies,
    `/api/v1/resources?integration=${encodeURIComponent(integrationID)}&limit=100`,
  );
  assert(response.ok, `resource list returned ${response.status}`);
  const body = await response.json();
  return body.items.map((item) => item.health.state);
}

async function waitForSSE(stack, cookies, operation) {
  const controller = new AbortController();
  const response = await fetch(`${stack.webURL}/api/v1/events/stream`, {
    headers: { Cookie: cookieHeader(cookies) },
    signal: controller.signal,
  });
  assert(
    response.ok && response.body,
    `SSE connection returned ${response.status}`,
  );
  const reader = response.body.getReader();
  const event = (async () => {
    const decoder = new TextDecoder();
    let body = "";
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      const { done, value } = await reader.read();
      if (done) break;
      body += decoder.decode(value, { stream: true });
      if (body.includes("event:") && body.includes("data:")) return body;
    }
    throw new Error("timed out waiting for SSE invalidation");
  })();
  await operation();
  try {
    return await event;
  } finally {
    controller.abort();
  }
}

function adminCommand(stack, args, password) {
  const input =
    password === undefined ? undefined : `${password}\n${password}\n`;
  return compose(stack, ["run", "--rm", "-T", "core", "admin", ...args], {
    input,
    capture: true,
  });
}

async function startStack(stack) {
  compose(stack, ["up", "-d", "--build"]);
  await waitHTTP(`${stack.coreURL}/api/v1/health/ready`);
  await waitHTTP(`${stack.webURL}/`);
}

async function runAcceptance() {
  const primary = await createStack("primary");
  process.stdout.write(
    "[1/10] Starting a clean stack and applying migrations\n",
  );
  await startStack(primary);

  process.stdout.write(
    "[2/10] Bootstrapping an administrator through hidden stdin\n",
  );
  adminCommand(primary, ["bootstrap", "--username", "admin"], adminPassword);

  process.stdout.write(
    "[3/10] Signing in through Web and proving anonymous denial\n",
  );
  const anonymous = await fetch(`${primary.coreURL}/api/v1/overview`);
  assert(
    anonymous.status === 401,
    `anonymous Core request returned ${anonymous.status}`,
  );
  const admin = await login(primary, "admin", adminPassword);

  process.stdout.write(
    "[4/10] Collecting authoritative healthy and warning resources\n",
  );
  const healthyID = await createIntegration(
    primary,
    admin,
    "Acceptance healthy",
    "healthy",
  );
  const warningID = await createIntegration(
    primary,
    admin,
    "Acceptance warning",
    "warning",
  );
  await waitFor(
    "healthy sample resource",
    async () =>
      (await resourceStates(primary, admin, healthyID)).includes("healthy"),
    30_000,
  );
  await waitFor(
    "warning sample resource",
    async () =>
      (await resourceStates(primary, admin, warningID)).includes("warning"),
    30_000,
  );
  const dashboard = await api(primary, admin, "/dashboard");
  const dashboardHTML = await dashboard.text();
  assert(
    dashboard.ok &&
      dashboardHTML.includes("Acceptance healthy") &&
      dashboardHTML.includes("Acceptance warning"),
    "Dashboard did not render collected integration data",
  );

  process.stdout.write(
    "[5/10] Crashing an adapter and observing stale then unknown state\n",
  );
  const healthyConfig = {
    scenario: "healthy",
    count: 1,
    fault_mode: "none",
    expected_refresh_seconds: 20,
  };
  const faultConfig = { ...healthyConfig, fault_mode: "crash_before_response" };
  await updateIntegration(primary, admin, healthyID, false, faultConfig);
  await waitFor("adapter stop", async () => {
    const current = await integration(primary, admin, healthyID);
    return current.value.instance?.state === "stopped";
  });
  await updateIntegration(primary, admin, healthyID, true, faultConfig);
  await waitFor(
    "isolated adapter failure",
    async () => {
      const current = await integration(primary, admin, healthyID);
      const ready = await fetch(`${primary.coreURL}/api/v1/health/ready`);
      return (
        current.value.instance?.consecutive_failures > 0 &&
        current.value.instance?.last_error_code &&
        ready.ok
      );
    },
    30_000,
  );
  await waitFor(
    "stale resource state",
    async () =>
      (await resourceStates(primary, admin, healthyID)).includes("stale"),
    58_000,
  );
  await waitFor(
    "unknown resource state",
    async () =>
      (await resourceStates(primary, admin, healthyID)).includes("unknown"),
    20_000,
  );

  process.stdout.write(
    "[6/10] Recovering the adapter and receiving an SSE invalidation\n",
  );
  await updateIntegration(primary, admin, healthyID, false, healthyConfig);
  await waitFor("failed adapter stop", async () => {
    const current = await integration(primary, admin, healthyID);
    return current.value.instance?.state === "stopped";
  });
  const liveEvent = await waitForSSE(primary, admin, () =>
    updateIntegration(primary, admin, healthyID, true, healthyConfig),
  );
  assert(
    liveEvent.includes("integration_changed"),
    "recovery did not emit an integration SSE invalidation",
  );
  await waitFor(
    "recovered healthy resource",
    async () =>
      (await resourceStates(primary, admin, healthyID)).includes("healthy"),
    30_000,
  );

  process.stdout.write(
    "[7/10] Assigning Viewer and proving direct administrator denial\n",
  );
  adminCommand(
    primary,
    ["user", "create", "--username", "phase-viewer", "--role", "operator"],
    viewerPassword,
  );
  adminCommand(primary, [
    "user",
    "role",
    "--username",
    "phase-viewer",
    "--role",
    "viewer",
  ]);
  const viewer = await login(primary, "phase-viewer", viewerPassword);
  const denied = await fetch(`${primary.coreURL}/api/v1/audit?limit=10`, {
    headers: { Cookie: cookieHeader(viewer) },
  });
  assert(denied.status === 403, `Viewer audit read returned ${denied.status}`);

  process.stdout.write(
    "[8/10] Reviewing redacted bootstrap/auth/lifecycle/role audit evidence\n",
  );
  const auditResponse = await api(primary, admin, "/api/v1/audit?limit=200");
  assert(auditResponse.ok, `audit read returned ${auditResponse.status}`);
  const audit = await auditResponse.json();
  const actions = new Set(audit.items.map((item) => item.action));
  for (const action of [
    "auth.local.bootstrap",
    "auth.login.succeeded",
    "auth.local.used",
    "auth.role.assigned",
    "integration.created",
    "integration.adapter.failed",
    "integration.adapter.recovered",
  ]) {
    assert(actions.has(action), `audit action ${action} was not recorded`);
  }
  assert(
    !JSON.stringify(audit).includes(adminPassword) &&
      !JSON.stringify(audit).includes(viewerPassword),
    "audit response disclosed a password",
  );

  process.stdout.write("[9/10] Proving bounded shutdown during collection\n");
  const delayedConfig = { ...healthyConfig, delay_ms: 5000 };
  await updateIntegration(primary, admin, healthyID, false, delayedConfig);
  await waitFor("adapter stop before delayed restart", async () => {
    const current = await integration(primary, admin, healthyID);
    return current.value.instance?.state === "stopped";
  });
  await updateIntegration(primary, admin, healthyID, true, delayedConfig);
  await waitFor(
    "delayed adapter start",
    async () =>
      (await integration(primary, admin, healthyID)).value.runtime_state ===
      "healthy",
  );
  await new Promise((resolve) => setTimeout(resolve, 1000));
  const shutdownStarted = Date.now();
  compose(primary, ["stop", "--timeout", "15", "core"]);
  const shutdownDuration = Date.now() - shutdownStarted;
  assert(shutdownDuration < 20_000, `Core shutdown took ${shutdownDuration}ms`);
  compose(primary, ["up", "-d", "core"]);
  await waitHTTP(`${primary.coreURL}/api/v1/health/ready`);

  process.stdout.write(
    "[10/10] Backing up and restoring auth/current state into a clean stack\n",
  );
  const dump = compose(
    primary,
    [
      "exec",
      "-T",
      "postgres",
      "pg_dump",
      "--username",
      "espial",
      "--dbname",
      "espial",
      "--clean",
      "--if-exists",
      "--no-owner",
      "--no-privileges",
    ],
    { capture: true },
  );
  assert(
    dump.includes("CREATE TABLE") && !dump.includes(adminPassword),
    "backup was empty or disclosed a password",
  );

  const restored = await createStack("restored");
  compose(restored, ["up", "-d", "postgres"]);
  await waitFor(
    "restored PostgreSQL readiness",
    async () => {
      try {
        compose(
          restored,
          [
            "exec",
            "-T",
            "postgres",
            "psql",
            "--username",
            "espial",
            "--dbname",
            "espial",
            "--no-psqlrc",
            "--tuples-only",
            "--command",
            "SELECT current_database()",
          ],
          { capture: true },
        );
        return true;
      } catch {
        return false;
      }
    },
    30_000,
  );
  compose(
    restored,
    [
      "exec",
      "-T",
      "postgres",
      "psql",
      "--username",
      "espial",
      "--dbname",
      "espial",
    ],
    {
      input: dump,
      capture: true,
    },
  );
  compose(restored, ["up", "-d", "--build"]);
  await waitHTTP(`${restored.coreURL}/api/v1/health/ready`);
  await waitHTTP(`${restored.webURL}/`);
  const restoredAdmin = await login(restored, "admin", adminPassword);
  const restoredResources = await api(
    restored,
    restoredAdmin,
    "/api/v1/resources?limit=100",
  );
  const restoredBody = await restoredResources.json();
  assert(
    restoredResources.ok && restoredBody.items.length >= 2,
    "restored stack lost current resource state",
  );

  const stats = compose(
    restored,
    [
      "stats",
      "--no-stream",
      "--format",
      "{{.Name}} {{.CPUPerc}} {{.MemUsage}}",
    ],
    { capture: true },
  );
  process.stdout.write(`\nAcceptance resource sample:\n${stats}\n`);
  process.stdout.write(
    `Phase 1 vertical acceptance passed; bounded Core shutdown was ${shutdownDuration}ms.\n`,
  );
}

try {
  await runAcceptance();
} finally {
  for (const stack of projects.reverse()) {
    try {
      compose(stack, ["down", "--volumes", "--remove-orphans"], {
        capture: true,
      });
    } catch (error) {
      process.stderr.write(
        `cleanup warning for ${stack.project}: ${error.message}\n`,
      );
    }
  }
  rmSync(temporaryRoot, { recursive: true, force: true });
}

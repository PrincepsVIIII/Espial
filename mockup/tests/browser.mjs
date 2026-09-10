import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { chromium } from "../../web/node_modules/playwright/index.mjs";
import AxeBuilder from "../../web/node_modules/@axe-core/playwright/dist/index.mjs";

// The prototype itself has no dependencies. Verification reuses the repository's
// existing browser tooling and starts an isolated, static-only preview server.
const root = fileURLToPath(new URL("../", import.meta.url));
const resultsDir = new URL("../test-results/", import.meta.url);
const port = process.env.MOCKUP_TEST_PORT || "4176";
const origin = `http://127.0.0.1:${port}`;
const routes = [
  "Dashboard",
  "Alerts",
  "Datacenter",
  "Hypervisor",
  "Webpages",
  "Audit",
];
const failures = [];
const browserErrors = [];
const forbiddenRequests = [];
const requestedPaths = new Set();
const checks = [];
let browser;
let page;
let server;

const visible = async (locator) => locator.waitFor({ state: "visible" });
const heading = (name, level = 1) =>
  page.getByRole("heading", { name, exact: true, level });
const dialog = () => page.getByRole("dialog");

async function reset() {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`${origin}/`, { waitUntil: "networkidle" });
  await visible(heading("Dashboard"));
}

async function navigate(name) {
  await page
    .getByRole("navigation", { name: "Primary", exact: true })
    .getByRole("link", { name, exact: true })
    .click();
  await visible(heading(name));
}

async function noOverflow(label) {
  const dimensions = await page.evaluate(() => ({
    width: window.innerWidth,
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
  }));
  assert.ok(
    dimensions.document <= dimensions.width &&
      dimensions.body <= dimensions.width,
    `${label}: horizontal document overflow ${JSON.stringify(dimensions)}`,
  );
}

async function accessible(label) {
  const result = await new AxeBuilder({ page }).analyze();
  const violations = result.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    description: violation.description,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      summary: node.failureSummary,
    })),
  }));
  await writeFile(
    new URL(`axe-${label}.json`, resultsDir),
    JSON.stringify(violations, null, 2),
  );
  assert.deepEqual(
    violations,
    [],
    `${label}: accessibility violations\n${JSON.stringify(violations, null, 2)}`,
  );
}

async function test(name, run, { fresh = true } = {}) {
  const started = Date.now();
  try {
    if (fresh) await reset();
    await run();
    checks.push({ name, passed: true, durationMs: Date.now() - started });
    console.log(`PASS ${name}`);
  } catch (error) {
    const context =
      page && !page.isClosed()
        ? await page
            .evaluate(() => ({
              url: location.href,
              focused: document.activeElement?.outerHTML.slice(0, 400),
              accountExpanded: document
                .querySelector(".account-trigger")
                ?.getAttribute("aria-expanded"),
              menuExpanded: document
                .querySelector(".menu-toggle")
                ?.getAttribute("aria-expanded"),
              dialogOpen: document.querySelector("dialog")?.open,
            }))
            .catch(() => null)
        : null;
    failures.push({ name, message: error.stack || String(error), context });
    checks.push({ name, passed: false, durationMs: Date.now() - started });
    console.error(
      `FAIL ${name}\n${error.stack || error}\n${JSON.stringify(context)}`,
    );
    if (page && !page.isClosed())
      await page
        .screenshot({
          path: fileURLToPath(
            new URL(`failure-${checks.length}.png`, resultsDir),
          ),
          fullPage: true,
        })
        .catch(() => {});
  }
}

async function startServer() {
  server = spawn(process.execPath, ["server.mjs"], {
    cwd: root,
    env: { ...process.env, PORT: port },
    stdio: ["ignore", "pipe", "pipe"],
  });
  await new Promise((resolve, reject) => {
    let log = "";
    const timeout = setTimeout(
      () => reject(new Error(`Preview server did not start: ${log}`)),
      10_000,
    );
    server.stdout.on("data", (chunk) => {
      log += chunk;
      if (log.includes("Espial design preview:")) {
        clearTimeout(timeout);
        resolve();
      }
    });
    server.stderr.on("data", (chunk) => {
      log += chunk;
    });
    server.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    server.once("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`Preview server exited (${code}): ${log}`));
    });
  });
}

try {
  await mkdir(resultsDir, { recursive: true });
  await startServer();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: "reduce",
  });
  context.on("request", (request) => {
    const url = new URL(request.url());
    const allowedPaths = new Set([
      "/",
      "/index.html",
      "/app.js",
      "/data.js",
      "/datacenter.js",
      "/styles.css",
      "/datacenter.css",
      "/assets/ubnetdef.png",
    ]);
    requestedPaths.add(url.pathname);
    if (
      url.origin !== origin ||
      !allowedPaths.has(url.pathname) ||
      request.method() !== "GET"
    )
      forbiddenRequests.push(`${request.method()} ${request.url()}`);
  });
  // Record attempted external/API requests above, and prevent a regression from
  // actually contacting an external service during these local-only checks.
  await context.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (
      url.origin !== origin ||
      url.pathname.startsWith("/api/") ||
      request.method() !== "GET"
    )
      await route.abort("blockedbyclient");
    else await route.continue();
  });
  page = await context.newPage();
  page.setDefaultTimeout(7000);
  page.on("pageerror", (error) => browserErrors.push(error.message));

  await test("Dashboard essentials and unmodified supplied logo", async () => {
    const source = await readFile(
      new URL("../../web/src/lib/components/BrandLogo.svelte", import.meta.url),
      "utf8",
    );
    const encodedLogo = source.match(
      /data:image\/png;base64,([A-Za-z0-9+/=]+)/,
    )?.[1];
    assert.ok(encodedLogo, "The supplied source logo must exist.");
    assert.deepEqual(
      await readFile(new URL("../assets/ubnetdef.png", import.meta.url)),
      Buffer.from(encodedLogo, "base64"),
    );
    await visible(page.getByRole("img", { name: "UBNetDef", exact: true }));
    for (const [name, percent] of [
      ["CPU", "38"],
      ["Memory", "62"],
      ["Storage", "66.5"],
    ]) {
      await visible(
        page.getByRole("link", {
          name: `Inspect ${name}: ${percent} percent used`,
          exact: true,
        }),
      );
    }
    await visible(
      page.getByRole("link", {
        name: "Inspect load balancing: uneven compute distribution",
        exact: true,
      }),
    );
    assert.match(
      await page.locator(".coverage").innerText(),
      /6 hosts\s*\/ 36 running VMs/,
    );
    assert.equal(
      await page
        .getByRole("navigation", { name: "Primary", exact: true })
        .getByRole("link")
        .allTextContents()
        .then((items) => items.map((item) => item.trim()).join("|")),
      routes.join("|"),
    );
    assert.match(
      await page.locator("footer").innerText(),
      /All data is fictional/,
    );
  });

  await test("Cluster scope changes capacities, host distribution, and attention together", async () => {
    await page
      .getByRole("combobox", { name: "Cluster scope" })
      .selectOption("Research");
    await visible(
      page.getByText("No active alerts in this cluster.", { exact: true }),
    );
    await visible(page.getByRole("link", { name: "No hosts need attention" }));
    for (const [name, percent] of [
      ["CPU", "28.5"],
      ["Memory", "58"],
      ["Storage", "62"],
    ]) {
      await visible(
        page.getByRole("link", {
          name: `Inspect ${name}: ${percent} percent used`,
          exact: true,
        }),
      );
    }
    assert.equal(
      await page.getByRole("link", { name: /^Inspect pve-\d\d, CPU/ }).count(),
      2,
    );
    assert.match(
      await page.locator(".coverage").innerText(),
      /2 hosts\s*\/ 10 running VMs/,
    );
    await page
      .getByRole("link", {
        name: "Inspect CPU: 28.5 percent used",
        exact: true,
      })
      .click();
    await visible(
      dialog().getByRole("heading", { name: "CPU utilization", exact: true }),
    );
    assert.match(
      await dialog().innerText(),
      /64 physical cores across 2 hosts/,
    );
    assert.match(
      await dialog()
        .getByRole("slider", { name: "Inspect history sample" })
        .getAttribute("aria-valuetext"),
      /28.5 percent/,
    );
  });

  await test("History ranges, keyboard exploration, and exact textual values agree", async () => {
    await page
      .getByRole("link", { name: "Inspect CPU: 38 percent used", exact: true })
      .click();
    await visible(
      dialog().getByRole("heading", { name: "CPU utilization", exact: true }),
    );
    const initialPath = await dialog().locator("svg path").getAttribute("d");
    await dialog().getByRole("button", { name: "24h", exact: true }).click();
    assert.notEqual(
      await dialog().locator("svg path").getAttribute("d"),
      initialPath,
    );
    assert.equal(
      await dialog()
        .getByRole("button", { name: "24h", exact: true })
        .getAttribute("aria-pressed"),
      "true",
    );
    const scrubber = dialog().getByRole("slider", {
      name: "Inspect history sample",
    });
    await scrubber.focus();
    await page.keyboard.press("Home");
    assert.equal(await scrubber.inputValue(), "0");
    assert.match(await scrubber.getAttribute("aria-valuetext"), /14:32 UTC/);
    const firstReading = await dialog().locator("output").innerText();
    await page.keyboard.press("ArrowRight");
    assert.equal(await scrubber.inputValue(), "1");
    assert.notEqual(await dialog().locator("output").innerText(), firstReading);
    await page.keyboard.press("End");
    assert.equal(await scrubber.inputValue(), "24");
    assert.match(await scrubber.getAttribute("aria-valuetext"), /38 percent/);
    await dialog()
      .getByText("View exact sample values", { exact: true })
      .click();
    const table = dialog().getByRole("table", {
      name: "CPU sample history",
      exact: true,
    });
    assert.equal(await table.locator("tbody tr").count(), 25);
    assert.match(
      await table.locator("tbody tr").last().innerText(),
      /2026-09-10 14:32:00\s+38%/,
    );
    await accessible("cpu-dialog");
    await page.keyboard.press("Escape");
    await dialog().waitFor({ state: "hidden" });
    assert.equal(
      await page
        .getByRole("link", {
          name: "Inspect CPU: 38 percent used",
          exact: true,
        })
        .evaluate((element) => document.activeElement === element),
      true,
    );
  });

  await test("Host comparison switches metric and exposes an equivalent data table", async () => {
    await page.getByRole("button", { name: "Memory", exact: true }).click();
    await visible(
      page.getByRole("link", {
        name: "Inspect pve-03, Memory 84 percent, warning",
        exact: true,
      }),
    );
    await page.getByRole("button", { name: "Storage", exact: true }).click();
    await visible(
      page.getByRole("link", {
        name: "Inspect pve-03, Storage 86 percent, warning",
        exact: true,
      }),
    );
    await page.getByRole("button", { name: "CPU", exact: true }).click();
    await visible(
      page.getByRole("link", {
        name: "Inspect pve-03, CPU 78 percent, warning",
        exact: true,
      }),
    );
    await page
      .getByRole("button", { name: "Show data table", exact: true })
      .click();
    const row = page
      .getByRole("row")
      .filter({ has: page.getByRole("link", { name: "pve-03", exact: true }) });
    assert.match(await row.innerText(), /78%\s+84%\s+86%\s+9\s+Warning/);
    await page
      .getByRole("button", { name: "Show distribution", exact: true })
      .click();
    await page
      .getByRole("link", {
        name: "Inspect pve-03, CPU 78 percent, warning",
        exact: true,
      })
      .focus();
    await page.keyboard.press("Enter");
    await visible(
      dialog().getByRole("heading", { name: "pve-03", exact: true }),
    );
    await dialog().getByRole("button", { name: "Memory", exact: true }).click();
    await visible(
      dialog().getByRole("heading", { name: "Memory history", exact: true }),
    );
    await dialog()
      .getByRole("link", { name: "Inspect 9 VMs →", exact: true })
      .click();
    await visible(
      dialog().getByRole("heading", {
        name: "Virtual machines / pve-03",
        exact: true,
      }),
    );
    assert.equal(
      await dialog().getByRole("table").locator("tbody tr").count(),
      9,
    );
    assert.equal(
      await dialog()
        .getByRole("cell", { name: "Running", exact: true })
        .count(),
      9,
    );
  });

  await test("Alert acknowledgement, assignment, and notes each link to matching audit evidence", async () => {
    await navigate("Alerts");
    await page
      .getByRole("link")
      .filter({ has: heading("Storage approaching capacity", 2) })
      .click();
    await visible(
      dialog().getByRole("button", { name: "Acknowledge alert", exact: true }),
    );
    const receipts = [];
    const captureReceipt = async (action) => {
      await visible(dialog().getByText("Demo change saved", { exact: true }));
      const link = dialog().getByRole("link", { name: /^View audit receipt/ });
      const url = await link.getAttribute("href");
      receipts.push(url);
      await link.click();
      await visible(heading("Audit"));
      const matchingRow = page.getByRole("table").locator("tbody tr");
      assert.equal(await matchingRow.count(), 1);
      assert.match(await matchingRow.innerText(), new RegExp(action));
      const id = new URLSearchParams(url.split("?")[1]).get("receipt");
      await matchingRow.getByRole("link", { name: new RegExp(id) }).click();
      await visible(
        dialog().getByRole("heading", { name: action, exact: true }),
      );
      assert.match(await dialog().innerText(), new RegExp(id));
      await page.keyboard.press("Escape");
      await dialog().waitFor({ state: "hidden" });
      await navigate("Alerts");
      await page
        .getByRole("link")
        .filter({ has: heading("Storage approaching capacity", 2) })
        .click();
      await visible(
        dialog().getByRole("heading", {
          name: "Storage approaching capacity",
          exact: true,
        }),
      );
    };
    await dialog()
      .getByRole("button", { name: "Acknowledge alert", exact: true })
      .click();
    await captureReceipt("Alert acknowledged");
    assert.equal(
      await dialog()
        .getByRole("button", { name: "Acknowledged", exact: true })
        .isDisabled(),
      true,
    );
    await dialog()
      .getByRole("button", { name: "Assign to me", exact: true })
      .click();
    await captureReceipt("Alert assigned");
    assert.equal(
      await dialog()
        .getByRole("button", { name: "Assigned to you", exact: true })
        .isDisabled(),
      true,
    );
    await dialog()
      .getByRole("textbox", { name: "Add context for the next operator" })
      .fill(
        "Reviewed pve-03 storage. Check sample retention before changing capacity.",
      );
    await dialog()
      .getByRole("button", { name: "Save demo note", exact: true })
      .click();
    await captureReceipt("Investigation note added");
    assert.equal(new Set(receipts).size, 3);
    await visible(
      dialog().getByText(
        "Reviewed pve-03 storage. Check sample retention before changing capacity.",
        { exact: false },
      ),
    );
    await accessible("alert-dialog");
    await page.keyboard.press("Escape");
    await navigate("Audit");
    await page
      .getByRole("searchbox", { name: "Search audit events" })
      .fill("Investigation note");
    assert.equal(await page.getByRole("table").locator("tbody tr").count(), 1);
    assert.match(
      await page.getByRole("table").innerText(),
      /Investigation note added/,
    );
    await page
      .getByRole("searchbox", { name: "Search audit events" })
      .fill("no-such-resource");
    await visible(
      page.getByRole("cell", {
        name: "No audit events match your search.",
        exact: true,
      }),
    );
    await page.reload();
    await visible(heading("Audit"));
    assert.equal(
      await page.getByRole("table").locator("tbody tr").count(),
      3,
      "Reload must reset local-only changes.",
    );
  });

  await test("Datacenter rack, chassis, and bay support deep links, keyboard, and history", async () => {
    await navigate("Datacenter");
    await page.getByRole("link", { name: /^Rack A02/ }).click();
    await visible(heading("Rack A02", 2));
    assert.equal(new URL(page.url()).hash, "#/datacenter?rack=A02");
    await page.getByRole("link", { name: /^Inspect pve-03, rack A02/ }).focus();
    await page.keyboard.press("Enter");
    await visible(heading("pve-03 chassis", 2));
    assert.equal(new URL(page.url()).hash, "#/datacenter?rack=A02&host=pve-03");
    await page
      .getByRole("link", {
        name: "Inspect bay 2, online, 2 TiB sample drive",
        exact: true,
      })
      .focus();
    await page.keyboard.press("Space");
    await visible(heading("Drive · bay 2", 2));
    assert.equal(
      new URL(page.url()).hash,
      "#/datacenter?rack=A02&host=pve-03&bay=2",
    );
    assert.match(
      await page.getByRole("region", { name: "Drive · bay 2" }).innerText(),
      /1.72 \/ 2 TiB/,
    );
    await accessible("drive");
    await page.screenshot({
      path: fileURLToPath(new URL("datacenter-drive-1440.png", resultsDir)),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await heading("Drive · bay 2", 2).waitFor({ state: "hidden" });
    assert.equal(new URL(page.url()).hash, "#/datacenter?rack=A02&host=pve-03");
    await page.goBack();
    await visible(heading("Drive · bay 2", 2));
    await page.reload();
    await visible(heading("Drive · bay 2", 2));
    await page.keyboard.press("Escape");
    await heading("Drive · bay 2", 2).waitFor({ state: "hidden" });
    await page.keyboard.press("Escape");
    await heading("pve-03 chassis", 2).waitFor({ state: "hidden" });
    await visible(heading("Rack A02", 2));
    await page.keyboard.press("Escape");
    await visible(heading("Rack overview", 2));
  });

  await test("Website details expose check history and internally consistent response timings", async () => {
    await navigate("Webpages");
    await page
      .getByRole("link")
      .filter({ has: heading("Documentation", 2) })
      .click();
    await visible(
      dialog().getByRole("heading", { name: "Documentation", exact: true }),
    );
    assert.match(await dialog().innerText(), /86\s*ms/);
    await dialog().getByText("View individual checks", { exact: true }).click();
    assert.equal(
      await dialog().getByRole("table").locator("tbody tr").count(),
      25,
    );
    await accessible("website-dialog");
    await page.keyboard.press("Escape");
    await page.getByRole("link", { name: "Certificates", exact: true }).click();
    const certificateTable = page.getByRole("table", {
      name: /Sample certificate inventory/,
    });
    await visible(certificateTable);
    assert.equal(await certificateTable.locator("tbody tr").count(), 3);
  });

  await test("Stale and disconnected states label last-known readings; blocked states withhold values", async () => {
    for (const state of ["stale", "disconnected"]) {
      await page
        .getByRole("combobox", { name: "Preview state" })
        .selectOption(state);
      await visible(
        page.getByRole("button", { name: "Restore sample", exact: true }),
      );
      assert.match(await page.getByRole("main").innerText(), /[Ll]ast.known/);
      await visible(
        page.getByRole("link", {
          name: "Inspect CPU: 38 percent used",
          exact: true,
        }),
      );
      await page
        .getByRole("button", { name: "Restore sample", exact: true })
        .click();
    }
    for (const [state, title] of [
      ["empty", "No resources in this view"],
      ["denied", "This view requires permission"],
      ["unavailable", "Core is unavailable"],
      ["error", "This view could not be loaded"],
    ]) {
      await page
        .getByRole("combobox", { name: "Preview state" })
        .selectOption(state);
      await visible(heading(title, 2));
      assert.equal(
        await page.getByRole("link", { name: /^Inspect CPU:/ }).count(),
        0,
      );
      await accessible(`state-${state}`);
      await page
        .getByRole("button", {
          name: state === "empty" ? "Load sample data" : "Retry sample view",
          exact: true,
        })
        .click();
      await visible(
        page.getByRole("link", {
          name: "Inspect CPU: 38 percent used",
          exact: true,
        }),
      );
    }
  });

  for (const name of routes) {
    await test(`${name} route accessibility and desktop containment`, async () => {
      await navigate(name);
      await accessible(name.toLowerCase());
      await noOverflow(name);
    });
  }

  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
    { width: 390, height: 844 },
  ]) {
    await test(`Dashboard visual capture ${viewport.width}x${viewport.height}`, async () => {
      await page.setViewportSize(viewport);
      await noOverflow(`Dashboard ${viewport.width}`);
      await page.screenshot({
        path: fileURLToPath(
          new URL(
            `dashboard-${viewport.width}x${viewport.height}.png`,
            resultsDir,
          ),
        ),
        fullPage: true,
      });
      if (viewport.width === 390) await accessible("dashboard-narrow");
    });
  }

  await test("Narrow menu reaches every section and closes with Escape", async () => {
    await page.setViewportSize({ width: 390, height: 844 });
    for (const name of routes) {
      await page.getByRole("button", { name: "Menu", exact: true }).click();
      await visible(
        page.getByRole("navigation", { name: "Primary", exact: true }),
      );
      await navigate(name);
      await page
        .locator('.menu-toggle[aria-expanded="false"]')
        .waitFor({ state: "visible" });
      assert.equal(
        await page
          .getByRole("button", { name: "Menu", exact: true })
          .getAttribute("aria-expanded"),
        "false",
      );
      await noOverflow(`${name} narrow`);
    }
    await page.getByRole("button", { name: "Menu", exact: true }).click();
    await page.keyboard.press("Escape");
    assert.equal(
      await page
        .getByRole("button", { name: "Menu", exact: true })
        .getAttribute("aria-expanded"),
      "false",
    );
    assert.equal(
      await page
        .getByRole("button", { name: "Menu", exact: true })
        .evaluate((element) => document.activeElement === element),
      true,
    );
    await page.getByRole("button", { name: "Menu", exact: true }).click();
    await page
      .getByRole("navigation", { name: "Primary", exact: true })
      .getByRole("button", { name: "Sign out of demo", exact: true })
      .click();
    await visible(heading("Espial design preview"));
    await page.getByRole("button", { name: "Enter demo", exact: true }).click();
    await visible(heading("Audit"));
  });

  await test("Desktop account menu supports pointer, focus, click, and Escape", async () => {
    const trigger = page.getByRole("button", { name: "Operator", exact: true });
    await trigger.hover();
    await visible(
      page.getByRole("button", { name: "Reset sample data", exact: true }),
    );
    await page.mouse.move(400, 400);
    await page
      .getByRole("button", { name: "Reset sample data", exact: true })
      .waitFor({ state: "hidden" });
    await trigger.focus();
    await visible(
      page.getByRole("button", { name: "Reset sample data", exact: true }),
    );
    await page.keyboard.press("Escape");
    assert.equal(await trigger.getAttribute("aria-expanded"), "false");
    await trigger.click();
    await visible(
      page.getByRole("button", { name: "Reset sample data", exact: true }),
    );
    await page.keyboard.press("Escape");
    assert.equal(await trigger.getAttribute("aria-expanded"), "false");
  });

  await test(
    "No production API requests, external assets, or browser runtime errors",
    async () => {
      assert.deepEqual(forbiddenRequests, []);
      assert.deepEqual(browserErrors, []);
      assert.ok(
        requestedPaths.has("/app.js") &&
          requestedPaths.has("/assets/ubnetdef.png"),
        "The browser must have loaded the actual preview.",
      );
    },
    { fresh: false },
  );
} catch (error) {
  failures.push({
    name: "Browser test setup",
    message: error.stack || String(error),
  });
  console.error(error);
} finally {
  if (browser) await browser.close();
  if (server && server.exitCode === null) {
    const stopped = new Promise((resolve) => server.once("exit", resolve));
    server.kill("SIGTERM");
    await stopped;
  }
  await mkdir(resultsDir, { recursive: true });
  await writeFile(
    new URL("report.json", resultsDir),
    JSON.stringify(
      {
        checks,
        failures,
        requestedPaths: [...requestedPaths],
        forbiddenRequests,
        browserErrors,
      },
      null,
      2,
    ),
  );
}

console.log(
  `\n${checks.filter((check) => check.passed).length}/${checks.length} checks passed. Screenshots and axe reports: mockup/test-results/`,
);
if (failures.length) process.exitCode = 1;

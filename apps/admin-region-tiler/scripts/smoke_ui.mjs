#!/usr/bin/env node

import { spawn, execFile } from "node:child_process";
import { once } from "node:events";
import { createServer } from "node:net";
import { request as httpRequest } from "node:http";
import { existsSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join, resolve, delimiter as pathDelimiter } from "node:path";
import { rm } from "node:fs/promises";
import process from "node:process";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const appDir = resolve(scriptDir, "..");
const devUsername = "admin";
const devPassword = "adminmap"; // Documented development default; production must override it.

const options = parseArgs(process.argv.slice(2));
const startedProcesses = [];
const cleanupPaths = [];
let cleaningUp = false;

process.on("SIGINT", async () => {
  await cleanup();
  process.exit(130);
});

process.on("SIGTERM", async () => {
  await cleanup();
  process.exit(143);
});

try {
  const target = options.url || await startApp();
  const result = await runSmoke(target);
  printResult(result);
} catch (error) {
  console.error(`UI smoke failed: ${error.message}`);
  process.exitCode = 1;
} finally {
  await cleanup();
}

function parseArgs(args) {
  const parsed = {
    headed: false,
    keepServer: false,
    timeoutMs: 60000,
    url: "",
    port: 0,
  };

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--headed") {
      parsed.headed = true;
    } else if (arg === "--keep-server") {
      parsed.keepServer = true;
    } else if (arg === "--url") {
      parsed.url = normalizeBaseURL(args[index + 1] || "");
      index += 1;
    } else if (arg.startsWith("--url=")) {
      parsed.url = normalizeBaseURL(arg.slice("--url=".length));
    } else if (arg === "--port") {
      parsed.port = Number.parseInt(args[index + 1], 10) || 0;
      index += 1;
    } else if (arg.startsWith("--port=")) {
      parsed.port = Number.parseInt(arg.slice("--port=".length), 10) || 0;
    } else if (arg === "--timeout-ms") {
      parsed.timeoutMs = Number.parseInt(args[index + 1], 10) || parsed.timeoutMs;
      index += 1;
    } else if (arg.startsWith("--timeout-ms=")) {
      parsed.timeoutMs = Number.parseInt(arg.slice("--timeout-ms=".length), 10) || parsed.timeoutMs;
    } else if (arg === "--help" || arg === "-h") {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }

  return parsed;
}

function printHelp() {
  console.log(`Usage:
  node scripts/smoke_ui.mjs [options]

Options:
  --url <url>          Test an already running app instead of starting go run .
  --port <number>     Port to use when starting the app. Defaults to a free port.
  --headed            Show the browser while running the smoke flow.
  --keep-server       Leave the started Go server running after the smoke run.
  --timeout-ms <ms>   Browser assertion timeout. Default: 60000.

Playwright lookup:
  The script tries local playwright, PLAYWRIGHT_MODULE_PATH, then npm root -g.
`);
}

async function loadPlaywright() {
  const localModule = await import("playwright").catch(() => null);
  if (localModule?.chromium) {
    return localModule;
  }

  const candidates = [];
  if (process.env.PLAYWRIGHT_MODULE_PATH) {
    candidates.push(process.env.PLAYWRIGHT_MODULE_PATH);
  }

  const npmRoot = await execFileOutput(npmCommand(), ["root", "-g"]).catch(() => "");
  if (npmRoot.trim()) {
    candidates.push(join(npmRoot.trim(), "playwright"));
  }
  candidates.push(...playwrightCandidatesFromPath());

  for (const candidate of candidates) {
    const resolved = resolvePlaywrightEntry(candidate);
    if (!resolved) {
      continue;
    }
    const module = await import(pathToFileURL(resolved).href).catch(() => null);
    if (module?.chromium) {
      return module;
    }
  }

  throw new Error([
    "Playwright is not available to this script.",
    "Install it with `npm install -g playwright` and `npx playwright install chromium`,",
    "or set PLAYWRIGHT_MODULE_PATH to a playwright package directory or index.mjs file.",
  ].join(" "));
}

function resolvePlaywrightEntry(candidate) {
  if (!candidate) {
    return "";
  }
  const normalized = resolve(candidate);
  if (existsSync(normalized) && normalized.endsWith(".mjs")) {
    return normalized;
  }
  const indexMjs = join(normalized, "index.mjs");
  if (existsSync(indexMjs)) {
    return indexMjs;
  }
  const nestedIndexMjs = join(normalized, "node_modules", "playwright", "index.mjs");
  if (existsSync(nestedIndexMjs)) {
    return nestedIndexMjs;
  }
  return "";
}

function playwrightCandidatesFromPath() {
  const pathValue = process.env.Path || process.env.PATH || "";
  const entries = pathValue.split(pathDelimiter).filter(Boolean);
  const candidates = [];
  for (const entry of entries) {
    candidates.push(join(entry, "node_modules", "playwright"));
    if (existsSync(join(entry, "playwright.cmd")) || existsSync(join(entry, "playwright"))) {
      candidates.push(join(entry, "node_modules", "playwright"));
    }
  }
  return Array.from(new Set(candidates));
}

function execFileOutput(command, args) {
  return new Promise((resolveOutput, rejectOutput) => {
    execFile(command, args, { windowsHide: true }, (error, stdout, stderr) => {
      if (error) {
        rejectOutput(new Error(stderr || error.message));
        return;
      }
      resolveOutput(stdout);
    });
  });
}

function npmCommand() {
  return process.platform === "win32" ? "npm.cmd" : "npm";
}

function normalizeBaseURL(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  return trimmed.replace(/\/+$/, "");
}

async function startApp() {
  const port = options.port || await getFreePort();
  const dbName = `smoke-ui-${Date.now()}-${process.pid}.db`;
  cleanupPaths.push(
    join(appDir, "data", dbName),
    join(appDir, "data", `${dbName}-shm`),
    join(appDir, "data", `${dbName}-wal`),
  );

  const child = spawn("go", ["run", "."], {
    cwd: appDir,
    env: {
      ...process.env,
      APP_PORT: String(port),
      APP_DATABASE: dbName,
      AUTH_DEFAULT_USERNAME: devUsername,
      AUTH_DEFAULT_PASSWORD: devPassword,
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
    detached: process.platform !== "win32",
  });
  startedProcesses.push(child);

  let recentOutput = "";
  child.stdout.on("data", (chunk) => {
    recentOutput = trimRecentOutput(recentOutput + chunk.toString());
  });
  child.stderr.on("data", (chunk) => {
    recentOutput = trimRecentOutput(recentOutput + chunk.toString());
  });
  child.on("exit", (code, signal) => {
    if (cleaningUp) {
      return;
    }
    if (code !== null && code !== 0 && process.exitCode !== 1) {
      console.error(`Go app exited early with code ${code}${recentOutput ? `\n${recentOutput}` : ""}`);
    }
    if (signal && process.exitCode !== 1) {
      console.error(`Go app exited early with signal ${signal}${recentOutput ? `\n${recentOutput}` : ""}`);
    }
  });

  const url = `http://127.0.0.1:${port}`;
  await waitForHTTP(url, recentOutput);
  return url;
}

async function getFreePort() {
  const server = createServer();
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const { port } = server.address();
  server.close();
  await once(server, "close");
  return port;
}

function waitForHTTP(baseURL, recentOutput) {
  const startedAt = Date.now();

  return new Promise((resolveWait, rejectWait) => {
    const attempt = () => {
      if (Date.now() - startedAt > options.timeoutMs) {
        rejectWait(new Error(`timed out waiting for ${baseURL}${recentOutput ? `\n${recentOutput}` : ""}`));
        return;
      }

      const req = httpRequest(`${baseURL}/`, { method: "GET", timeout: 2000 }, (res) => {
        res.resume();
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 500) {
          resolveWait();
          return;
        }
        setTimeout(attempt, 500);
      });
      req.on("timeout", () => {
        req.destroy();
        setTimeout(attempt, 500);
      });
      req.on("error", () => setTimeout(attempt, 500));
      req.end();
    };

    attempt();
  });
}

async function runSmoke(baseURL) {
  const { chromium } = await loadPlaywright();
  const browser = await chromium.launch({ headless: !options.headed });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  const createdPayloads = [];
  const confirmMessages = [];
  const consoleErrors = [];

  page.on("console", (message) => {
    if (message.type() === "error" && !isExpectedConsoleError(message.text())) {
      consoleErrors.push(message.text());
    }
  });
  page.on("pageerror", (error) => {
    consoleErrors.push(error.message);
  });
  page.on("dialog", async (dialog) => {
    confirmMessages.push(dialog.message());
    await dialog.accept();
  });

  await page.route("**/api/tasks", async (route) => {
    const request = route.request();
    if (request.method() === "POST") {
      const body = JSON.parse(request.postData() || "{}");
      createdPayloads.push(body);
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id: `smoke-${createdPayloads.length}`,
          kind: "group",
          name: body.name,
          status: "scheduled",
          scheduleMode: body.scheduleMode || "immediate",
          runAt: new Date().toISOString(),
          artifactStatus: "none",
        }),
      });
      return;
    }

    if (request.method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: "[]",
      });
      return;
    }

    await route.continue();
  });

  try {
    await page.goto(`${baseURL}/`, { waitUntil: "networkidle", timeout: options.timeoutMs });
    await expectTitle(page);
    await login(page);
    await smokeRegionFlow(page, createdPayloads);
    await smokeRangeFlow(page, createdPayloads);

    if (consoleErrors.length > 0) {
      throw new Error(`browser console errors:\n${consoleErrors.join("\n")}`);
    }

    return {
      baseURL,
      payloads: createdPayloads,
      confirms: confirmMessages.length,
    };
  } finally {
    await browser.close();
  }
}

async function expectTitle(page) {
  const title = await page.title();
  assert(title.includes("地图图片下载中心"), `unexpected page title: ${title}`);
}

async function login(page) {
  await page.waitForFunction(() => {
    const loginView = document.getElementById("loginView");
    const appView = document.getElementById("appView");
    return Boolean(
      loginView && appView &&
      (!loginView.classList.contains("is-hidden") || !appView.classList.contains("is-hidden"))
    );
  }, { timeout: options.timeoutMs });
  const appVisible = await page.locator("#appView").evaluate((element) => !element.classList.contains("is-hidden"));
  if (appVisible) {
    await page.locator("#taskForm").waitFor({ state: "visible", timeout: options.timeoutMs });
    await page.locator("#taskHistoryTab").waitFor({ state: "visible", timeout: options.timeoutMs });
    return;
  }

  await page.locator("input[name='username']").fill(devUsername);
  await page.locator("input[name='password']").fill(devPassword);
  await page.locator("#loginForm button[type='submit']").click();
  await page.locator("#appView").waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("#taskForm").waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("#taskHistoryTab").waitFor({ state: "visible", timeout: options.timeoutMs });
}

async function smokeRegionFlow(page, createdPayloads) {
  await showCreateTab(page);
  await page.locator("[data-task-mode='region']").click();
  await page.locator("#regionModePanel").waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("#rangeModePanel").waitFor({ state: "hidden", timeout: options.timeoutMs });
  await page.locator("#regionList .region-row").first().waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("#tilemapSelector .source-card").first().waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.waitForFunction(() => document.querySelectorAll("#adminRegionMap path[data-admin-region-id]").length > 0, null, { timeout: options.timeoutMs });
  await page.locator("#adminRegionMap path[data-admin-region-id]").first().click({ force: true });
  await page.waitForFunction(() => {
    const cityTab = document.querySelector("[data-admin-region-level='city']");
    const checkedLevels = document.querySelectorAll(".level-toggle:checked").length;
    return Boolean(cityTab?.classList.contains("is-active") && checkedLevels === 1);
  }, null, { timeout: options.timeoutMs });

  const beforeCount = createdPayloads.length;
  await page.locator("input[name='name']").fill(`smoke region ${Date.now()}`);
  await page.locator("input[name='tiandituToken']").fill("smoke-test-token");
  await page.locator("#taskForm button[type='submit']").click();
  await waitForPayloadCount(createdPayloads, beforeCount + 1);

  const payload = createdPayloads.at(-1);
  assert(Array.isArray(payload.levels) && payload.levels.length > 0, "region payload should include levels");
  assert(payload.levels.length === 1, "focused region map selection should submit only the selected level by default");
  assert(Array.isArray(payload.sources) && payload.sources.length > 0, "region payload should include sources");
  assert(!payload.mode, "region payload should use the legacy-compatible region request shape");
  assert(payload.scheduleMode === "immediate", "region payload should create an immediate task");
  assert(payload.output?.format === "zip", "region payload should default to zip output");
  assert(payload.sources.every((source) => !String(source.url || "").includes("YOUR_TIANDITU_TOKEN")), "region payload should replace Tianditu token placeholders");
  assert(payload.sources.some((source) => String(source.url || "").includes("smoke-test-token")), "region payload should include the supplied Tianditu token");
}

async function smokeRangeFlow(page, createdPayloads) {
  await showCreateTab(page);
  await page.locator("[data-task-mode='bbox']").click();
  await page.locator("#rangeModePanel").waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("#regionModePanel").waitFor({ state: "hidden", timeout: options.timeoutMs });
  await page.locator("#rangeMap").waitFor({ state: "visible", timeout: options.timeoutMs });

  const rangeMap = page.locator("#rangeMap");
  await rangeMap.scrollIntoViewIfNeeded();
  await rangeMap.click({ position: { x: 220, y: 220 }, force: true });
  await rangeMap.click({ position: { x: 420, y: 380 }, force: true });

  await page.locator("input[name='name']").fill(`smoke bbox ${Date.now()}`);
  await page.locator("input[name='tiandituToken']").fill("smoke-test-token");
  await page.locator("input[name='rangeMinLon']").fill("116.300000");
  await page.locator("input[name='rangeMaxLon']").fill("116.460000");
  await page.locator("input[name='rangeMinLat']").fill("39.860000");
  await page.locator("input[name='rangeMaxLat']").fill("39.980000");
  await page.locator("input[name='rangeMinZoom']").fill("10");
  await page.locator("input[name='rangeMaxZoom']").fill("10");
  await page.locator("input[name='rangeLayers'][value='cia']").setChecked(false);
  await page.locator("input[name='rangeLayers'][value='vec']").setChecked(false);
  await page.locator("input[name='scheduleMode'][value='once']").check({ force: true });
  await page.locator("#runAtField").waitFor({ state: "visible", timeout: options.timeoutMs });
  await page.locator("input[name='runAt']").fill(formatDateTimeLocal(new Date(Date.now() + 60 * 60 * 1000)));
  await page.locator("select[name='outputFormat']").selectOption("mbtiles");

  await waitForTextNotEqual(page.locator("#rangeTileCount"), "-");
  await waitForTextNotEqual(page.locator("#rangeTotalTileCount"), "-");

  const beforeCount = createdPayloads.length;
  await page.locator("#taskForm button[type='submit']").click();
  await waitForPayloadCount(createdPayloads, beforeCount + 1);

  const payload = createdPayloads.at(-1);
  assert(payload.mode === "bbox", "range payload should use bbox mode");
  assert(payload.area?.bbox, "range payload should include area.bbox");
  assert(payload.zoom?.min === 10 && payload.zoom?.max === 10, "range payload should include the requested zoom");
  assert(payload.scheduleMode === "once", "range payload should support scheduled once mode");
  assert(typeof payload.runAt === "string" && payload.runAt.includes("T"), "range payload should include scheduled runAt");
  assert(payload.output?.format === "mbtiles", "range payload should include selected mbtiles output");
  assert(Array.isArray(payload.sources) && payload.sources.length === 1, "range payload should include the selected layer source");
  assert(String(payload.sources[0].name || "").includes("img"), "range payload should include the img layer source");
  assert(String(payload.sources[0].url || "").includes("smoke-test-token"), "range source URL should include the supplied token");
}

async function showCreateTab(page) {
  await page.locator("#createTaskTab").click();
  await page.locator("#taskForm").waitFor({ state: "visible", timeout: options.timeoutMs });
}

function formatDateTimeLocal(date) {
  const pad = (value) => String(value).padStart(2, "0");
  return [
    date.getFullYear(),
    "-",
    pad(date.getMonth() + 1),
    "-",
    pad(date.getDate()),
    "T",
    pad(date.getHours()),
    ":",
    pad(date.getMinutes()),
  ].join("");
}

async function waitForPayloadCount(payloads, count) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < options.timeoutMs) {
    if (payloads.length >= count) {
      return;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 50));
  }
  throw new Error(`timed out waiting for ${count} task payloads; observed ${payloads.length}`);
}

async function waitForTextNotEqual(locator, disallowed) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < options.timeoutMs) {
    const value = (await locator.textContent())?.trim();
    if (value && value !== disallowed) {
      return value;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 50));
  }
  throw new Error(`timed out waiting for ${await locator.evaluate((node) => node.id)} text to change`);
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function isExpectedConsoleError(text) {
  return String(text || "").includes("Failed to load resource: the server responded with a status of 401");
}

function printResult(result) {
  const regionPayload = result.payloads.find((payload) => !payload.mode);
  const rangePayload = result.payloads.find((payload) => payload.mode === "bbox");
  console.log("UI smoke passed");
  console.log(`- URL: ${result.baseURL}`);
  console.log(`- Region payload: ${regionPayload.sources.length} source(s), ${regionPayload.levels.length} level(s)`);
  console.log(`- Range payload: ${rangePayload.sources.length} source(s), zoom ${rangePayload.zoom.min}-${rangePayload.zoom.max}`);
  console.log(`- Confirm dialogs accepted: ${result.confirms}`);
}

function trimRecentOutput(value) {
  return value.split(/\r?\n/).slice(-40).join("\n");
}

async function cleanup() {
  cleaningUp = true;
  for (const child of startedProcesses) {
    if (!options.keepServer) {
      await stopProcessTree(child);
    }
  }

  await cleanupGeneratedFiles();
}

async function cleanupGeneratedFiles() {
  for (let attempt = 0; attempt < 8; attempt += 1) {
    for (const filePath of cleanupPaths) {
      await rm(filePath, { force: true }).catch(() => {});
    }
    if (!cleanupPaths.some((filePath) => existsSync(filePath))) {
      return;
    }
    await sleep(250);
  }
}

async function stopProcessTree(child) {
  if (!child.pid) {
    return;
  }

  if (process.platform === "win32") {
    await new Promise((resolveKill) => {
      execFile("taskkill", ["/pid", String(child.pid), "/T", "/F"], { windowsHide: true }, () => resolveKill());
    });
    return;
  }

  let exited = false;
  const exitPromise = once(child, "exit").then(() => {
    exited = true;
  });

  try {
    process.kill(-child.pid, "SIGTERM");
  } catch {
    child.kill("SIGTERM");
  }
  await Promise.race([exitPromise, sleep(3000)]);

  if (!exited) {
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch {
      child.kill("SIGKILL");
    }
    await Promise.race([exitPromise, sleep(1000)]);
  }
}

function sleep(ms) {
  return new Promise((resolveSleep) => setTimeout(resolveSleep, ms));
}

#!/usr/bin/env node

import { spawn } from "node:child_process";
import process from "node:process";

const checks = [
  {
    label: "Go test suite",
    command: "go",
    args: ["test", "./..."],
  },
  {
    label: "Frontend script syntax",
    command: "node",
    args: ["--check", "./static/script.js"],
  },
  {
    label: "UI smoke script syntax",
    command: "node",
    args: ["--check", "./scripts/smoke_ui.mjs"],
  },
  {
    label: "Browser UI smoke",
    command: "node",
    args: ["./scripts/smoke_ui.mjs"],
  },
];
let gitRootCache = "";

for (const check of checks) {
  await run(check);
}
await runSensitiveValueScan();
await runGeneratedFileScan();

console.log("Release preflight passed");

function run(check) {
  console.log(`\n==> ${check.label}`);
  console.log(`$ ${[check.command, ...check.args].join(" ")}`);

  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(resolveCommand(check.command), check.args, {
      cwd: process.cwd(),
      env: process.env,
      stdio: "inherit",
      windowsHide: true,
    });

    child.on("error", (error) => {
      rejectRun(new Error(`${check.label} could not start: ${error.message}`));
    });

    child.on("exit", (code, signal) => {
      if (code === 0) {
        resolveRun();
        return;
      }
      if (signal) {
        rejectRun(new Error(`${check.label} stopped by signal ${signal}`));
        return;
      }
      rejectRun(new Error(`${check.label} failed with exit code ${code}`));
    });
  }).catch((error) => {
    console.error(`Release preflight failed: ${error.message}`);
    process.exit(1);
  });
}

function resolveCommand(command) {
  if (process.platform !== "win32") {
    return command;
  }
  if (command === "go") {
    return "go.exe";
  }
  if (command === "node") {
    return "node.exe";
  }
  return command;
}

async function runSensitiveValueScan() {
  console.log("\n==> Sensitive value scan");
  const blockedExactValues = [
    ["75f0434f", "240669f4", "a2df6359", "275146d2"].join(""),
    ["0ffbf631", "2cecabb7", "b5e4b2a7", "986ab098"].join(""),
    ["Bo", "wee"].join(""),
  ];
  const pattern = `(${blockedExactValues.map(escapeRegExp).join("|")}|pk\\.eyJ|access_token=[^Y[:space:]"'&]+|tk=[A-Za-z0-9]+)`;
  const output = await runCapture({
    label: "git grep sensitive values",
    command: "git",
    args: ["-c", "core.quotePath=false", "grep", "-n", "-I", "-E", pattern, "--", "."],
    cwd: await gitRoot(),
    allowExitCodes: [0, 1],
  });
  const findings = output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .filter((line) => !(line.includes("scripts/release_preflight.mjs") && line.includes("const pattern =")))
    .filter((line) => !line.includes("YOUR_TIANDITU_TOKEN") && !line.includes("YOUR_MAPBOX_TOKEN") && !line.includes("YOUR_MAPBOX_SKU"));

  if (findings.length > 0) {
    throwPreflightError(`sensitive value scan found blocked values:\n${findings.join("\n")}`);
  }
  console.log("Sensitive value scan passed");
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function runGeneratedFileScan() {
  console.log("\n==> Generated file scan");
  const files = await gitLines(["ls-files"]);
  const blocked = files.filter(isBlockedGeneratedPath);
  if (blocked.length > 0) {
    throwPreflightError(`generated file scan found blocked tracked paths:\n${blocked.join("\n")}`);
  }
  console.log("Generated file scan passed");
}

function isBlockedGeneratedPath(file) {
  const normalized = file.replace(/\\/g, "/");
  const parts = normalized.split("/");
  if (parts.some((part) => ["data", "output", "tiles", "bin", "obj"].includes(part))) {
    return true;
  }
  const base = parts.at(-1) || "";
  if (/^publish($|[-_])/.test(base) || /^publish($|[-_])/.test(normalized)) {
    return true;
  }
  return /\.(zip|tgz|db|log|exe)$/i.test(base) || /\.tar\.gz$/i.test(base);
}

async function gitLines(args) {
  const gitArgs = ["-c", "core.quotePath=false", ...args];
  const output = await runCapture({
    label: `git ${gitArgs.join(" ")}`,
    command: "git",
    args: gitArgs,
    cwd: await gitRoot(),
  });
  return output.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

async function gitRoot() {
  if (gitRootCache) {
    return gitRootCache;
  }
  gitRootCache = (await runCapture({
    label: "git rev-parse --show-toplevel",
    command: "git",
    args: ["rev-parse", "--show-toplevel"],
  })).trim();
  return gitRootCache;
}

function runCapture(check) {
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(resolveCommand(check.command), check.args, {
      cwd: check.cwd || process.cwd(),
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });
    child.on("error", (error) => {
      rejectRun(new Error(`${check.label} could not start: ${error.message}`));
    });
    child.on("exit", (code, signal) => {
      const allowedCodes = check.allowExitCodes || [0];
      if (allowedCodes.includes(code)) {
        resolveRun(stdout);
        return;
      }
      if (signal) {
        rejectRun(new Error(`${check.label} stopped by signal ${signal}`));
        return;
      }
      rejectRun(new Error(`${check.label} failed with exit code ${code}${stderr ? `: ${stderr}` : ""}`));
    });
  }).catch((error) => {
    throwPreflightError(error.message);
  });
}

function throwPreflightError(message) {
  console.error(`Release preflight failed: ${message}`);
  process.exit(1);
}

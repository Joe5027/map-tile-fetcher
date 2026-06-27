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

for (const check of checks) {
  await run(check);
}

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

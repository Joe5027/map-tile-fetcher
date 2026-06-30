#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const conventionalTypes = [
  "feat",
  "fix",
  "docs",
  "chore",
  "refactor",
  "test",
  "ci",
  "style",
  "perf",
  "build",
  "revert",
];

function usage() {
  console.error(`Usage:
  node scripts/validate_commit_message.mjs --file <commit-message-file>
  node scripts/validate_commit_message.mjs --commit <rev>
  node scripts/validate_commit_message.mjs --range <base>..<head>
  node scripts/validate_commit_message.mjs --self-test`);
}

function stripCommentLines(message) {
  return message
    .replace(/\r\n/g, "\n")
    .split("\n")
    .filter((line) => !line.startsWith("#"))
    .join("\n")
    .trim();
}

function splitSections(lines) {
  const sections = new Map();
  let current = null;

  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === "English:" || trimmed === "中文:" || trimmed === "Validation:") {
      current = trimmed;
      sections.set(current, []);
      continue;
    }
    if (current) {
      sections.get(current).push(line);
    }
  }

  return sections;
}

function hasChinese(text) {
  return /[\u3400-\u9fff]/u.test(text);
}

function hasAsciiLetter(text) {
  return /[A-Za-z]/.test(text);
}

function countBulletLines(lines) {
  return lines.filter((line) => /^\s*-\s+\S/.test(line)).length;
}

function hasPlaceholder(text) {
  return /<[^>]+>|What changed\.|Why it changed\.|User or developer impact\.|修改了什么。|为什么修改。|对用户或开发者的影响。|Exact commands or checks run\.|Any checks intentionally not run/i.test(text);
}

function validateMessage(message, label = "commit message") {
  const cleaned = stripCommentLines(message);
  const lines = cleaned.split("\n");
  const subject = lines.find((line) => line.trim().length > 0)?.trim() ?? "";
  const errors = [];

  const typePattern = conventionalTypes.join("|");
  const subjectMatch = subject.match(new RegExp(`^(${typePattern})(\\([^)]+\\))?!?:\\s+(.+)\\s+/\\s+(.+)$`));

  if (!subjectMatch) {
    errors.push(
      `subject must be '<type>(<scope>): <English summary> / <中文摘要>' with an allowed Conventional Commit type (${conventionalTypes.join(", ")})`,
    );
  } else {
    const englishSummary = subjectMatch[3].trim();
    const chineseSummary = subjectMatch[4].trim();
    if (!hasAsciiLetter(englishSummary)) {
      errors.push("subject English summary must contain ASCII English text before the slash");
    }
    if (!hasChinese(chineseSummary)) {
      errors.push("subject Chinese summary must contain Chinese text after the slash");
    }
  }

  const sections = splitSections(lines);
  for (const heading of ["English:", "中文:", "Validation:"]) {
    if (!sections.has(heading)) {
      errors.push(`missing required '${heading}' section`);
    }
  }

  const englishBullets = countBulletLines(sections.get("English:") ?? []);
  const chineseBullets = countBulletLines(sections.get("中文:") ?? []);
  const validationBullets = countBulletLines(sections.get("Validation:") ?? []);

  if (englishBullets < 3) {
    errors.push("English section must contain at least 3 bullet lines: what changed, why, and impact");
  }
  if (chineseBullets < 3) {
    errors.push("中文 section must contain at least 3 bullet lines: 修改内容、原因、影响");
  }
  if (validationBullets < 1) {
    errors.push("Validation section must contain at least 1 bullet line naming the checks run or intentionally skipped");
  }
  if (hasPlaceholder(cleaned)) {
    errors.push("message still contains template placeholder text");
  }

  if (errors.length > 0) {
    return {
      ok: false,
      label,
      errors,
    };
  }

  return { ok: true, label, errors: [] };
}

function printResult(result) {
  if (result.ok) {
    console.log(`commit message ok: ${result.label}`);
    return;
  }

  console.error(`commit message invalid: ${result.label}`);
  for (const error of result.errors) {
    console.error(`- ${error}`);
  }
}

function gitOutput(args) {
  return execFileSync("git", args, { encoding: "utf8" }).trim();
}

function validateCommit(rev) {
  const hash = gitOutput(["rev-parse", "--verify", rev]);
  const message = execFileSync("git", ["log", "-1", "--pretty=%B", hash], { encoding: "utf8" });
  return validateMessage(message, hash);
}

function validateRange(range) {
  const revs = gitOutput(["rev-list", "--reverse", range])
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  if (revs.length === 0) {
    console.log(`commit message ok: no commits in range ${range}`);
    return [];
  }

  return revs.map((rev) => validateCommit(rev));
}

function runSelfTest() {
  const valid = `fix(admin-region-tiler): preserve focused region selection / 保留点选区域层级控制

English:
- Restored focused region-level selection behavior.
- Prevented accidental ancestor-level payload submission.
- Keeps the task creation flow predictable for users.

中文:
- 恢复点选区域时的单层级选择行为。
- 避免任务 payload 意外带上上级层级。
- 让用户创建任务时的层级控制更明确。

Validation:
- node scripts/validate_commit_message.mjs --self-test
`;

  const invalid = `feat: optimize admin tiler task UI`;
  const results = [
    validateMessage(valid, "self-test valid sample"),
    validateMessage(invalid, "self-test invalid sample"),
  ];

  if (!results[0].ok) {
    printResult(results[0]);
    return false;
  }
  if (results[1].ok) {
    console.error("self-test failed: invalid sample unexpectedly passed");
    return false;
  }

  console.log("self-test ok");
  return true;
}

const args = process.argv.slice(2);
let results = [];

if (args.length === 1 && args[0] === "--self-test") {
  process.exit(runSelfTest() ? 0 : 1);
} else if (args.length === 2 && args[0] === "--file") {
  results = [validateMessage(readFileSync(args[1], "utf8"), args[1])];
} else if (args.length === 2 && args[0] === "--commit") {
  results = [validateCommit(args[1])];
} else if (args.length === 2 && args[0] === "--range") {
  results = validateRange(args[1]);
} else {
  usage();
  process.exit(2);
}

let ok = true;
for (const result of results) {
  printResult(result);
  ok = ok && result.ok;
}

process.exit(ok ? 0 : 1);

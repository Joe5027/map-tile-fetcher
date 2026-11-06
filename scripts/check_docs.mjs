#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const files = execFileSync("git", ["ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "*.md"], {
  cwd: root, encoding: "utf8", windowsHide: true,
}).split("\0").filter(Boolean);
const required = [
  "AGENTS.md", "README.md", "PROJECT_MANIFEST.md", "docs/project-map.md",
  "docs/done-definition.md", "docs/validation-chain.md", "docs/ai-operating-handbook.md",
  "docs/knowledge-graph.md", "docs/long-term-memory.md", "docs/automation-guardrails.md",
  "docs/merge-plan.md", ".codex/skills/two-projects-handoff/SKILL.md",
];
const errors = [];
const skipped = [];
let checkedLinks = 0;

for (const file of required) {
  if (!existsSync(resolve(root, file))) errors.push(`${file}: required entrypoint is missing`);
}

for (const file of new Set(files)) {
  const absolute = resolve(root, file);
  if (!existsSync(absolute)) continue; // A tracked file may be intentionally deleted in this batch.
  const text = readFileSync(absolute, "utf8");
  // Preserve newlines for useful locations while excluding fenced examples.
  const prose = text.replace(/^(`{3,}|~{3,})[^\n]*\n[\s\S]*?^\1\s*$/gm, (match) => match.replace(/[^\n]/g, " "));
  const links = /!?\[[^\]\n]*\]\(\s*(?:<([^>\n]+)>|([^\s)]+))(?:\s+["'][^\n]*?["'])?\s*\)|^\s{0,3}\[[^\]\n]+\]:\s*(?:<([^>\n]+)>|(\S+))/gm;
  for (const match of prose.matchAll(links)) {
    const target = match[1] || match[2] || match[3] || match[4];
    const location = `${file}:${prose.slice(0, match.index).split("\n").length}`;
    if (/^[A-Za-z]:[\\/]|^file:|^\\\\/.test(target)) {
      skipped.push(`${location}: historical absolute link ${target}`);
      continue;
    }
    if (/^[a-z][a-z\d+.-]*:|^\/\/|^#/i.test(target)) continue;
    const localPath = target.split(/[?#]/, 1)[0];
    if (!localPath) continue;
    let decoded;
    try { decoded = decodeURIComponent(localPath); }
    catch { errors.push(`${location}: invalid encoded path ${target}`); continue; }
    const destination = decoded.startsWith("/")
      ? resolve(root, `.${decoded}`)
      : resolve(dirname(absolute), decoded);
    checkedLinks++;
    if (!existsSync(destination)) errors.push(`${location}: missing link/image ${target}`);
  }
  if (file === "docs/long-term-memory.md") {
    const sections = [...prose.matchAll(/^##\s+(.+)\s*$/gm)].map((match) => match[1].trim());
    const expected = ["Facts", "Decisions", "Assumptions", "Validation", "Next Action"];
    if (JSON.stringify(sections) !== JSON.stringify(expected)) {
      errors.push(`${file}: expected ordered sections ${expected.join(", ")}`);
    }
  }
}

for (const warning of skipped) console.warn(`Skipped: ${warning}`);
for (const error of errors) console.error(error);
console.log(`Documentation check: ${files.length} Markdown files, ${checkedLinks} local links/images, ${skipped.length} absolute links skipped, ${errors.length} errors.`);
console.log("External URLs and heading anchors are not checked; compare behavior and images against the application.");
if (errors.length) process.exitCode = 1;

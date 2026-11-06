import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import vm from "node:vm";

const source = process.argv[2]
  ? execFileSync("git", ["show", `${process.argv[2]}:apps/admin-region-tiler/static/script.js`], { encoding: "utf8" })
  : readFileSync(new URL("../static/script.js", import.meta.url), "utf8");
const context = vm.createContext({ document: { addEventListener() {} }, console });
vm.runInContext(source, context);
const hostile = '<img src=x onerror="alert(1)">';
for (const render of ["renderStandaloneTask", "renderChildTask", "renderGroupTask"]) {
  const html = context[render]({ id: "safe-id", name: hostile, sourceName: hostile, errorMessage: hostile, artifactName: hostile, status: "completed", artifactStatus: "ready", downloadUrl: "javascript:alert(1)" });
  assert(!html.includes(hostile), `${render} inserted active markup`);
  assert(!html.includes('href="javascript:'), `${render} accepted a script URL`);
  assert(html.includes("&lt;img"), `${render} lost the literal name`);
}
const grantURL = `/api/downloads/${"a1".repeat(32)}`;
for (const url of ["https://example.test/file", "//example.test/file", "/api/tasks/other/download", "/api/downloads/invalid", `${grantURL}?redirect=//example.test`, `${grantURL}/../../auth/logout`]) {
  const html = context.renderChildTask({ id: "safe-id", status: "completed", artifactStatus: "ready", downloadUrl: url });
  assert(!html.includes('class="artifact-link"'), `accepted unmanaged download: ${url}`);
}
const html = context.renderChildTask({ id: "safe-id", status: "completed", artifactStatus: "ready", downloadUrl: "/api/tasks/safe-id/download" });
assert(html.includes('href="/api/tasks/safe-id/download"'), "managed download missing");
for (const render of ["renderStandaloneTask", "renderChildTask"]) {
  const directHTML = context[render]({ id: "safe-id", status: "completed", artifactStatus: "ready", downloadUrl: grantURL });
  assert(directHTML.includes(`href="${grantURL}"`), `${render} dropped the artifact grant link`);
}
console.log("Rendering and managed download regression checks passed");

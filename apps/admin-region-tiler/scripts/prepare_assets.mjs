#!/usr/bin/env node
import { cp, mkdir, rm } from "node:fs/promises";
import { resolve, dirname, sep } from "node:path";
import { fileURLToPath } from "node:url";
const app = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = resolve(app, "node_modules/cesium/Build/Cesium");
const vendor = resolve(app, "static/vendor");
const target = resolve(vendor, "cesium");
// This directory contains only reproducible npm assets, never application data.
if (target !== vendor + sep + "cesium") throw new Error("Unexpected asset directory");
await mkdir(vendor, {recursive:true});
await rm(target, {recursive:true,force:true});
await cp(source, target, {recursive:true});
for (const name of ["LICENSE.md", "ThirdParty.json", "ThirdParty.extra.json"]) {
  await cp(resolve(app, "node_modules/cesium", name), resolve(target, name));
}
console.log("Prepared local Cesium assets for offline preview.");

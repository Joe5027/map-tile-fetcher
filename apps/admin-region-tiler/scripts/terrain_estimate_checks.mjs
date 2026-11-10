#!/usr/bin/env node
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const app = new URL("../", import.meta.url);
const context = vm.createContext({ document: { addEventListener() {} }, URL });
vm.runInContext(readFileSync(new URL("static/script.js", app), "utf8"), context);
const call = (expression) => JSON.parse(JSON.stringify(vm.runInContext(expression, context, { timeout: 2000 })));
const sources = call("FALLBACK_TILEMAPS");
const terrain = sources.find((source) => source.id === 13);
const raster = sources.find((source) => source.id === 11);
const example = { minLon: 1, minLat: 1, maxLon: 2, maxLat: 2 };
const count = (bbox, zoom) => call(`calculateTerrainTileCounts(${JSON.stringify(bbox)},${JSON.stringify(zoom)})`);
const estimate = (bbox, zoom, selected, format) => call(`estimateRangeDownloads(${JSON.stringify(bbox)},${JSON.stringify(zoom)},${JSON.stringify(selected)},${JSON.stringify(format)})`);

assert.deepEqual(count(example, { min: 2, max: 2 }), { rawTiles: 14, meshTiles: 3, selectedTiles: 1 });
assert.equal(estimate(example, { min: 2, max: 2 }, [terrain], "cesium").rawTiles, 14);
assert.equal(estimate(example, { min: 2, max: 2 }, [terrain, raster], "cesium").rawTiles, 15);
assert.equal(estimate(example, { min: 2, max: 2 }, [terrain, terrain], "cesium").rawTiles, 28);
for (const format of ["tar.gz", "mbtiles"]) {
    const result = estimate(example, { min: 2, max: 2 }, [terrain, raster], format);
    assert.equal(result.rawTiles, 2);
    assert.equal(result.meshTiles, 0);
}

// Independent low-zoom enumeration checks the interval union against the
// backend's ancestors + wrapped/clamped 3x3 source-neighbourhood contract.
function bruteForce(bbox, zoom) {
    const meshes = new Set();
    let selectedTiles = 0;
    function point(lon, lat, z) {
        const n = 2 ** z;
        return [Math.max(0, Math.min(n - 1, Math.floor((lon + 180) * n / 360))),
            Math.max(0, Math.min(n - 1, Math.floor((1 - Math.asinh(Math.tan(lat * Math.PI / 180)) / Math.PI) * n / 2)))];
    }
    for (let z = zoom.min; z <= zoom.max; z++) {
        const [left, top] = point(bbox.minLon, bbox.maxLat, z);
        const [right, bottom] = point(bbox.maxLon, bbox.minLat, z);
        for (let x = left; x <= right; x++) for (let y = top; y <= bottom; y++) {
            selectedTiles++;
            for (let a = z; a >= 0; a--) meshes.add(`${a}/${Math.floor(x / 2 ** (z - a))}/${Math.floor(y / 2 ** (z - a))}`);
        }
    }
    const raw = new Set();
    for (const key of meshes) {
        const [z, x, y] = key.split("/").map(Number);
        const n = 2 ** z;
        for (let dx = -1; dx <= 1; dx++) for (let dy = -1; dy <= 1; dy++) {
            raw.add(`${z}/${(x + dx + n) % n}/${Math.max(0, Math.min(n - 1, y + dy))}`);
        }
    }
    return { rawTiles: raw.size, meshTiles: meshes.size, selectedTiles };
}
let comparisons = 0;
for (const bbox of [example,
    { minLon: -180, minLat: -85.05112878, maxLon: 180, maxLat: 85.05112878 },
    { minLon: -180, minLat: 70, maxLon: -179, maxLat: 85 },
    { minLon: 179, minLat: -85, maxLon: 180, maxLat: -70 },
    { minLon: -127, minLat: -23, maxLon: 65, maxLat: 54 },
    { minLon: 112, minLat: 32, maxLon: 118, maxLat: 38 }]) {
    for (let min = 0; min <= 4; min++) for (let max = min; max <= 4; max++) {
        assert.deepEqual(count(bbox, { min, max }), bruteForce(bbox, { min, max }), `${JSON.stringify(bbox)} z${min}-${max}`);
        comparisons++;
    }
}
const world = { minLon: -180, minLat: -85.05112878, maxLon: 180, maxLat: 85.05112878 };
assert.deepEqual(count(world, { min: 15, max: 15 }), { rawTiles: (4 ** 16 - 1) / 3, meshTiles: (4 ** 16 - 1) / 3, selectedTiles: 4 ** 15 });

// Every fallback field comes from the shipped TOML catalogue; in particular,
// an unavailable /api/tilemaps cannot remove terrain or mislabel vectors.
const config = readFileSync(new URL("conf.toml", app), "utf8");
const blocks = config.split("[[tilemaps]]").slice(1).map((block) => block.split("[[")[0]);
for (const source of sources) {
    const block = blocks.find((value) => new RegExp(`^\\s*id = ${source.id}\\s*$`, "m").test(value));
    assert.ok(block, `fallback id ${source.id} exists in conf.toml`);
    for (const field of ["name", "url", "format", "schema"]) {
        assert.equal(source[field], new RegExp(`^\\s*${field} = "([^"]*)"`, "m").exec(block)?.[1]);
    }
    for (const field of ["min_zoom", "max_zoom"]) assert.equal(source[field], Number(new RegExp(`^\\s*${field} = (\\d+)`, "m").exec(block)?.[1]));
    for (const [field, value] of Object.entries(source.metadata || {})) {
        const configField = field.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`);
        const literal = typeof value === "number" ? String(value) : JSON.stringify(value);
        assert.ok(block.includes(`${configField} = ${literal}`), `${source.id} metadata.${field} matches conf.toml`);
    }
}
assert.equal(call(`validateOutputSources([FALLBACK_TILEMAPS.find(s=>s.id===13),FALLBACK_TILEMAPS.find(s=>s.id===11)],{format:"cesium"})`).length > 0, true);
console.log(`Terrain estimates passed: 14 raw / 3 meshes, ${comparisons} low-zoom comparisons, world z15 bounded union, mixed-source/TAR.GZ behavior, ${sources.length} fallback sources.`);

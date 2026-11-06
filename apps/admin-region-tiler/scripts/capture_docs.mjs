#!/usr/bin/env node
// Actual application screenshots, using local synthetic data and the locked browser.
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, mkdtemp, readFile, writeFile, rm } from "node:fs/promises";
import { dirname, resolve, join, relative, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "node:http";
import { once } from "node:events";
import { createHash } from "node:crypto";

const exec = promisify(execFile);
const appDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const repoDir = resolve(appDir, "../..");
const assetsDir = join(repoDir, "docs/assets");

export async function captureMainScreenshots(page) {
  await mkdir(assetsDir, { recursive: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const task = { id: "docs-terrain", kind: "single", name: "本地合成高程夹具 · 示例任务", status: "completed", progress: 100,
    area: {mode:"bbox",bbox:{minLon:116.30,minLat:39.86,maxLon:116.46,maxLat:39.98}},
    total: 85, current: 85, successCount: 85, failureCount: 0, artifactStatus: "ready", artifact: { status: "ready", format: "cesium" },
    downloadUrl: "/api/tasks/docs-terrain/download", startedAt: "2026-11-06T01:00:00Z", finishedAt: "2026-11-06T01:00:06Z",
    phase: { name: "completed", current: 85, total: 85 }, integrity: { status: "complete", expected: 85, available: 85 } };
  await page.route("**/api/tasks/page?*", route => route.fulfill({ json: { items: [task], total: 1, counts: { all: 1, completed: 1 } } }));
  await page.route("**/api/tasks/docs-terrain/area*", route => route.fulfill({ json: {
    type: "FeatureCollection", features: [{ type: "Feature", properties: {}, geometry: { type: "Polygon",
      coordinates: [[[116.30,39.86],[116.46,39.86],[116.46,39.98],[116.30,39.98],[116.30,39.86]]] } }],
    levels: [{ index: 0, minZoom: 14, maxZoom: 14 }], selectedLevel: 0
  } }));
  await page.evaluate(async task => {
    stopTaskPolling();
    for (const name of ["tiandituToken", "mapboxToken", "mapboxSku"]) document.getElementById("taskForm").elements[name].value = "";
    document.getElementById("buildVersion").textContent = "本地文档夹具";
    cachedTasks = [task]; taskPageTotal = 1; taskPageCounts = { all: 1, completed: 1 };
    expandedGroupTasks.clear(); setTaskMode("tasks"); await loadTasks(); renderTaskList(cachedTasks);
  }, task);
  await page.screenshot({ path: join(assetsDir, "dashboard-overview.png"), fullPage: true });
  await page.locator('[data-group-task-id="docs-terrain"] > summary').click();
  await page.waitForFunction(() => taskAreaPreviewLayer !== null);
  await page.screenshot({ path: join(assetsDir, "artifact-download.png"), fullPage: true });
  await page.locator("[data-task-mode='bbox']").click();
  await page.locator("input[name='name']").fill("Terrarium 高程下载配置");
  await page.locator("input[name='rangeMinLon']").fill("116.30");
  await page.locator("input[name='rangeMaxLon']").fill("116.46");
  await page.locator("input[name='rangeMinLat']").fill("39.86");
  await page.locator("input[name='rangeMaxLat']").fill("39.98");
  await page.locator("input[name='rangeMinZoom']").fill("14");
  await page.locator("input[name='rangeMaxZoom']").fill("14");
  await page.evaluate(() => { hideMessage("taskCreateNotice"); updateRangeEstimate(); });
  await page.screenshot({ path: join(assetsDir, "bbox-task-creation.png"), fullPage: true });
}

export async function runTerrainPreviewChecks(context, baseURL, { capture = false } = {}) {
  const fixtureParent = join(appDir, "tmp");
  await mkdir(fixtureParent, { recursive: true });
  const fixtureDir = await mkdtemp(join(fixtureParent, "terrain-browser-"));
  const page = await context.newPage();
  const external = [], errors = [], requestedTiles = new Set();
  let fixtureServer;
  try {
    await exec("go", ["test", ".", "-run", "^TestTerrainBrowserFixture$", "-count=1"], {
      cwd: appDir, env: { ...process.env, TILER_TERRAIN_FIXTURE_DIR: fixtureDir }, windowsHide: true, timeout: 180000
    });
    const info = JSON.parse(await readFile(join(fixtureDir, "terrain-info.json"), "utf8"));
    fixtureServer = createServer(async (request, response) => {
      const pathname = new URL(request.url, "http://fixture.local").pathname.slice(1);
      const path = resolve(fixtureDir, pathname);
      const rel = relative(fixtureDir, path);
      if (rel.startsWith("..") || isAbsolute(rel)) { response.writeHead(400).end(); return; }
      try {
        const body = await readFile(path);
        const mesh = path.endsWith(".terrain");
        if (mesh) requestedTiles.add(pathname);
        response.writeHead(200, { "content-type": mesh ? "application/vnd.quantized-mesh" : "application/json",
          "access-control-allow-origin": "*", ...(mesh ? { "content-encoding": "gzip" } : {}) });
        response.end(body);
      } catch { response.writeHead(404).end(); }
    });
    fixtureServer.listen(0, "127.0.0.1");
    await once(fixtureServer, "listening");
    const fixtureOrigin = `http://127.0.0.1:${fixtureServer.address().port}`;
    info.url = `${fixtureOrigin}/`;
    page.on("pageerror", error => errors.push(error.message));
    page.on("console", message => { if (message.type() === "error") errors.push(message.text()); });
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (/^https?:$/.test(url.protocol) && url.origin !== new URL(baseURL).origin && url.origin !== fixtureOrigin) {
        external.push(url.origin); await route.abort(); return;
      }
      await route.continue();
    });
    await page.route("**/api/tasks/fixture-terrain/terrain-info", route => route.fulfill({ json: info }));
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`${baseURL}/static/terrain-preview.html?task=fixture-terrain`, { waitUntil: "networkidle" });
    await page.waitForFunction(() => window.terrainPreview?.ready, null, { timeout: 60000 });
    const geometry = await page.evaluate(async () => {
      const { provider } = window.terrainPreview;
      const root = await provider.requestTileGeometry(0, 0, 0);
      const mesh = await root.createMesh({ tilingScheme: provider.tilingScheme, x: 0, y: 0, level: 0, throttle: false });
      const sampled = await Cesium.sampleTerrain(provider, 3, [-120, 0, 120].map(lon => Cesium.Cartographic.fromDegrees(lon, 35)));
      return { finiteRoot: Array.from(mesh.vertices).every(Number.isFinite), rootVertices: mesh.vertices.length,
        rootRadius: mesh.boundingSphere3D.radius, heights: sampled.map(position => position.height), cesium: Cesium.VERSION };
    });
    assert(geometry.finiteRoot && geometry.rootVertices > 0 && Number.isFinite(geometry.rootRadius), "root mesh must decode to finite, usable geometry");
    assert(geometry.heights.every(height => Number.isFinite(height) && height > 0 && height < 1100), "sampled heights must be real finite fixture elevations");
    assert(new Set(geometry.heights.map(Math.round)).size === 3, "separated longitude samples must retain different source elevations");
    const views = [];
    for (const longitude of [null, -120, 0, 120]) {
      if (longitude === null) await page.locator("#terrain-world").click();
      else await page.evaluate(lon => terrainPreview.viewer.camera.setView({ destination: Cesium.Cartesian3.fromDegrees(lon, 35, 1800000) }), longitude);
      await settleTerrain(page);
      const visible = await page.evaluate(() => {
        const scene = terrainPreview.viewer.scene;
        scene.render();
        const tiles = scene.globe._surface._tilesToRender;
        const pixels = scene.context.readPixels({ x: scene.drawingBufferWidth >> 1, y: scene.drawingBufferHeight >> 1, width: 1, height: 1 });
        return { tiles: tiles.length, meshTiles: tiles.filter(tile => tile.data?.mesh).length, centerPixel: Array.from(pixels) };
      });
      assert(visible.meshTiles > 0 && visible.centerPixel.slice(0, 3).some(value => value > 30), `camera ${longitude} must render actual visible terrain meshes`);
      views.push({ longitude, ...visible });
    }
    for (const width of [390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      const bounds = await page.locator(".toolbar").boundingBox();
      assert(bounds.x >= 0 && bounds.x + bounds.width <= width + 1, `terrain toolbar must fit ${width}px`);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `terrain page must not overflow at ${width}px`);
    }
    assert(await page.locator("#terrain-error").isHidden(), "terrain provider must not report a rendering/data error");
    assert.deepEqual(errors, [], "Cesium must have no console or page errors");
    assert.deepEqual(external, [], "offline terrain preview must not contact an external service");
    assert(await page.locator("#terrain-color").getAttribute("aria-pressed") === "true", "elevation coloring should be enabled by default");
    await page.locator("#terrain-color").click();
    assert(await page.locator("#elevation-legend").isHidden(), "plain-color mode should hide the elevation legend");
    await page.locator("#terrain-color").click();
    assert(await page.evaluate(()=>terrainPreview.viewer.scene.globe.material.type === "ElevationRamp"), "coloring toggle should restore the height material");
    if (capture) {
      await mkdir(assetsDir, { recursive: true });
      await page.evaluate(() => {
        document.querySelector(".toolbar h1").textContent = "本地合成高程夹具";
        terrainPreview.viewer.camera.setView({ destination: Cesium.Cartesian3.fromDegrees(115, 32, 350000), orientation: { heading: 0, pitch: -0.65, roll: 0 } });
      });
      await settleTerrain(page);
      await page.screenshot({ path: join(assetsDir, "terrain-preview.png") });
    }
    return { ...geometry, tileRequests: requestedTiles.size, views, externalRequests: external.length, responsiveWidths: [390, 768, 1440] };
  } finally {
    await page.close();
    if (fixtureServer) await new Promise(resolveClose => { fixtureServer.close(resolveClose); fixtureServer.closeAllConnections(); });
    // mkdtemp returned this owned child; verify the boundary before recursive cleanup.
    assert(dirname(fixtureDir) === fixtureParent && fixtureDir.startsWith(join(fixtureParent, "terrain-browser-")));
    await rm(fixtureDir, { recursive: true, force: true });
  }
}

async function settleTerrain(page) {
  // A camera move queues work on the next render; do not accept tilesLoaded from the previous view.
  await page.evaluate(() => new Promise(resolveFrame => requestAnimationFrame(() => requestAnimationFrame(resolveFrame))));
  await page.waitForFunction(() => terrainPreview.viewer.scene.globe.tilesLoaded && terrainPreview.viewer.scene.globe._surface._tilesToRender.length > 0, null, { timeout: 60000 });
}

export async function writeScreenshotManifest(terrain) {
  const [head, status, packageText] = await Promise.all([
    exec("git", ["rev-parse", "HEAD"], { cwd: repoDir, windowsHide: true }),
    exec("git", ["status", "--porcelain"], { cwd: repoDir, windowsHide: true }),
    readFile(join(appDir, "package.json"), "utf8")
  ]);
  const packages = JSON.parse(packageText);
  const sourceHashes = Object.fromEntries(await Promise.all(["index.html", "script.js", "styles.css", "terrain-preview.html", "terrain-preview.js"].map(async name => [
    `apps/admin-region-tiler/static/${name}`, createHash("sha256").update(await readFile(join(appDir, "static", name))).digest("hex")
  ])));
  await writeFile(join(assetsDir, "screenshots.json"), JSON.stringify({
    generatedAt: new Date().toISOString(), sourceCommit: head.stdout.trim(), workingTreeDirty: Boolean(status.stdout.trim()),
    sourceHashes,
    reproduce: "cd apps/admin-region-tiler && npm ci && npx playwright install chromium && node scripts/capture_docs.mjs",
    versions: { playwright: packages.devDependencies.playwright, cesium: packages.dependencies.cesium }, viewport: { width: 1440, height: 1000 },
    fixture: "本地合成高程夹具，非真实地区 DEM。85 generated quantized-mesh tiles (z0–3); base=600+400*sin(lonRadians)*cos(latRadians), local peaks=2000*exp(-((lonDegrees-115)^2+(latDegrees-35)^2)/3)*sin((lonDegrees-112)*PI/2)^2. No external terrain or imagery requests.",
    images: ["dashboard-overview.png", "bbox-task-creation.png", "artifact-download.png", "terrain-preview.png"], verification: terrain
  }, null, 2) + "\n");
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const result = await exec(process.execPath, [join(appDir, "scripts/smoke_ui.mjs"), "--capture-docs", ...process.argv.slice(2)], { cwd: appDir, windowsHide: true, timeout: 300000, maxBuffer: 4 * 1024 * 1024 });
    process.stdout.write(result.stdout); process.stderr.write(result.stderr);
  } catch (error) {
    process.stdout.write(error.stdout || ""); process.stderr.write(error.stderr || error.message);
    process.exitCode = 1;
  }
}

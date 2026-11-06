#!/usr/bin/env node
// Browser actions use the real API, SQLite and worker. Only the upstream tile service is a fixture.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { once } from "node:events";
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { deflateSync, inflateRawSync, gunzipSync } from "node:zlib";

const appDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export async function prepareWorkflowFixture(directory) {
  const parent = join(directory, "tmp");
  await mkdir(parent, { recursive: true });
  const root = await mkdtemp(join(parent, "workflow-browser-"));
  const png = terrainPNG();
  const waiting = new Set();
  const requests = [];
  const external = [];
  let hold = true, failing = true;
  const server = createServer(async (request, response) => {
    const path = new URL(request.url, "http://fixture.local").pathname;
    requests.push(path);
    if (hold) await new Promise(release => {
      const finish = () => { waiting.delete(finish); release(); };
      waiting.add(finish); response.once("close", finish);
    });
    if (response.destroyed) return;
    if (failing && path === "/1/1/1.png") { response.writeHead(503).end("fixture retry required"); return; }
    response.writeHead(200, { "content-type": "image/png" }).end(png);
  });
  server.listen(0, "127.0.0.1"); await once(server, "listening");
  const origin = `http://127.0.0.1:${server.address().port}`;
  const configPath = join(root, "fixture.toml");
  await writeFile(configPath, `[output]
format = "file"
directory = '${join(root, "output").replaceAll("\\", "/")}'
[task]
workers = 1
savepipe = 1
timedelay = 0
time_jitter_ms = 0
max_retries = 0
retry_passes = 0
retry_backoff_ms = 0
slow_backoff_ms = 0
max_slow_backoff_ms = 0
request_timeout_seconds = 30
[[tilemaps]]
id = 1
name = "本地高程工作流夹具"
url = "${origin}/{z}/{x}/{y}.png"
format = "png"
schema = "xyz"
min_zoom = 0
max_zoom = 1
metadata = { kind = "elevation", encoding = "terrarium", projection = "EPSG:3857", min_zoom = 0, max_zoom = 1, default_zoom = 1, attribution = "本地合成100米高程夹具", vertical_datum = "Synthetic test fixture" }
[[download_regions]]
id = "world"
name = "全球"
min_zoom = 0
max_zoom = 0
geojson = "./geojson/global.geojson"
[[download_regions]]
id = "china"
name = "中国"
min_zoom = 0
max_zoom = 0
geojson = "./geojson/china.geojson"
`);
  return {
    configPath, root, png, requests, external,
    reset() { hold = true; failing = true; requests.length = 0; },
    release() { hold = false; for (const finish of waiting) finish(); },
    repair() { failing = false; },
    async guard(context, baseURL) {
      await context.route("**/*", async route => {
        const url = new URL(route.request().url());
        if (/^https?:$/.test(url.protocol) && ![new URL(baseURL).origin, origin].includes(url.origin)) {
          external.push(url.origin); await route.abort(); return;
        }
        await route.continue();
      });
    },
    async cleanup() {
      this.release();
      await new Promise(done => { server.close(done); server.closeAllConnections(); });
      assert(dirname(root) === parent && root.startsWith(join(parent, "workflow-browser-")));
      await rm(root, { recursive: true, force: true });
    }
  };
}

export async function runWorkflowAcceptance(page, baseURL, fixture) {
  const client = page.context().request;
  const owned = [];
  const results = [];
  const errors = [];
  let acceptDialogs = true;
  const dialogs = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("dialog", async dialog => {
    dialogs.push(dialog.message());
    await (acceptDialogs ? dialog.accept() : dialog.dismiss());
  });
  const get = async id => {
    const response = await client.get(`${baseURL}/api/tasks/${id}`);
    assert(response.ok(), `task ${id} read failed: ${response.status()}`);
    return response.json();
  };
  const waitTask = (id, predicate, label) => eventually(async () => {
    const task = await get(id);
    return predicate(task) ? task : null;
  }, label);
  const refresh = async () => {
    const response = page.waitForResponse(response => new URL(response.url()).pathname === "/api/tasks/page");
    await page.locator("#refreshBtn").click(); await response;
    await page.waitForFunction(() => taskLoadPromise === null);
  };
  const action = async (id, name, method, endpoint, status) => {
    await refresh();
    await page.locator(`[data-task-menu-toggle="menu-${id}"]`).click();
    const response = page.waitForResponse(response => new URL(response.url()).pathname === endpoint && response.request().method() === method);
    await page.locator(`#menu-${id} [data-task-action="${name}"]`).click();
    assert.equal((await response).status(), status, `${name} API response`);
  };
  const create = async () => {
    const response = page.waitForResponse(response => new URL(response.url()).pathname === "/api/tasks" && response.request().method() === "POST");
    await page.locator("#createTaskBtn").click();
    const created = await response;
    assert.equal(created.status(), 201, await created.text());
    const task = await created.json(); owned.push(task.id);
    assert.equal(task.children.length, 1);
    return { ...task, submitted: created.request().postDataJSON() };
  };
  const openArtifact = async task => {
    await refresh();
    const card = page.locator(`[data-group-task-id="${task.id}"]`);
    // Opening a task also requests its actual range geometry from the server.
    const area = page.waitForResponse(response => new URL(response.url()).pathname === `/api/tasks/${task.id}/area`);
    await card.locator(":scope > summary").click();
    assert((await area).ok(), "task range preview API failed");
    await page.waitForFunction(id => taskPreviewTask?.id === id && taskAreaPreviewLayer?.toGeoJSON().features.length > 0, task.id);
    if (!(await card.evaluate(element => element.open))) await card.locator(":scope > summary").click();
    const child = card.locator(`[data-child-task-id="${task.children[0].id}"]`);
    if (!(await child.evaluate(element => element.open))) await child.locator(":scope > summary").click();
    return child;
  };
  const download = async (child, name) => {
    const pending = page.waitForEvent("download");
    await child.locator(".artifact-link").click();
    const file = await pending;
    assert.equal(await file.failure(), null, "browser download failed");
    const path = join(fixture.root, name); await file.saveAs(path);
    return readZIP(await readFile(path));
  };
  try {
    await checkSourceFallbackAndEstimate(page);
    for (const width of [390, 768, 1440]) for (const mode of ["rectangle", "polygon", "region"]) {
      console.log(`Starting real workflow ${width}px ${mode}`);
      await page.setViewportSize({ width, height: 1000 });
      fixture.reset();
      await page.locator(`[data-task-mode='${mode === "region" ? "region" : "bbox"}']`).click();
      await page.locator("input[name='name']").fill(`工作流 ${width} ${mode}`);
      if (width === 390 && mode === "rectangle") await assertSourceSelectionScroll(page);
      for (const [name, value] of Object.entries({workers:1,timeDelay:0})) await page.locator(`input[name='${name}']`).fill(String(value));
      if (mode === "region") {
        for (const id of [1, 2]) for (const bound of ["min", "max"]) await page.locator(`.level-${bound}[data-id='${id}']`).fill("1");
      } else {
        await page.locator("[data-range-draw-mode='rectangle']").click();
        await page.locator("#clearRangeBtn").click();
        for (const [name, value] of Object.entries({rangeMinLon:-10,rangeMaxLon:10,rangeMinLat:-10,rangeMaxLat:10,rangeMinZoom:1,rangeMaxZoom:1})) await page.locator(`input[name='${name}']`).fill(String(value));
        await page.locator("#fitRangeBtn").click();
        await page.locator("#rangeMap").scrollIntoViewIfNeeded();
        await page.locator(`[data-range-draw-mode='${mode}']`).click();
        const coordinates = mode === "polygon" ? [[-10,-10],[-10,10],[10,0]] : [[-10,-10],[10,10]];
        for (const [lat, lon] of coordinates) {
          const point = await page.evaluate(([lat,lon]) => { const value=rangeMap.latLngToContainerPoint([lat,lon]); return {x:value.x,y:value.y}; }, [lat,lon]);
          await page.locator("#rangeMap").click({position:point});
        }
        assert.equal(await page.evaluate(()=>rangeClickPoints.length), mode === "polygon" ? 3 : 2, "map selection did not retain clicked points");
      }
      await page.locator("select[name='outputFormat']").selectOption("cesium");
      const task = await create();
      assert.equal(task.submitted.workers,1); assert.equal(task.submitted.timeDelay,0);
      assert.equal(task.submitted.sources[0].metadata.encoding,"terrarium");
      if (mode === "region") assert(task.submitted.levels.length === 2 && task.submitted.levels.every(level=>level.minZoom===1&&level.maxZoom===1), "region parameters were not submitted");
      else if (mode === "polygon") assert.equal(task.submitted.area.polygon.length,3,"polygon parameters were not submitted");
      else assert(task.submitted.area.bbox.minLon<0&&task.submitted.area.bbox.maxLon>0,"rectangle parameters were not submitted");
      await eventually(() => fixture.requests.length > 0, "real worker to request the local source");
      await waitTask(task.id, task => task.status === "running", "running task");
      await action(task.id, "pause", "PUT", `/api/tasks/${task.id}/pause`, 200);
      await waitTask(task.id, task => task.status === "paused", "paused task");
      fixture.release();
      await page.waitForTimeout(250);
      assert.equal((await get(task.id)).status, "paused", "task resumed without the user's resume action");
      await action(task.id, "resume", "PUT", `/api/tasks/${task.id}/resume`, 200);
      const partial = await waitTask(task.id, task => ["failed","partial_failed"].includes(task.status) && task.canRetryFailures, "retryable failed terrain task");
      const partialChild = await openArtifact(partial);
      const pendingPartialPreview = page.waitForEvent("popup");
      await partialChild.locator(".terrain-preview-link").click();
      const partialPreview = await pendingPartialPreview;
      await partialPreview.setViewportSize({width,height:1000});
      assert.equal(await partialPreview.evaluate(()=>innerWidth),width,"partial preview must use the scenario's viewport");
      partialPreview.on("pageerror", error => errors.push(error.message));
      const fallback = await assertMissingChildFallback(partialPreview);
      await partialPreview.close();
      fixture.repair();
      const requestsBeforeRetry = fixture.requests.length;
      await action(task.id, "retryFailures", "POST", `/api/tasks/${task.id}/retry-failures`, 202);
      const complete = await waitTask(task.id, task => task.status === "completed" && task.children[0].artifactStatus === "ready", "published complete terrain task");
      assert.equal(complete.failureCount, 0);
      const retryRequests = fixture.requests.slice(requestsBeforeRetry);
      assert.deepEqual(retryRequests, ["/1/1/1.png"], "retry must reuse every previously committed raw tile");
      const child = await openArtifact(complete);
      const pendingPreview = page.waitForEvent("popup");
      await child.locator(".terrain-preview-link").click();
      const preview = await pendingPreview;
      await preview.setViewportSize({width,height:1000});
      assert.equal(await preview.evaluate(()=>innerWidth),width,"complete preview must use the scenario's viewport");
      preview.on("pageerror", error => errors.push(error.message));
      await preview.waitForFunction(() => window.terrainPreview?.ready, null, {timeout:60000});
      await preview.evaluate(() => {
        window.terrainRenderedFrames=0;
        window.terrainLastRenderedAt=performance.now();
        const scene=terrainPreview.viewer.scene;
        scene.postRender.addEventListener(()=>{
          terrainRenderedFrames++;
          window.terrainLastRenderedAt=performance.now();
          window.terrainRenderedPixel=Array.from(scene.context.readPixels({x:scene.drawingBufferWidth>>1,y:scene.drawingBufferHeight>>1,width:1,height:1}));
        });
      });
      await preview.waitForFunction(() => terrainPreview.viewer.scene.globe.tilesLoaded && terrainPreview.viewer.scene.globe._surface._tilesToRender.some(tile => tile.data?.mesh), null, {timeout:60000});
      await preview.waitForFunction(()=>terrainPreview.viewer.scene.globe.tilesLoaded && !terrainPreview.viewer.scene._renderRequested
        && performance.now()-terrainLastRenderedAt>=200);
      const idleFrames = await preview.evaluate(async () => {
        const before=terrainRenderedFrames;
        await new Promise(resolve=>setTimeout(resolve,500));
        return {count:terrainRenderedFrames-before,requestRenderMode:terrainPreview.viewer.scene.requestRenderMode};
      });
      assert(idleFrames.requestRenderMode && idleFrames.count <= 1, `idle terrain must not keep rendering: ${JSON.stringify(idleFrames)}`);
      const height = await preview.evaluate(async () => (await Cesium.sampleTerrain(terrainPreview.provider, 1, [Cesium.Cartographic.fromDegrees(0, 0)]))[0].height);
      assert(Math.abs(height - 100) < 0.01, `published terrain height differs: ${height}`);
      assert(await preview.locator("#terrain-error").isHidden());
      assert(await preview.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
      const interactionFrames = [];
      const redraw = async selector => {
        // Locator actionability can itself span frames; start counting at the
        // real click event, immediately before the application's click handler.
        await preview.locator(selector).evaluate(button=>button.addEventListener("click",()=>{window.terrainActionStartFrames=terrainRenderedFrames;},{capture:true,once:true}));
        await preview.locator(selector).click();
        await preview.waitForFunction(()=>terrainRenderedFrames>terrainActionStartFrames);
        interactionFrames.push({selector,frames:await preview.evaluate(()=>terrainRenderedFrames-terrainActionStartFrames)});
      };
      await redraw("#terrain-world");
      await redraw("#terrain-fit");
      await preview.waitForFunction(()=>terrainPreview.viewer.scene.globe.tilesLoaded);
      const coloredPixel=await preview.evaluate(()=>terrainRenderedPixel);
      await redraw("#terrain-color");
      assert(await preview.locator("#elevation-legend").isHidden());
      assert(await preview.evaluate(()=>terrainPreview.viewer.scene.globe.material===undefined),"plain-color mode must remove the height material");
      const plainPixel=await preview.evaluate(()=>terrainRenderedPixel);
      assert(coloredPixel.slice(0,3).some((value,i)=>Math.abs(value-plainPixel[i])>=5),`color toggle must redraw visibly different pixels: ${JSON.stringify({coloredPixel,plainPixel})}`);
      await redraw("#terrain-color");
      assert(await preview.evaluate(()=>terrainPreview.viewer.scene.globe.material.type==="ElevationRamp"),"height material must be restored");
      const restoredPixel=await preview.evaluate(()=>terrainRenderedPixel);
      assert(coloredPixel.slice(0,3).every((value,i)=>Math.abs(value-restoredPixel[i])<=2),"restoring height coloring must restore rendered pixels");
      await preview.close();
      const terrain = await download(child, `${width}-${mode}-terrain.zip`);
      const layer = [...terrain].find(([name]) => name.endsWith("layer.json"));
      assert(layer && JSON.parse(layer[1]).format === "quantized-mesh-1.0", "downloaded ZIP lacks the real terrain manifest");
      const meshes = [...terrain].filter(([name]) => name.endsWith(".terrain"));
      assert.equal(meshes.length, 5, "expected root and four level-one meshes");
      assert(meshes.some(([name]) => name.endsWith("0/0/0.terrain")), "download lacks root mesh");
      for (const [, bytes] of meshes) {
        const mesh = gunzipSync(bytes);
        assert.equal(mesh.readUInt32LE(88), 4225, "mesh vertex count");
        assert.equal(mesh.readFloatLE(24), 100, "mesh minimum height");
        assert.equal(mesh.readFloatLE(28), 100, "mesh maximum height");
      }
      results.push({width, mode, actions:"create/pause/resume/partial-parent-fallback/retry/area-preview/3D-preview/download", terrainMeshes:meshes.length, height, fallback, idleFrames:idleFrames.count, interactionFrames, colorPixels:{colored:coloredPixel,plain:plainPixel,restored:restoredPixel}, retryRequests:retryRequests.length});
      console.log(`Real workflow ${width}px ${mode} passed: create/pause/resume, missing-child parent fallback (${fallback.sampledHeight}m), retry, 2D/3D previews and ZIP contents`);
    }
    for (const width of [390,768,1440]) {
      await page.setViewportSize({width,height:1000});
      const selection = page.locator("#taskList .task-check").last();
      if (!(await selection.locator("input").isChecked())) await selection.click();
      const before = await page.evaluate(() => {
        for (const selector of ["#appView",".dashboard",".panel--form",".task-layout",".task-left-column","#taskHistoryPanel","#taskList"]) document.querySelector(selector).scrollTop = 120;
        window.scrollTo(0,320);
        return {window:scrollY,containers:Object.fromEntries(["#appView",".dashboard",".panel--form",".task-layout",".task-left-column","#taskHistoryPanel","#taskList"].map(selector=>[selector,document.querySelector(selector).scrollTop])),selected:[...selectedTaskIds]};
      });
      assert(before.window>0||Object.values(before.containers).some(value=>value>0), `no actual scroll position exercised at ${width}px`);
      assert(before.selected.length>0, `no selected task exercised at ${width}px`);
      await page.evaluate(()=>loadTasks());
      const after = await page.evaluate(()=>({window:scrollY,containers:Object.fromEntries(["#appView",".dashboard",".panel--form",".task-layout",".task-left-column","#taskHistoryPanel","#taskList"].map(selector=>[selector,document.querySelector(selector).scrollTop])),selected:[...selectedTaskIds]}));
      assert.deepEqual(after,before, `background API refresh lost scroll or selection at ${width}px`);
      console.log(`Real task background refresh ${width}px preserved scroll and selection: ${JSON.stringify(after)}`);
    }
    const deletedID = owned.at(-1);
    const purges = [];
    const recordPurge = request => {
      if (request.method() === "DELETE" && new URL(request.url()).pathname === `/api/tasks/${deletedID}/purge`) purges.push(request.url());
    };
    page.on("request", recordPurge);
    await refresh();
    await page.locator(`[data-task-menu-toggle="menu-${deletedID}"]`).click();
    acceptDialogs = false;
    const cancelledDialog = page.waitForEvent("dialog");
    await page.locator(`#menu-${deletedID} [data-task-action="delete"]`).click();
    await cancelledDialog;
    await page.evaluate(() => new Promise(requestAnimationFrame));
    assert.equal(purges.length, 0, "dismissing permanent-delete confirmation must not send a purge request");
    assert((await get(deletedID)).id === deletedID, "cancelled deletion removed the task");
    assert(dialogs.at(-1).includes("不可恢复"), "permanent-delete confirmation must explain irreversibility");
    acceptDialogs = true;
    await action(deletedID, "delete", "DELETE", `/api/tasks/${deletedID}/purge`, 200);
    assert.equal(purges.length, 1, "confirmed deletion must send exactly one purge request");
    assert.equal((await client.get(`${baseURL}/api/tasks/${deletedID}`)).status(), 404, "confirmed deletion must remove the real task record");
    await page.locator(`[data-group-task-id="${deletedID}"]`).waitFor({state:"detached"});
    page.off("request", recordPurge);
    owned.pop();
    console.log("Permanent deletion passed: cancel sent zero purge requests; confirm deleted one isolated task and removed its card");
    assert.deepEqual(errors, [], "browser page errors");
    assert.deepEqual(fixture.external, [], "workflow must remain on loopback services");
    return results;
  } finally {
    fixture.release(); fixture.repair();
    // These IDs belong only to this temporary DB; purge their generated geometry before the server stops.
    for (const id of owned) {
      await client.delete(`${baseURL}/api/tasks/${id}`);
      await eventually(async () => (await client.delete(`${baseURL}/api/tasks/${id}/purge`)).ok(), `cleanup ${id}`, 10000).catch(error => console.error(error.message));
    }
  }
}

async function assertMissingChildFallback(page) {
  await page.waitForFunction(() => window.terrainPreview?.ready, null, {timeout:60000});
  const availability = await page.evaluate(async () => {
    const {provider, viewer, info} = terrainPreview;
    const position = Cesium.Cartographic.fromDegrees(20, -20); // XYZ z1/x1/y1 is the intentionally absent source tile.
    const sampled = await Cesium.sampleTerrainMostDetailed(provider, [position]);
    viewer.camera.setView({destination:Cesium.Cartesian3.fromDegrees(20, -20, 2000000)});
    return {rootAvailable:provider.getTileDataAvailable(0,0,0), childAvailable:provider.getTileDataAvailable(1,1,1),
      sampledHeight:sampled[0].height, available:info.available, expected:info.expected};
  });
  assert(availability.rootAvailable && availability.childAvailable === false, "partial manifest must publish the root and exclude the missing child");
  assert(availability.available < availability.expected, "fallback must be checked before the retry completes the terrain");
  assert(Math.abs(availability.sampledHeight - 100) < 0.01, `missing child must inherit the known parent height, got ${availability.sampledHeight}`);
  await page.evaluate(() => new Promise(resolveFrame => requestAnimationFrame(() => requestAnimationFrame(resolveFrame))));
  await page.waitForFunction(() => {
    const point = Cesium.Cartographic.fromDegrees(20,-20);
    return terrainPreview.viewer.scene.globe.tilesLoaded && terrainPreview.viewer.scene.globe._surface._tilesToRender
      .some(tile => tile.data?.mesh && Cesium.Rectangle.contains(tile.rectangle, point));
  }, null, {timeout:60000});
  const rendered = await page.evaluate(() => {
    const scene = terrainPreview.viewer.scene;
    const point = Cesium.Cartographic.fromDegrees(20,-20);
    // Refresh the drawing buffer for readPixels after automatic rendering has settled.
    scene.requestRender();
    scene.render();
    const tile = scene.globe._surface._tilesToRender.find(tile => tile.data?.mesh && Cesium.Rectangle.contains(tile.rectangle, point));
    const data = tile.data.terrainData;
    const mesh = tile.data.mesh;
    let minimumHeight = Infinity, maximumHeight = -Infinity;
    // Inspect rendered vertices without skirts. The most-detailed provider
    // sample above independently checks the missing child's geographic point.
    for (let i=0;i<mesh.vertexCountWithoutSkirts;i++) {
      const height = mesh.encoding.decodeHeight(mesh.vertices, i);
      minimumHeight = Math.min(minimumHeight, height); maximumHeight = Math.max(maximumHeight, height);
    }
    const pixel = scene.context.readPixels({x:scene.drawingBufferWidth>>1,y:scene.drawingBufferHeight>>1,width:1,height:1});
    return {level:tile.level, upsampled:data.wasCreatedByUpsampling(), minimumHeight, maximumHeight,
      vertices:mesh.vertexCountWithoutSkirts, centerPixel:Array.from(pixel)};
  });
  assert(rendered.level === 0 || rendered.upsampled, "visible missing-child area must use the published parent or its upsampled mesh");
  assert(rendered.vertices > 0 && Math.abs(rendered.minimumHeight - 100) < 0.01 && Math.abs(rendered.maximumHeight - 100) < 0.01,
    `visible fallback mesh must retain nonzero parent elevation: ${JSON.stringify(rendered)}`);
  assert(rendered.centerPixel.slice(0,3).some(value=>value>30), "fallback must render visible pixels, not an empty globe");
  assert(await page.locator("#terrain-error").isHidden(), "missing available child must not cause a provider error");
  return {...availability, ...rendered};
}

async function checkSourceFallbackAndEstimate(page) {
  const handler = route => route.fulfill({ status:503,json:{error:"local catalogue failure fixture"} });
  await page.route("**/api/config/tilemaps",handler);
  try {
    await page.reload({waitUntil:"networkidle"});
    await page.locator("[data-task-mode='bbox']").click();
    await page.locator(".source-card__main[data-provider-select='高程数据']").click();
    assert(await page.locator(".tilemap-source-option[value='13']").isChecked(),"failed catalogue request lost the elevation fallback source");
    for (const [name,value] of Object.entries({rangeMinLon:1,rangeMaxLon:2,rangeMinLat:1,rangeMaxLat:2,rangeMinZoom:2,rangeMaxZoom:2})) await page.locator(`input[name='${name}']`).fill(String(value));
    assert.equal(await page.locator("#rangeTotalTileCount").innerText(),"14","Cesium estimate must include ancestors and halo requests");
    assert((await page.locator("#rangeTerrainEstimate").innerText()).includes("3 个地形网格"));
    await page.locator("select[name='outputFormat']").selectOption("zip");
    assert.equal(await page.locator("#rangeTotalTileCount").innerText(),"1","raw ZIP estimate must not include Cesium auxiliary data");
  } finally {
    await page.unroute("**/api/config/tilemaps",handler);
    await page.reload({waitUntil:"networkidle"});
    await page.locator(".tilemap-source-option[value='1']").waitFor();
  }
  console.log("Catalogue-failure fallback and real DOM terrain estimate (14 requests/3 meshes vs ZIP 1) passed");
}

async function assertSourceSelectionScroll(page) {
  const source = page.locator(".tilemap-source-option").first();
  const label = page.locator(".source-option").first();
  await label.scrollIntoViewIfNeeded();
  const position = () => page.evaluate(() => ({ window:scrollY, panel:document.querySelector(".task-left-column").scrollTop }));
  const before = await position();
  let box = await label.boundingBox(); await page.mouse.click(box.x+box.width/2,box.y+box.height/2);
  assert(!(await source.isChecked()), "source selection did not turn off");
  assert.deepEqual(await position(),before,"deselecting visible source moved the viewport");
  box = await label.boundingBox(); await page.mouse.click(box.x+box.width/2,box.y+box.height/2);
  assert(await source.isChecked(), "source selection did not turn on");
  assert.deepEqual(await position(),before,"selecting visible source moved the viewport");
}

async function eventually(check, label, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check(); if (result) return result;
    await new Promise(resolveWait => setTimeout(resolveWait, 100));
  }
  throw new Error(`Timed out: ${label}`);
}

function terrainPNG() {
  const rows = Buffer.alloc(256 * (1 + 256 * 4));
  for (let y = 0; y < 256; y++) for (let x = 0; x < 256; x++) rows.set([128,100,0,255], y * 1025 + 1 + x * 4);
  const header = Buffer.alloc(13); header.writeUInt32BE(256,0); header.writeUInt32BE(256,4); header[8]=8; header[9]=6;
  const chunk = (type, data) => {
    const result = Buffer.alloc(data.length + 12); result.writeUInt32BE(data.length); result.write(type,4); data.copy(result,8);
    result.writeUInt32BE(crc32(result.subarray(4,-4)),result.length-4); return result;
  };
  return Buffer.concat([Buffer.from([137,80,78,71,13,10,26,10]),chunk("IHDR",header),chunk("IDAT",deflateSync(rows)),chunk("IEND",Buffer.alloc(0))]);
}

function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) { crc ^= byte; for (let bit=0;bit<8;bit++) crc=(crc>>>1)^((crc&1)?0xedb88320:0); }
  return (crc ^ 0xffffffff) >>> 0;
}

function readZIP(bytes) {
  let end = bytes.length - 22;
  while (end >= 0 && bytes.readUInt32LE(end) !== 0x06054b50) end--;
  assert(end >= 0, "download is not a ZIP archive");
  const result = new Map(); let offset = bytes.readUInt32LE(end+16);
  for (let i=0;i<bytes.readUInt16LE(end+10);i++) {
    assert.equal(bytes.readUInt32LE(offset),0x02014b50);
    const length=bytes.readUInt16LE(offset+28), extra=bytes.readUInt16LE(offset+30), comment=bytes.readUInt16LE(offset+32);
    const name=bytes.subarray(offset+46,offset+46+length).toString("utf8"), method=bytes.readUInt16LE(offset+10), size=bytes.readUInt32LE(offset+20), local=bytes.readUInt32LE(offset+42);
    const start=local+30+bytes.readUInt16LE(local+26)+bytes.readUInt16LE(local+28), compressed=bytes.subarray(start,start+size);
    assert([0,8].includes(method),`unsupported fixture ZIP method ${method}`);
    const data=method===8?inflateRawSync(compressed):compressed;
    assert.equal(crc32(data),bytes.readUInt32LE(offset+16),`ZIP CRC mismatch: ${name}`);
    result.set(name,data); offset+=46+length+extra+comment;
  }
  return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const child = spawn(process.execPath, [join(appDir,"scripts/smoke_ui.mjs"),"--workflow",...process.argv.slice(2)], {cwd:appDir,stdio:"inherit",windowsHide:true});
  const [code] = await once(child,"exit"); process.exitCode=code ?? 1;
}

const fields = {
  taskName: document.querySelector("#taskName"),
  token: document.querySelector("#token"),
  maxLayer: document.querySelector("#maxLayer"),
  leftUpLon: document.querySelector("#leftUpLon"),
  leftUpLat: document.querySelector("#leftUpLat"),
  rightDownLon: document.querySelector("#rightDownLon"),
  rightDownLat: document.querySelector("#rightDownLat"),
  downloadPath: document.querySelector("#downloadPath")
};

const ui = {
  estimate: document.querySelector("#estimate"),
  message: document.querySelector("#message"),
  tileCount: document.querySelector("#tileCount"),
  totalTileCount: document.querySelector("#totalTileCount"),
  sizeEstimate: document.querySelector("#sizeEstimate"),
  hoverLngLat: document.querySelector("#hoverLngLat"),
  clickLngLat: document.querySelector("#clickLngLat"),
  allCount: document.querySelector("#allCount"),
  runningCount: document.querySelector("#runningCount"),
  doneCount: document.querySelector("#doneCount"),
  failedCount: document.querySelector("#failedCount"),
  jobList: document.querySelector("#jobList"),
  footerLog: document.querySelector("#footerLog"),
  previewSource: document.querySelector("#previewSource"),
  clearRangeBtn: document.querySelector("#clearRangeBtn"),
  resetRangeBtn: document.querySelector("#resetRangeBtn"),
  fitRangeBtn: document.querySelector("#fitRangeBtn")
};

const layerMeta = {
  img: { name: "卫星图", tone: "blue" },
  cia: { name: "路网", tone: "green" },
  vec: { name: "电子图", tone: "amber" }
};

const previewBoundsLimit = {
  west: -180,
  south: -85.05112878,
  east: 180,
  north: 85.05112878
};

const expandedJobIds = new Set();
let latestJobs = [];
let pollTimer = 0;
let rangeMap = null;
let rangeTileLayer = null;
let rangeRoadLayer = null;
let rangeRectangle = null;
let selectedRangePoints = [];
let defaultConfig = null;

document.querySelector("#calculateBtn").addEventListener("click", calculate);
document.querySelector("#startBtn").addEventListener("click", startJob);
ui.clearRangeBtn.addEventListener("click", clearRangeSelection);
ui.resetRangeBtn.addEventListener("click", resetRangeToConfig);
ui.fitRangeBtn.addEventListener("click", fitRangeToInputs);
ui.previewSource.addEventListener("change", setPreviewLayers);
ui.jobList.addEventListener("click", handleJobListAction);
Object.values(fields).forEach(input => {
  input.addEventListener("change", () => {
    calculate();
    updateRangeOverlay();
    if (input === fields.token) setPreviewLayers();
  });
});
document.querySelectorAll("input[name='layers']").forEach(input => {
  input.addEventListener("change", calculate);
});

loadConfig();
pollJobs();
startPolling();

async function loadConfig() {
  const config = await fetchJson("/api/config");
  defaultConfig = config;
  setField("taskName", "");
  setField("token", config.token);
  setField("maxLayer", config.maxLayer);
  setField("leftUpLon", config.leftUpLon);
  setField("leftUpLat", config.leftUpLat);
  setField("rightDownLon", config.rightDownLon);
  setField("rightDownLat", config.rightDownLat);
  setField("downloadPath", config.downloadPath || "/data/tiles");
  setSelectedLayers(config.layers || ["img", "cia", "vec"]);
  initRangeMap();
  calculate(false);
}

function setField(name, value) {
  fields[name].value = value ?? "";
}

function setSelectedLayers(layers) {
  document.querySelectorAll("input[name='layers']").forEach(input => {
    input.checked = layers.includes(input.value);
  });
}

function getSelectedLayers() {
  return [...document.querySelectorAll("input[name='layers']:checked")].map(input => input.value);
}

async function startJob() {
  clearMessage();
  try {
    const request = readRequest();
    const validation = validate(request);
    if (validation) {
      showMessage(validation);
      return;
    }

    const status = await fetchJson("/api/jobs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request)
    });
    ui.footerLog.textContent = `主任务已创建：${status.jobId}，输出目录：${status.outputPath}`;
    await pollJobs();
  } catch (error) {
    showMessage(error.message);
  }
}

async function handleJobListAction(event) {
  const actionTarget = event.target.closest("[data-action]");
  if (actionTarget) {
    event.preventDefault();
    event.stopPropagation();
    const jobId = actionTarget.dataset.jobId;
    const layer = actionTarget.dataset.layer;
    const action = actionTarget.dataset.action;
    clearMessage();

    if (action === "toggle") {
      toggleJob(jobId);
      previewJobRange(jobId);
      return;
    }

    if (action === "stop-job") {
      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}/stop`, { method: "POST" });
      await pollJobs();
      return;
    }

    if (action === "resume-job") {
      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}/resume`, { method: "POST" });
      expandedJobIds.add(jobId);
      await pollJobs();
      return;
    }

    if (action === "delete-job") {
      if (!window.confirm("确定删除这个任务及其已下载文件、产物和失败记录吗？")) {
        return;
      }

      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}`, { method: "DELETE" });
      expandedJobIds.delete(jobId);
      clearRangeSelection();
      await pollJobs();
      return;
    }

    if (action === "stop-layer") {
      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}/layers/${encodeURIComponent(layer)}/stop`, { method: "POST" });
      await pollJobs();
      return;
    }

    if (action === "resume-layer") {
      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}/layers/${encodeURIComponent(layer)}/resume`, { method: "POST" });
      expandedJobIds.add(jobId);
      await pollJobs();
      return;
    }

    if (action === "retry-layer") {
      await fetchJson(`/api/jobs/${encodeURIComponent(jobId)}/layers/${encodeURIComponent(layer)}/retry`, { method: "POST" });
      expandedJobIds.add(jobId);
      await pollJobs();
    }
    return;
  }

  const header = event.target.closest("[data-job-toggle]");
  if (header) {
    toggleJob(header.dataset.jobToggle);
    previewJobRange(header.dataset.jobToggle);
  }
}

function toggleJob(jobId) {
  if (expandedJobIds.has(jobId)) {
    expandedJobIds.delete(jobId);
  } else {
    expandedJobIds.add(jobId);
  }
  pollJobs();
}

function startPolling() {
  window.clearInterval(pollTimer);
  pollTimer = window.setInterval(pollJobs, 1000);
}

async function pollJobs() {
  try {
    const jobs = await fetchJson("/api/jobs");
    latestJobs = jobs || [];
    renderJobs(jobs || []);
  } catch {
  }
}

function renderJobs(jobs) {
  const running = jobs.filter(job => ["Running", "Pending"].includes(job.state)).length;
  const done = jobs.filter(job => job.state === "Completed").length;
  const failed = jobs.filter(job => ["Failed", "CompletedWithFailures"].includes(job.state) || (job.failed || 0) > 0).length;
  ui.allCount.textContent = String(jobs.length);
  ui.runningCount.textContent = String(running);
  ui.doneCount.textContent = String(done);
  ui.failedCount.textContent = String(failed);

  if (!jobs.length) {
    ui.jobList.innerHTML = `<div class="empty-jobs">暂无任务</div>`;
    return;
  }

  ui.jobList.innerHTML = jobs.map(renderJobCard).join("");
}

function renderJobCard(job) {
  const total = job.total || 0;
  const processed = job.processed || 0;
  const percent = total === 0 ? 0 : Math.round((processed / total) * 100);
  const layers = job.layers || [];
  const done = layers.filter(layer => layer.state === "Completed").length;
  const isRunning = ["Running", "Pending"].includes(job.state);
  const canResume = job.state === "Stopped";
  const expanded = expandedJobIds.has(job.jobId);
  const artifactLabel = job.artifact?.fileName || "下载主任务";
  const statusClass = getStatusClass(job.state);
  const layerKeys = (job.request?.layers || layers.map(layer => layer.key)).join(" / ");
  const rangeText = formatRange(job.request);
  const maxLayer = job.request?.maxLayer ?? "-";
  return `
    <article class="job-card ${expanded ? "expanded" : ""}">
      <div class="job-head" data-job-toggle="${escapeHtml(job.jobId)}">
        <div class="job-icon">›</div>
        <div class="job-main">
          <div class="job-name">
            <strong>${escapeHtml(job.name || "天地图瓦片下载任务")}</strong>
            <span class="status-tag ${statusClass}">${formatState(job.state || "Idle")}</span>
          </div>
          <div class="job-meta">
            <span title="${escapeHtml(rangeText)}">范围：<b>${escapeHtml(rangeText)}</b></span>
            <span>最大层级：<b>${escapeHtml(maxLayer)}</b></span>
            <span title="${escapeHtml(layerKeys)}">图层：<b>${escapeHtml(layerKeys)}</b></span>
            <span>子任务：<b>${done}/${layers.length}</b></span>
          </div>
        </div>
        <button class="icon-button" type="button" data-action="toggle" data-job-id="${escapeHtml(job.jobId)}">${expanded ? "收起" : "展开"}</button>
      </div>
      <div class="progress-track"><div class="progress-bar ${statusClass.replace("status-", "state-")}" style="width:${percent}%"></div></div>
      <div class="job-stats">
        <span>进度：<b>${processed}/${total}</b></span>
        <span>成功：<b>${job.completed || 0}</b></span>
        <span>失败：<b>${job.failed || 0}</b></span>
        <span>完成度：<b>${percent}%</b></span>
      </div>
      <div class="job-ops">
        <button type="button" data-action="stop-job" data-job-id="${escapeHtml(job.jobId)}" ${isRunning ? "" : "disabled"}>停止主任务</button>
        <button type="button" data-action="resume-job" data-job-id="${escapeHtml(job.jobId)}" ${canResume ? "" : "disabled"}>继续主任务</button>
        <a href="/api/jobs/${encodeURIComponent(job.jobId)}/archive">${escapeHtml(artifactLabel)}</a>
        <button class="danger-action" type="button" data-action="delete-job" data-job-id="${escapeHtml(job.jobId)}">删除</button>
      </div>
      <div class="layer-cards ${expanded ? "" : "collapsed"}">
        ${expanded ? layers.sort((a, b) => a.order - b.order).map(layer => renderLayerCard(job.jobId, layer)).join("") : ""}
      </div>
    </article>
  `;
}

function renderLayerCard(jobId, layer) {
  const total = layer.total || 0;
  const processed = layer.processed || 0;
  const percent = total === 0 ? 0 : Math.round((processed / total) * 100);
  const tone = layerMeta[layer.key]?.tone || "blue";
  const hasFailures = (layer.failed || layer.failureQueueCount || 0) > 0;
  const isRunning = ["Running", "Pending"].includes(layer.state);
  const canResume = layer.state === "Stopped";
  const artifactLabel = layer.artifact?.fileName || "下载子任务";
  const statusClass = getStatusClass(layer.state);
  return `
    <article class="layer-card ${tone}">
      <div class="layer-card-head">
        <div>
          <strong>${escapeHtml(layer.name)}（${escapeHtml(layer.key)}）</strong>
          <span>${escapeHtml(layer.outputPath || "-")}</span>
        </div>
        <b class="${statusClass}">${formatState(layer.state || "Idle")}</b>
      </div>
      <div class="progress-track small"><div class="progress-bar ${statusClass.replace("status-", "state-")}" style="width:${percent}%"></div></div>
      <div class="layer-metrics">
        <span>进度：<strong>${processed}/${total}</strong></span>
        <span>成功：<strong class="ok">${layer.completed || 0}</strong></span>
        <span>失败：<strong class="bad">${layer.failed || 0}</strong></span>
        <span>完成度：<strong>${percent}%</strong></span>
      </div>
      <div class="layer-actions">
        <button type="button" data-action="stop-layer" data-job-id="${escapeHtml(jobId)}" data-layer="${escapeHtml(layer.key)}" ${isRunning ? "" : "disabled"}>停止</button>
        <button type="button" data-action="resume-layer" data-job-id="${escapeHtml(jobId)}" data-layer="${escapeHtml(layer.key)}" ${canResume ? "" : "disabled"}>继续</button>
        <button type="button" data-action="retry-layer" data-job-id="${escapeHtml(jobId)}" data-layer="${escapeHtml(layer.key)}" ${hasFailures && !isRunning ? "" : "disabled"}>重试失败</button>
        <a href="/api/jobs/${encodeURIComponent(jobId)}/layers/${encodeURIComponent(layer.key)}/archive">${escapeHtml(artifactLabel)}</a>
        <a class="${hasFailures ? "" : "disabled"}" href="/api/jobs/${encodeURIComponent(jobId)}/layers/${encodeURIComponent(layer.key)}/failures">失败记录</a>
      </div>
    </article>
  `;
}

function initRangeMap() {
  if (rangeMap || typeof L === "undefined") {
    return;
  }

  const previewBounds = getPreviewWorldBounds();
  rangeMap = L.map("rangeMap", {
    zoomControl: false,
    attributionControl: false,
    worldCopyJump: false,
    maxBounds: previewBounds,
    maxBoundsViscosity: 1
  }).setView([35.8617, 104.1954], 5);

  rangeMap.on("mousemove", event => {
    ui.hoverLngLat.textContent = formatLngLat(toDownloadLatLng(normalizePreviewLatLng(event.latlng)));
  });

  rangeMap.on("click", event => {
    const latlng = toDownloadLatLng(normalizePreviewLatLng(event.latlng));
    ui.clickLngLat.textContent = formatLngLat(latlng);
    handleRangeClick(latlng);
  });
  rangeMap.on("resize", syncPreviewMinZoom);

  setPreviewLayers();
  window.setTimeout(() => {
    rangeMap.invalidateSize();
    syncPreviewMinZoom();
    rangeMap.panInsideBounds(previewBounds, { animate: false });
  }, 50);
}

function setPreviewLayers() {
  if (!rangeMap) return;

  if (rangeTileLayer) {
    rangeMap.removeLayer(rangeTileLayer);
  }
  if (rangeRoadLayer) {
    rangeMap.removeLayer(rangeRoadLayer);
    rangeRoadLayer = null;
  }

  const baseOptions = {
    minZoom: 1,
    maxZoom: 18,
    tileSize: 256,
    updateWhenIdle: true,
    updateWhenZooming: false,
    keepBuffer: 1,
    noWrap: true,
    bounds: getPreviewWorldBounds()
  };

  if (ui.previewSource.value === "amap") {
    rangeTileLayer = L.tileLayer(buildAmapTileUrl("satellite"), {
      ...baseOptions,
      subdomains: ["1", "2", "3", "4"]
    }).addTo(rangeMap);

    rangeRoadLayer = L.tileLayer(buildAmapTileUrl("road"), {
      ...baseOptions,
      subdomains: ["1", "2", "3", "4"],
      opacity: 0.9
    }).addTo(rangeMap);
  } else if (ui.previewSource.value === "google") {
    rangeTileLayer = L.tileLayer(buildGoogleTileUrl(), {
      ...baseOptions,
      subdomains: ["0", "1", "2", "3"]
    }).addTo(rangeMap);
  } else {
    rangeTileLayer = L.tileLayer(buildTianDiTuTileUrl("img"), {
      ...baseOptions,
      subdomains: ["0", "1", "2", "3", "4", "5", "6", "7"]
    }).addTo(rangeMap);

    rangeRoadLayer = L.tileLayer(buildTianDiTuTileUrl("cia"), {
      ...baseOptions,
      subdomains: ["0", "1", "2", "3", "4", "5", "6", "7"],
      opacity: 0.92
    }).addTo(rangeMap);
  }

  if (rangeRectangle) {
    updateRangeOverlay(readBoundsFromFields());
  }
}

function buildTianDiTuTileUrl(layer) {
  const token = encodeURIComponent(fields.token.value.trim());
  return `/api/tiles/${layer}/{z}/{x}/{y}?tk=${token}`;
}

function buildAmapTileUrl(kind) {
  const style = kind === "road" ? 8 : 6;
  return `https://webst0{s}.is.autonavi.com/appmaptile?style=${style}&x={x}&y={y}&z={z}`;
}

function buildGoogleTileUrl() {
  return "https://mt{s}.google.com/vt/lyrs=s&x={x}&y={y}&z={z}";
}

function updateRangeOverlay(bounds = readBoundsFromFields(), fit = false) {
  if (!rangeMap) return;
  if (!bounds) return;

  const previewBounds = toPreviewBounds(bounds);

  if (!rangeRectangle) {
    rangeRectangle = L.rectangle(previewBounds, {
      color: "#2d6cdf",
      weight: 2,
      dashArray: "8 8",
      fillColor: "#2d6cdf",
      fillOpacity: 0.15
    }).addTo(rangeMap);
  } else {
    rangeRectangle.setBounds(previewBounds);
  }

  if (fit) {
    rangeMap.fitBounds(previewBounds.pad(0.35), { animate: false, maxZoom: 16 });
    rangeMap.panInsideBounds(getPreviewWorldBounds(), { animate: false });
  }
}

function previewJobRange(jobId) {
  const job = latestJobs.find(item => item.jobId === jobId);
  const bounds = readBoundsFromRequest(job?.request);
  if (!bounds) return;
  selectedRangePoints = [];
  updateRangeOverlay(bounds, true);
}

function clearRangeSelection() {
  selectedRangePoints = [];
  if (rangeRectangle) {
    rangeMap.removeLayer(rangeRectangle);
    rangeRectangle = null;
  }
  ui.clickLngLat.textContent = "-";
  clearMessage();
}

function resetRangeToConfig() {
  if (!defaultConfig) return;
  selectedRangePoints = [];
  setField("leftUpLon", defaultConfig.leftUpLon);
  setField("leftUpLat", defaultConfig.leftUpLat);
  setField("rightDownLon", defaultConfig.rightDownLon);
  setField("rightDownLat", defaultConfig.rightDownLat);
  calculate();
  fitRangeToInputs();
  clearMessage();
}

function handleRangeClick(latlng) {
  if (selectedRangePoints.length >= 2) {
    selectedRangePoints = [];
  }

  selectedRangePoints.push(latlng);
  if (selectedRangePoints.length === 1) {
    setRangeFields(latlng, latlng);
    clearMessage();
    showMessage("已记录第一个点，请点击第二个点生成范围。");
    calculate();
    updateRangeOverlay();
    return;
  }

  const [first, second] = selectedRangePoints;
  setRangeFields(first, second);
  clearMessage();
  calculate();
  updateRangeOverlay();
}

function setRangeFields(first, second) {
  const left = Math.min(first.lng, second.lng);
  const right = Math.max(first.lng, second.lng);
  const top = Math.max(first.lat, second.lat);
  const bottom = Math.min(first.lat, second.lat);

  setField("leftUpLon", left.toFixed(6));
  setField("leftUpLat", top.toFixed(6));
  setField("rightDownLon", right.toFixed(6));
  setField("rightDownLat", bottom.toFixed(6));
}

function fitRangeToInputs() {
  if (!rangeMap) return;
  const bounds = readBoundsFromFields();
  if (!bounds) return;
  updateRangeOverlay(bounds, true);
}

function readBoundsFromFields() {
  const request = readRequest();
  return readBoundsFromRequest(request);
}

function readBoundsFromRequest(request) {
  if (!request) return null;
  if (!Number.isFinite(request.leftUpLon) || !Number.isFinite(request.leftUpLat) || !Number.isFinite(request.rightDownLon) || !Number.isFinite(request.rightDownLat)) {
    return null;
  }
  if (request.leftUpLon > request.rightDownLon || request.leftUpLat < request.rightDownLat) {
    return null;
  }
  return L.latLngBounds(
    L.latLng(request.rightDownLat, request.leftUpLon),
    L.latLng(request.leftUpLat, request.rightDownLon)
  );
}

function toPreviewBounds(bounds) {
  return L.latLngBounds([
    toPreviewLatLng(bounds.getSouthWest()),
    toPreviewLatLng(bounds.getNorthWest()),
    toPreviewLatLng(bounds.getSouthEast()),
    toPreviewLatLng(bounds.getNorthEast())
  ]);
}

function toPreviewLatLng(latlng) {
  return ui.previewSource.value === "amap" ? wgs84ToGcj02(latlng) : latlng;
}

function toDownloadLatLng(latlng) {
  return ui.previewSource.value === "amap" ? gcj02ToWgs84(latlng) : latlng;
}

function wgs84ToGcj02(latlng) {
  if (isOutsideChina(latlng.lng, latlng.lat)) return latlng;
  const delta = transformChinaDelta(latlng.lng, latlng.lat);
  return L.latLng(latlng.lat + delta.lat, latlng.lng + delta.lng);
}

function gcj02ToWgs84(latlng) {
  if (isOutsideChina(latlng.lng, latlng.lat)) return latlng;
  const delta = transformChinaDelta(latlng.lng, latlng.lat);
  return L.latLng(latlng.lat - delta.lat, latlng.lng - delta.lng);
}

function transformChinaDelta(lng, lat) {
  const a = 6378245.0;
  const ee = 0.00669342162296594323;
  let dLat = transformLat(lng - 105.0, lat - 35.0);
  let dLng = transformLng(lng - 105.0, lat - 35.0);
  const radLat = lat / 180.0 * Math.PI;
  let magic = Math.sin(radLat);
  magic = 1 - ee * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  dLat = (dLat * 180.0) / ((a * (1 - ee)) / (magic * sqrtMagic) * Math.PI);
  dLng = (dLng * 180.0) / (a / sqrtMagic * Math.cos(radLat) * Math.PI);
  return { lat: dLat, lng: dLng };
}

function transformLat(x, y) {
  let ret = -100.0 + 2.0 * x + 3.0 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  ret += (20.0 * Math.sin(6.0 * x * Math.PI) + 20.0 * Math.sin(2.0 * x * Math.PI)) * 2.0 / 3.0;
  ret += (20.0 * Math.sin(y * Math.PI) + 40.0 * Math.sin(y / 3.0 * Math.PI)) * 2.0 / 3.0;
  ret += (160.0 * Math.sin(y / 12.0 * Math.PI) + 320 * Math.sin(y * Math.PI / 30.0)) * 2.0 / 3.0;
  return ret;
}

function transformLng(x, y) {
  let ret = 300.0 + x + 2.0 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  ret += (20.0 * Math.sin(6.0 * x * Math.PI) + 20.0 * Math.sin(2.0 * x * Math.PI)) * 2.0 / 3.0;
  ret += (20.0 * Math.sin(x * Math.PI) + 40.0 * Math.sin(x / 3.0 * Math.PI)) * 2.0 / 3.0;
  ret += (150.0 * Math.sin(x / 12.0 * Math.PI) + 300.0 * Math.sin(x / 30.0 * Math.PI)) * 2.0 / 3.0;
  return ret;
}

function isOutsideChina(lng, lat) {
  return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271;
}

function getPreviewWorldBounds() {
  return L.latLngBounds(
    L.latLng(previewBoundsLimit.south, previewBoundsLimit.west),
    L.latLng(previewBoundsLimit.north, previewBoundsLimit.east)
  );
}

function normalizePreviewLatLng(latlng) {
  return L.latLng(
    clamp(latlng.lat, previewBoundsLimit.south, previewBoundsLimit.north),
    clamp(latlng.lng, previewBoundsLimit.west, previewBoundsLimit.east)
  );
}

function syncPreviewMinZoom() {
  if (!rangeMap) return;
  const width = Math.max(rangeMap.getSize().x, 256);
  const minZoom = Math.max(1, Math.ceil(Math.log2(width / 256)));
  rangeMap.setMinZoom(minZoom);
  if (rangeMap.getZoom() < minZoom) {
    rangeMap.setZoom(minZoom, { animate: false });
  }
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function formatLngLat(latlng) {
  return `${latlng.lng.toFixed(6)},${latlng.lat.toFixed(6)}`;
}

function calculate(updateOverlay = true) {
  clearMessage();
  try {
    const request = readRequest();
    const validation = validate(request, true);
    if (validation) {
      showMessage(validation);
      return;
    }

    const count = calculateTiles(request).length;
    const layerCount = request.layers.length;
    const total = count * layerCount;
    const mb = ((total * 8) / 1024).toFixed(2);
    if (ui.tileCount) ui.tileCount.textContent = String(count);
    if (ui.totalTileCount) ui.totalTileCount.textContent = String(total);
    if (ui.sizeEstimate) ui.sizeEstimate.textContent = `${mb} MB`;
    ui.estimate.textContent = `单图层 ${count} 个瓦片，${layerCount} 个子任务合计 ${total} 个，按原程序估算约 ${mb} MB。`;
    if (updateOverlay) updateRangeOverlay();
  } catch (error) {
    showMessage(error.message);
  }
}

function readRequest() {
  return {
    name: fields.taskName.value.trim() || "天地图瓦片下载任务",
    token: fields.token.value.trim(),
    layers: getSelectedLayers(),
    maxLayer: Number(fields.maxLayer.value),
    leftUpLon: Number(fields.leftUpLon.value),
    leftUpLat: Number(fields.leftUpLat.value),
    rightDownLon: Number(fields.rightDownLon.value),
    rightDownLat: Number(fields.rightDownLat.value),
    downloadPath: fields.downloadPath.value.trim() || "/data/tiles"
  };
}

function validate(request, estimateOnly = false) {
  if (!request.token && !estimateOnly) return "Token 不能为空。";
  if (!request.layers.length) return "至少选择一个图层。";
  if (!Number.isInteger(request.maxLayer) || request.maxLayer < 1 || request.maxLayer > 18) return "最大层级必须在 1 到 18 之间。";
  for (const name of ["leftUpLon", "leftUpLat", "rightDownLon", "rightDownLat"]) {
    if (!Number.isFinite(request[name])) return "经纬度必须是数字。";
  }
  if (request.leftUpLon > request.rightDownLon) return "左上经度不能大于右下经度。";
  if (request.leftUpLat < request.rightDownLat) return "左上纬度不能小于右下纬度。";
  return "";
}

function calculateTiles(request) {
  const tiles = [];
  for (let layer = 0; layer < request.maxLayer; layer += 1) {
    const leftUp = getTileId(request.leftUpLon, request.leftUpLat, layer);
    const rightDown = getTileId(request.rightDownLon, request.rightDownLat, layer);
    for (let x = leftUp.x; x <= rightDown.x; x += 1) {
      for (let y = leftUp.y; y <= rightDown.y; y += 1) {
        tiles.push({ z: layer, x, y });
      }
    }
  }
  return tiles;
}

function getTileId(lon, lat, layer) {
  const x = Math.trunc(Math.pow(2, layer - 1) * (lon / 180 + 1));
  const rad = Math.PI * lat / 180;
  const y = Math.trunc(Math.pow(2, layer - 1) * (1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI));
  return { z: layer, x, y };
}

async function fetchJson(url, options = {}) {
  const response = await fetch(url, options);
  if (!response.ok) {
    let message = `请求失败：${response.status}`;
    try {
      const body = await response.json();
      message = body.error || message;
    } catch {
    }
    throw new Error(message);
  }
  return response.json();
}

function formatTime(value) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function formatRange(request) {
  if (!request) return "-";
  return `${formatNumber(request.leftUpLon)},${formatNumber(request.leftUpLat)} - ${formatNumber(request.rightDownLon)},${formatNumber(request.rightDownLat)}`;
}

function formatNumber(value) {
  return Number.isFinite(Number(value)) ? Number(value).toFixed(4) : "-";
}

function getStatusClass(state) {
  return `status-${String(state || "Idle").toLowerCase()}`;
}

function formatState(state) {
  const states = {
    Idle: "等待中",
    Pending: "计划中",
    Running: "运行中",
    Completed: "已完成",
    Failed: "失败",
    Stopped: "已停止",
    CompletedWithFailures: "完成有失败"
  };
  return states[state] || state || "等待中";
}

function showMessage(text) {
  ui.message.textContent = text;
}

function clearMessage() {
  ui.message.textContent = "";
}

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, char => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#039;"
  })[char]);
}

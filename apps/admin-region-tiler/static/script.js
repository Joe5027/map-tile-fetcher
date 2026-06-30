let tilemaps = [];
let regionCatalog = [];
let missingRegionCatalog = [];
let levelConfigs = [];
let activeLevelCount = 2;
let selectedProvider = "";
let selectedSourceIds = new Set();
let taskMode = "region";
let rangeMap = null;
let rangeBaseLayer = null;
let rangeRectangle = null;
let rangePolyline = null;
let rangePolygon = null;
let rangePointMarkers = [];
let rangeClickPoints = [];
let rangeDrawMode = "rectangle";
let rangeMapExpanded = false;
let rangeMapOriginalParent = null;
let rangeMapOriginalNextSibling = null;
let adminRegionMap = null;
let adminRegionBaseLayer = null;
let adminRegionLayerGroup = null;
let adminRegionSelectedLayer = null;
let adminRegionLevel = "province";
let adminRegionRenderToken = 0;
let adminRegionMapInitRetries = 0;
let rangeMapInitRetries = 0;
let activeWorkspaceTab = "create";
let currentTaskFilter = "all";
let cachedTasks = [];
const expandedProviders = new Set();
const expandedGroupTasks = new Set();
const expandedChildTasks = new Set();
const selectedTaskIds = new Set();
const adminRegionGeoJSONCache = new Map();
const WEB_MERCATOR_MAX_LAT = 85.05112878;
const HORIZONTAL_DRAG_LIMIT_LNG = 360000;
const MAP_LATITUDE_BOUNDS = [
    [-WEB_MERCATOR_MAX_LAT, -HORIZONTAL_DRAG_LIMIT_LNG],
    [WEB_MERCATOR_MAX_LAT, HORIZONTAL_DRAG_LIMIT_LNG]
];
const MAP_MIN_ZOOM = 2;
const MAP_INIT_RETRY_LIMIT = 30;

const REGION_LEVEL_RULES = [
    { id: 1, label: "第1级", regionLevel: "world", fixedRegionId: "world", minZoom: 0, maxZoom: 5 },
    { id: 2, label: "第2级", regionLevel: "country", fixedRegionId: "china", minZoom: 6, maxZoom: 8 },
    { id: 3, label: "第3级", regionLevel: "province", parentConfigId: 2, minZoom: 9, maxZoom: 10 },
    { id: 4, label: "第4级", regionLevel: "city", parentConfigId: 3, minZoom: 11, maxZoom: 12 },
    { id: 5, label: "第5级", regionLevel: "district", parentConfigId: 4, minZoom: 13, maxZoom: 14 }
];

const FALLBACK_TILEMAPS = [
    { id: 1, name: "电子地图 (vec_w)", url: "https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 18 },
    { id: 2, name: "路网 (cia_w)", url: "https://t0.tianditu.gov.cn/DataServer?T=cia_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 18 },
    { id: 3, name: "卫星图 (img_w)", url: "https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 18 },
    { id: 6, name: "Google电子图", url: "http://mt0.google.com/vt/lyrs=m&x={x}&y={y}&z={z}", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 20 },
    { id: 7, name: "Google路网", url: "http://mt0.google.com/vt/lyrs=h&x={x}&y={y}&z={z}", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 20 },
    { id: 8, name: "Google卫星图", url: "http://mt0.google.com/vt/lyrs=s&x={x}&y={y}&z={z}", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 20 },
    { id: 9, name: "Mapbox街道图", url: "https://api.mapbox.com/v4/mapbox.mapbox-streets-v8/{z}/{x}/{y}.vector.pbf?sku=YOUR_MAPBOX_SKU&access_token=YOUR_MAPBOX_TOKEN", format: "pbf", schema: "xyz", min_zoom: 0, max_zoom: 14 },
    { id: 10, name: "Mapbox地形图", url: "https://api.mapbox.com/v4/mapbox.mapbox-terrain-v2/{z}/{x}/{y}.vector.pbf?sku=YOUR_MAPBOX_SKU&access_token=YOUR_MAPBOX_TOKEN", format: "pbf", schema: "xyz", min_zoom: 0, max_zoom: 14 }
];

const RANGE_LAYER_NAMES = {
    img: "天地图 img 卫星图",
    cia: "天地图 cia 路网",
    vec: "天地图 vec 电子图"
};

const ADMIN_REGION_LEVEL_CONFIG_IDS = {
    province: 3,
    city: 4,
    district: 5
};

const ADMIN_REGION_LEVEL_LABELS = {
    province: "省级",
    city: "市级",
    district: "区县"
};

const ADMIN_REGION_NEXT_LEVELS = {
    province: "city",
    city: "district"
};

const ADMIN_REGION_PREVIOUS_LEVELS = {
    city: "province",
    district: "city"
};

document.addEventListener("DOMContentLoaded", async () => {
    bindEvents();
    await bootstrap();
});

function bindEvents() {
    document.getElementById("loginForm").addEventListener("submit", login);
    document.getElementById("logoutBtn").addEventListener("click", logout);
    document.getElementById("taskForm").addEventListener("submit", createTask);
    document.getElementById("refreshBtn").addEventListener("click", loadTasks);
    document.getElementById("addLevelBtn").addEventListener("click", addLevelConfig);
    document.getElementById("fitAdminRegionMapBtn").addEventListener("click", fitAdminRegionMapToSelection);

    document.querySelectorAll("[data-workspace-tab]").forEach((button) => {
        button.addEventListener("click", () => setWorkspaceTab(button.dataset.workspaceTab));
    });

    document.querySelectorAll("[data-task-mode]").forEach((button) => {
        button.addEventListener("click", () => setTaskMode(button.dataset.taskMode));
    });

    document.querySelectorAll("[data-admin-region-level]").forEach((button) => {
        button.addEventListener("click", () => setAdminRegionLevel(button.dataset.adminRegionLevel));
    });

    document.querySelectorAll("input[name='scheduleMode']").forEach((element) => {
        element.addEventListener("change", syncScheduleControls);
    });

    document.querySelectorAll("[name^='range'], [name='tiandituToken'], [name='mapboxToken'], [name='mapboxSku']").forEach((element) => {
        element.addEventListener("input", () => {
            updateRangeEstimate();
            updateRangeOverlay();
            if (element.name === "tiandituToken") {
                updateRangeBaseLayer();
            }
        });
    });

    document.getElementById("rangePreviewSource").addEventListener("change", updateRangeBaseLayer);

    document.querySelectorAll("[data-range-draw-mode]").forEach((button) => {
        button.addEventListener("click", () => setRangeDrawMode(button.dataset.rangeDrawMode));
    });

    document.querySelectorAll(".range-layer-option").forEach((element) => {
        element.addEventListener("change", updateRangeEstimate);
    });

    document.getElementById("undoRangePointBtn").addEventListener("click", undoRangePoint);
    document.getElementById("clearRangeBtn").addEventListener("click", clearRangeSelection);
    document.getElementById("fitRangeBtn").addEventListener("click", fitRangeMapToFields);
    document.getElementById("expandRangeMapBtn").addEventListener("click", () => setRangeMapExpanded(true));
    document.getElementById("finishRangeMapBtn").addEventListener("click", () => setRangeMapExpanded(false));

    document.getElementById("tilemapSelector").addEventListener("change", (event) => {
        if (event.target.matches(".tilemap-source-option")) {
            syncSelectedSourceIds();
            renderTilemapSelector();
        }
    });

    document.getElementById("tilemapSelector").addEventListener("click", (event) => {
        const selectButton = event.target.closest("[data-provider-select]");
        if (selectButton) {
            selectedProvider = selectButton.dataset.providerSelect;
            expandedProviders.add(selectedProvider);
            ensureProviderSelection();
            renderTilemapSelector();
            return;
        }

        const toggleButton = event.target.closest("[data-provider-toggle]");
        if (toggleButton) {
            const provider = toggleButton.dataset.providerToggle;
            if (selectedProvider !== provider) {
                selectedProvider = provider;
                ensureProviderSelection();
            }
            if (expandedProviders.has(provider)) {
                expandedProviders.delete(provider);
            } else {
                expandedProviders.add(provider);
            }
            renderTilemapSelector();
            return;
        }

        const selectAllButton = event.target.closest("[data-provider-select-all]");
        if (selectAllButton) {
            const provider = selectAllButton.dataset.providerSelectAll;
            selectAllProviderItems(provider);
            renderTilemapSelector();
            return;
        }

        const clearButton = event.target.closest("[data-provider-clear]");
        if (clearButton) {
            const provider = clearButton.dataset.providerClear;
            clearProviderItems(provider);
            renderTilemapSelector();
        }
    });

    document.getElementById("taskStats").addEventListener("click", (event) => {
        const button = event.target.closest("[data-task-filter]");
        if (!button) {
            return;
        }
        currentTaskFilter = button.dataset.taskFilter;
        renderTaskStats(cachedTasks);
        renderTaskList(cachedTasks);
    });

    document.getElementById("taskMoreBtn").addEventListener("click", (event) => {
        event.stopPropagation();
        document.getElementById("taskMoreMenu").classList.toggle("is-hidden");
    });

    document.getElementById("taskMoreMenu").addEventListener("click", async (event) => {
        const button = event.target.closest("[data-bulk-action]");
        if (!button) {
            return;
        }
        document.getElementById("taskMoreMenu").classList.add("is-hidden");
        await handleBulkAction(button.dataset.bulkAction);
    });

    document.getElementById("taskList").addEventListener("toggle", (event) => {
        const groupTask = event.target.closest("details[data-group-task-id]");
        if (groupTask) {
            const id = groupTask.dataset.groupTaskId;
            if (groupTask.open) {
                expandedGroupTasks.add(id);
            } else {
                expandedGroupTasks.delete(id);
            }
        }

        const childTask = event.target.closest("details[data-child-task-id]");
        if (childTask) {
            const id = childTask.dataset.childTaskId;
            if (childTask.open) {
                expandedChildTasks.add(id);
            } else {
                expandedChildTasks.delete(id);
            }
        }
    }, true);

    document.getElementById("taskList").addEventListener("change", (event) => {
        if (event.target.matches(".task-select")) {
            const taskId = event.target.dataset.taskId;
            if (!taskId) {
                return;
            }
            if (event.target.checked) {
                selectedTaskIds.add(taskId);
            } else {
                selectedTaskIds.delete(taskId);
            }
            renderSelectionMeta();
        }
    });

    document.getElementById("taskList").addEventListener("click", async (event) => {
        const menuToggle = event.target.closest("[data-task-menu-toggle]");
        if (menuToggle) {
            event.preventDefault();
            event.stopPropagation();
            toggleTaskMenu(menuToggle.dataset.taskMenuToggle);
            return;
        }

        const actionButton = event.target.closest("[data-task-action]");
        if (actionButton) {
            event.preventDefault();
            event.stopPropagation();
            await handleTaskAction(
                actionButton.dataset.taskAction,
                actionButton.dataset.taskId,
                actionButton.dataset.taskStatus
            );
            return;
        }
    });

    document.addEventListener("click", () => {
        document.getElementById("taskMoreMenu").classList.add("is-hidden");
        closeAllTaskMenus();
    });

    document.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && rangeMapExpanded) {
            setRangeMapExpanded(false);
        }
    });
}

async function bootstrap() {
    const me = await fetchJSON("/api/auth/me", { allowUnauthorized: true });
    if (!me.ok) {
        showLogin();
        return;
    }

    showApp(me.data);
    await Promise.all([loadTilemaps(), loadRegionCatalog()]);
    initDefaultLevelConfigs();
    renderLevelConfigs();
    setTaskMode(taskMode);
    initAdminRegionMap();
    syncScheduleControls();
    updateRangeEstimate();
    await loadTasks();
    window.setInterval(loadTasks, 5000);
}

function showLogin() {
    document.getElementById("loginView").classList.remove("is-hidden");
    document.getElementById("appView").classList.add("is-hidden");
}

function showApp(user) {
    document.getElementById("currentUsername").textContent = user.username;
    document.getElementById("loginView").classList.add("is-hidden");
    document.getElementById("appView").classList.remove("is-hidden");
}

function setWorkspaceTab(tab) {
    activeWorkspaceTab = tab === "tasks" ? "tasks" : "create";

    document.querySelectorAll("[data-workspace-tab]").forEach((button) => {
        const active = button.dataset.workspaceTab === activeWorkspaceTab;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-selected", active ? "true" : "false");
        button.tabIndex = active ? 0 : -1;
    });

    document.querySelectorAll("[data-workspace-view]").forEach((view) => {
        view.classList.toggle("is-hidden", view.dataset.workspaceView !== activeWorkspaceTab);
    });

    if (activeWorkspaceTab === "create" && taskMode === "bbox" && rangeMap) {
        window.setTimeout(() => rangeMap.invalidateSize(), 40);
    }
    if (activeWorkspaceTab === "create" && taskMode === "region" && adminRegionMap) {
        window.setTimeout(() => adminRegionMap.invalidateSize(), 40);
    }
}

function setTaskMode(mode) {
    taskMode = mode === "bbox" ? "bbox" : "region";

    document.querySelectorAll("[data-task-mode]").forEach((button) => {
        const active = button.dataset.taskMode === taskMode;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-pressed", active ? "true" : "false");
    });

    document.getElementById("sourceSection").classList.toggle("is-hidden", taskMode !== "region");
    document.getElementById("regionConfigPanel").classList.toggle("is-hidden", taskMode !== "region");
    document.getElementById("regionModePanel").classList.toggle("is-hidden", taskMode !== "region");
    document.getElementById("rangeModePanel").classList.toggle("is-hidden", taskMode !== "bbox");
    document.getElementById("rangePreviewPanel").classList.toggle("is-hidden", taskMode !== "bbox");
    document.getElementById("addLevelBtn").classList.toggle("is-hidden", taskMode !== "region");
    document.getElementById("taskForm").classList.toggle("task-form--range", taskMode === "bbox");
    if (taskMode === "bbox") {
        initRangeMap();
        updateRangeDrawControls();
        updateRangeEstimate();
        updateRangeOverlay();
        window.setTimeout(() => {
            if (rangeMap) {
                rangeMap.invalidateSize();
            }
        }, 40);
    } else {
        initAdminRegionMap();
        window.setTimeout(() => {
            if (adminRegionMap) {
                adminRegionMap.invalidateSize();
                fitAdminRegionMapToSelection();
            }
        }, 40);
    }
}

function syncScheduleControls() {
    const selected = document.querySelector("input[name='scheduleMode']:checked");
    const runAtField = document.getElementById("runAtField");
    if (!selected || !runAtField) {
        return;
    }
    const scheduled = selected.value === "once";
    runAtField.classList.toggle("is-hidden", !scheduled);
    const input = runAtField.querySelector("input[name='runAt']");
    if (input) {
        input.required = scheduled;
    }
}

function readScheduleRequest(formData) {
    const scheduleMode = String(formData.get("scheduleMode") || "immediate");
    if (scheduleMode !== "once") {
        return {
            scheduleMode: "immediate",
            runAt: "",
            label: "立即执行"
        };
    }

    const rawRunAt = String(formData.get("runAt") || "").trim();
    if (!rawRunAt) {
        return { error: "请选择计划执行时间。" };
    }
    const runAt = new Date(rawRunAt);
    if (Number.isNaN(runAt.getTime())) {
        return { error: "计划执行时间无效。" };
    }
    return {
        scheduleMode: "once",
        runAt: runAt.toISOString(),
        label: `定时执行：${formatDate(runAt.toISOString())}`
    };
}

function readOutputRequest(formData) {
    const format = String(formData.get("outputFormat") || "zip").trim().toLowerCase();
    if (format === "mbtiles") {
        return {
            format: "mbtiles",
            label: "MBTiles"
        };
    }
    return {
        format: "zip",
        label: "ZIP 文件树"
    };
}

function readCredentialRequest(formData) {
    return {
        tiandituToken: String(formData.get("tiandituToken") || "").trim(),
        mapboxToken: String(formData.get("mapboxToken") || "").trim(),
        mapboxSku: String(formData.get("mapboxSku") || "").trim()
    };
}

function hasUsableCredential(value, placeholder) {
    const trimmed = String(value || "").trim();
    return Boolean(trimmed && trimmed !== placeholder);
}

function applySourceCredentials(rawURL, credentials) {
    let url = String(rawURL || "");
    if (hasUsableCredential(credentials.tiandituToken, "YOUR_TIANDITU_TOKEN")) {
        url = url.replaceAll("YOUR_TIANDITU_TOKEN", encodeURIComponent(credentials.tiandituToken));
    }
    if (hasUsableCredential(credentials.mapboxToken, "YOUR_MAPBOX_TOKEN")) {
        url = url.replaceAll("YOUR_MAPBOX_TOKEN", encodeURIComponent(credentials.mapboxToken));
    }
    if (url.includes("YOUR_MAPBOX_SKU")) {
        if (hasUsableCredential(credentials.mapboxSku, "YOUR_MAPBOX_SKU")) {
            url = url.replaceAll("YOUR_MAPBOX_SKU", encodeURIComponent(credentials.mapboxSku));
        } else {
            url = url
                .replace(/([?&])sku=YOUR_MAPBOX_SKU&/g, "$1")
                .replace(/[?&]sku=YOUR_MAPBOX_SKU/g, "");
        }
    }
    return url;
}

function validateSourceCredentials(sources, credentials) {
    if (sources.some((source) => String(source.url || "").includes("YOUR_TIANDITU_TOKEN")) && !hasUsableCredential(credentials.tiandituToken, "YOUR_TIANDITU_TOKEN")) {
        return "所选天地图源需要填写天地图 Token。";
    }
    if (sources.some((source) => String(source.url || "").includes("YOUR_MAPBOX_TOKEN")) && !hasUsableCredential(credentials.mapboxToken, "YOUR_MAPBOX_TOKEN")) {
        return "所选 Mapbox 源需要填写 Mapbox Token。";
    }
    return "";
}

function initAdminRegionMap() {
    if (adminRegionMap || taskMode !== "region") {
        return;
    }
    if (!window.L) {
        if (adminRegionMapInitRetries < MAP_INIT_RETRY_LIMIT) {
            adminRegionMapInitRetries += 1;
            window.setTimeout(initAdminRegionMap, 100);
        }
        return;
    }
    adminRegionMapInitRetries = 0;

    adminRegionMap = L.map("adminRegionMap", {
        zoomControl: true,
        attributionControl: false,
        minZoom: MAP_MIN_ZOOM,
        maxBounds: MAP_LATITUDE_BOUNDS,
        maxBoundsViscosity: 1,
        worldCopyJump: false
    }).setView([35.8617, 104.1954], 4);

    adminRegionBaseLayer = L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
        minZoom: MAP_MIN_ZOOM,
        maxZoom: 10
    }).addTo(adminRegionMap);
    adminRegionLayerGroup = L.layerGroup().addTo(adminRegionMap);
    adminRegionMap.on("click", handleAdminRegionMapClick);

    void renderAdminRegionMap();
}

function setAdminRegionLevel(level) {
    if (!Object.prototype.hasOwnProperty.call(ADMIN_REGION_LEVEL_CONFIG_IDS, level)) {
        return;
    }
    adminRegionLevel = resolveAdminRegionLevel(level);
    updateAdminRegionLevelTabs();
    void renderAdminRegionMap();
}

function updateAdminRegionLevelTabs() {
    document.querySelectorAll("[data-admin-region-level]").forEach((button) => {
        const level = button.dataset.adminRegionLevel;
        const active = level === adminRegionLevel;
        const disabled = !canUseAdminRegionLevel(level);
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-pressed", active ? "true" : "false");
        button.disabled = disabled;
    });
}

async function renderAdminRegionMap() {
    if (!adminRegionMap || !adminRegionLayerGroup) {
        return;
    }

    const token = ++adminRegionRenderToken;
    updateAdminRegionLevelTabs();
    updateAdminRegionMapStatus("loading");
    adminRegionLayerGroup.clearLayers();
    adminRegionSelectedLayer = null;

    const config = getAdminRegionLevelConfig(adminRegionLevel);
    if (!config) {
        updateAdminRegionMapStatus("empty");
        return;
    }

    const regions = config.options.filter((item) => item.maintained !== false && item.geojson);
    if (regions.length === 0) {
        updateAdminRegionMapStatus("empty");
        return;
    }

    const selectedID = config.selectedRegionId;
    let layerCount = 0;
    await Promise.all(regions.map(async (region) => {
        try {
            const geojson = await fetchRegionGeoJSON(region.id);
            if (token !== adminRegionRenderToken) {
                return;
            }
            const selected = region.id === selectedID;
            const layer = L.geoJSON(geojson, {
                style: () => adminRegionFeatureStyle(selected),
                onEachFeature: (_feature, featureLayer) => {
                    featureLayer.bindTooltip(region.name, {
                        direction: "center",
                        className: "admin-region-tooltip",
                        sticky: true
                    });
                    featureLayer.on("mouseover", () => {
                        if (featureLayer !== adminRegionSelectedLayer) {
                            featureLayer.setStyle(adminRegionHoverStyle());
                        }
                    });
                    featureLayer.on("mouseout", () => {
                        featureLayer.setStyle(adminRegionFeatureStyle(featureLayer === adminRegionSelectedLayer));
                    });
                }
            });
            layer.addTo(adminRegionLayerGroup);
            bindAdminRegionLayerElements(layer, region);
            layerCount += 1;
            if (selected) {
                adminRegionSelectedLayer = layer;
                layer.setStyle(adminRegionFeatureStyle(true));
            }
        } catch (error) {
            console.warn("failed to load region geojson", region.id, error);
        }
    }));

    if (token !== adminRegionRenderToken) {
        return;
    }
    if (layerCount === 0) {
        updateAdminRegionMapStatus("empty");
        return;
    }
    fitAdminRegionMapToSelection();
    updateAdminRegionMapStatus("ready");
}

async function fetchRegionGeoJSON(regionID) {
    if (adminRegionGeoJSONCache.has(regionID)) {
        return adminRegionGeoJSONCache.get(regionID);
    }
    const response = await fetchJSON(`/api/config/region-catalog/${encodeURIComponent(regionID)}/geojson`);
    if (!response.ok) {
        throw new Error(response.data.error || "failed to load region geojson");
    }
    adminRegionGeoJSONCache.set(regionID, response.data);
    return response.data;
}

function selectAdminRegionFromMap(region) {
    const config = levelConfigs.find((item) => item.regionLevel === region.level);
    if (!config) {
        return;
    }

    applyAdminRegionSelection(config, region.id);
}

function applyAdminRegionSelection(config, regionID) {
    if (!config || config.locked || !regionID) {
        return;
    }

    activeLevelCount = Math.max(activeLevelCount, config.id);
    clearLevelConfigsFrom(config.id + 1);
    levelConfigs.forEach((item) => {
        if (item.id <= activeLevelCount) {
            item.enabled = false;
        }
    });
    config.enabled = true;
    config.selectedRegionId = regionID;
    syncRegionConfigs();
    renderLevelConfigs();

    const nextLevel = nextAdminRegionLevel(config.regionLevel);
    if (nextLevel && hasRenderableAdminRegionOptions(nextLevel)) {
        adminRegionLevel = nextLevel;
    } else if (Object.prototype.hasOwnProperty.call(ADMIN_REGION_LEVEL_CONFIG_IDS, config.regionLevel)) {
        adminRegionLevel = config.regionLevel;
    }
    void renderAdminRegionMap();
}

function nextAdminRegionLevel(level) {
    return ADMIN_REGION_NEXT_LEVELS[level] || "";
}

function previousAdminRegionLevel(level) {
    return ADMIN_REGION_PREVIOUS_LEVELS[level] || "";
}

function handleAdminRegionMapClick(event) {
    const target = event.originalEvent && event.originalEvent.target;
    const regionElement = target && target.closest ? target.closest("[data-admin-region-id]") : null;
    const region = regionElement ? getRegionByID(regionElement.dataset.adminRegionId) : null;
    if (region) {
        selectAdminRegionFromMap(region);
        return;
    }

    handleAdminRegionMapBlankClick();
}

function handleAdminRegionMapBlankClick() {
    const currentConfig = getAdminRegionLevelConfig(adminRegionLevel);
    if (!currentConfig) {
        adminRegionLevel = "province";
        activeLevelCount = 2;
        clearLevelConfigsFrom(3);
        syncRegionConfigs();
        renderLevelConfigs();
        void renderAdminRegionMap();
        return;
    }

    clearLevelConfigsFrom(currentConfig.id);
    activeLevelCount = Math.max(2, currentConfig.id - 1);
    adminRegionLevel = previousAdminRegionLevel(adminRegionLevel) || "province";
    syncRegionConfigs();
    if (activeLevelCount <= 2) {
        levelConfigs.slice(0, 2).forEach((config) => {
            if (config.options.length > 0) {
                config.enabled = true;
            }
        });
    } else {
        const fallbackConfig = getAdminRegionLevelConfig(adminRegionLevel);
        if (fallbackConfig && fallbackConfig.id <= activeLevelCount && fallbackConfig.options.length > 0) {
            fallbackConfig.enabled = true;
        }
    }
    renderLevelConfigs();
    void renderAdminRegionMap();
}

function bindAdminRegionLayerElements(layer, region) {
    if (!layer || !region || typeof layer.eachLayer !== "function") {
        return;
    }
    layer.eachLayer((featureLayer) => {
        const element = typeof featureLayer.getElement === "function" ? featureLayer.getElement() : null;
        if (element) {
            element.dataset.adminRegionId = region.id;
        }
    });
}

function getAdminRegionLevelConfig(level) {
    const configID = ADMIN_REGION_LEVEL_CONFIG_IDS[level];
    if (!configID) {
        return null;
    }
    return findLevelConfig(configID);
}

function getAdminRegionLevelOptions(level) {
    const config = getAdminRegionLevelConfig(level);
    return config ? config.options : [];
}

function hasRenderableAdminRegionOptions(level) {
    return getAdminRegionLevelOptions(level).some((item) => item.maintained !== false && item.geojson);
}

function canUseAdminRegionLevel(level) {
    if (level === "province") {
        return true;
    }
    if (level === "city") {
        const provinceConfig = getAdminRegionLevelConfig("province");
        return Boolean(provinceConfig && provinceConfig.selectedRegionId);
    }
    if (level === "district") {
        const cityConfig = getAdminRegionLevelConfig("city");
        return Boolean(cityConfig && cityConfig.selectedRegionId);
    }
    return false;
}

function resolveAdminRegionLevel(level) {
    if (level === "district" && !canUseAdminRegionLevel("district")) {
        return canUseAdminRegionLevel("city") ? "city" : "province";
    }
    if (level === "city" && !canUseAdminRegionLevel("city")) {
        return "province";
    }
    return level;
}

function fitAdminRegionMapToSelection() {
    if (!adminRegionMap || !adminRegionLayerGroup) {
        return;
    }
    if (!adminRegionLayerGroup.getBounds) {
        return;
    }
    const bounds = adminRegionLayerGroup.getBounds();
    if (!bounds || !bounds.isValid()) {
        return;
    }
    adminRegionMap.fitBounds(bounds, { padding: [18, 18], maxZoom: adminRegionLevel === "district" ? 10 : 7 });
}

function adminRegionFeatureStyle(selected) {
    return {
        color: selected ? "#111827" : "#2563eb",
        weight: selected ? 3 : 1,
        opacity: selected ? 0.95 : 0.72,
        fillColor: selected ? "#3b82f6" : "#93c5fd",
        fillOpacity: selected ? 0.26 : 0.12
    };
}

function adminRegionHoverStyle() {
    return {
        color: "#0f172a",
        weight: 2,
        opacity: 0.9,
        fillColor: "#60a5fa",
        fillOpacity: 0.22
    };
}

function updateAdminRegionMapStatus(state) {
    const current = document.getElementById("adminRegionMapCurrent");
    const hint = document.getElementById("adminRegionMapHint");
    const config = getAdminRegionLevelConfig(adminRegionLevel);
    const selectedRegion = getAdminRegionContextRegion();
    const label = ADMIN_REGION_LEVEL_LABELS[adminRegionLevel] || "区域";

    if (current) {
        current.textContent = selectedRegion ? `当前：${selectedRegion.name}` : "当前：中国";
    }
    if (!hint) {
        return;
    }
    if (state === "loading") {
        hint.textContent = `${label}边界加载中`;
    } else if (state === "empty") {
        hint.textContent = `${label}暂无可用边界`;
    } else {
        const count = config ? config.options.filter((item) => item.maintained !== false && item.geojson).length : 0;
        hint.textContent = `${label}边界 ${count} 个`;
    }
}

function getAdminRegionContextRegion() {
    const config = getAdminRegionLevelConfig(adminRegionLevel);
    const selectedRegion = config ? getRegionByID(config.selectedRegionId) : null;
    if (selectedRegion) {
        return selectedRegion;
    }

    const parentLevel = previousAdminRegionLevel(adminRegionLevel);
    if (parentLevel) {
        const parentConfig = getAdminRegionLevelConfig(parentLevel);
        const parentRegion = parentConfig ? getRegionByID(parentConfig.selectedRegionId) : null;
        if (parentRegion) {
            return parentRegion;
        }
    }

    return getRegionByID("china") || { name: "中国" };
}

function initRangeMap() {
    if (rangeMap) {
        return;
    }
    if (!window.L) {
        if (rangeMapInitRetries < MAP_INIT_RETRY_LIMIT) {
            rangeMapInitRetries += 1;
            window.setTimeout(initRangeMap, 100);
        }
        return;
    }
    rangeMapInitRetries = 0;

    rangeMap = L.map("rangeMap", {
        zoomControl: true,
        attributionControl: false,
        minZoom: MAP_MIN_ZOOM,
        maxBounds: MAP_LATITUDE_BOUNDS,
        maxBoundsViscosity: 1,
        worldCopyJump: false
    }).setView([35.8617, 104.1954], 4);

    updateRangeBaseLayer();

    rangeMap.on("mousemove", (event) => {
        document.getElementById("rangeHoverCoord").textContent = formatLngLat(event.latlng);
    });

    rangeMap.on("click", (event) => {
        document.getElementById("rangeClickCoord").textContent = formatLngLat(event.latlng);
        handleRangeMapClick(event.latlng);
    });
}

function updateRangeBaseLayer() {
    if (!rangeMap || !window.L) {
        return;
    }
    const source = document.getElementById("rangePreviewSource")?.value || "osm";
    const credentials = readCredentialRequest(new FormData(document.getElementById("taskForm")));
    const hasToken = hasUsableCredential(credentials.tiandituToken, "YOUR_TIANDITU_TOKEN");
    const hint = document.getElementById("rangePreviewHint");

    let url = "https://tile.openstreetmap.org/{z}/{x}/{y}.png";
    let maxZoom = 19;
    let hintText = "OSM 预览底图";
    if (source === "tdt-img" && hasToken) {
        url = `https://t0.tianditu.gov.cn/DataServer?T=img_w&x={x}&y={y}&l={z}&tk=${encodeURIComponent(credentials.tiandituToken)}`;
        maxZoom = 18;
        hintText = "天地图影像预览";
    } else if (source === "tdt-vec" && hasToken) {
        url = `https://t0.tianditu.gov.cn/DataServer?T=vec_w&x={x}&y={y}&l={z}&tk=${encodeURIComponent(credentials.tiandituToken)}`;
        maxZoom = 18;
        hintText = "天地图矢量预览";
    } else if (source.startsWith("tdt-")) {
        hintText = "输入天地图 Token 后显示天地图预览";
    }

    if (rangeBaseLayer) {
        rangeMap.removeLayer(rangeBaseLayer);
    }
    rangeBaseLayer = L.tileLayer(url, {
        minZoom: MAP_MIN_ZOOM,
        maxZoom
    }).addTo(rangeMap);
    if (hint) {
        hint.textContent = hintText;
    }
}

function handleRangeMapClick(latlng) {
    document.getElementById("rangeClickCoord").textContent = formatLngLat(latlng);
    if (rangeDrawMode === "polygon") {
        handleRangePolygonClick(latlng);
        return;
    }

    if (rangeClickPoints.length >= 2) {
        rangeClickPoints = [];
    }
    rangeClickPoints.push(latlng);

    if (rangeClickPoints.length === 1) {
        setRangeFieldsFromPoints(latlng, latlng);
        updateRangeOverlay();
        updateRangeEstimate();
        return;
    }

    setRangeFieldsFromPoints(rangeClickPoints[0], rangeClickPoints[1]);
    updateRangeOverlay(true);
    updateRangeEstimate();
}

function handleRangePolygonClick(latlng) {
    rangeClickPoints.push(latlng);
    setRangeFieldsFromPolygon(rangeClickPoints);
    updateRangeOverlay();
    updateRangeEstimate();
    updateRangeDrawControls();
}

function setRangeFieldsFromPoints(first, second) {
    const form = document.getElementById("taskForm");
    form.elements.rangeMinLon.value = Math.min(first.lng, second.lng).toFixed(6);
    form.elements.rangeMaxLon.value = Math.max(first.lng, second.lng).toFixed(6);
    form.elements.rangeMinLat.value = Math.min(first.lat, second.lat).toFixed(6);
    form.elements.rangeMaxLat.value = Math.max(first.lat, second.lat).toFixed(6);
}

function setRangeFieldsFromPolygon(points) {
    if (!Array.isArray(points) || points.length === 0) {
        return;
    }
    const first = points[0];
    const bounds = points.reduce((acc, point) => ({
        minLon: Math.min(acc.minLon, point.lng),
        minLat: Math.min(acc.minLat, point.lat),
        maxLon: Math.max(acc.maxLon, point.lng),
        maxLat: Math.max(acc.maxLat, point.lat)
    }), {
        minLon: first.lng,
        minLat: first.lat,
        maxLon: first.lng,
        maxLat: first.lat
    });
    const form = document.getElementById("taskForm");
    form.elements.rangeMinLon.value = bounds.minLon.toFixed(6);
    form.elements.rangeMaxLon.value = bounds.maxLon.toFixed(6);
    form.elements.rangeMinLat.value = bounds.minLat.toFixed(6);
    form.elements.rangeMaxLat.value = bounds.maxLat.toFixed(6);
}

function formatLngLat(latlng) {
    if (!latlng || !Number.isFinite(latlng.lng) || !Number.isFinite(latlng.lat)) {
        return "-";
    }
    return `${latlng.lng.toFixed(6)}, ${latlng.lat.toFixed(6)}`;
}

function setRangeDrawMode(mode) {
    const nextMode = mode === "polygon" ? "polygon" : "rectangle";
    if (rangeDrawMode === nextMode) {
        return;
    }
    rangeDrawMode = nextMode;
    rangeClickPoints = [];
    clearRangeOverlays();
    updateRangeDrawControls();
    updateRangeEstimate();
    updateRangeOverlay();
}

function setRangeMapExpanded(expanded) {
    rangeMapExpanded = Boolean(expanded);
    const card = document.getElementById("rangeMapCard");
    if (rangeMapExpanded && card.parentElement !== document.body) {
        rangeMapOriginalParent = card.parentElement;
        rangeMapOriginalNextSibling = card.nextSibling;
        document.body.appendChild(card);
    } else if (!rangeMapExpanded && rangeMapOriginalParent && card.parentElement === document.body) {
        rangeMapOriginalParent.insertBefore(card, rangeMapOriginalNextSibling);
    }
    card.classList.toggle("is-expanded", rangeMapExpanded);
    document.body.classList.toggle("range-map-expanded", rangeMapExpanded);
    document.getElementById("expandRangeMapBtn").classList.toggle("is-hidden", rangeMapExpanded);
    document.getElementById("finishRangeMapBtn").classList.toggle("is-hidden", !rangeMapExpanded);

    window.setTimeout(() => {
        if (rangeMap) {
            rangeMap.invalidateSize();
            fitRangeMapToFields();
        }
    }, 80);
}

function updateRangeDrawControls() {
    document.querySelectorAll("[data-range-draw-mode]").forEach((button) => {
        const active = button.dataset.rangeDrawMode === rangeDrawMode;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-pressed", active ? "true" : "false");
    });

    const undoButton = document.getElementById("undoRangePointBtn");
    if (undoButton) {
        undoButton.disabled = rangeClickPoints.length === 0;
    }

    const hint = document.getElementById("rangeDrawHint");
    if (!hint) {
        return;
    }
    if (rangeDrawMode === "polygon") {
        if (rangeClickPoints.length === 0) {
            hint.textContent = "多边形：在地图上逐点点击，至少 3 个点";
        } else if (rangeClickPoints.length < 3) {
            hint.textContent = `多边形：已选 ${rangeClickPoints.length} 个点，还需至少 ${3 - rangeClickPoints.length} 个点`;
        } else {
            hint.textContent = `多边形：已选 ${rangeClickPoints.length} 个点，可撤销、清除或直接创建任务`;
        }
        return;
    }
    if (rangeClickPoints.length === 0) {
        hint.textContent = "矩形：点击第一个角点";
    } else if (rangeClickPoints.length === 1) {
        hint.textContent = "矩形：点击对角点生成范围";
    } else {
        hint.textContent = "矩形：已生成范围，可重新点击开始新范围";
    }
}

function undoRangePoint() {
    if (rangeClickPoints.length === 0) {
        return;
    }
    rangeClickPoints.pop();
    if (rangeClickPoints.length === 0) {
        clearRangeOverlays();
        updateRangeEstimate();
        updateRangeDrawControls();
        return;
    }
    if (rangeDrawMode === "polygon") {
        setRangeFieldsFromPolygon(rangeClickPoints);
    } else if (rangeClickPoints.length === 1) {
        setRangeFieldsFromPoints(rangeClickPoints[0], rangeClickPoints[0]);
    }
    updateRangeOverlay();
    updateRangeEstimate();
    updateRangeDrawControls();
}

function clearRangeSelection() {
    rangeClickPoints = [];
    clearRangeOverlays();
    document.getElementById("rangeClickCoord").textContent = "-";
    updateRangeEstimate();
    updateRangeDrawControls();
}

function clearRangeOverlays() {
    if (rangeRectangle && rangeMap) {
        rangeMap.removeLayer(rangeRectangle);
        rangeRectangle = null;
    }
    if (rangePolyline && rangeMap) {
        rangeMap.removeLayer(rangePolyline);
        rangePolyline = null;
    }
    if (rangePolygon && rangeMap) {
        rangeMap.removeLayer(rangePolygon);
        rangePolygon = null;
    }
    rangePointMarkers.forEach((marker) => {
        if (rangeMap) {
            rangeMap.removeLayer(marker);
        }
    });
    rangePointMarkers = [];
}

function fitRangeMapToFields() {
    if (!rangeMap) {
        return;
    }
    const bounds = readRangeBoundsFromFields();
    if (!bounds) {
        return;
    }
    updateRangeOverlay(true);
}

function updateRangeOverlay(fit = false) {
    if (!rangeMap) {
        return;
    }
    if (rangeDrawMode === "polygon") {
        updateRangePolygonOverlay(fit);
        return;
    }
    updateRangeRectangleOverlay(fit);
}

function updateRangeRectangleOverlay(fit = false) {
    if (!rangeMap) {
        return;
    }
    if (rangePolyline || rangePolygon || rangePointMarkers.length > 0) {
        clearRangeOverlays();
    }
    const bounds = readRangeBoundsFromFields();
    if (!bounds) {
        return;
    }

    if (!rangeRectangle) {
        rangeRectangle = L.rectangle(bounds, {
            color: "#2563eb",
            weight: 2,
            dashArray: "6 6",
            fillColor: "#2563eb",
            fillOpacity: 0.14
        }).addTo(rangeMap);
    } else {
        rangeRectangle.setBounds(bounds);
    }

    if (fit) {
        rangeMap.fitBounds(bounds.pad(0.3), { animate: false, maxZoom: 16 });
    }
    updateRangeDrawControls();
}

function updateRangePolygonOverlay(fit = false) {
    if (!rangeMap) {
        return;
    }
    if (rangeRectangle) {
        rangeMap.removeLayer(rangeRectangle);
        rangeRectangle = null;
    }
    if (rangePolyline) {
        rangeMap.removeLayer(rangePolyline);
        rangePolyline = null;
    }
    if (rangePolygon) {
        rangeMap.removeLayer(rangePolygon);
        rangePolygon = null;
    }
    rangePointMarkers.forEach((marker) => rangeMap.removeLayer(marker));
    rangePointMarkers = [];

    if (rangeClickPoints.length === 0) {
        updateRangeDrawControls();
        return;
    }

    rangePointMarkers = rangeClickPoints.map((point) => L.circleMarker(point, {
        radius: 5,
        color: "#1d4ed8",
        weight: 2,
        fillColor: "#ffffff",
        fillOpacity: 1
    }).addTo(rangeMap));

    if (rangeClickPoints.length >= 2) {
        rangePolyline = L.polyline(rangeClickPoints, {
            color: "#2563eb",
            weight: 2,
            dashArray: "5 5"
        }).addTo(rangeMap);
    }

    if (rangeClickPoints.length >= 3) {
        rangePolygon = L.polygon(rangeClickPoints, {
            color: "#2563eb",
            weight: 2,
            fillColor: "#2563eb",
            fillOpacity: 0.16
        }).addTo(rangeMap);
    }

    if (fit) {
        const bounds = L.latLngBounds(rangeClickPoints);
        if (bounds.isValid()) {
            rangeMap.fitBounds(bounds.pad(0.3), { animate: false, maxZoom: 16 });
        }
    }
    updateRangeDrawControls();
}

function readRangeBoundsFromFields() {
    const request = readRangeRequest(new FormData(document.getElementById("taskForm")));
    if (!isValidBBox(request.bbox, false)) {
        return null;
    }
    return L.latLngBounds(
        L.latLng(request.bbox.minLat, request.bbox.minLon),
        L.latLng(request.bbox.maxLat, request.bbox.maxLon)
    );
}

async function login(event) {
    event.preventDefault();
    hideMessage("loginError");

    const formData = new FormData(event.target);
    const response = await fetchJSON("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({
            username: formData.get("username"),
            password: formData.get("password")
        })
    });

    if (!response.ok) {
        showMessage("loginError", response.data.error || "登录失败");
        return;
    }

    event.target.reset();
    await bootstrap();
}

async function logout() {
    await fetchJSON("/api/auth/logout", { method: "POST", allowUnauthorized: true });
    showLogin();
}

async function loadTilemaps() {
    const response = await fetchJSON("/api/config/tilemaps");
    if (!response.ok) {
        tilemaps = FALLBACK_TILEMAPS.slice();
    } else {
        tilemaps = Array.isArray(response.data) && response.data.length > 0
            ? response.data
            : FALLBACK_TILEMAPS.slice();
    }
    const groups = getProviderGroups();
    if (!selectedProvider && groups.length > 0) {
        selectedProvider = groups[0].id;
    }
    ensureProviderSelection();
    expandedProviders.add(selectedProvider);
    renderTilemapSelector();
}

async function loadRegionCatalog() {
    const response = await fetchJSON("/api/config/region-catalog");
    if (!response.ok) {
        return;
    }

    regionCatalog = response.data.available || [];
    missingRegionCatalog = response.data.missing || [];
}

function initDefaultLevelConfigs() {
    activeLevelCount = 2;
    levelConfigs = REGION_LEVEL_RULES.map((rule) => ({
        id: rule.id,
        label: rule.label,
        regionLevel: rule.regionLevel,
        parentConfigId: rule.parentConfigId || null,
        minZoom: rule.minZoom,
        maxZoom: rule.maxZoom,
        enabled: rule.id <= 2,
        fixedRegionId: rule.fixedRegionId || "",
        selectedRegionId: "",
        geojson: "",
        options: [],
        locked: Boolean(rule.fixedRegionId)
    }));

    syncRegionConfigs();
}

function addLevelConfig() {
    if (activeLevelCount >= REGION_LEVEL_RULES.length) {
        return;
    }
    activeLevelCount += 1;
    const config = levelConfigs[activeLevelCount - 1];
    if (config && config.options.length > 0) {
        config.enabled = true;
    }
    syncRegionConfigs();
    renderLevelConfigs();
    void renderAdminRegionMap();
}

function removeLevelConfig(id) {
    if (id <= 2 || id !== activeLevelCount) {
        return;
    }
    clearLevelConfigsFrom(id);
    activeLevelCount -= 1;
    adminRegionLevel = resolveAdminRegionLevel(previousAdminRegionLevel(getRegionLevelByConfigId(id)) || "province");
    syncRegionConfigs();
    renderLevelConfigs();
    void renderAdminRegionMap();
}

function clearLevelConfigsFrom(id) {
    levelConfigs.forEach((config) => {
        if (config.id >= id && !config.fixedRegionId) {
            config.enabled = false;
            config.selectedRegionId = "";
            config.geojson = "";
        }
    });
}

function getRegionLevelByConfigId(id) {
    const config = findLevelConfig(id);
    return config ? config.regionLevel : "";
}

function renderLevelConfigs() {
    const container = document.getElementById("regionList");
    container.innerHTML = "";

    levelConfigs.slice(0, activeLevelCount).forEach((config) => {
        const row = document.createElement("div");
        const toggleDisabled = config.options.length === 0;
        const selectDisabled = config.locked || config.options.length === 0;
        const canRemove = config.id > 2 && config.id === activeLevelCount;

        row.className = "region-row";
        row.innerHTML = `
            <div class="region-row__title">
                ${icon("layers")}
                <span>
                    <strong>${config.label}</strong>
                    <span>${getRegionHelperText(config)}</span>
                </span>
            </div>
            <div class="region-row__grid">
                <label class="field">
                    <span class="field-label">最小级别</span>
                    <input type="number" min="0" max="20" class="level-min" data-id="${config.id}" value="${config.minZoom}">
                </label>
                <label class="field">
                    <span class="field-label">最大级别</span>
                    <input type="number" min="0" max="20" class="level-max" data-id="${config.id}" value="${config.maxZoom}">
                </label>
                <label class="field">
                    <span class="field-label">区域</span>
                    <select class="level-region" data-id="${config.id}" ${selectDisabled ? "disabled" : ""}>
                        ${renderRegionOptions(config)}
                    </select>
                </label>
            </div>
            <div class="region-row__actions">
                <label class="switch" aria-label="包含${config.label}">
                    <input class="level-toggle" type="checkbox" data-id="${config.id}" ${config.enabled ? "checked" : ""} ${toggleDisabled ? "disabled" : ""}>
                    <span class="switch__track"></span>
                </label>
                ${canRemove ? `<button type="button" class="danger-icon-button remove-level" data-id="${config.id}" aria-label="删除区域">${icon("delete")}</button>` : `<span class="region-row__action-spacer" aria-hidden="true"></span>`}
            </div>
        `;
        container.appendChild(row);
    });

    container.querySelectorAll(".level-toggle").forEach((element) => {
        element.addEventListener("change", (event) => {
            const config = findLevelConfig(event.target.dataset.id);
            if (!config) {
                return;
            }
            config.enabled = event.target.checked;
            syncRegionConfigs();
            renderLevelConfigs();
            void renderAdminRegionMap();
        });
    });

    container.querySelectorAll(".level-min").forEach((element) => {
        element.addEventListener("input", (event) => {
            const config = findLevelConfig(event.target.dataset.id);
            if (config) {
                config.minZoom = Number.parseInt(event.target.value, 10) || 0;
            }
        });
    });

    container.querySelectorAll(".level-max").forEach((element) => {
        element.addEventListener("input", (event) => {
            const config = findLevelConfig(event.target.dataset.id);
            if (config) {
                config.maxZoom = Number.parseInt(event.target.value, 10) || 0;
            }
        });
    });

    container.querySelectorAll(".level-region").forEach((element) => {
        element.addEventListener("change", (event) => {
            const config = findLevelConfig(event.target.dataset.id);
            if (!config || config.locked) {
                return;
            }
            applyAdminRegionSelection(config, event.target.value);
        });
    });

    container.querySelectorAll(".remove-level").forEach((element) => {
        element.addEventListener("click", (event) => {
            removeLevelConfig(Number.parseInt(event.currentTarget.dataset.id, 10));
        });
    });
}

function findLevelConfig(id) {
    return levelConfigs.find((config) => config.id === Number.parseInt(id, 10));
}

function syncRegionConfigs() {
    levelConfigs.forEach((config) => {
        const isActive = config.id <= activeLevelCount;
        config.options = getRegionOptionsForConfig(config);

        if (config.fixedRegionId) {
            config.selectedRegionId = config.fixedRegionId;
        } else if (!isActive) {
            config.selectedRegionId = "";
            config.enabled = false;
        } else if (!config.options.some((item) => item.id === config.selectedRegionId)) {
            config.selectedRegionId = config.options[0] ? config.options[0].id : "";
        }

        const selectedRegion = getRegionByID(config.selectedRegionId);
        config.geojson = selectedRegion ? selectedRegion.geojson : "";

        if (config.options.length === 0) {
            config.enabled = false;
        } else if (!isActive) {
            config.enabled = false;
        }
    });
}

function getRegionOptionsForConfig(config) {
    if (config.fixedRegionId) {
        const region = getRegionByID(config.fixedRegionId);
        return region ? [region] : [];
    }

    if (!config.parentConfigId) {
        return getAllRegions().filter((item) => item.level === config.regionLevel);
    }

    const parentConfig = findLevelConfig(config.parentConfigId);
    if (!parentConfig || !parentConfig.selectedRegionId) {
        return [];
    }

    return getAllRegions().filter((item) => item.level === config.regionLevel && item.parentId === parentConfig.selectedRegionId);
}

function getRegionByID(id) {
    return getAllRegions().find((item) => item.id === id);
}

function getAllRegions() {
    return [
        ...regionCatalog.map((item) => ({ ...item, maintained: true })),
        ...missingRegionCatalog.map((item) => ({ ...item, maintained: false }))
    ];
}

function renderRegionOptions(config) {
    if (config.options.length === 0) {
        return `<option value="">暂无可选区域</option>`;
    }

    return config.options
        .map((item) => `<option value="${item.id}" ${item.id === config.selectedRegionId ? "selected" : ""}>${item.name}</option>`)
        .join("");
}

function getRegionHelperText(config) {
    switch (config.regionLevel) {
    case "world":
        return "范围：0 ~ 5 级 ｜ 区域：全球";
    case "country":
        return "范围：6 ~ 8 级 ｜ 区域：中国";
    case "province":
        return "范围：9 ~ 10 级 ｜ 区域：省级";
    case "city":
        return config.options.length > 0 ? "范围：11 ~ 12 级 ｜ 区域：地市级" : "当前上级区域暂无可选城市";
    case "district":
        return config.options.length > 0 ? "范围：13 ~ 14 级 ｜ 区域：区县级" : "当前上级城市暂无可选区县";
    default:
        return "按已维护区域选择";
    }
}

function renderTilemapSelector() {
    const container = document.getElementById("tilemapSelector");
    const groups = getProviderGroups();
    container.innerHTML = groups.map((group) => renderProviderCard(group)).join("");
}

function renderProviderCard(group) {
    const selected = selectedProvider === group.id;
    const expanded = expandedProviders.has(group.id);
    const selectedCount = group.items.filter((item) => selectedSourceIds.has(String(item.id))).length;
    return `
        <div class="source-card ${selected ? "is-active" : ""}">
            <button type="button" class="source-card__selector" data-provider-select="${group.id}">
                <span class="source-card__radio ${selected ? "is-checked" : ""}"></span>
            </button>
            <button type="button" class="source-card__main" data-provider-select="${group.id}">
                <span class="source-card__title">
                    <strong>${group.name}</strong>
                    ${icon("map")}
                </span>
                <span class="source-card__meta">${group.items.length} 个图层，${group.description}</span>
            </button>
            <span class="source-card__tools">
                <span class="source-card__count">${selected ? `已选 ${selectedCount}/${group.items.length}` : `${group.items.length} 项`}</span>
                <button type="button" class="source-card__toggle" data-provider-toggle="${group.id}" aria-label="${expanded ? "收起图层" : "展开图层"}">
                    ${icon(expanded ? "chevron-up" : "chevron-down")}
                </button>
            </span>
            ${selected && expanded ? `
                <span class="source-card__details">
                    <span class="source-card__detail-head">
                        <strong>选择要下载的图层</strong>
                        <span class="source-card__detail-actions">
                            <button type="button" data-provider-select-all="${group.id}">全选</button>
                            <button type="button" data-provider-clear="${group.id}">清空</button>
                        </span>
                    </span>
                    <span class="source-card__option-list">
                        ${group.items.map((tilemap) => renderTilemapOption(tilemap)).join("")}
                    </span>
                </span>
            ` : ""}
        </div>
    `;
}

function renderTilemapOption(tilemap) {
    const checked = selectedSourceIds.has(String(tilemap.id));
    return `
        <label class="source-option" onclick="event.stopPropagation()">
            <input class="tilemap-source-option" type="checkbox" value="${tilemap.id}" ${checked ? "checked" : ""}>
            <span class="source-option__check" aria-hidden="true"></span>
            <span class="source-option__body">
                <span class="source-option__title">
                    <span class="source-option__name">
                        ${icon(childTaskIcon({ sourceName: tilemap.name }), "source-option__icon")}
                        <strong>${tilemap.name}</strong>
                    </span>
                    <span>${String(getTaskTilemapFormat(tilemap)).toUpperCase()}</span>
                </span>
                <span class="source-option__meta">
                    <span>${tilemap.min_zoom}-${tilemap.max_zoom}级</span>
                    <span>${formatTilemapHost(tilemap.url)}</span>
                    <span>${tilemap.schema.toUpperCase()}</span>
                </span>
            </span>
        </label>
    `;
}

function getProviderGroups() {
    const groups = new Map();

    tilemaps.forEach((tilemap) => {
        const provider = getTilemapGroupName(tilemap);
        if (!groups.has(provider)) {
            groups.set(provider, []);
        }
        groups.get(provider).push(tilemap);
    });

    return Array.from(groups.entries())
        .sort((left, right) => compareTilemapGroups(left[0], right[0]))
        .map(([name, items]) => ({
            id: name,
            name,
            items,
            description: describeProvider(name, items)
        }));
}

function describeProvider(name, items) {
    if (name === "天地图") {
        return "影像 & 动态影像地图";
    }
    if (name === "Google") {
        return "影像 & 动态影像地图";
    }
    if (name === "Mapbox") {
        return "矢量街道 & 地形瓦片";
    }
    return `包含 ${items.length} 个可用图层`;
}

function getSelectedProviderGroup() {
    return getProviderGroups().find((group) => group.id === selectedProvider) || null;
}

function getSelectedTilemaps() {
    const group = getSelectedProviderGroup();
    return group ? group.items.filter((item) => selectedSourceIds.has(String(item.id))) : [];
}

function getRequestedImageFormat() {
    return "png";
}

function getTaskTilemapFormat(tilemap) {
    const sourceFormat = String(tilemap.format || "").toLowerCase();
    if (sourceFormat === "pbf") {
        return sourceFormat;
    }
    return getRequestedImageFormat();
}

function getTilemapGroupName(tilemap) {
    const name = String(tilemap.name || "");
    const url = String(tilemap.url || "").toLowerCase();

    if (name.includes("天地图") || url.includes("tianditu.gov.cn")) {
        return "天地图";
    }
    if (name.toLowerCase().includes("google") || url.includes("google.com")) {
        return "Google";
    }
    if (name.toLowerCase().includes("mapbox") || url.includes("mapbox.com")) {
        return "Mapbox";
    }
    if (name.includes("高德") || url.includes("amap.com") || url.includes("autonavi.com")) {
        return "高德";
    }
    if (name.includes("arcgis") || url.includes("arcgis")) {
        return "ArcGIS";
    }
    if (name.includes("必应") || url.includes("bing.com") || url.includes("virtualearth")) {
        return "Bing Maps";
    }

    return "其他地图源";
}

function compareTilemapGroups(left, right) {
    const priority = ["天地图", "Google", "Mapbox", "高德", "ArcGIS", "Bing Maps", "其他地图源"];
    const leftIndex = priority.indexOf(left);
    const rightIndex = priority.indexOf(right);

    if (leftIndex !== -1 || rightIndex !== -1) {
        if (leftIndex === -1) {
            return 1;
        }
        if (rightIndex === -1) {
            return -1;
        }
        return leftIndex - rightIndex;
    }

    return left.localeCompare(right, "zh-CN");
}

async function createRangeTask(event, formData) {
    const request = readRangeRequest(formData);
    const validation = validateRangeRequest(request);
    if (validation) {
        showMessage("taskError", validation);
        return;
    }

    const workerCount = Number.parseInt(formData.get("workers"), 10) || 0;
    const savePipe = Number.parseInt(formData.get("savePipe"), 10) || 0;
    const timeDelay = Number.parseInt(formData.get("timeDelay"), 10) || 0;
    if (timeDelay < 50) {
        showMessage("taskError", "请求间隔不能小于 50ms。");
        return;
    }
    const schedule = readScheduleRequest(formData);
    if (schedule.error) {
        showMessage("taskError", schedule.error);
        return;
    }
    const output = readOutputRequest(formData);

    const tileCount = calculateRangeTileCount(request.bbox, request.zoom);
    const sources = request.layers.map((layer, index) => buildTianDiTuSource(layer, request.credentials.tiandituToken, index));
    const areaSummary = request.selectionMode === "polygon"
        ? `多边形：${request.polygon.length} 个顶点`
        : `矩形：${request.bbox.minLon},${request.bbox.minLat} - ${request.bbox.maxLon},${request.bbox.maxLat}`;
    const summary = [
        `任务名称：${request.name}`,
        `下载模式：范围框选`,
        `范围：${areaSummary}`,
        `包围盒：${request.bbox.minLon},${request.bbox.minLat} - ${request.bbox.maxLon},${request.bbox.maxLat}`,
        `层级：${request.zoom.min}-${request.zoom.max}`,
        `图层：${request.layers.join("、")}`,
        `单图层瓦片估算：${tileCount}`,
        `总瓦片估算：${tileCount * request.layers.length}`,
        `执行时间：${schedule.label}`,
        `产物格式：${output.label}`,
        `下载线程：${workerCount}`,
        `保存线程：${savePipe}`,
        `请求间隔：${timeDelay}ms`
    ].join("\n");

    if (!window.confirm(`请确认本次任务信息：\n\n${summary}`)) {
        return;
    }

    const response = await fetchJSON("/api/tasks", {
        method: "POST",
        body: JSON.stringify({
            name: request.name,
            mode: "bbox",
            workers: workerCount,
            savePipe,
            timeDelay,
            scheduleMode: schedule.scheduleMode,
            runAt: schedule.runAt,
            area: request.selectionMode === "polygon"
                ? { polygon: request.polygon }
                : { bbox: request.bbox },
            zoom: request.zoom,
            sources,
            output: { format: output.format }
        })
    });

    if (!response.ok) {
        showMessage("taskError", response.data.error || "范围任务创建失败，请稍后重试。");
        return;
    }

    event.target.reset();
    rangeClickPoints = [];
    syncScheduleControls();
    updateRangeEstimate();
    updateRangeOverlay(true);
    await loadTasks();
    setWorkspaceTab("tasks");
}

function readRangeRequest(formData) {
    const bbox = {
        minLon: Number(formData.get("rangeMinLon")),
        minLat: Number(formData.get("rangeMinLat")),
        maxLon: Number(formData.get("rangeMaxLon")),
        maxLat: Number(formData.get("rangeMaxLat"))
    };
    return {
        name: String(formData.get("name") || "").trim() || "天地图范围下载任务",
        credentials: readCredentialRequest(formData),
        layers: getSelectedRangeLayers(),
        selectionMode: rangeDrawMode,
        bbox,
        polygon: getRangePolygonRequest(),
        zoom: {
            min: Number.parseInt(formData.get("rangeMinZoom"), 10),
            max: Number.parseInt(formData.get("rangeMaxZoom"), 10)
        }
    };
}

function getSelectedRangeLayers() {
    return Array.from(document.querySelectorAll(".range-layer-option:checked")).map((element) => element.value);
}

function getRangePolygonRequest() {
    return rangeClickPoints.map((point) => ({
        lon: Number(point.lng.toFixed(6)),
        lat: Number(point.lat.toFixed(6))
    }));
}

function validateRangeRequest(request) {
    if (!hasUsableCredential(request.credentials.tiandituToken, "YOUR_TIANDITU_TOKEN")) {
        return "请输入真实的天地图 Token。";
    }
    if (request.layers.length === 0) {
        return "请至少选择一个天地图图层。";
    }
    if (request.selectionMode === "polygon" && !isValidPolygon(request.polygon)) {
        return "请在地图上点击至少 3 个有效点形成多边形范围。";
    }
    if (!isValidBBox(request.bbox, true)) {
        return "范围坐标无效，请确认最小经度、最小纬度、最大经度、最大纬度。";
    }
    if (!Number.isInteger(request.zoom.min) || !Number.isInteger(request.zoom.max)) {
        return "层级必须是整数。";
    }
    if (request.zoom.min < 0 || request.zoom.max > 18 || request.zoom.min > request.zoom.max) {
        return "天地图范围下载层级必须在 0 到 18 之间，且最小级别不能大于最大级别。";
    }
    return "";
}

function isValidRangeSelection(request) {
    if (request.selectionMode === "polygon") {
        return isValidPolygon(request.polygon) && isValidBBox(request.bbox, true);
    }
    return isValidBBox(request.bbox, true);
}

function isValidPolygon(points) {
    if (!Array.isArray(points) || points.length < 3) {
        return false;
    }
    const unique = new Set();
    for (const point of points) {
        if (!point || !Number.isFinite(point.lon) || !Number.isFinite(point.lat)) {
            return false;
        }
        if (point.lon < -180 || point.lon > 180 || point.lat < -85.05112878 || point.lat > 85.05112878) {
            return false;
        }
        unique.add(`${point.lon.toFixed(6)},${point.lat.toFixed(6)}`);
    }
    return unique.size >= 3;
}

function isValidBBox(bbox, requireArea) {
    if (!bbox) {
        return false;
    }
    const values = [bbox.minLon, bbox.minLat, bbox.maxLon, bbox.maxLat];
    if (!values.every(Number.isFinite)) {
        return false;
    }
    if (bbox.minLon < -180 || bbox.maxLon > 180) {
        return false;
    }
    if (bbox.minLat < -85.05112878 || bbox.maxLat > 85.05112878) {
        return false;
    }
    if (requireArea) {
        return bbox.minLon < bbox.maxLon && bbox.minLat < bbox.maxLat;
    }
    return bbox.minLon <= bbox.maxLon && bbox.minLat <= bbox.maxLat;
}

function updateRangeEstimate() {
    const request = readRangeRequest(new FormData(document.getElementById("taskForm")));
    const tileElement = document.getElementById("rangeTileCount");
    const totalElement = document.getElementById("rangeTotalTileCount");
    const sizeElement = document.getElementById("rangeSizeEstimate");

    if (!isValidRangeSelection(request) || !Number.isInteger(request.zoom.min) || !Number.isInteger(request.zoom.max) || request.zoom.min < 0 || request.zoom.max > 18 || request.zoom.min > request.zoom.max || request.layers.length === 0) {
        tileElement.textContent = "-";
        totalElement.textContent = "-";
        sizeElement.textContent = "-";
        return;
    }

    const tileCount = calculateRangeTileCount(request.bbox, request.zoom);
    const total = tileCount * request.layers.length;
    tileElement.textContent = String(tileCount);
    totalElement.textContent = String(total);
    sizeElement.textContent = `${((total * 8) / 1024).toFixed(2)} MB`;
}

function calculateRangeTileCount(bbox, zoom) {
    let count = 0;
    for (let z = zoom.min; z <= zoom.max; z += 1) {
        const leftTop = rangeTileForLonLat(bbox.minLon, bbox.maxLat, z);
        const rightBottom = rangeTileForLonLat(bbox.maxLon, bbox.minLat, z);
        count += (rightBottom.x - leftTop.x + 1) * (rightBottom.y - leftTop.y + 1);
    }
    return count;
}

function rangeTileForLonLat(lon, lat, z) {
    const n = 2 ** z;
    const max = n - 1;
    const x = Math.floor(((lon + 180) / 360) * n);
    const rad = (lat * Math.PI) / 180;
    const y = Math.floor(((1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI) / 2) * n);
    return {
        x: clampInt(x, 0, max),
        y: clampInt(y, 0, max)
    };
}

function clampInt(value, min, max) {
    return Math.min(max, Math.max(min, value));
}

function buildTianDiTuSource(layer, token, index) {
    return {
        id: index + 1,
        name: RANGE_LAYER_NAMES[layer] || `天地图 ${layer}`,
        url: `https://t0.tianditu.gov.cn/DataServer?T=${layer}_w&x={x}&y={y}&l={z}&tk=${encodeURIComponent(token)}`,
        format: "png",
        schema: "xyz"
    };
}

async function createTask(event) {
    event.preventDefault();
    hideMessage("taskError");

    const formData = new FormData(event.target);
    if (taskMode === "bbox") {
        await createRangeTask(event, formData);
        return;
    }

    const provider = getSelectedProviderGroup();
    if (!provider) {
        showMessage("taskError", "请选择地图源。");
        return;
    }

    const selectedTilemaps = getSelectedTilemaps();
    if (selectedTilemaps.length === 0) {
        showMessage("taskError", "请至少勾选一个要下载的图层。");
        return;
    }

    const levels = levelConfigs
        .slice(0, activeLevelCount)
        .filter((config) => config.enabled)
        .map((config) => ({
            minZoom: config.minZoom,
            maxZoom: config.maxZoom,
            geojson: config.geojson
        }));

    if (levels.length === 0) {
        showMessage("taskError", "请至少启用一个下载区域。");
        return;
    }

    const invalidConfig = levelConfigs
        .slice(0, activeLevelCount)
        .filter((config) => config.enabled)
        .find((config) => {
            const region = getRegionByID(config.selectedRegionId);
            return !region || !config.selectedRegionId || !region.geojson || region.maintained === false;
        });

    if (invalidConfig) {
        const region = getRegionByID(invalidConfig.selectedRegionId);
        const regionName = region ? region.name : `${invalidConfig.label}区域`;
        const expectedPath = region && region.geojson ? region.geojson : `./geojson/${regionName}.geojson`;
        showMessage("taskError", `${invalidConfig.label} 选择的 ${regionName} 还未维护 GeoJSON，请先补文件：${expectedPath}`);
        return;
    }

    const workerCount = Number.parseInt(formData.get("workers"), 10) || 0;
    const savePipe = Number.parseInt(formData.get("savePipe"), 10) || 0;
    const timeDelay = Number.parseInt(formData.get("timeDelay"), 10) || 0;

    if (timeDelay < 50) {
        showMessage("taskError", "请求间隔不能小于 50ms。");
        return;
    }
    const schedule = readScheduleRequest(formData);
    if (schedule.error) {
        showMessage("taskError", schedule.error);
        return;
    }
    const output = readOutputRequest(formData);
    const credentials = readCredentialRequest(formData);
    const credentialError = validateSourceCredentials(selectedTilemaps, credentials);
    if (credentialError) {
        showMessage("taskError", credentialError);
        return;
    }

    const sources = selectedTilemaps.map((tilemap, index) => ({
        id: toNumericSourceId(tilemap.id, index),
        name: tilemap.name,
        url: applySourceCredentials(tilemap.url, credentials),
        format: getTaskTilemapFormat(tilemap),
        schema: tilemap.schema
    }));

    const effectiveWorkerLabel = provider.id === "天地图" ? "1（天地图稳定模式）" : String(workerCount);
    const selectedFormats = Array.from(new Set(
        selectedTilemaps.map((item) => getTaskTilemapFormat(item).toUpperCase())
    )).join("、");
    const summary = [
        `任务名称：${formData.get("name")}`,
        `地图源：${provider.name}`,
        `图层数量：${selectedTilemaps.length}`,
        `图层明细：${selectedTilemaps.map((item) => item.name).join("、")}`,
        `瓦片格式：${selectedFormats}`,
        `执行时间：${schedule.label}`,
        `产物格式：${output.label}`,
        `下载线程：${effectiveWorkerLabel}`,
        `保存线程：${savePipe}`,
        `请求间隔：${timeDelay}ms`
    ].join("\n");

    if (!window.confirm(`请确认本次任务信息：\n\n${summary}`)) {
        return;
    }

    const response = await fetchJSON("/api/tasks", {
        method: "POST",
        body: JSON.stringify({
            name: formData.get("name"),
            workers: workerCount,
            savePipe,
            timeDelay,
            scheduleMode: schedule.scheduleMode,
            runAt: schedule.runAt,
            levels,
            sources,
            output: { format: output.format }
        })
    });

    if (!response.ok) {
        showMessage("taskError", response.data.error || "任务创建失败，请稍后重试。");
        return;
    }

    event.target.reset();
    syncScheduleControls();
    selectedProvider = getProviderGroups()[0] ? getProviderGroups()[0].id : "";
    ensureProviderSelection();
    initDefaultLevelConfigs();
    renderTilemapSelector();
    renderLevelConfigs();
    adminRegionLevel = "province";
    void renderAdminRegionMap();
    await loadTasks();
    setWorkspaceTab("tasks");
}

async function loadTasks() {
    const response = await fetchJSON("/api/tasks", { allowUnauthorized: true });
    if (!response.ok) {
        if (response.status === 401) {
            showLogin();
        }
        return;
    }

    cachedTasks = Array.isArray(response.data) ? response.data : [];
    cleanupSelections(cachedTasks);
    updateWorkspaceTaskCount(cachedTasks);
    renderTaskStats(cachedTasks);
    renderTaskList(cachedTasks);
    renderSelectionMeta();
}

function updateWorkspaceTaskCount(tasks) {
    const element = document.getElementById("workspaceTaskCount");
    if (!element) {
        return;
    }
    element.textContent = String(Array.isArray(tasks) ? tasks.length : 0);
}

function renderTaskStats(tasks) {
    const counts = countTasks(tasks);
    const stats = [
        { id: "all", label: "全部任务", count: counts.all },
        { id: "running", label: "运行中", count: counts.running },
        { id: "scheduled", label: "计划中", count: counts.scheduled + counts.pending },
        { id: "completed", label: "已完成", count: counts.completed },
        { id: "failed", label: "失败", count: counts.failed + counts.partial_failed },
        { id: "paused", label: "已暂停", count: counts.paused }
    ];

    document.getElementById("taskStats").innerHTML = stats.map((stat) => `
        <button type="button" class="stat-chip ${currentTaskFilter === stat.id ? "is-active" : ""}" data-task-filter="${stat.id}">
            <span>${stat.label}</span>
            <small>${stat.count}</small>
        </button>
    `).join("");
}

function renderTaskList(tasks) {
    const list = document.getElementById("taskList");
    const filtered = applyTaskFilter(tasks);

    if (filtered.length === 0) {
        list.innerHTML = `
            <div class="task-empty">
                <img src="/static/assets/images/empty-state.svg" alt="暂无任务">
                <h3>暂无任务</h3>
                <p>暂无任务，创建一个下载任务后可在这里查看进度。</p>
            </div>
        `;
        return;
    }

    list.innerHTML = filtered.map((task) => {
        if (task.kind === "group") {
            return renderGroupTask(task);
        }
        return renderStandaloneTask(task);
    }).join("");
}

function renderGroupTask(task) {
    const children = Array.isArray(task.children) ? task.children : [];
    const progress = toPercent(task.progress);
    const riskHint = summarizeTaskRisk(task, children);
    const isOpen = expandedGroupTasks.has(task.id);
    const menuId = `menu-${task.id}`;

    return `
        <details class="task-card" data-group-task-id="${task.id}" ${isOpen ? "open" : ""}>
            <summary>
                <div class="task-card__summary">
                    <label class="task-check" onclick="event.stopPropagation()">
                        <input class="task-select" type="checkbox" data-task-id="${task.id}" ${selectedTaskIds.has(task.id) ? "checked" : ""}>
                    </label>
                    <span class="task-illustration">${icon("layers")}</span>
                    <div class="task-main">
                        <div class="task-main__title">
                            <h3>${task.name}</h3>
                            ${renderStatusPill(task.status)}
                            ${riskHint ? renderWarningPill(riskHint.short) : ""}
                        </div>
                        <div class="task-metadata">
                            <span>子任务：<strong>${task.completedChildren || 0}/${task.totalChildren || children.length}</strong></span>
                            <span>开始时间：<strong>${formatTaskStart(task, children)}</strong></span>
                            <span>结束时间：<strong>${formatTaskEnd(task, children)}</strong></span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar__value" style="width:${progress}%"></div>
                        </div>
                        <div class="task-metrics">
                            <span>运行中：<strong>${task.runningChildren || 0}</strong></span>
                            <span>暂停：<strong>${task.pausedChildren || 0}</strong></span>
                            <span>失败：<strong>${task.failedChildren || 0}</strong></span>
                            <span>总进度：<strong>${progress}%</strong></span>
                        </div>
                    </div>
                    <div class="task-actions">
                        <div class="task-menu" onclick="event.stopPropagation()">
                            <button type="button" class="icon-mini-button" data-task-menu-toggle="${menuId}" aria-label="更多操作">
                                ${icon("more")}
                            </button>
                            <div id="${menuId}" class="task-menu__panel is-hidden">
                                <button type="button" data-task-action="pause" data-task-id="${task.id}" data-task-status="${task.status}">暂停全部</button>
                                <button type="button" data-task-action="resume" data-task-id="${task.id}" data-task-status="${task.status}">恢复全部</button>
                                <button type="button" data-task-action="cancel" data-task-id="${task.id}" data-task-status="${task.status}">取消全部</button>
                                ${canRetryFailures(task) ? `<button type="button" data-task-action="retryFailures" data-task-id="${task.id}" data-task-status="${task.status}">重试失败瓦片</button>` : ""}
                                <button type="button" data-task-action="delete" data-task-id="${task.id}" data-task-status="${task.status}">删除任务</button>
                            </div>
                        </div>
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="task-card__content">
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                <div class="child-task-list">
                    ${children.map((child) => renderChildTask(child)).join("")}
                </div>
            </div>
        </details>
    `;
}

function renderStandaloneTask(task) {
    const progress = toPercent(task.progress);
    const riskHint = detectTaskRisk(task);
    const menuId = `menu-${task.id}`;
    return `
        <details class="task-card" data-group-task-id="${task.id}" ${expandedGroupTasks.has(task.id) ? "open" : ""}>
            <summary>
                <div class="task-card__summary">
                    <label class="task-check" onclick="event.stopPropagation()">
                        <input class="task-select" type="checkbox" data-task-id="${task.id}" ${selectedTaskIds.has(task.id) ? "checked" : ""}>
                    </label>
                    <span class="task-illustration">${icon("task")}</span>
                    <div class="task-main">
                        <div class="task-main__title">
                            <h3>${task.name}</h3>
                            ${renderStatusPill(task.status)}
                            ${riskHint ? renderWarningPill(riskHint.short) : ""}
                        </div>
                        <div class="task-metadata">
                            <span>开始时间：<strong>${formatTaskStart(task)}</strong></span>
                            <span>结束时间：<strong>${formatTaskEnd(task)}</strong></span>
                            <span>产物状态：<strong>${translateArtifactStatus(task.artifactStatus)}</strong></span>
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar__value" style="width:${progress}%"></div>
                        </div>
                        <div class="task-metrics">
                            <span>进度：<strong>${task.current || 0}/${task.total || 0}</strong></span>
                            <span>成功：<strong>${task.successCount || 0}</strong></span>
                            <span>失败：<strong>${task.failureCount || 0}</strong></span>
                            <span>总进度：<strong>${progress}%</strong></span>
                        </div>
                    </div>
                    <div class="task-actions">
                        <div class="task-menu" onclick="event.stopPropagation()">
                            <button type="button" class="icon-mini-button" data-task-menu-toggle="${menuId}" aria-label="更多操作">
                                ${icon("more")}
                            </button>
                            <div id="${menuId}" class="task-menu__panel is-hidden">
                                <button type="button" data-task-action="pause" data-task-id="${task.id}" data-task-status="${task.status}">暂停任务</button>
                                <button type="button" data-task-action="resume" data-task-id="${task.id}" data-task-status="${task.status}">恢复任务</button>
                                <button type="button" data-task-action="cancel" data-task-id="${task.id}" data-task-status="${task.status}">取消任务</button>
                                ${canRetryFailures(task) ? `<button type="button" data-task-action="retryFailures" data-task-id="${task.id}" data-task-status="${task.status}">重试失败瓦片</button>` : ""}
                                <button type="button" data-task-action="delete" data-task-id="${task.id}" data-task-status="${task.status}">删除任务</button>
                            </div>
                        </div>
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="task-card__content">
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                ${renderStandaloneDetail(task)}
            </div>
        </details>
    `;
}

function renderStandaloneDetail(task) {
    const artifactAction = task.artifactStatus === "ready"
        ? `<a href="${task.downloadUrl}" class="artifact-link">${icon("download")}<span>下载产物</span></a>`
        : `<span class="artifact-text">产物：${translateArtifactStatus(task.artifactStatus)}</span>`;
    return `
        <div class="child-task-list">
            <div class="child-task" open>
                <div class="child-task__content">
                    <div class="child-task__footer">
                        <span class="artifact-text">${task.errorMessage || `开始：${task.startedAt ? formatDate(task.startedAt) : "-"} ｜ 完成：${task.finishedAt ? formatDate(task.finishedAt) : "-"}`}</span>
                        ${artifactAction}
                    </div>
                </div>
            </div>
        </div>
    `;
}

function renderChildTask(task) {
    const progress = toPercent(task.progress);
    const riskHint = detectTaskRisk(task);
    const isOpen = expandedChildTasks.has(task.id);
    const artifactAction = task.artifactStatus === "ready"
        ? `<a href="${task.downloadUrl}" class="artifact-link">${icon("download")}<span>下载产物</span></a>`
        : `<span class="artifact-text">产物：${translateArtifactStatus(task.artifactStatus)}</span>`;

    return `
        <details class="child-task" data-child-task-id="${task.id}" ${isOpen ? "open" : ""}>
            <summary>
                <div class="child-task__summary">
                    <div>
                        <div class="child-task__title">
                            ${icon(childTaskIcon(task), "child-task__icon")}
                            <strong>${task.sourceName || task.name}</strong>
                            ${renderStatusPill(task.status, true)}
                            ${renderArtifactPill(task.artifactStatus)}
                            ${riskHint ? renderWarningPill(riskHint.short) : ""}
                        </div>
                        <div class="progress-bar">
                            <div class="progress-bar__value" style="width:${progress}%"></div>
                        </div>
                        <div class="child-task__meta">
                            <span>进度：<strong>${task.current || 0}/${task.total || 0}</strong></span>
                            <span>成功：<strong>${task.successCount || 0}</strong></span>
                            <span>失败：<strong>${task.failureCount || 0}</strong></span>
                            <span>完成度：<strong>${progress}%</strong></span>
                        </div>
                    </div>
                    <div class="child-task__side">
                        <span>${task.finishedAt ? formatDate(task.finishedAt) : "进行中"}</span>
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="child-task__content">
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                <div class="child-task__footer">
                    <span class="artifact-text">${task.errorMessage || `开始：${task.startedAt ? formatDate(task.startedAt) : "-"} ｜ 完成：${task.finishedAt ? formatDate(task.finishedAt) : "-"}`}</span>
                    ${artifactAction}
                </div>
            </div>
        </details>
    `;
}

function childTaskIcon(task) {
    const name = String(task.sourceName || task.name || "").toLowerCase();
    if (name.includes("img") || name.includes("卫星")) {
        return "satellite";
    }
    if (name.includes("cia") || name.includes("路网")) {
        return "road";
    }
    return "layers";
}

function renderStatusPill(status, compact = false) {
    const extra = compact ? "status-pill--compact" : "";
    return `<span class="status-pill status-${status} ${extra}">${translateStatus(status)}</span>`;
}

function renderArtifactPill(status) {
    return `<span class="artifact-pill">产物：${translateArtifactStatus(status)}</span>`;
}

function renderWarningPill(text) {
    return `<span class="warning-pill">${text}</span>`;
}

function countTasks(tasks) {
    const counters = {
        all: tasks.length,
        scheduled: 0,
        pending: 0,
        running: 0,
        paused: 0,
        completed: 0,
        failed: 0,
        partial_failed: 0,
        cancelled: 0
    };

    tasks.forEach((task) => {
        counters[task.status] = (counters[task.status] || 0) + 1;
    });

    return counters;
}

function applyTaskFilter(tasks) {
    if (currentTaskFilter === "all") {
        return tasks;
    }
    if (currentTaskFilter === "scheduled") {
        return tasks.filter((task) => task.status === "scheduled" || task.status === "pending");
    }
    if (currentTaskFilter === "failed") {
        return tasks.filter((task) => task.status === "failed" || task.status === "partial_failed");
    }
    return tasks.filter((task) => task.status === currentTaskFilter);
}

function renderSelectionMeta() {
    const count = selectedTaskIds.size;
    document.getElementById("taskSelectionMeta").textContent = count > 0 ? `已选择 ${count} 个任务` : "未选择任务";
}

function cleanupSelections(tasks) {
    const taskIds = new Set(tasks.map((task) => task.id));
    Array.from(selectedTaskIds).forEach((id) => {
        if (!taskIds.has(id)) {
            selectedTaskIds.delete(id);
        }
    });
}

function toggleTaskMenu(menuId) {
    const target = document.getElementById(menuId);
    if (!target) {
        return;
    }
    const willOpen = target.classList.contains("is-hidden");
    closeAllTaskMenus();
    if (willOpen) {
        target.classList.remove("is-hidden");
    }
}

function closeAllTaskMenus() {
    document.querySelectorAll(".task-menu__panel").forEach((menu) => {
        menu.classList.add("is-hidden");
    });
}

async function handleTaskAction(action, taskId, status) {
    closeAllTaskMenus();
    switch (action) {
    case "pause":
        await pauseTask(taskId, status);
        break;
    case "resume":
        await resumeTask(taskId, status);
        break;
    case "cancel":
        await cancelTask(taskId, status);
        break;
    case "retryFailures":
        await retryFailures(taskId);
        break;
    case "delete":
        await purgeTask(taskId, status);
        break;
    default:
        break;
    }
}

async function handleBulkAction(action) {
    const filtered = applyTaskFilter(cachedTasks);

    switch (action) {
    case "pauseFiltered":
        await runBulkMutation(filtered.filter((task) => canPause(task.status)), (task) => pauseTask(task.id, task.status, true));
        break;
    case "resumeFiltered":
        await runBulkMutation(filtered.filter((task) => canResume(task.status)), (task) => resumeTask(task.id, task.status, true));
        break;
    case "cancelFiltered":
        if (!window.confirm("确定取消当前筛选范围内未完成任务吗？")) {
            return;
        }
        await runBulkMutation(filtered.filter((task) => canCancel(task.status)), (task) => cancelTask(task.id, task.status, true));
        break;
    case "deleteSelected":
        if (selectedTaskIds.size === 0) {
            alert("请先选择至少一个任务。");
            return;
        }
        if (!window.confirm("确定删除已选任务吗？删除后任务记录将不可恢复，但已下载文件不会自动删除。")) {
            return;
        }
        await runBulkMutation(
            cachedTasks.filter((task) => selectedTaskIds.has(task.id) && canDelete(task.status)),
            (task) => purgeTask(task.id, task.status, true)
        );
        selectedTaskIds.clear();
        break;
    default:
        break;
    }

    renderSelectionMeta();
}

async function runBulkMutation(tasks, fn) {
    if (tasks.length === 0) {
        return;
    }
    for (const task of tasks) {
        await fn(task);
    }
    await loadTasks();
}

function toPercent(progress) {
    if (progress && typeof progress === "object") {
        if (Number.isFinite(progress.ratio)) {
            return Math.max(0, Math.min(100, Math.round(progress.ratio * 100)));
        }
        if (Number.isFinite(progress.total) && progress.total > 0 && Number.isFinite(progress.current)) {
            return Math.max(0, Math.min(100, Math.round((progress.current / progress.total) * 100)));
        }
        return 0;
    }
    if (!Number.isFinite(progress)) {
        return 0;
    }
    return Math.max(0, Math.min(100, Math.round(progress * 100)));
}

function detectTaskRisk(task) {
    const errorMessage = String(task?.errorMessage || "");
    if (!errorMessage) {
        return null;
    }

    if (errorMessage.includes("unexpected status code: 418")) {
        return {
            code: "ip_blocked",
            short: "IP疑似被封",
            long: "地图源返回 HTTP 418。结合当前服务器现状，这通常表示出口 IP 已被上游地图服务风控或封禁，继续重试大概率仍会失败。"
        };
    }

    if (errorMessage.includes("unexpected status code: 429")) {
        return {
            code: "rate_limited",
            short: "请求过快",
            long: "地图源返回 HTTP 429，说明当前请求频率过高，建议降低并发、加大请求间隔，或者切换出口。"
        };
    }

    if (errorMessage.includes("unexpected status code: 502") || errorMessage.includes("unexpected status code: 503") || errorMessage.includes("unexpected status code: 504")) {
        return {
            code: "upstream_busy",
            short: "上游不稳定",
            long: "地图源返回 502/503/504，说明上游服务当前不稳定或网关异常。可以稍后重试，或切换出口和代理。"
        };
    }

    if (errorMessage.includes("category=proxy")) {
        return {
            code: "proxy_failed",
            short: "代理异常",
            long: "当前请求被归类为代理异常，通常是代理连接失败、隧道失败，或者代理出口已不可用。"
        };
    }

    if (errorMessage.includes("category=network")) {
        return {
            code: "network_failed",
            short: "网络异常",
            long: "当前请求被归类为网络异常，通常是连接超时、连接被重置，或者出口网络不稳定。"
        };
    }

    if (errorMessage.includes("category=blocked")) {
        return {
            code: "blocked",
            short: "访问被拦截",
            long: "地图源已明确表现出拦截迹象，继续使用当前出口或当前策略成功率会很低。"
        };
    }

    if (errorMessage.includes("category=throttle")) {
        return {
            code: "throttle",
            short: "被限流",
            long: "地图源已进入限流状态，建议降低请求速度、减少并发，或更换出口 IP。"
        };
    }

    return null;
}

function summarizeTaskRisk(task, children) {
    const childRisks = children.map(detectTaskRisk).filter(Boolean);
    if (childRisks.length > 0) {
        return childRisks.find((item) => item.code === "ip_blocked") || childRisks[0];
    }
    return detectTaskRisk(task);
}

async function pauseTask(id, status, silent = false) {
    if (!canPause(status)) {
        return;
    }
    await mutateTask(`/api/tasks/${id}/pause`, "PUT", silent);
}

async function resumeTask(id, status, silent = false) {
    if (!canResume(status)) {
        return;
    }
    await mutateTask(`/api/tasks/${id}/resume`, "PUT", silent);
}

async function cancelTask(id, status, silent = false) {
    if (!canCancel(status)) {
        return;
    }
    if (!silent && !window.confirm("确定要取消这个任务吗？")) {
        return;
    }
    await mutateTask(`/api/tasks/${id}`, "DELETE", silent);
}

async function purgeTask(id, status, silent = false) {
    if (!canDelete(status)) {
        return;
    }
    if (!silent && !window.confirm("确定删除该任务吗？删除后任务记录将不可恢复，但已下载文件不会自动删除。")) {
        return;
    }
    await mutateTask(`/api/tasks/${id}/purge`, "DELETE", silent);
}

async function retryFailures(id) {
    if (!window.confirm("确定只重试该任务记录中的可重试失败瓦片吗？")) {
        return;
    }
    await mutateTask(`/api/tasks/${id}/retry-failures`, "POST");
}

async function mutateTask(url, method = "PUT", silent = false) {
    const response = await fetchJSON(url, { method });
    if (!response.ok) {
        alert(response.data.error || "任务操作失败");
        return;
    }
    if (!silent) {
        await loadTasks();
    }
}

async function fetchJSON(url, options = {}) {
    const config = {
        method: options.method || "GET",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin"
    };

    if (options.body) {
        config.body = options.body;
    }

    const response = await fetch(url, config);
    let data = {};
    try {
        data = await response.json();
    } catch (_error) {
        data = {};
    }

    if (response.status === 401 && !options.allowUnauthorized) {
        showLogin();
    }

    return { ok: response.ok, status: response.status, data };
}

function translateStatus(status) {
    const map = {
        scheduled: "计划中",
        pending: "等待中",
        running: "运行中",
        paused: "已暂停",
        completed: "已完成",
        partial_failed: "部分失败",
        cancelled: "已取消",
        failed: "失败"
    };
    return map[status] || status;
}

function translateArtifactStatus(status) {
    const map = {
        none: "未生成",
        packing: "打包中",
        ready: "可下载",
        failed: "生成失败"
    };
    return map[status] || status;
}

function canPause(status) {
    return status === "running";
}

function canResume(status) {
    return status === "paused";
}

function canCancel(status) {
    return status === "scheduled" || status === "pending" || status === "running" || status === "paused";
}

function canDelete(status) {
    return status === "completed" || status === "failed" || status === "cancelled" || status === "partial_failed";
}

function canRetryFailures(task) {
    return Boolean(task && task.canRetryFailures && Number(task.retryableFailureCount || 0) > 0);
}

function formatDate(value) {
    if (!value) {
        return "-";
    }

    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return "-";
    }

    return date.toLocaleString("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false
    }).replace(/\//g, "/");
}

function formatTaskStart(task, children = []) {
    if (task?.startedAt) {
        return formatDate(task.startedAt);
    }
    if ((task?.status === "scheduled" || task?.status === "pending") && task?.runAt) {
        return `计划 ${formatDate(task.runAt)}`;
    }
    const startedCandidates = children
        .map((item) => item?.startedAt)
        .filter(Boolean)
        .map((value) => new Date(value).getTime())
        .filter((value) => Number.isFinite(value));
    if (startedCandidates.length === 0) {
        return "-";
    }
    return formatDate(new Date(Math.min(...startedCandidates)).toISOString());
}

function formatTaskEnd(task, children = []) {
    if (task?.finishedAt) {
        return formatDate(task.finishedAt);
    }
    const finishedCandidates = children
        .map((item) => item?.finishedAt)
        .filter(Boolean)
        .map((value) => new Date(value).getTime())
        .filter((value) => Number.isFinite(value));
    if (finishedCandidates.length === 0) {
        return "-";
    }
    return formatDate(new Date(Math.max(...finishedCandidates)).toISOString());
}

function showMessage(id, message) {
    const element = document.getElementById(id);
    element.textContent = message;
    element.classList.remove("is-hidden");
}

function hideMessage(id) {
    document.getElementById(id).classList.add("is-hidden");
}

function icon(name, className = "") {
    return `<img src="/static/assets/icons/${name}.svg" alt="" class="${className}">`;
}

function toNumericSourceId(value, index = 0) {
    const numeric = Number.parseInt(String(value), 10);
    if (Number.isFinite(numeric)) {
        return numeric;
    }
    return index + 1;
}

function ensureProviderSelection() {
    const group = getSelectedProviderGroup();
    if (!group) {
        selectedSourceIds = new Set();
        return;
    }

    const providerIds = new Set(group.items.map((item) => String(item.id)));
    const existing = Array.from(selectedSourceIds).filter((id) => providerIds.has(id));
    if (existing.length === 0) {
        selectedSourceIds = new Set(defaultSelectedSourceIds(group));
        return;
    }

    selectedSourceIds = new Set(existing);
}

function syncSelectedSourceIds() {
    selectedSourceIds = new Set(
        Array.from(document.querySelectorAll(".tilemap-source-option:checked")).map((element) => String(element.value))
    );
}

function selectAllProviderItems(providerName) {
    const provider = getProviderGroups().find((group) => group.id === providerName);
    if (!provider) {
        return;
    }
    selectedSourceIds = new Set(provider.items.map((item) => String(item.id)));
}

function clearProviderItems(providerName) {
    const provider = getProviderGroups().find((group) => group.id === providerName);
    if (!provider) {
        return;
    }
    provider.items.forEach((item) => selectedSourceIds.delete(String(item.id)));
}

function formatTilemapHost(url) {
    try {
        return new URL(url).host;
    } catch (_error) {
        return "自定义地图源";
    }
}

function defaultSelectedSourceIds(group) {
    if (group.id !== "天地图") {
        return group.items.map((item) => String(item.id));
    }

    return group.items
        .filter((item) => {
            const name = String(item.name || "").toLowerCase();
            return !name.includes("ter_w") && !name.includes("cva_w");
        })
        .map((item) => String(item.id));
}

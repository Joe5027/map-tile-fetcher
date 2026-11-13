let tilemaps = [];
let regionCatalog = [];
let missingRegionCatalog = [];
let levelConfigs = [];
let activeLevelCount = 2;
let selectedProvider = "";
let selectedSourceIds = new Set();
let taskMode = "region";
let taskCreatePending = false;
let sourceCapabilityKey = "";
let activeMapMode = "region";
let rangeMap = null;
let rangeBaseLayer = null;
let rangeLabelLayer = null;
let rangeRectangle = null;
let rangePolyline = null;
let rangePolygon = null;
let rangePointMarkers = [];
let rangeClickPoints = [];
let rangeDrawMode = "rectangle";
let rangeMapExpanded = false;
let rangeMapOriginalParent = null;
let rangeMapOriginalNextSibling = null;
let rangeBaseLayerRenderToken = 0;
let rangeTiandituPreviewTokenValue = "";
let rangeTiandituPreviewTokenId = "";
let rangeTiandituPreviewTokenPromise = null;
let previewTokenExpiresAt = 0;
let previewTokenRefreshTimer = null;
let previewTokenGeneration = 0;
let previewRecoveryUsed = false;
let previewBaseLayerRefreshTimer = null;
let adminRegionMap = null;
let adminRegionBaseLayer = null;
let adminRegionLabelLayer = null;
let adminRegionLayerGroup = null;
let adminRegionSelectedLayer = null;
let taskAreaPreviewLayer = null;
let taskPreviewBaseLayersHidden = false;
let taskPreviewSequence = 0;
let taskPreviewAbort = null;
let taskPreviewTask = null;
let adminRegionLevel = "province";
let adminRegionRenderToken = 0;
let adminRegionBaseLayerRenderToken = 0;
let adminRegionMapInitRetries = 0;
let adminRegionPendingFocusRegionId = "";
let rangeMapInitRetries = 0;
let currentTaskFilter = "all";
let cachedTasks = [];
const TASK_PAGE_LIMIT = 50;
let taskPageOffset = 0;
let taskPageTotal = 0;
let taskCounts = null;
let taskListLoaded = false;
const renderedTaskCards = new Map();
let taskPollingTimer = null;
let taskLoadPromise = null;
let taskLoadGeneration = 0;
let authenticated = false;
let allRegions = [];
let regionsByID = new Map();
let regionsByLevel = new Map();
let regionsByParentAndLevel = new Map();
const expandedProviders = new Set();
const expandedGroupTasks = new Set();
const expandedSubtaskLists = new Set();
const expandedChildTasks = new Set();
const selectedTaskIds = new Set();
const adminRegionGeoJSONCache = new Map();
const adminRegionGeoJSONRequests = new Map();
const adminRegionLayerCache = new Map();
const taskAreaPreviewCache = new Map();
const WEB_MERCATOR_MAX_LAT = 85.05112878;
const HORIZONTAL_DRAG_LIMIT_LNG = 180;
const MAP_WORLD_BOUNDS = [
    [-WEB_MERCATOR_MAX_LAT, -HORIZONTAL_DRAG_LIMIT_LNG],
    [WEB_MERCATOR_MAX_LAT, HORIZONTAL_DRAG_LIMIT_LNG]
];
const MAP_MIN_ZOOM = 2;
const WEB_MERCATOR_TILE_SIZE = 256;
const MAP_INIT_RETRY_LIMIT = 30;
const ADMIN_REGION_GEOJSON_BATCH_SIZE = 64;
const ADMIN_REGION_LAYER_CACHE_LIMIT = 6;
const TASK_AREA_PREVIEW_CACHE_LIMIT = 12;
const MAP_TILE_OPTIONS = {
    minZoom: MAP_MIN_ZOOM,
    noWrap: true,
    updateWhenIdle: true,
    updateWhenZooming: false,
    keepBuffer: 1,
    detectRetina: false
};
const TOKEN_STORAGE_KEY = "mapTileFetcher.serviceCredentials.v1";

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
    { id: 10, name: "Mapbox地形等高线（矢量，非高程DEM）", url: "https://api.mapbox.com/v4/mapbox.mapbox-terrain-v2/{z}/{x}/{y}.vector.pbf?sku=YOUR_MAPBOX_SKU&access_token=YOUR_MAPBOX_TOKEN", format: "pbf", schema: "xyz", min_zoom: 0, max_zoom: 14, metadata: { kind: "vector", minZoom: 0, maxZoom: 14 } },
    { id: 11, name: "天地图地形底图 (ter_w)", url: "https://t0.tianditu.gov.cn/DataServer?T=ter_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 14, metadata: { kind: "raster", minZoom: 0, maxZoom: 14 } },
    { id: 12, name: "天地图地形注记 (cta_w)", url: "https://t0.tianditu.gov.cn/DataServer?T=cta_w&x={x}&y={y}&l={z}&tk=YOUR_TIANDITU_TOKEN", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 14, metadata: { kind: "raster", minZoom: 0, maxZoom: 14 } },
    { id: 13, name: "公开高程 Terrarium（Cesium）", url: "https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png", format: "png", schema: "xyz", min_zoom: 0, max_zoom: 15, metadata: { kind: "elevation", encoding: "terrarium", projection: "EPSG:3857", minZoom: 0, maxZoom: 15, defaultZoom: 14, attribution: "Mapzen / Tilezen Terrain Tiles; https://github.com/tilezen/joerd/blob/master/docs/attribution.md", verticalDatum: "Source-dependent; no vertical-datum transformation. Visualization only." } }
];

const RANGE_LAYER_NAMES = {
    img: "天地图 img 卫星图",
    cia: "天地图 cia 路网",
    vec: "天地图 vec 电子图",
    ter: "天地图 ter 地形图",
    cta: "天地图 cta 地形标注"
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
    void loadBuildVersion();
    await bootstrap();
});

async function loadBuildVersion() {
    const response = await fetchJSON("/api/version", {allowUnauthorized:true});
    const display = document.getElementById("buildVersion");
    display.textContent = response.ok ? response.data.version : "unknown";
    display.title = response.ok ? `${response.data.commit} | ${response.data.builtAt}` : "unknown";
}

function bindEvents() {
    document.getElementById("openPasswordDialogBtn").addEventListener("click", () => {
        closeAccountMenu();
        document.getElementById("passwordForm").reset();
        hideMessage("passwordError");
        document.getElementById("passwordDialog").showModal();
    });
    document.getElementById("cancelPasswordBtn").addEventListener("click", () => document.getElementById("passwordDialog").close());
    document.getElementById("passwordForm").addEventListener("submit", changePassword);
    document.getElementById("taskPreviewLevel").addEventListener("change",(event)=>{
        if(taskPreviewTask) void previewTaskArea(taskPreviewTask,Number(event.target.value));
    });
    document.getElementById("loginForm").addEventListener("submit", login);
    document.getElementById("logoutBtn").addEventListener("click", logout);
    document.getElementById("accountMenuBtn").addEventListener("click", (event) => {
        event.stopPropagation();
        toggleAccountMenu();
    });
    document.getElementById("accountMenu").addEventListener("click", (event) => {
        event.stopPropagation();
    });
    document.getElementById("openTokenDialogBtn").addEventListener("click", () => {
        closeAccountMenu();
        openTokenDialog();
    });
    document.getElementById("tokenCredentialForm").addEventListener("submit", saveTokenCredentials);
    document.getElementById("clearSavedTokenBtn").addEventListener("click", clearSavedTokenCredentials);
    document.querySelectorAll("[data-close-token-dialog]").forEach((button) => {
        button.addEventListener("click", closeTokenDialog);
    });
    document.getElementById("tokenDialog").addEventListener("click", (event) => {
        if (event.target.id === "tokenDialog") {
            closeTokenDialog();
        }
    });
    document.getElementById("taskForm").addEventListener("submit", createTask);
    document.querySelectorAll('[name="outputFormat"]').forEach((input) => {
        input.addEventListener("change", () => {
            updateOutputSettings();
            updateRangeEstimate();
        });
    });
    document.getElementById("refreshBtn").addEventListener("click", loadTasks);
    document.getElementById("taskPagePrev").addEventListener("click", () => changeTaskPage(Math.max(0, taskPageOffset - TASK_PAGE_LIMIT)));
    document.getElementById("taskPageNext").addEventListener("click", () => changeTaskPage(taskPageOffset + TASK_PAGE_LIMIT));
    document.addEventListener("visibilitychange", () => {
        startTaskPolling();
        if (!document.hidden && authenticated && taskMode === "tasks") void loadTasks();
    });
    document.getElementById("addLevelBtn").addEventListener("click", addLevelConfig);
    document.getElementById("fitAdminRegionMapBtn").addEventListener("click", fitAdminRegionMapToSelection);
    document.getElementById("toggleSelectTasksBtn").addEventListener("click", toggleSelectVisibleTasks);

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
                schedulePreviewBaseLayerRefresh();
            }
        });
    });

    document.getElementById("rangePreviewSource").addEventListener("change", updateRangeBaseLayer);
    document.getElementById("adminRegionPreviewSource").addEventListener("change", updateAdminRegionBaseLayer);

    document.querySelectorAll("[data-range-draw-mode]").forEach((button) => {
        button.addEventListener("click", () => setRangeDrawMode(button.dataset.rangeDrawMode));
    });

    document.querySelectorAll(".range-layer-option").forEach((element) => {
        element.addEventListener("change", () => {
            const source = tilemaps.find((item) => tiandituLayer(item) === element.value);
            if (source) {
                if (element.checked) selectedSourceIds.add(String(source.id));
                else selectedSourceIds.delete(String(source.id));
            }
            renderTilemapSelector();
        });
    });

    document.getElementById("undoRangePointBtn").addEventListener("click", undoRangePoint);
    document.getElementById("clearRangeBtn").addEventListener("click", clearRangeSelection);
    document.getElementById("fitRangeBtn").addEventListener("click", fitRangeMapToFields);
    document.getElementById("expandRangeMapBtn").addEventListener("click", () => setRangeMapExpanded(true));
    document.getElementById("finishRangeMapBtn").addEventListener("click", () => setRangeMapExpanded(false));

    document.getElementById("tilemapSelector").addEventListener("change", (event) => {
        if (event.target.matches(".terrain-addon__input")) {
            if (event.target.checked) selectedSourceIds.add(String(event.target.value));
            else selectedSourceIds.delete(String(event.target.value));
            renderTilemapSelector();
            return;
        }
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
        changeTaskPage(0);
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

        const subtaskList = event.target.closest("details[data-subtask-list-id]");
        if (subtaskList) {
            const id = subtaskList.dataset.subtaskListId;
            if (subtaskList.open) {
                expandedSubtaskLists.add(id);
            } else {
                expandedSubtaskLists.delete(id);
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

        const taskCard = event.target.closest("details[data-group-task-id]");
        if (taskCard && !event.target.closest(".task-select, .task-subtasks, a, button")) {
            const task = cachedTasks.find((item) => item.id === taskCard.dataset.groupTaskId);
            if (task) {
                void previewTaskArea(task);
            }
        }
    });

    document.addEventListener("click", () => {
        closeAccountMenu();
        document.getElementById("taskMoreMenu").classList.add("is-hidden");
        closeAllTaskMenus();
    });

    document.addEventListener("keydown", (event) => {
        if (event.key !== "Escape") {
            return;
        }
        if (!document.getElementById("tokenDialog").classList.contains("is-hidden")) {
            closeTokenDialog();
            return;
        }
        closeAccountMenu();
        if (rangeMapExpanded) {
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
    applySavedCredentialsToTaskForm();
    await Promise.all([loadTilemaps(), loadRegionCatalog(), loadTaskLimits()]);
    initDefaultLevelConfigs();
    renderLevelConfigs();
    setTaskMode(taskMode);
    syncScheduleControls();
    updateRangeEstimate();
    await loadTasks();
    startTaskPolling();
}

async function loadTaskLimits() {
    const response = await fetchJSON("/api/config/limits");
    if (!response.ok) return;
    const input = document.querySelector('[name="workers"]');
    input.min = response.data.minWorkers;
    input.max = response.data.maxWorkers;
    input.defaultValue = response.data.defaultWorkers;
    input.value = response.data.defaultWorkers;
}

function showLogin() {
    document.getElementById("passwordDialog").close();
    document.getElementById("passwordForm").reset();
    authenticated = false;
    taskLoadGeneration++;
    resetPreviewCredentials();
    clearTaskPreview();
    stopTaskPolling();
    document.getElementById("loginView").classList.remove("is-hidden");
    document.getElementById("appView").classList.add("is-hidden");
}

function showApp(user) {
    document.getElementById("openPasswordDialogBtn").classList.toggle("is-hidden", user.authEnabled === false);
    authenticated = true;
    taskLoadGeneration++;
    taskLoadPromise = null;
    taskPageOffset = 0;
    taskCounts = null;
    taskListLoaded = false;
    selectedTaskIds.clear();
    renderedTaskCards.clear();
    document.getElementById("currentUsername").textContent = user.username;
    document.getElementById("loginView").classList.add("is-hidden");
    document.getElementById("appView").classList.remove("is-hidden");
}

function startTaskPolling() {
    stopTaskPolling();
    if (!authenticated || taskMode !== "tasks" || document.hidden) return;
    taskPollingTimer = window.setInterval(() => {
        void loadTasks();
    }, 5000);
}

function stopTaskPolling() {
    if (taskPollingTimer !== null) {
        window.clearInterval(taskPollingTimer);
        taskPollingTimer = null;
    }
}

function setWorkspaceTab(tab) {
    document.querySelectorAll("[data-workspace-tab]").forEach((button) => {
        const active = button.dataset.workspaceTab === (tab === "tasks" ? "tasks" : "create");
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-selected", active ? "true" : "false");
        button.tabIndex = active ? 0 : -1;
    });

    document.querySelectorAll("[data-workspace-view]").forEach((view) => {
        view.classList.toggle("is-hidden", view.dataset.workspaceView !== "create");
    });

    if (tab === "tasks") {
        setTaskMode("tasks");
        return;
    }

    if (taskMode === "tasks") {
        setTaskMode(activeMapMode);
        return;
    }

    if (activeMapMode === "bbox" && rangeMap) {
        window.setTimeout(() => rangeMap.invalidateSize(), 40);
    }
    if (activeMapMode === "region" && adminRegionMap) {
        window.setTimeout(() => adminRegionMap.invalidateSize(), 40);
    }
}

function setTaskMode(mode) {
    if(mode !== "tasks") clearTaskPreview();
    taskMode = mode === "tasks" ? "tasks" : mode === "bbox" ? "bbox" : "region";
    if (taskMode !== "tasks") {
        activeMapMode = taskMode;
    }

    const isTasksMode = taskMode === "tasks";
    const mapMode = activeMapMode;

    document.querySelectorAll("[data-task-mode]").forEach((button) => {
        const active = button.dataset.taskMode === taskMode;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-pressed", active ? "true" : "false");
    });

    document.querySelectorAll(".task-config-control").forEach((element) => {
        element.classList.toggle("is-hidden", isTasksMode);
    });

    document.getElementById("taskHistoryPanel").classList.toggle("is-hidden", !isTasksMode);
    document.querySelector(".task-left-column")?.classList.toggle("task-left-column--tasks", isTasksMode);
    document.getElementById("sourceSection").classList.toggle("is-hidden", isTasksMode);
    document.getElementById("regionConfigPanel").classList.toggle("is-hidden", isTasksMode || taskMode !== "region");
    document.getElementById("rangeModePanel").classList.toggle("is-hidden", isTasksMode || taskMode !== "bbox");
    document.getElementById("regionModePanel").classList.toggle("is-hidden", mapMode !== "region");
    document.getElementById("rangePreviewPanel").classList.toggle("is-hidden", mapMode !== "bbox");
    document.getElementById("addLevelBtn").classList.toggle("is-hidden", isTasksMode || taskMode !== "region");
    document.getElementById("taskForm").classList.toggle("task-form--range", mapMode === "bbox");
    document.getElementById("taskForm").classList.toggle("task-form--tasks", isTasksMode);
    if (mapMode === "bbox") {
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
                if (!isTasksMode) {
                    fitAdminRegionMapToSelection();
                }
            }
        }, 40);
    }
    startTaskPolling();
    if (isTasksMode && authenticated && !document.hidden) void loadTasks();
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

function readDownloadSettings(formData) {
    const workers = Number(formData.get("workers"));
    const savePipe = formData.get("outputFormat") === "mbtiles" ? 1 : Number(formData.get("savePipe"));
    const rawDelay = String(formData.get("timeDelay") ?? "").trim();
    const timeDelay = Number(rawDelay);
    const workerInput = document.querySelector('[name="workers"]');
    if (!Number.isInteger(workers) || workers < Number(workerInput.min) || workers > Number(workerInput.max)) {
        return { error: `下载线程须为 ${workerInput.min}～${workerInput.max} 的整数。` };
    }
    if (!Number.isInteger(savePipe) || savePipe < 1 || savePipe > 4) {
        return { error: "请选择 1～4 个保存线程。" };
    }
    if (rawDelay === "" || !Number.isSafeInteger(timeDelay) || timeDelay < 0) {
        return { error: "请求间隔须为大于或等于 0 的整数毫秒。" };
    }
    return { workers, savePipe, timeDelay };
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
    const format = String(formData.get("outputFormat") || "tar.gz").trim().toLowerCase();
    if (format === "cesium") return { format, label: "Cesium 三维地形包" };
    if (format === "mbtiles") {
        return {
            format: "mbtiles",
            label: "MBTiles"
        };
    }
    return {
        format: "tar.gz",
        label: "TAR.GZ 压缩包"
    };
}

function updateOutputSettings() {
    const form = document.getElementById("taskForm");
    const save = form.elements.savePipe;
    const singleWriter = form.elements.outputFormat.value === "mbtiles";
    if (singleWriter && !save.disabled) {
        save.dataset.previousValue = save.value;
        save.value = "1";
    } else if (!singleWriter && save.disabled) {
        save.value = save.dataset.previousValue || "1";
    }
    save.disabled = singleWriter;
    document.getElementById("savePipeHint").classList.toggle("is-hidden", !singleWriter);
}

function readCredentialRequest(formData) {
    return {
        tiandituToken: String(formData.get("tiandituToken") || "").trim(),
        mapboxToken: String(formData.get("mapboxToken") || "").trim(),
        mapboxSku: String(formData.get("mapboxSku") || "").trim()
    };
}

function getCredentialInputs() {
    const form = document.getElementById("taskForm");
    return {
        tiandituToken: form?.elements.tiandituToken || null,
        mapboxToken: form?.elements.mapboxToken || null,
        mapboxSku: form?.elements.mapboxSku || null
    };
}

function readTaskFormCredentials() {
    return readCredentialRequest(new FormData(document.getElementById("taskForm")));
}

function readSavedCredentials() {
    try {
        const raw = window.localStorage.getItem(TOKEN_STORAGE_KEY);
        if (!raw) {
            return null;
        }
        const parsed = JSON.parse(raw);
        return {
            tiandituToken: String(parsed.tiandituToken || "").trim(),
            mapboxToken: String(parsed.mapboxToken || "").trim(),
            mapboxSku: String(parsed.mapboxSku || "").trim()
        };
    } catch (_error) {
        return null;
    }
}

function writeSavedCredentials(credentials) {
    window.localStorage.setItem(TOKEN_STORAGE_KEY, JSON.stringify({
        tiandituToken: String(credentials.tiandituToken || "").trim(),
        mapboxToken: String(credentials.mapboxToken || "").trim(),
        mapboxSku: String(credentials.mapboxSku || "").trim()
    }));
}

function applyCredentialsToTaskForm(credentials) {
    resetPreviewCredentials();
    const inputs = getCredentialInputs();
    Object.entries(inputs).forEach(([key, input]) => {
        if (!input) {
            return;
        }
        input.value = String(credentials[key] || "").trim();
        input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    refreshCredentialDependentViews();
}

function applySavedCredentialsToTaskForm() {
    const credentials = readSavedCredentials();
    if (!credentials) {
        return;
    }
    applyCredentialsToTaskForm(credentials);
}

function refreshCredentialDependentViews() {
    updateRangeEstimate();
    updateRangeOverlay();
    updateRangeBaseLayer();
    updateAdminRegionBaseLayer();
}

function toggleAccountMenu(force) {
    const menu = document.getElementById("accountMenu");
    const button = document.getElementById("accountMenuBtn");
    const shouldOpen = typeof force === "boolean" ? force : menu.classList.contains("is-hidden");
    menu.classList.toggle("is-hidden", !shouldOpen);
    button.setAttribute("aria-expanded", shouldOpen ? "true" : "false");
}

function closeAccountMenu() {
    toggleAccountMenu(false);
}

function openTokenDialog() {
    const savedCredentials = readSavedCredentials() || {};
    const formCredentials = readTaskFormCredentials();
    const credentials = {
        tiandituToken: formCredentials.tiandituToken || savedCredentials.tiandituToken || "",
        mapboxToken: formCredentials.mapboxToken || savedCredentials.mapboxToken || "",
        mapboxSku: formCredentials.mapboxSku || savedCredentials.mapboxSku || ""
    };
    document.getElementById("tokenTiandituInput").value = credentials.tiandituToken || "";
    document.getElementById("tokenMapboxInput").value = credentials.mapboxToken || "";
    document.getElementById("tokenMapboxSkuInput").value = credentials.mapboxSku || "";
    hideMessage("tokenDialogMessage");
    document.getElementById("tokenDialog").classList.remove("is-hidden");
    window.setTimeout(() => document.getElementById("tokenTiandituInput").focus(), 0);
}

function closeTokenDialog() {
    document.getElementById("tokenDialog").classList.add("is-hidden");
    hideMessage("tokenDialogMessage");
}

function saveTokenCredentials(event) {
    event.preventDefault();
    const credentials = {
        tiandituToken: document.getElementById("tokenTiandituInput").value.trim(),
        mapboxToken: document.getElementById("tokenMapboxInput").value.trim(),
        mapboxSku: document.getElementById("tokenMapboxSkuInput").value.trim()
    };
    writeSavedCredentials(credentials);
    applyCredentialsToTaskForm(credentials);
    showMessage("tokenDialogMessage", "已保存并同步到服务凭证。");
    window.setTimeout(closeTokenDialog, 650);
}

function clearSavedTokenCredentials() {
    window.localStorage.removeItem(TOKEN_STORAGE_KEY);
    const emptyCredentials = { tiandituToken: "", mapboxToken: "", mapboxSku: "" };
    document.getElementById("tokenTiandituInput").value = "";
    document.getElementById("tokenMapboxInput").value = "";
    document.getElementById("tokenMapboxSkuInput").value = "";
    applyCredentialsToTaskForm(emptyCredentials);
    showMessage("tokenDialogMessage", "已清空保存的 Token。");
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

function getWorldCoveringMinZoom(mapElementID) {
    const mapElement = document.getElementById(mapElementID);
    const viewportSize = Math.max(mapElement?.clientWidth || 0, mapElement?.clientHeight || 0);
    if (viewportSize <= WEB_MERCATOR_TILE_SIZE) {
        return MAP_MIN_ZOOM;
    }
    return Math.max(MAP_MIN_ZOOM, Math.ceil(Math.log2(viewportSize / WEB_MERCATOR_TILE_SIZE)));
}

function keepMapWorldCoveringViewport(map, mapElementID) {
    const minZoom = getWorldCoveringMinZoom(mapElementID);
    if (map.getMinZoom() !== minZoom) {
        map.setMinZoom(minZoom);
    }
    if (map.getZoom() < minZoom) {
        map.setZoom(minZoom, { animate: false });
    }
}

function initAdminRegionMap() {
    if (adminRegionMap) {
        // The map may have been created before the asynchronous region catalog finished loading.
        if (taskMode !== "tasks" && hasRenderableAdminRegionOptions(adminRegionLevel)) {
            void renderAdminRegionMap();
        }
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
        preferCanvas: true,
        zoomAnimation: false,
        fadeAnimation: false,
        markerZoomAnimation: false,
        minZoom: getWorldCoveringMinZoom("adminRegionMap"),
        maxBounds: MAP_WORLD_BOUNDS,
        maxBoundsViscosity: 1,
        worldCopyJump: false
    }).setView([35.8617, 104.1954], 5);

    keepMapWorldCoveringViewport(adminRegionMap, "adminRegionMap");
    adminRegionMap.on("resize", () => keepMapWorldCoveringViewport(adminRegionMap, "adminRegionMap"));
    updateAdminRegionBaseLayer();
    adminRegionLayerGroup = L.layerGroup().addTo(adminRegionMap);
    adminRegionMap.on("click", handleAdminRegionMapClick);

    if (taskMode !== "tasks") void renderAdminRegionMap();
}

async function updateAdminRegionBaseLayer() {
    if (!adminRegionMap || !window.L) {
        return;
    }
    const renderToken = ++adminRegionBaseLayerRenderToken;
    const source = document.getElementById("adminRegionPreviewSource")?.value || "tdt-img";
    const credentials = readCredentialRequest(new FormData(document.getElementById("taskForm")));
    const hasToken = hasUsableCredential(credentials.tiandituToken, "YOUR_TIANDITU_TOKEN");
    const hint = document.getElementById("adminRegionMapHint");

    let url = "";
    let labelUrl = "";
    let maxZoom = 18;
    if ((source === "tdt-img" || source === "tdt-vec") && hasToken) {
        const previewTokenId = await ensureTiandituPreviewToken(credentials.tiandituToken);
        if (renderToken !== adminRegionBaseLayerRenderToken) {
            return;
        }
        if (previewTokenId) {
            const layer = source === "tdt-img" ? "img" : "vec";
            url = `/api/tile-preview/tianditu/${encodeURIComponent(previewTokenId)}/${layer}/{z}/{x}/{y}.png`;
            labelUrl = `/api/tile-preview/tianditu/${encodeURIComponent(previewTokenId)}/${source === "tdt-img" ? "cia" : "cva"}/{z}/{x}/{y}.png`;
            maxZoom = 18;
        } else if (hint) {
            hint.textContent = "天地图 Token 预览代理不可用";
        }
    } else if (source.startsWith("tdt-") && hint) {
        hint.textContent = "输入天地图 Token 后显示天地图预览";
    }

    if (renderToken !== adminRegionBaseLayerRenderToken) {
        return;
    }
    if (adminRegionBaseLayer) {
        adminRegionMap.removeLayer(adminRegionBaseLayer);
    }
    if (adminRegionLabelLayer) {
        adminRegionMap.removeLayer(adminRegionLabelLayer);
        adminRegionLabelLayer = null;
    }
    adminRegionBaseLayer = null;
    if (url) {
        adminRegionBaseLayer = L.tileLayer(url, {
            ...MAP_TILE_OPTIONS,
            maxZoom,
            pane: "tilePane"
        });
        adminRegionBaseLayer.on("tileerror", () => {
            if (source.startsWith("tdt-")) void recoverPreviewCredentials(credentials.tiandituToken);
            if (hint) {
                hint.textContent = "天地图预览瓦片加载失败，请检查 Token 权限";
            }
        });
        adminRegionBaseLayer.addTo(adminRegionMap);
    }
    if (labelUrl) {
        adminRegionLabelLayer = L.tileLayer(labelUrl, {
            ...MAP_TILE_OPTIONS,
            maxZoom,
            opacity: 1
        });
        adminRegionLabelLayer.on("tileerror", () => {
            void recoverPreviewCredentials(credentials.tiandituToken);
            if (hint) {
                hint.textContent = "天地图标注加载失败，请检查 Token 权限";
            }
        });
        adminRegionLabelLayer.addTo(adminRegionMap);
    }
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
    const cacheKey = getAdminRegionLayerCacheKey(regions);
    let layerEntry = getCachedAdminRegionLayers(cacheKey);
    if (!layerEntry) {
        let geoJSONByRegionID;
        try {
            geoJSONByRegionID = await fetchRegionGeoJSONs(regions);
        } catch (error) {
            if (token === adminRegionRenderToken) {
                console.warn("failed to load region geojson batch", error);
                updateAdminRegionMapStatus("empty");
            }
            return;
        }
        if (token !== adminRegionRenderToken) {
            return;
        }
        layerEntry = createAdminRegionLayerEntry(regions, geoJSONByRegionID);
        cacheAdminRegionLayers(cacheKey, layerEntry);
    }

    if (token !== adminRegionRenderToken) {
        return;
    }
    if (layerEntry.layers.length === 0) {
        updateAdminRegionMapStatus("empty");
        return;
    }
    showAdminRegionLayerEntry(layerEntry, selectedID);
    const focused = await focusAdminRegionPendingSelection(token);
    if (!focused) {
        fitAdminRegionMapToSelection();
    }
    updateAdminRegionMapStatus("ready");
}

async function fetchRegionGeoJSON(regionID) {
    const geoJSONByRegionID = await fetchRegionGeoJSONs([{ id: regionID }]);
    return geoJSONByRegionID.get(regionID);
}

async function fetchRegionGeoJSONs(regions) {
    const regionIDs = [...new Set(regions.map((region) => String(region.id || "").trim()).filter(Boolean))];
    const missingIDs = regionIDs.filter((regionID) => !adminRegionGeoJSONCache.has(regionID) && !adminRegionGeoJSONRequests.has(regionID));
    for (let index = 0; index < missingIDs.length; index += ADMIN_REGION_GEOJSON_BATCH_SIZE) {
        requestAdminRegionGeoJSONBatch(missingIDs.slice(index, index + ADMIN_REGION_GEOJSON_BATCH_SIZE));
    }

    await Promise.all(regionIDs.map((regionID) => {
        if (adminRegionGeoJSONCache.has(regionID)) {
            return adminRegionGeoJSONCache.get(regionID);
        }
        const request = adminRegionGeoJSONRequests.get(regionID);
        if (!request) {
            throw new Error(`missing region geojson request for ${regionID}`);
        }
        return request;
    }));

    return new Map(regionIDs.map((regionID) => [regionID, adminRegionGeoJSONCache.get(regionID)]));
}

function requestAdminRegionGeoJSONBatch(regionIDs) {
    if (regionIDs.length === 0) {
        return;
    }
    const query = new URLSearchParams();
    regionIDs.forEach((regionID) => query.append("id", regionID));
    const batchRequest = fetchJSON(`/api/config/region-catalog/geojson-batch?${query.toString()}`)
        .then((response) => {
            if (!response.ok) {
                throw new Error(response.data.error || "failed to load region geojson batch");
            }
            if (!response.data || typeof response.data.regions !== "object" || Array.isArray(response.data.regions)) {
                throw new Error("invalid region geojson batch response");
            }
            return response.data.regions;
        });

    regionIDs.forEach((regionID) => {
        const request = batchRequest.then((geoJSONByRegionID) => {
            const geojson = geoJSONByRegionID[regionID];
            if (!geojson) {
                throw new Error(`region geojson is missing from batch response: ${regionID}`);
            }
            adminRegionGeoJSONCache.set(regionID, geojson);
            return geojson;
        });
        adminRegionGeoJSONRequests.set(regionID, request);
        request.then(
            () => {
                if (adminRegionGeoJSONRequests.get(regionID) === request) {
                    adminRegionGeoJSONRequests.delete(regionID);
                }
            },
            () => {
                if (adminRegionGeoJSONRequests.get(regionID) === request) {
                    adminRegionGeoJSONRequests.delete(regionID);
                }
            }
        );
    });
}

function getAdminRegionLayerCacheKey(regions) {
    return regions.map((region) => region.id).join(",");
}

function getCachedAdminRegionLayers(cacheKey) {
    const entry = adminRegionLayerCache.get(cacheKey);
    if (!entry) {
        return null;
    }
    adminRegionLayerCache.delete(cacheKey);
    adminRegionLayerCache.set(cacheKey, entry);
    return entry;
}

function cacheAdminRegionLayers(cacheKey, entry) {
    adminRegionLayerCache.delete(cacheKey);
    adminRegionLayerCache.set(cacheKey, entry);
    while (adminRegionLayerCache.size > ADMIN_REGION_LAYER_CACHE_LIMIT) {
        const oldestKey = adminRegionLayerCache.keys().next().value;
        adminRegionLayerCache.delete(oldestKey);
    }
}

function createAdminRegionLayerEntry(regions, geoJSONByRegionID) {
    const layers = regions.map((region) => {
        const layer = L.geoJSON(geoJSONByRegionID.get(region.id), {
            bubblingMouseEvents: false,
            style: () => adminRegionFeatureStyle(false),
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
        return { region, layer };
    });
    return { layers };
}

function showAdminRegionLayerEntry(entry, selectedID) {
    adminRegionLayerGroup.clearLayers();
    adminRegionSelectedLayer = null;
    entry.layers.forEach(({ region, layer }) => {
        const selected = region.id === selectedID;
        layer.setStyle(adminRegionFeatureStyle(selected));
        layer.addTo(adminRegionLayerGroup);
        layer.eachLayer((featureLayer) => {
            featureLayer.off("click");
            featureLayer.on("click", () => {
                if (taskMode !== "tasks") selectAdminRegionFromMap(region);
            });
        });
        if (selected) {
            adminRegionSelectedLayer = layer;
        }
    });
}

function selectAdminRegionFromMap(region) {
    const config = levelConfigs.find((item) => item.regionLevel === region.level);
    if (!config) {
        return;
    }

    adminRegionPendingFocusRegionId = region.id;
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
    if(taskMode === "tasks") return;
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
        adminRegionPendingFocusRegionId = "china";
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
    adminRegionPendingFocusRegionId = getAdminRegionReturnFocusRegionId();
    void renderAdminRegionMap();
}

function getAdminRegionReturnFocusRegionId() {
    const currentConfig = getAdminRegionLevelConfig(adminRegionLevel);
    if (currentConfig && currentConfig.selectedRegionId) {
        return currentConfig.selectedRegionId;
    }
    if (activeLevelCount <= 2) {
        return "china";
    }
    const contextRegion = getAdminRegionContextRegion();
    return contextRegion ? contextRegion.id : "china";
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
    adminRegionMap.fitBounds(bounds, {
        animate: false,
        padding: [18, 18],
        maxZoom: adminRegionLevel === "district" ? 11 : 8
    });
}

async function focusAdminRegionPendingSelection(token) {
    if (!adminRegionMap || !adminRegionPendingFocusRegionId) {
        return false;
    }

    const regionID = adminRegionPendingFocusRegionId;
    adminRegionPendingFocusRegionId = "";

    try {
        const geojson = await fetchRegionGeoJSON(regionID);
        if (token !== adminRegionRenderToken) {
            return false;
        }
        const focusLayer = L.geoJSON(geojson);
        const bounds = focusLayer.getBounds();
        if (!bounds || !bounds.isValid()) {
            return false;
        }
        adminRegionMap.fitBounds(bounds.pad(0.12), {
            animate: false,
            padding: [26, 26],
            maxZoom: adminRegionFocusMaxZoom(getRegionByID(regionID))
        });
        return true;
    } catch (error) {
        console.warn("failed to focus admin region", regionID, error);
        return false;
    }
}

function adminRegionFocusMaxZoom(region) {
    if (!region) {
        return adminRegionLevel === "district" ? 11 : 8;
    }
    if (region.level === "province") {
        return 8;
    }
    if (region.level === "city") {
        return 11;
    }
    if (region.level === "district") {
        return 12;
    }
    return 8;
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
        preferCanvas: true,
        zoomAnimation: false,
        fadeAnimation: false,
        markerZoomAnimation: false,
        minZoom: getWorldCoveringMinZoom("rangeMap"),
        maxBounds: MAP_WORLD_BOUNDS,
        maxBoundsViscosity: 1,
        worldCopyJump: false
    }).setView([35.8617, 104.1954], 5);

    keepMapWorldCoveringViewport(rangeMap, "rangeMap");
    rangeMap.on("resize", () => keepMapWorldCoveringViewport(rangeMap, "rangeMap"));
    updateRangeBaseLayer();

    const updateHoverCoordinate = rafThrottle((latlng) => {
        document.getElementById("rangeHoverCoord").textContent = formatLngLat(latlng);
    });
    rangeMap.on("mousemove", (event) => updateHoverCoordinate(event.latlng));

    rangeMap.on("click", (event) => {
        document.getElementById("rangeClickCoord").textContent = formatLngLat(event.latlng);
        handleRangeMapClick(event.latlng);
    });
}

async function updateRangeBaseLayer() {
    if (!rangeMap || !window.L) {
        return;
    }
    const renderToken = ++rangeBaseLayerRenderToken;
    const source = document.getElementById("rangePreviewSource")?.value || "tdt-img";
    const credentials = readCredentialRequest(new FormData(document.getElementById("taskForm")));
    const hasToken = hasUsableCredential(credentials.tiandituToken, "YOUR_TIANDITU_TOKEN");
    const hint = document.getElementById("rangePreviewHint");

    let url = "";
    let labelUrl = "";
    let maxZoom = 18;
    let hintText = "输入天地图 Token 后显示天地图预览";
    if ((source === "tdt-img" || source === "tdt-vec") && hasToken) {
        const previewTokenId = await ensureTiandituPreviewToken(credentials.tiandituToken);
        if (renderToken !== rangeBaseLayerRenderToken) {
            return;
        }
        if (previewTokenId) {
            const layer = source === "tdt-img" ? "img" : "vec";
            url = `/api/tile-preview/tianditu/${encodeURIComponent(previewTokenId)}/${layer}/{z}/{x}/{y}.png`;
            if (source === "tdt-img") {
                labelUrl = `/api/tile-preview/tianditu/${encodeURIComponent(previewTokenId)}/cia/{z}/{x}/{y}.png`;
            } else if (source === "tdt-vec") {
                labelUrl = `/api/tile-preview/tianditu/${encodeURIComponent(previewTokenId)}/cva/{z}/{x}/{y}.png`;
            }
            maxZoom = 18;
            hintText = source === "tdt-img" ? "天地图影像 + 路网预览" : "天地图矢量 + 标注预览";
        } else {
            hintText = "天地图 Token 预览代理不可用";
        }
    } else if (source.startsWith("tdt-")) {
        hintText = "输入天地图 Token 后显示天地图预览";
    }

    if (renderToken !== rangeBaseLayerRenderToken) {
        return;
    }
    if (rangeBaseLayer) {
        rangeMap.removeLayer(rangeBaseLayer);
    }
    if (rangeLabelLayer) {
        rangeMap.removeLayer(rangeLabelLayer);
        rangeLabelLayer = null;
    }
    rangeBaseLayer = null;
    if (url) {
        rangeBaseLayer = L.tileLayer(url, {
            ...MAP_TILE_OPTIONS,
            maxZoom,
            pane: "tilePane"
        });
        rangeBaseLayer.on("tileerror", () => {
            if (source.startsWith("tdt-")) void recoverPreviewCredentials(credentials.tiandituToken);
            if (hint) {
                hint.textContent = "天地图预览瓦片加载失败，请检查 Token 权限";
            }
        });
        rangeBaseLayer.addTo(rangeMap);
    }
    if (labelUrl) {
        rangeLabelLayer = L.tileLayer(labelUrl, {
            ...MAP_TILE_OPTIONS,
            maxZoom,
            pane: "tilePane",
            opacity: 1
        });
        rangeLabelLayer.on("tileerror", () => {
            void recoverPreviewCredentials(credentials.tiandituToken);
            if (hint) {
                hint.textContent = "天地图标注加载失败，请检查 Token 权限";
            }
        });
        rangeLabelLayer.addTo(rangeMap);
    }
    if (hint) {
        hint.textContent = hintText;
    }
}

async function ensureTiandituPreviewToken(token) {
    const normalized = String(token || "").trim();
    if (!hasUsableCredential(normalized, "YOUR_TIANDITU_TOKEN")) {
        return "";
    }
    if(rangeTiandituPreviewTokenValue && rangeTiandituPreviewTokenValue !== normalized) {
        previewTokenGeneration++;
        window.clearTimeout(previewTokenRefreshTimer);
        rangeTiandituPreviewTokenPromise=null;
        previewRecoveryUsed=false;
    }
    if (rangeTiandituPreviewTokenValue === normalized && rangeTiandituPreviewTokenId && Date.now() < previewTokenExpiresAt - 60000) {
        return rangeTiandituPreviewTokenId;
    }
    if (rangeTiandituPreviewTokenPromise && rangeTiandituPreviewTokenValue === normalized) {
        return rangeTiandituPreviewTokenPromise;
    }

    rangeTiandituPreviewTokenValue = normalized;
    rangeTiandituPreviewTokenId = "";
    const generation = previewTokenGeneration;
    rangeTiandituPreviewTokenPromise = fetchJSON("/api/tile-preview/tianditu-token", {
        method: "POST",
        body: JSON.stringify({ token: normalized })
    }).then((response) => {
        if (!response.ok || !response.data.id || generation !== previewTokenGeneration || normalized !== rangeTiandituPreviewTokenValue) {
            return "";
        }
        rangeTiandituPreviewTokenId = String(response.data.id);
        previewTokenExpiresAt = Date.parse(response.data.expiresAt) || Date.now();
        window.clearTimeout(previewTokenRefreshTimer);
        previewTokenRefreshTimer = window.setTimeout(async () => {
            if(generation !== previewTokenGeneration || !authenticated) return;
            previewRecoveryUsed = false;
            await ensureTiandituPreviewToken(normalized);
            await Promise.all([updateRangeBaseLayer(),updateAdminRegionBaseLayer()]);
        },Math.max(1000,previewTokenExpiresAt - Date.now() - 60000));
        return rangeTiandituPreviewTokenId;
    }).finally(() => {
        if(generation === previewTokenGeneration) rangeTiandituPreviewTokenPromise = null;
    });

    return rangeTiandituPreviewTokenPromise;
}

function resetPreviewCredentials() {
    previewTokenGeneration++;
    rangeBaseLayerRenderToken++;
    adminRegionBaseLayerRenderToken++;
    window.clearTimeout(previewTokenRefreshTimer);
    previewTokenRefreshTimer = null;
    previewTokenExpiresAt = 0;
    rangeTiandituPreviewTokenId = "";
    rangeTiandituPreviewTokenValue = "";
    rangeTiandituPreviewTokenPromise = null;
    previewRecoveryUsed = false;
    window.clearTimeout(previewBaseLayerRefreshTimer);
    previewBaseLayerRefreshTimer = null;
    for(const layer of [rangeBaseLayer,rangeLabelLayer,adminRegionBaseLayer,adminRegionLabelLayer]) layer?.remove();
    rangeBaseLayer = rangeLabelLayer = adminRegionBaseLayer = adminRegionLabelLayer = null;
}

function schedulePreviewBaseLayerRefresh() {
    window.clearTimeout(previewBaseLayerRefreshTimer);
    previewBaseLayerRefreshTimer = window.setTimeout(() => {
        previewBaseLayerRefreshTimer = null;
        void Promise.all([updateRangeBaseLayer(), updateAdminRegionBaseLayer()]);
    }, 350);
}

async function recoverPreviewCredentials(token) {
    if(previewRecoveryUsed || !authenticated) return;
    previewRecoveryUsed = true;
    rangeTiandituPreviewTokenId = "";
    previewTokenExpiresAt = 0;
    if(await ensureTiandituPreviewToken(token)) {
        await Promise.all([updateRangeBaseLayer(),updateAdminRegionBaseLayer()]);
    }
}

function handleRangeMapClick(latlng) {
    if(taskMode === "tasks") return;
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

function rafThrottle(callback) {
    let frame = 0;
    let latestValue;
    return (value) => {
        latestValue = value;
        if (frame) return;
        frame = window.requestAnimationFrame(() => {
            frame = 0;
            callback(latestValue);
        });
    };
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
        rangeMap.fitBounds(bounds.pad(0.3), { animate: false, maxZoom: 17 });
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
            rangeMap.fitBounds(bounds.pad(0.3), { animate: false, maxZoom: 17 });
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
    stopTaskPolling();
    showLogin();
}

let passwordBusy = false;
async function changePassword(event) {
    event.preventDefault();
    if (passwordBusy) return;
    hideMessage("passwordError");
    const fields = new FormData(event.target);
    const currentPassword = fields.get("currentPassword");
    const newPassword = fields.get("newPassword");
    if (newPassword !== fields.get("confirmPassword")) {
        showMessage("passwordError", "两次输入的新密码不一致"); return;
    }
    if ([...newPassword].length < 12 || new TextEncoder().encode(newPassword).length > 72 || newPassword === currentPassword) {
        showMessage("passwordError", "新密码至少 12 个字符、最多 72 字节，且不能与原密码相同"); return;
    }
    passwordBusy = true;
    document.getElementById("savePasswordBtn").disabled = true;
    const generation = taskLoadGeneration;
    try {
        const response = await fetchJSON("/api/auth/password", {method:"POST", body:JSON.stringify({currentPassword, newPassword})});
        if (generation !== taskLoadGeneration) return;
        if (!response.ok) { showMessage("passwordError", response.data.error || "修改密码失败"); return; }
        showLogin();
        showMessage("loginError", "密码已修改，请使用新密码重新登录");
    } finally {
        passwordBusy = false;
        document.getElementById("savePasswordBtn").disabled = false;
    }
}

async function loadTilemaps() {
    const response = await fetchJSON("/api/config/tilemaps");
    if (!response.ok) {
        tilemaps = FALLBACK_TILEMAPS.slice();
    } else {
        tilemaps = Array.isArray(response.data) && response.data.length > 0
            ? response.data
            : FALLBACK_TILEMAPS.slice();
        if (!tilemaps.some((source) => source.metadata?.kind === "elevation")) {
            tilemaps.push(...FALLBACK_TILEMAPS.filter((source) => source.metadata?.kind === "elevation"));
        }
    }
    const groups = getProviderGroups().filter((group) => group.id !== "地形高程（Terrarium）");
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
    indexRegionCatalog();
    const initialRegions = getRegionOptionsForParent("province", "china")
        .filter((item) => item.maintained !== false && item.geojson);
    void fetchRegionGeoJSONs(initialRegions).catch((error) => {
        console.warn("failed to preload province geojson", error);
    });
}

function indexRegionCatalog() {
    allRegions = [
        ...regionCatalog.map((item) => ({ ...item, maintained: true })),
        ...missingRegionCatalog.map((item) => ({ ...item, maintained: false }))
    ];
    regionsByID = new Map();
    regionsByLevel = new Map();
    regionsByParentAndLevel = new Map();
    allRegions.forEach((region) => {
        regionsByID.set(region.id, region);
        if (!regionsByLevel.has(region.level)) {
            regionsByLevel.set(region.level, []);
        }
        regionsByLevel.get(region.level).push(region);

        const key = getRegionParentLevelKey(region.level, region.parentId);
        if (!regionsByParentAndLevel.has(key)) {
            regionsByParentAndLevel.set(key, []);
        }
        regionsByParentAndLevel.get(key).push(region);
    });
}

function getRegionParentLevelKey(level, parentID) {
    return `${level}\u0000${parentID || ""}`;
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
        const selectDisabled = config.locked || config.options.length === 0;
        const canRemove = config.id > 2 && config.id === activeLevelCount;

        row.className = "region-row";
        row.innerHTML = `
            <div class="region-row__title">
                ${icon("layers")}
                <span>
                    <strong>${escapeHTML(config.label)}</strong>
                    <span>${escapeHTML(getRegionHelperText(config))}</span>
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
                ${canRemove ? `<button type="button" class="danger-icon-button remove-level" data-id="${config.id}" aria-label="删除区域">${icon("delete")}</button>` : `<span class="region-row__action-spacer" aria-hidden="true"></span>`}
            </div>
            ${config.options.filter(item => item.maintained === false).map(item => `<p class="message error">${escapeHTML(item.name)}：${escapeHTML(item.reason || "区域边界文件缺失")}</p>`).join("")}
        `;
        container.appendChild(row);
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
        } else if (!config.options.some((item) => item.id === config.selectedRegionId && item.maintained !== false)) {
            config.selectedRegionId = config.options.find(item => item.maintained !== false)?.id || "";
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
        return regionsByLevel.get(config.regionLevel) || [];
    }

    const parentConfig = findLevelConfig(config.parentConfigId);
    if (!parentConfig || !parentConfig.selectedRegionId) {
        return [];
    }

    return getRegionOptionsForParent(config.regionLevel, parentConfig.selectedRegionId);
}

function getRegionOptionsForParent(level, parentID) {
    return regionsByParentAndLevel.get(getRegionParentLevelKey(level, parentID)) || [];
}

function getRegionByID(id) {
    return regionsByID.get(id) || null;
}

function getAllRegions() {
    return allRegions;
}

function renderRegionOptions(config) {
    if (config.options.length === 0) {
        return `<option value="">暂无可选区域</option>`;
    }

    return config.options
        .map((item) => `<option value="${escapeAttribute(item.id)}" ${item.maintained === false ? "disabled" : ""} ${item.id === config.selectedRegionId ? "selected" : ""}>${escapeHTML(item.name)}${item.maintained === false ? ` (${escapeHTML(item.reason || "边界缺失")})` : ""}</option>`)
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
    const groups = getProviderGroups().filter((group) => group.id !== "地形高程（Terrarium）");
    container.innerHTML = groups.map((group) => renderProviderCard(group)).join("") + renderTerrainAddon();
    updateSourceCapabilities();
    updateRangeEstimate();
}

function renderTerrainAddon() {
    const terrain = tilemaps.find(isTerrariumSource);
    if (!terrain) return "";
    const checked = selectedSourceIds.has(String(terrain.id));
    return `
        <label class="terrain-addon">
            <input class="terrain-addon__input" type="checkbox" value="${escapeAttribute(terrain.id)}" ${checked ? "checked" : ""}>
            <span class="source-option__check" aria-hidden="true"></span>
            <span class="terrain-addon__body">
                <strong>附加地形高程</strong>
                <small>作为额外子任务下载 Terrarium，并自动生成 Cesium 地形包</small>
            </span>
            <span class="terrain-addon__badge">+1 子任务</span>
        </label>
    `;
}

function tiandituLayer(source) {
    return /[?&]T=(img|cia|vec|ter|cta)_w(?:&|$)/i.exec(String(source.url || ""))?.[1].toLowerCase() || "";
}

function isTerrariumSource(source) {
    return source.metadata?.kind === "elevation" && source.metadata?.encoding === "terrarium"
        && String(source.format).toLowerCase() === "png" && String(source.schema).toLowerCase() === "xyz";
}

function selectedZoomRange(sources = getSelectedTilemaps()) {
    return {
        min: Math.max(0, ...sources.map((source) => Number(source.metadata?.minZoom ?? source.min_zoom ?? 0))),
        max: Math.min(20, ...sources.map((source) => Number(source.metadata?.maxZoom ?? source.max_zoom ?? 20)))
    };
}

function updateSourceCapabilities() {
    const sources = getSelectedTilemaps();
    const terrainSelected = sources.some(isTerrariumSource);
    const terrarium = sources.length > 0 && sources.every(isTerrariumSource);
    const key = sources.map((source) => source.id).join(",");
    const range = selectedZoomRange(sources);
    const preferredZoom = terrarium ? Math.min(range.max, sources[0].metadata.defaultZoom || 14) : range.min;
    const form = document.getElementById("taskForm");
    const output = form.elements.outputFormat;
    if (key !== sourceCapabilityKey) {
        if (terrarium) {
            form.elements.rangeMinZoom.value = preferredZoom;
            form.elements.rangeMaxZoom.value = preferredZoom;
        }
        sourceCapabilityKey = key;
    }
    if (terrarium) output.value = "cesium";
    else if (output.value === "cesium") output.value = "tar.gz";
    for (const input of document.querySelectorAll('.output-format-card input[name="outputFormat"]')) {
        input.disabled = terrarium;
    }
    document.querySelector(".output-format-options").classList.toggle("is-disabled", terrarium);
    for (const name of ["rangeMinZoom", "rangeMaxZoom"]) {
        form.elements[name].min = range.min;
        form.elements[name].max = range.max;
    }
    document.getElementById("outputFormatHint").textContent = terrarium
        ? `地形高程源：默认 ${preferredZoom} 级，最高 ${range.max} 级；下载后自动生成可三维预览的 Cesium 地形包。`
        : terrainSelected
            ? "普通地图子任务使用当前 TAR.GZ / MBTiles 设置；附加地形高程子任务自动生成 Cesium 地形包。"
            : "普通地图源可选择 TAR.GZ 或 MBTiles；需要真实地形时可勾选下方「附加地形高程」。";
    document.getElementById("rangeTiandituShortcuts").classList.toggle("is-hidden", selectedProvider !== "天地图");
    for (const checkbox of document.querySelectorAll(".range-layer-option")) {
        const source = tilemaps.find((item) => tiandituLayer(item) === checkbox.value);
        checkbox.disabled = !source;
        checkbox.checked = Boolean(source && selectedSourceIds.has(String(source.id)));
    }
    document.getElementById("rangeTerrainEstimate").classList.toggle("is-hidden", output.value !== "cesium");
    updateOutputSettings();
}

function validateOutputSources(sources, output) {
    if (output.format === "cesium" && sources.some((source) => !isTerrariumSource(source))) {
        return "Cesium 三维地形需要 Terrarium 高程源。请在地图源中选择「地形高程（Terrarium）」；普通地图图片或矢量图层请使用 TAR.GZ / MBTiles。";
    }
    return "";
}

function buildSelectedSources(sources, credentials) {
    return sources.map((source, index) => ({
        id: toNumericSourceId(source.id, index), name: source.name,
        url: applySourceCredentials(source.url, credentials),
        format: getTaskTilemapFormat(source), schema: source.schema,
        outputFormat: isTerrariumSource(source) ? "cesium" : undefined,
        ...(source.metadata ? {metadata: {...source.metadata}} : {})
    }));
}

function renderProviderCard(group) {
    const rawGroupID = String(group.id ?? "");
    const selected = selectedProvider === rawGroupID;
    const expanded = expandedProviders.has(rawGroupID);
    const selectedCount = group.items.filter((item) => selectedSourceIds.has(String(item.id))).length;
    group = {
        ...group,
        id: escapeAttribute(rawGroupID),
        name: escapeHTML(group.name),
        description: escapeHTML(group.description)
    };
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
    const sourceFormat = escapeHTML(String(getTaskTilemapFormat(tilemap)).toUpperCase());
    const sourceHost = escapeHTML(formatTilemapHost(tilemap.url));
    const minZoom = Number.isFinite(Number(tilemap.min_zoom)) ? Number(tilemap.min_zoom) : 0;
    const maxZoom = Number.isFinite(Number(tilemap.max_zoom)) ? Number(tilemap.max_zoom) : 0;
    const dataLabel = tilemap.metadata?.kind === "elevation" ? "真实高程 · Terrarium 编码"
        : tilemap.format === "pbf" ? "矢量数据，非高程模型" : "地图图片";
    tilemap = {
        ...tilemap,
        id: escapeAttribute(tilemap.id),
        name: escapeHTML(tilemap.name),
        min_zoom: minZoom,
        max_zoom: maxZoom,
        schema: escapeHTML(String(tilemap.schema || ""))
    };
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
                    <span>${sourceFormat}</span>
                </span>
                <span class="source-option__meta">
                    <span>${tilemap.min_zoom}-${tilemap.max_zoom}级</span>
                    <span>${sourceHost}</span>
                    <span>${tilemap.schema.toUpperCase()}</span>
                    <span>${dataLabel}</span>
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
    if (name === "地形高程（Terrarium）") return "真实高程数据，自动生成 Cesium 三维地形包";
    if (name === "天地图") {
        return "影像、电子地图与地形图片";
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
    const selected = group ? group.items.filter((item) => selectedSourceIds.has(String(item.id))) : [];
    const terrain = tilemaps.find((item) => isTerrariumSource(item) && selectedSourceIds.has(String(item.id)));
    if (terrain && !selected.some((item) => String(item.id) === String(terrain.id))) selected.push(terrain);
    return selected;
}

function getRequestedImageFormat() {
    return "png";
}

function getTaskTilemapFormat(tilemap) {
    const sourceFormat = String(tilemap.format || "").toLowerCase();
    if (sourceFormat === "pbf" || tilemap.metadata?.kind === "elevation") {
        return sourceFormat;
    }
    return getRequestedImageFormat();
}

function getTilemapGroupName(tilemap) {
    if (tilemap.metadata?.kind === "elevation") return "地形高程（Terrarium）";
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
    const priority = ["天地图", "地形高程（Terrarium）", "Google", "Mapbox", "高德", "ArcGIS", "Bing Maps", "其他地图源"];
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

async function createRangeTask(event, formData, settings) {
    const request = readRangeRequest(formData);
    const validation = validateRangeRequest(request);
    if (validation) {
        showMessage("taskError", validation);
        return;
    }

    const { workers: workerCount, savePipe, timeDelay } = settings;
    const schedule = readScheduleRequest(formData);
    if (schedule.error) {
        showMessage("taskError", schedule.error);
        return;
    }
    const output = readOutputRequest(formData);
    const outputError = validateOutputSources(request.sources, output);
    if (outputError) { showMessage("taskError", outputError); return; }
    const sources = buildSelectedSources(request.sources, request.credentials);

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

    resetTaskFormKeepingOptions(event.target, formData);
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
        name: String(formData.get("name") || "").trim() || "范围下载任务",
        credentials: readCredentialRequest(formData),
        sources: getSelectedTilemaps(),
        selectionMode: rangeDrawMode,
        bbox,
        polygon: getRangePolygonRequest(),
        zoom: {
            min: Number.parseInt(formData.get("rangeMinZoom"), 10),
            max: Number.parseInt(formData.get("rangeMaxZoom"), 10)
        }
    };
}

function getRangePolygonRequest() {
    return rangeClickPoints.map((point) => ({
        lon: Number(point.lng.toFixed(6)),
        lat: Number(point.lat.toFixed(6))
    }));
}

function validateTaskName(name) {
    const normalized = String(name || "").trim();
    if (!normalized) {
        return "任务名称不能为空。";
    }
    if (Array.from(normalized).length > 80) {
        return "任务名称不能超过 80 个字符。";
    }
    return "";
}

function validateRangeRequest(request) {
    const nameError = validateTaskName(document.getElementById("taskForm")?.elements.name?.value);
    if (nameError) {
        return nameError;
    }
    if (request.sources.length === 0) return "请至少选择一个地图源图层。";
    const credentialError = validateSourceCredentials(request.sources, request.credentials);
    if (credentialError) return credentialError;
    if (request.selectionMode === "polygon" && !isValidPolygon(request.polygon)) {
        return "请在地图上点击至少 3 个有效点形成多边形范围。";
    }
    if (!isValidBBox(request.bbox, true)) {
        return "范围坐标无效，请确认最小经度、最小纬度、最大经度、最大纬度。";
    }
    if (!Number.isInteger(request.zoom.min) || !Number.isInteger(request.zoom.max)) {
        return "层级必须是整数。";
    }
    const range = selectedZoomRange(request.sources);
    if (request.zoom.min < range.min || request.zoom.max > range.max || request.zoom.min > request.zoom.max) {
        return `所选来源层级范围为 ${range.min}～${range.max}，最小级别不能大于最大级别。`;
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

    const range = selectedZoomRange(request.sources);
    const outputFormat = document.getElementById("taskForm").elements.outputFormat.value;
    const hint = document.getElementById("rangeTerrainEstimate");
    hint.classList.toggle("is-hidden", outputFormat !== "cesium" && request.selectionMode !== "polygon");
    hint.textContent = "";
    if (!isValidRangeSelection(request) || !Number.isInteger(request.zoom.min) || !Number.isInteger(request.zoom.max) || request.zoom.min < range.min || request.zoom.max > range.max || request.zoom.min > request.zoom.max || request.sources.length === 0) {
        tileElement.textContent = "-";
        totalElement.textContent = "-";
        sizeElement.textContent = "-";
        return;
    }

    const estimate = estimateRangeDownloads(request.bbox, request.zoom, request.sources, outputFormat);
    const counts = estimate.perSource.map((source) => source.rawTiles);
    tileElement.textContent = new Set(counts).size === 1 ? String(counts[0]) : `${Math.min(...counts)}～${Math.max(...counts)}`;
    totalElement.textContent = String(estimate.rawTiles);
    sizeElement.textContent = request.sources.some((source) => source.metadata?.kind === "elevation")
        ? "以源瓦片和转换结果为准" : `${((estimate.rawTiles * 8) / 1024).toFixed(2)} MB`;
    const details = [];
    if (outputFormat === "cesium") details.push(`已计入祖先与接缝邻域：${estimate.rawTiles} 个原始请求，${estimate.meshTiles} 个地形网格。`);
    if (request.selectionMode === "polygon") details.push("按外接矩形保守估算；多边形实际请求数以准备阶段去重结果为准。");
    hint.textContent = details.join(" ");
}

function rangeTileRectangle(bbox, z) {
    const leftTop = rangeTileForLonLat(bbox.minLon, bbox.maxLat, z);
    const rightBottom = rangeTileForLonLat(bbox.maxLon, bbox.minLat, z);
    return { minX: leftTop.x, maxX: rightBottom.x, minY: leftTop.y, maxY: rightBottom.y };
}

function calculateRangeTileCount(bbox, zoom) {
    let count = 0;
    for (let z = zoom.min; z <= zoom.max; z += 1) {
        const rect = rangeTileRectangle(bbox, z);
        count += (rect.maxX - rect.minX + 1) * (rect.maxY - rect.minY + 1);
    }
    return count;
}

// Count integer rectangles by interval union. Work depends on zoom ranges,
// never on the number of tiles (a world-sized level 15 is still inexpensive).
function countTileRectangleUnion(rectangles) {
    const edges = [...new Set(rectangles.flatMap((rect) => [rect.minX, rect.maxX + 1]))].sort((a, b) => a - b);
    let count = 0;
    for (let i = 1; i < edges.length; i += 1) {
        const left = edges[i - 1];
        const intervals = rectangles.filter((rect) => rect.minX <= left && rect.maxX >= left)
            .map((rect) => [rect.minY, rect.maxY + 1]).sort((a, b) => a[0] - b[0]);
        let covered = 0;
        let end = -1;
        for (const [start, nextEnd] of intervals) {
            covered += Math.max(0, nextEnd - Math.max(start, end));
            end = Math.max(end, nextEnd);
        }
        count += (edges[i] - left) * covered;
    }
    return count;
}

function terrainHaloRectangles(rect, z) {
    const n = 2 ** z;
    const rows = { minY: Math.max(0, rect.minY - 1), maxY: Math.min(n - 1, rect.maxY + 1) };
    const left = rect.minX - 1;
    const right = rect.maxX + 1;
    if (right - left + 1 >= n) return [{ ...rows, minX: 0, maxX: n - 1 }];
    if (left < 0) return [{ ...rows, minX: 0, maxX: right }, { ...rows, minX: n + left, maxX: n - 1 }];
    if (right >= n) return [{ ...rows, minX: left, maxX: n - 1 }, { ...rows, minX: 0, maxX: right - n }];
    return [{ ...rows, minX: left, maxX: right }];
}

function calculateTerrainTileCounts(bbox, zoom) {
    const levels = Array.from({ length: zoom.max + 1 }, () => []);
    for (let z = zoom.min; z <= zoom.max; z += 1) {
        const rect = rangeTileRectangle(bbox, z);
        for (let ancestor = 0; ancestor <= z; ancestor += 1) {
            const scale = 2 ** (z - ancestor);
            levels[ancestor].push({ minX: Math.floor(rect.minX / scale), maxX: Math.floor(rect.maxX / scale),
                minY: Math.floor(rect.minY / scale), maxY: Math.floor(rect.maxY / scale) });
        }
    }
    let rawTiles = 0;
    let meshTiles = 0;
    levels.forEach((rectangles, z) => {
        meshTiles += countTileRectangleUnion(rectangles);
        rawTiles += countTileRectangleUnion(rectangles.flatMap((rect) => terrainHaloRectangles(rect, z)));
    });
    return { rawTiles, meshTiles, selectedTiles: calculateRangeTileCount(bbox, zoom) };
}

function estimateRangeDownloads(bbox, zoom, sources, outputFormat) {
    const selectedTiles = calculateRangeTileCount(bbox, zoom);
    const terrain = sources.some(isTerrariumSource) ? calculateTerrainTileCounts(bbox, zoom) : null;
    const perSource = sources.map((source) => {
        const counts = terrain && isTerrariumSource(source) ? terrain : { rawTiles: selectedTiles, meshTiles: 0 };
        return { id: source.id, rawTiles: counts.rawTiles, meshTiles: counts.meshTiles };
    });
    const rawTiles = perSource.reduce((sum, source) => sum + source.rawTiles, 0);
    return { rawTiles, meshTiles: perSource.reduce((sum, source) => sum + source.meshTiles, 0),
        auxiliaryTiles: rawTiles - selectedTiles * sources.length, perSource };
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

async function createTask(event) {
    event.preventDefault();
    if (taskMode === "tasks" || taskCreatePending) return;
    hideMessage("taskError");
    hideMessage("taskCreateNotice");
    const formData = new FormData(event.target);
    const settings = readDownloadSettings(formData);
    if (settings.error) {
        showMessage("taskError", settings.error);
        return;
    }
    const button = document.getElementById("createTaskBtn");
    taskCreatePending = true;
    button.disabled = true;
    button.setAttribute("aria-busy", "true");
    button.querySelector("span").textContent = "正在创建…";
    try {
        await createTaskFromForm(event, formData, settings);
    } finally {
        taskCreatePending = false;
        button.disabled = false;
        button.removeAttribute("aria-busy");
        button.querySelector("span").textContent = "创建任务";
    }
}

async function createTaskFromForm(event, formData, settings) {
    if (taskMode === "bbox") {
        await createRangeTask(event, formData, settings);
        return;
    }

    const nameError = validateTaskName(formData.get("name"));
    if (nameError) {
        showMessage("taskError", nameError);
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
        .map((config) => ({
            minZoom: config.minZoom,
            maxZoom: config.maxZoom,
            geojson: config.geojson
        }));

    if (levels.length === 0) {
        showMessage("taskError", "请至少保留一个下载区域。");
        return;
    }

    const invalidConfig = levelConfigs
        .slice(0, activeLevelCount)
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

    const { workers: workerCount, savePipe, timeDelay } = settings;
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

    const outputError = validateOutputSources(selectedTilemaps, output);
    if (outputError) { showMessage("taskError", outputError); return; }
    const sources = buildSelectedSources(selectedTilemaps, credentials);


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

    resetTaskFormKeepingOptions(event.target, formData);
    syncScheduleControls();
    const providerGroups = getProviderGroups().filter((group) => group.id !== "地形高程（Terrarium）");
    selectedProvider = providerGroups[0] ? providerGroups[0].id : "";
    ensureProviderSelection();
    initDefaultLevelConfigs();
    renderTilemapSelector();
    renderLevelConfigs();
    adminRegionLevel = "province";
    void renderAdminRegionMap();
    await loadTasks();
    setWorkspaceTab("tasks");
}

function resetTaskFormKeepingOptions(form, formData) {
    form.reset();
    for (const name of ["workers","savePipe","timeDelay","outputFormat","tiandituToken","mapboxToken","mapboxSku","rangeMinZoom","rangeMaxZoom"]) {
        form.elements[name].value = formData.get(name) ?? (name === "savePipe" ? "1" : "");
    }
    updateOutputSettings();
}

async function loadTasks() {
    if(taskLoadPromise) return taskLoadPromise;
    const generation = taskLoadGeneration;
    const pending = loadTasksOnce(generation).finally(()=>{if(taskLoadPromise === pending) taskLoadPromise=null;});
    taskLoadPromise = pending;
    return pending;
}

async function loadTasksOnce(generation) {
    if (!taskListLoaded) showMessage("taskListMessage","加载中...");
    const response = await fetchJSON(`/api/tasks/page?limit=${TASK_PAGE_LIMIT}&offset=${taskPageOffset}&status=${encodeURIComponent(currentTaskFilter)}`, { allowUnauthorized: true });
    if(generation !== taskLoadGeneration) return;
    if (!response.ok) {
        if (response.status === 401) {
            showLogin();
        } else {
            showMessage("taskListMessage",response.status === 0 ? "网络连接失败，等待重试。" : "任务加载失败，等待重试。");
        }
        return;
    }

    taskPageTotal = Number(response.data.total) || 0;
    taskCounts = response.data.counts || {};
    if (taskPageOffset >= taskPageTotal && taskPageOffset > 0) {
        taskPageOffset = Math.max(0, Math.floor((taskPageTotal - 1) / TASK_PAGE_LIMIT) * TASK_PAGE_LIMIT);
        return loadTasksOnce(generation);
    }
    cachedTasks = Array.isArray(response.data.items) ? response.data.items : [];
    taskListLoaded = true;
    hideMessage("taskListMessage");
    cleanupSelections(cachedTasks);
    updateWorkspaceTaskCount(cachedTasks);
    renderTaskStats(cachedTasks);
    renderTaskList(cachedTasks);
    renderSelectionMeta();
    renderTaskPagination();
}

function changeTaskPage(offset) {
    taskPageOffset = offset;
    taskLoadGeneration++;
    taskLoadPromise = null;
    selectedTaskIds.clear();
    void loadTasks();
}

function renderTaskPagination() {
    const pages = Math.max(1, Math.ceil(taskPageTotal / TASK_PAGE_LIMIT));
    document.getElementById("taskPageInfo").textContent = `第 ${Math.floor(taskPageOffset / TASK_PAGE_LIMIT) + 1}/${pages} 页 · ${taskPageTotal} 个任务`;
    document.getElementById("taskPagePrev").disabled = taskPageOffset === 0;
    document.getElementById("taskPageNext").disabled = taskPageOffset + TASK_PAGE_LIMIT >= taskPageTotal;
}

function updateWorkspaceTaskCount(tasks) {
    const element = document.getElementById("workspaceTaskCount");
    if (!element) {
        return;
    }
    element.textContent = String(taskCounts?.all ?? (Array.isArray(tasks) ? tasks.length : 0));
}

function renderTaskStats(tasks) {
    const counts = { ...countTasks([]), ...(taskCounts || countTasks(tasks)) };
    const stats = [
        { id: "all", label: "全部任务", count: counts.all },
        { id: "running", label: "运行中", count: counts.running },
        { id: "scheduled", label: "等待中", count: counts.scheduled + counts.pending + counts.queued },
        { id: "completed", label: "已完成", count: counts.completed },
        { id: "failed", label: "失败", count: counts.failed + counts.partial_failed },
        { id: "paused", label: "已暂停", count: counts.paused }
    ];

    const container = document.getElementById("taskStats");
    if (!container.children.length) container.innerHTML = stats.map((stat) => `
        <button type="button" class="stat-chip ${currentTaskFilter === stat.id ? "is-active" : ""}" data-task-filter="${stat.id}">
            <span>${stat.label}</span>
            <small>${stat.count}</small>
        </button>
    `).join("");
    for (const stat of stats) {
        const button = container.querySelector(`[data-task-filter="${stat.id}"]`);
        button.classList.toggle("is-active", currentTaskFilter === stat.id);
        button.querySelector("small").textContent = stat.count;
    }
}

function renderTaskList(tasks) {
    const list = document.getElementById("taskList");
    const filtered = applyTaskFilter(tasks);

    if (filtered.length === 0) {
        if (!list.querySelector(".task-empty")) list.innerHTML = `
            <div class="task-empty">
                <img src="/static/assets/images/empty-state.svg" alt="暂无任务">
                <h3>暂无任务</h3>
                <p>暂无任务，创建一个下载任务后可在这里查看进度。</p>
            </div>
        `;
        renderedTaskCards.clear();
        return;
    }

    const scrollTop = list.scrollTop;
    const wanted = new Set(filtered.map((task) => String(task.id)));
    for (const [id, cached] of renderedTaskCards) {
        if (!wanted.has(id)) { cached.node.remove(); renderedTaskCards.delete(id); }
    }
    for (const child of Array.from(list.children)) {
        if (!child.matches("[data-group-task-id]") || renderedTaskCards.get(child.dataset.groupTaskId)?.node !== child) child.remove();
    }
    filtered.forEach((task, index) => {
        const id = String(task.id);
        const signature = JSON.stringify(task);
        let cached = renderedTaskCards.get(id);
        if (!cached || cached.signature !== signature || cached.node.parentNode !== list) {
            const old = cached?.node;
            const focus = old?.contains(document.activeElement) ? taskFocusKey(document.activeElement) : null;
            const openMenus = old ? Array.from(old.querySelectorAll(".task-menu__panel:not(.is-hidden)")).map((menu) => menu.id) : [];
            const template = document.createElement("template");
            template.innerHTML = task.kind === "group" ? renderGroupTask(task) : renderStandaloneTask(task);
            const node = template.content.firstElementChild;
            if (old?.parentNode === list) old.replaceWith(node);
            else list.insertBefore(node, list.children[index] || null);
            for (const menuID of openMenus) {
                const menu = Array.from(node.querySelectorAll(".task-menu__panel")).find((item) => item.id === menuID);
                menu?.classList.remove("is-hidden");
            }
            if (focus) {
                Array.from(node.querySelectorAll("button,a,input,summary")).find((element) => taskFocusKey(element) === focus)?.focus({preventScroll:true});
            }
            cached = { node, signature };
            renderedTaskCards.set(id, cached);
        }
        if (list.children[index] !== cached.node) list.insertBefore(cached.node, list.children[index] || null);
        const checkbox = cached.node.querySelector(".task-select");
        if (checkbox) checkbox.checked = selectedTaskIds.has(id);
        cached.node.open = expandedGroupTasks.has(id);
        const subtaskList = cached.node.querySelector("details[data-subtask-list-id]");
        if (subtaskList) subtaskList.open = expandedSubtaskLists.has(id);
        for (const child of cached.node.querySelectorAll("details[data-child-task-id]")) {
            child.open = expandedChildTasks.has(child.dataset.childTaskId);
        }
    });
    list.scrollTop = scrollTop;
}

function taskFocusKey(element) {
    const details = element.closest("details");
    return JSON.stringify([element.tagName, element.dataset.taskAction, element.dataset.taskId,
        element.dataset.taskMenuToggle, details?.dataset.childTaskId || details?.dataset.groupTaskId,
        element.getAttribute("href")]);
}

function renderGroupTask(task) {
    const children = Array.isArray(task.children) ? task.children : [];
    const progress = toPercent(task.progress);
    const riskHint = summarizeTaskRisk(task, children);
    const rawTaskID = String(task.id || "");
    task = { ...task, id: escapeAttribute(rawTaskID), name: escapeHTML(task.name) };
    const isOpen = expandedGroupTasks.has(rawTaskID);
    const subtasksOpen = expandedSubtaskLists.has(rawTaskID);
    const menuId = `menu-${task.id}`;

    return `
        <details class="task-card" data-group-task-id="${task.id}" ${isOpen ? "open" : ""}>
            <summary>
                <div class="task-card__summary">
                    <label class="task-check" onclick="event.stopPropagation()">
                        <input class="task-select" type="checkbox" data-task-id="${task.id}" ${selectedTaskIds.has(task.id) ? "checked" : ""}>
                    </label>
                    <div class="task-main">
                        <div class="task-main__title">
                            <h3 title="${task.name}">${task.name}</h3>
                        </div>
                        <div class="task-progress-line">
                            <div class="progress-bar">
                                <div class="progress-bar__value" style="width:${progress}%"></div>
                            </div>
                            <strong>${progress}%</strong>
                        </div>
                    </div>
                    <div class="task-actions">
                        <div class="task-menu">
                            <button type="button" class="icon-mini-button" data-task-menu-toggle="${menuId}" aria-label="更多操作">
                                <span class="task-menu-dots" aria-hidden="true">•••</span>
                            </button>
                            <div id="${menuId}" class="task-menu__panel is-hidden">
                                <button type="button" data-task-action="pause" data-task-id="${task.id}" data-task-status="${task.status}">暂停全部</button>
                                <button type="button" data-task-action="resume" data-task-id="${task.id}" data-task-status="${task.status}">恢复全部</button>
                                <button type="button" data-task-action="cancel" data-task-id="${task.id}" data-task-status="${task.status}">取消全部</button>
                                ${canRetryFailures(task) ? `<button type="button" data-task-action="retryFailures" data-task-id="${task.id}" data-task-status="${task.status}">重试失败瓦片</button>` : ""}
                                <button type="button" data-task-action="delete" data-task-id="${task.id}" data-task-status="${task.status}">删除任务</button>
                                <button type="button" data-task-action="reconcile" data-task-id="${task.id}">核对本地产物</button>
                                <button type="button" data-task-action="recreate" data-task-id="${task.id}">重新创建完整任务</button>
                                <button type="button" data-task-action="editSource" data-task-id="${task.id}">修改来源后新建</button>
                            </div>
                        </div>
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="task-card__content">
                <div class="task-overview-line">
                    ${renderStatusPill(task.status)}
                    <span>运行：<strong>${task.runningChildren || 0}</strong></span>
                    <span>暂停：<strong>${task.pausedChildren || 0}</strong></span>
                    <span>失败：<strong>${task.failedChildren || 0}</strong></span>
                    <span>进度：<strong>${progress}%</strong></span>
                </div>
                <div class="task-time-list">
                    <span>开始时间：<strong>${formatTaskStart(task, children)}</strong></span>
                    <span>结束时间：<strong>${formatTaskEnd(task, children)}</strong></span>
                </div>
                ${renderTaskPhase(children.find((child) => ["running", "queued", "paused"].includes(child.status)) || task)}
                ${riskHint ? `<div class="task-inline-warning">${renderWarningPill(riskHint.short)}</div>` : ""}
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                <details class="task-subtasks" data-subtask-list-id="${task.id}" ${subtasksOpen ? "open" : ""}>
                    <summary>
                        <span>子任务明细（${children.length}）</span>
                        ${icon("chevron-down", "task-chevron")}
                    </summary>
                    <div class="child-task-list">
                        ${children.map((child) => renderChildTask(child)).join("")}
                    </div>
                </details>
            </div>
        </details>
    `;
}

function renderStandaloneTask(task) {
    const progress = toPercent(task.progress);
    const riskHint = detectTaskRisk(task);
    const rawTaskID = String(task.id || "");
    const terrainPreviewLink = renderTerrainPreviewLink(task);
    task = {
        ...task,
        terrainPreviewLink,
        id: escapeAttribute(rawTaskID),
        name: escapeHTML(task.name),
        errorMessage: escapeHTML(task.errorMessage),
        artifactName: escapeHTML(task.artifactName),
        downloadUrl: escapeAttribute(managedDownloadURL(task))
    };
    const menuId = `menu-${task.id}`;
    return `
        <details class="task-card" data-group-task-id="${task.id}" ${expandedGroupTasks.has(rawTaskID) ? "open" : ""}>
            <summary>
                <div class="task-card__summary">
                    <label class="task-check" onclick="event.stopPropagation()">
                        <input class="task-select" type="checkbox" data-task-id="${task.id}" ${selectedTaskIds.has(task.id) ? "checked" : ""}>
                    </label>
                    <div class="task-main">
                        <div class="task-main__title">
                            <h3 title="${task.name}">${task.name}</h3>
                        </div>
                        <div class="task-progress-line">
                            <div class="progress-bar">
                                <div class="progress-bar__value" style="width:${progress}%"></div>
                            </div>
                            <strong>${progress}%</strong>
                        </div>
                    </div>
                    <div class="task-actions">
                        <div class="task-menu">
                            <button type="button" class="icon-mini-button" data-task-menu-toggle="${menuId}" aria-label="更多操作">
                                <span class="task-menu-dots" aria-hidden="true">•••</span>
                            </button>
                            <div id="${menuId}" class="task-menu__panel is-hidden">
                                <button type="button" data-task-action="pause" data-task-id="${task.id}" data-task-status="${task.status}">暂停任务</button>
                                <button type="button" data-task-action="resume" data-task-id="${task.id}" data-task-status="${task.status}">恢复任务</button>
                                <button type="button" data-task-action="cancel" data-task-id="${task.id}" data-task-status="${task.status}">取消任务</button>
                                ${canRetryFailures(task) ? `<button type="button" data-task-action="retryFailures" data-task-id="${task.id}" data-task-status="${task.status}">重试失败瓦片</button>` : ""}
                                <button type="button" data-task-action="delete" data-task-id="${task.id}" data-task-status="${task.status}">删除任务</button>
                                <button type="button" data-task-action="reconcile" data-task-id="${task.id}">核对本地产物</button>
                                <button type="button" data-task-action="recreate" data-task-id="${task.id}">重新创建完整任务</button>
                                <button type="button" data-task-action="editSource" data-task-id="${task.id}">修改来源后新建</button>
                            </div>
                        </div>
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="task-card__content">
                <p class="task-full-name">${task.name}</p>
                <div class="task-detail-header">
                    ${renderStatusPill(task.status)}
                    ${riskHint ? renderWarningPill(riskHint.short) : ""}
                </div>
                <div class="task-metadata">
                    <span>开始时间：<strong>${formatTaskStart(task)}</strong></span>
                    <span>结束时间：<strong>${formatTaskEnd(task)}</strong></span>
                    <span>产物状态：<strong>${translateArtifactStatus(task.artifactStatus)}</strong></span>
                </div>
                ${renderTaskPhase(task)}
                <div class="task-metrics">
                    <span>进度：<strong>${task.current || 0}/${task.total || 0}</strong></span>
                    <span>成功：<strong>${task.successCount || 0}</strong></span>
                    <span>失败：<strong>${task.failureCount || 0}</strong></span>
                    <span>总进度：<strong>${progress}%</strong></span>
                </div>
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                ${renderStandaloneDetail(task)}
                ${renderIntegrity(task)}
            </div>
        </details>
    `;
}

function renderStandaloneDetail(task) {
    const artifactAction = task.artifactStatus === "ready" && task.downloadUrl
        ? `<a href="${task.downloadUrl}" class="artifact-link">${icon("download")}<span>下载产物</span></a>`
        : `<span class="artifact-text">产物：${translateArtifactStatus(task.artifactStatus)}</span>`;
    return `
        <div class="child-task-list">
            <div class="child-task" open>
                <div class="child-task__content">
                    <div class="child-task__footer">
                        <span class="artifact-text">${task.errorMessage || `开始：${task.startedAt ? formatDate(task.startedAt) : "-"} ｜ 完成：${task.finishedAt ? formatDate(task.finishedAt) : "-"}`}</span>
                        ${task.terrainPreviewLink || ""}${artifactAction}
                    </div>
                </div>
            </div>
        </div>
    `;
}

function renderChildTask(task) {
    const progress = toPercent(task.progress);
    const riskHint = detectTaskRisk(task);
    const rawTaskID = String(task.id || "");
    const terrainPreviewLink = renderTerrainPreviewLink(task);
    task = {
        ...task,
        id: escapeAttribute(rawTaskID),
        name: escapeHTML(task.name),
        sourceName: escapeHTML(task.sourceName),
        errorMessage: escapeHTML(task.errorMessage),
        artifactName: escapeHTML(task.artifactName),
        downloadUrl: escapeAttribute(managedDownloadURL(task))
    };
    const isOpen = expandedChildTasks.has(rawTaskID);
    const artifactAction = task.artifactStatus === "ready" && task.downloadUrl
        ? `<a href="${task.downloadUrl}" class="artifact-link">${icon("download")}<span>下载产物</span></a>`
        : `<span class="artifact-text">产物：${task.artifactStatus === "packing" && task.artifactName ? task.artifactName : translateArtifactStatus(task.artifactStatus)}</span>`;

    return `
        <details class="child-task" data-child-task-id="${task.id}" ${isOpen ? "open" : ""}>
            <summary>
                <div class="child-task__summary">
                    <div>
                        <div class="child-task__title">
                            ${icon(childTaskIcon(task), "child-task__icon")}
                            <strong title="${task.sourceName || task.name}">${task.sourceName || task.name}</strong>
                            ${renderStatusPill(task.status, true)}
                        </div>
                        <div class="task-progress-line">
                            <div class="progress-bar">
                                <div class="progress-bar__value" style="width:${progress}%"></div>
                            </div>
                            <strong>${progress}%</strong>
                        </div>
                    </div>
                    <div class="child-task__side">
                        ${icon("chevron-down", "task-chevron")}
                    </div>
                </div>
            </summary>
            <div class="child-task__content">
                <div class="task-detail-header">
                    ${renderArtifactPill(task.artifactStatus)}
                    ${riskHint ? renderWarningPill(riskHint.short) : ""}
                </div>
                ${renderTaskPhase(task)}
                <div class="child-task__meta">
                    <span>进度：<strong>${task.current || 0}/${task.total || 0}</strong></span>
                    <span>成功：<strong>${task.successCount || 0}</strong></span>
                    <span>失败：<strong>${task.failureCount || 0}</strong></span>
                    <span>完成度：<strong>${progress}%</strong></span>
                </div>
                ${riskHint ? `
                    <div class="task-risk-banner">
                        ${icon("warning")}
                        <span>${riskHint.long}</span>
                    </div>
                ` : ""}
                <div class="child-task__footer">
                    ${renderIntegrity(task)}
                    <span class="artifact-text">${task.errorMessage || `开始：${task.startedAt ? formatDate(task.startedAt) : "-"} ｜ 完成：${task.finishedAt ? formatDate(task.finishedAt) : "-"}`}</span>
                ${terrainPreviewLink}${artifactAction}
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

function clearTaskPreview() {
    taskPreviewSequence++;
    taskPreviewAbort?.abort();
    taskPreviewAbort=null;
    taskAreaPreviewLayer?.remove();
    taskAreaPreviewLayer=null;
    taskPreviewTask=null;
    restoreTaskPreviewBaseLayers();
    document.getElementById("taskPreviewControls")?.classList.add("is-hidden");
}

async function previewTaskArea(task, level) {
    clearTaskPreview();
    const sequence=taskPreviewSequence;
    taskPreviewTask=task;
    taskPreviewAbort=new AbortController();
    activeMapMode=task?.area?.bbox ? "bbox" : "region";
    setTaskMode("tasks");
    if(activeMapMode==="region") hideTaskPreviewBaseLayers(adminRegionMap);
    document.getElementById("taskPreviewControls").classList.remove("is-hidden");
    const status=document.getElementById("taskPreviewStatus");
    status.textContent=`${task.name || "任务"}：加载范围中...`;
    const query=Number.isInteger(level) ? `?level=${level}` : "";
    const cacheKey=`${task.id}:${Number.isInteger(level) ? level : "default"}`;
    let responseData=getCachedTaskAreaPreview(cacheKey);
    if(!responseData) {
        const response=await fetchJSON(`/api/tasks/${encodeURIComponent(task.id)}/area${query}`,{signal:taskPreviewAbort.signal});
        if(sequence!==taskPreviewSequence || taskMode!=="tasks") return;
        if(!response.ok) {
            status.textContent=response.status===0 ? "范围网络请求失败" : "任务范围加载失败";
            return;
        }
        responseData=response.data;
        cacheTaskAreaPreview(cacheKey,responseData);
    }
    if(sequence!==taskPreviewSequence || taskMode!=="tasks") return;
    const map=activeMapMode==="bbox" ? rangeMap : adminRegionMap;
    if(!map || !window.L) {
        status.textContent="任务范围加载失败";
        return;
    }
    const features=Array.isArray(responseData.features) ? responseData.features : [];
    if(features.length===0){status.textContent="该层级没有范围数据";return;}
    const select=document.getElementById("taskPreviewLevel");
    select.replaceChildren();
    for(const entry of responseData.levels || []) {
        const option=document.createElement("option");
        option.value=entry.index;
        option.textContent=`${Number(entry.minZoom)}-${Number(entry.maxZoom)} 级`;
        option.selected=entry.index===responseData.selectedLevel;
        select.append(option);
    }
    hideTaskPreviewBaseLayers(map);
    taskAreaPreviewLayer=L.geoJSON(responseData,{renderer:L.canvas({padding:0.25,tolerance:8}),interactive:false,smoothFactor:2,style:{color:"#2563eb",weight:2,fillColor:"#2563eb",fillOpacity:0.14}}).addTo(map);
    const bounds=taskAreaPreviewLayer.getBounds();
    if(bounds.isValid()) map.fitBounds(bounds.pad(0.12),{padding:[26,26],maxZoom:17});
    status.textContent=`${task.name || "任务"}：${features.length} 个区域`;
}

function getCachedTaskAreaPreview(cacheKey) {
    const data=taskAreaPreviewCache.get(cacheKey);
    if(!data) return null;
    taskAreaPreviewCache.delete(cacheKey);
    taskAreaPreviewCache.set(cacheKey,data);
    return data;
}

function cacheTaskAreaPreview(cacheKey,data) {
    taskAreaPreviewCache.delete(cacheKey);
    taskAreaPreviewCache.set(cacheKey,data);
    while(taskAreaPreviewCache.size>TASK_AREA_PREVIEW_CACHE_LIMIT) {
        taskAreaPreviewCache.delete(taskAreaPreviewCache.keys().next().value);
    }
}

function hideTaskPreviewBaseLayers(map) {
    if(map!==adminRegionMap || !adminRegionLayerGroup || !adminRegionMap.hasLayer(adminRegionLayerGroup)) return;
    adminRegionMap.removeLayer(adminRegionLayerGroup);
    taskPreviewBaseLayersHidden=true;
}

function restoreTaskPreviewBaseLayers() {
    if(!taskPreviewBaseLayersHidden || !adminRegionMap || !adminRegionLayerGroup) return;
    adminRegionLayerGroup.addTo(adminRegionMap);
    taskPreviewBaseLayersHidden=false;
}

function showTaskBBoxOnMap(bbox) {
    if (!rangeMap || !window.L) {
        return;
    }
    taskAreaPreviewLayer?.remove();
    const bounds = L.latLngBounds([bbox.minLat, bbox.minLon], [bbox.maxLat, bbox.maxLon]);
    taskAreaPreviewLayer = L.rectangle(bounds, {
        color: "#2563eb", weight: 3, dashArray: "6 6", fillColor: "#2563eb", fillOpacity: 0.14
    }).addTo(rangeMap);
    rangeMap.fitBounds(bounds.pad(0.2), { padding: [26, 26], maxZoom: 17 });
}

function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>'"]/g, (character) => ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        "'": "&#39;",
        "\"": "&quot;"
    }[character]));
}

function escapeAttribute(value) {
    return escapeHTML(value);
}

function managedDownloadURL(task) {
    const expected = `/api/tasks/${encodeURIComponent(String(task.id || ""))}/download`;
    const url = task.downloadUrl;
    if (url === expected) {
        return expected;
    }
    return typeof url === "string" && /^\/api\/downloads\/[a-f0-9]{64}$/.test(url) ? url : "";
}

function renderTerrainPreviewLink(task) {
    if (task.artifact?.format !== "cesium" || (task.artifact?.status || task.artifactStatus) !== "ready") return "";
    return `<a class="terrain-preview-link" href="/static/terrain-preview.html?task=${encodeURIComponent(String(task.id))}" target="_blank" rel="noopener">三维预览</a>`;
}

function renderTaskPhase(task) {
    if (task.queue?.state === "queued") {
        return `<p class="task-phase">排队等待执行 · 位置 ${Number(task.queue.position) || 0}</p>`;
    }
    if (task.queue?.state === "paused") return '<p class="task-phase">已暂停，等待恢复</p>';
    const phase = task.phase || {};
    if (!phase.name) return "";
    const labels = {preparing:"准备任务",enumerating:"统计瓦片",downloading:"下载瓦片",verifying:"核对完整度",packing:"打包产物",writing:"写入数据",converting:"转换地形",publishing:"发布产物",completed:"处理完成"};
    const progress = Number(phase.total) > 0 ? ` · ${Number(phase.current) || 0}/${Number(phase.total)}` : "";
    return `<p class="task-phase">${escapeHTML(labels[phase.name] || phase.name)}${progress}</p>`;
}

function renderIntegrity(task) {
    const state = task.integrity || {};
    const labels = { unchecked: "未核对", checking: "核对中", complete: "完整", incomplete: "存在缺口", interrupted: "核对中断" };
    return `<span class="artifact-text">累计产物：${labels[state.status] || "未核对"} ${Number(state.available) || 0}/${Number(state.expected) || 0}${state.error ? `；${escapeHTML(state.error)}` : ""}<br>生效线程：${Number(task.effectiveWorkers) || 0}；间隔：${Number(task.effectiveTimeDelay) || 0} ms${task.queue?.state === "queued" ? `；排队位置：${Number(task.queue.position) || 0}` : ""}</span>`;
}

function renderWarningPill(text) {
    return `<span class="warning-pill">${escapeHTML(text)}</span>`;
}

function countTasks(tasks) {
    const counters = {
        all: tasks.length,
        scheduled: 0,
        pending: 0,
        queued: 0,
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
        return tasks.filter((task) => task.status === "scheduled" || task.status === "queued" || task.status === "pending");
    }
    if (currentTaskFilter === "failed") {
        return tasks.filter((task) => task.status === "failed" || task.status === "partial_failed");
    }
    return tasks.filter((task) => task.status === currentTaskFilter);
}

function renderSelectionMeta() {
    const count = selectedTaskIds.size;
    document.getElementById("taskSelectionMeta").textContent = count > 0 ? `已选择 ${count} 个任务` : "未选择任务";
    syncSelectTasksButton();
}

function cleanupSelections(tasks) {
    const taskIds = new Set(tasks.map((task) => task.id));
    Array.from(selectedTaskIds).forEach((id) => {
        if (!taskIds.has(id)) {
            selectedTaskIds.delete(id);
        }
    });
}

function getVisibleTasks() {
    return applyTaskFilter(cachedTasks);
}

function toggleSelectVisibleTasks() {
    const visibleTasks = getVisibleTasks();
    if (visibleTasks.length === 0) {
        return;
    }

    const allSelected = visibleTasks.every((task) => selectedTaskIds.has(task.id));
    visibleTasks.forEach((task) => {
        if (allSelected) {
            selectedTaskIds.delete(task.id);
        } else {
            selectedTaskIds.add(task.id);
        }
    });

    renderTaskList(cachedTasks);
    renderSelectionMeta();
}

function syncSelectTasksButton() {
    const button = document.getElementById("toggleSelectTasksBtn");
    if (!button) {
        return;
    }
    const visibleTasks = getVisibleTasks();
    const allSelected = visibleTasks.length > 0 && visibleTasks.every((task) => selectedTaskIds.has(task.id));
    button.textContent = allSelected ? "取消全选" : "全选当前";
    button.disabled = visibleTasks.length === 0;
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
    case "reconcile":
        await mutateTask(`/api/tasks/${encodeURIComponent(taskId)}/reconcile`, "POST");
        break;
    case "recreate":
        if (window.confirm("重新创建将按原范围和地图源发起完整下载，保留旧任务及旧产物。确定继续吗？")) {
            await mutateTask(`/api/tasks/${encodeURIComponent(taskId)}/recreate`, "POST");
        }
        break;
    case "editSource":
        await editTaskSource(taskId);
        break;
    default:
        break;
    }
}

async function editTaskSource(taskID) {
    const response = await fetchJSON(`/api/tasks/${encodeURIComponent(taskID)}`);
    if (!response.ok) { showMessage("taskListMessage", "读取原任务失败，请刷新后重试。"); return; }
    const task = response.data;
    const regionLevels = task.area?.levels || [];
    const regionMatches = regionLevels.map((level) => regionCatalog.find((region) => String(region.geojson).replace(/^\.\//, "") === String(level.geojson).replace(/^\.\//, "")));
    const isRange = task.area?.mode === "bbox" || task.mode === "bbox";
    let polygon = null;
    if (isRange && !task.area?.bbox) {
        const area = await fetchJSON(`/api/tasks/${encodeURIComponent(taskID)}/area`);
        const geometry = area.data?.features?.[0]?.geometry;
        if (area.ok && geometry?.type === "Polygon" && geometry.coordinates.length === 1) polygon = geometry.coordinates[0];
        if (!polygon) { showMessage("taskListMessage", "原任务范围不能直接编辑；可重新创建完整任务，或在新建页重新选择范围与来源。"); return; }
    } else if (!isRange && (!regionLevels.length || regionLevels.length > REGION_LEVEL_RULES.length || regionMatches.some((region) => !region))) {
        showMessage("taskListMessage", "原任务使用自定义区域文件；可重新创建完整任务，或在新建页重新选择区域与来源。");
        return;
    }
    const form = document.getElementById("taskForm");
    const originalSources = task.sources || [];
    const matches = tilemaps.filter((source) => originalSources.some((original) => original.name === source.name));
    if (matches.length) {
        const primary = matches.find((source) => !isTerrariumSource(source)) || matches[0];
        selectedProvider = getTilemapGroupName(primary);
        selectedSourceIds = new Set(matches.map((source) => String(source.id)));
        expandedProviders.add(selectedProvider);
    }
    renderTilemapSelector();
    form.elements.name.value = Array.from(`${task.name}（新来源）`).slice(0,80).join("");
    if (isRange) {
        const bbox = task.area?.bbox || {minLon:Math.min(...polygon.map((point)=>point[0])),maxLon:Math.max(...polygon.map((point)=>point[0])),minLat:Math.min(...polygon.map((point)=>point[1])),maxLat:Math.max(...polygon.map((point)=>point[1]))};
        for (const [name,key] of [["rangeMinLon","minLon"],["rangeMaxLon","maxLon"],["rangeMinLat","minLat"],["rangeMaxLat","maxLat"]]) form.elements[name].value=bbox[key];
        form.elements.rangeMinZoom.value=task.zoom?.min ?? task.minZoom;
        form.elements.rangeMaxZoom.value=task.zoom?.max ?? task.maxZoom;
        rangeDrawMode=polygon?"polygon":"rectangle";
        rangeClickPoints=polygon?polygon.slice(0,-1).map(([lon,lat])=>L.latLng(lat,lon)):[];
    } else {
        initDefaultLevelConfigs();
        activeLevelCount=regionLevels.length;
        regionLevels.forEach((level,index)=>Object.assign(levelConfigs[index],{selectedRegionId:regionMatches[index].id,geojson:regionMatches[index].geojson,minZoom:level.minZoom,maxZoom:level.maxZoom,enabled:true}));
        renderLevelConfigs();
    }
    setTaskMode(isRange?"bbox":"region");
    showMessage("taskCreateNotice", "已载入原范围。选择新来源后创建，原任务与已有产物会保留。");
}

async function handleBulkAction(action) {
    const selectedTasks = cachedTasks.filter((task) => selectedTaskIds.has(task.id));

    switch (action) {
    case "pauseSelected":
        if (!ensureSelectedTasks(selectedTasks)) {
            return;
        }
        await runBulkMutation(selectedTasks.filter((task) => canPause(task.status)), (task) => pauseTask(task.id, task.status, true), "已选任务中没有可暂停的任务。");
        break;
    case "resumeSelected":
        if (!ensureSelectedTasks(selectedTasks)) {
            return;
        }
        await runBulkMutation(selectedTasks.filter((task) => canResume(task.status)), (task) => resumeTask(task.id, task.status, true), "已选任务中没有可恢复的任务。");
        break;
    case "cancelSelected":
        if (!ensureSelectedTasks(selectedTasks)) {
            return;
        }
        if (!window.confirm("确定取消已选任务中未完成的任务吗？")) {
            return;
        }
        await runBulkMutation(selectedTasks.filter((task) => canCancel(task.status)), (task) => cancelTask(task.id, task.status, true), "已选任务中没有可取消的任务。");
        break;
    case "deleteSelected":
        if (!ensureSelectedTasks(selectedTasks)) {
            return;
        }
        {
            const blockedTasks = selectedTasks.filter((task) => !canDelete(task.status));
            if (blockedTasks.length > 0) {
                alert(`已选下载中有 ${blockedTasks.length} 个任务还不能删除。\n\n运行中或已暂停的下载不能直接删除，请先执行「取消已选下载」，再删除。\n\n${formatBlockedTaskNames(blockedTasks)}`);
                return;
            }
        }
        if (!window.confirm("确定删除已选任务及其全部子任务、历史下载文件、TAR.GZ/MBTiles 和失败记录吗？此操作不可恢复。共享区域文件会保留至最后一个引用删除。")) {
            return;
        }
        await runBulkMutation(
            selectedTasks.filter((task) => canDelete(task.status)),
            (task) => purgeTask(task.id, task.status, true),
            "已选任务中没有可删除的任务。"
        );
        selectedTaskIds.clear();
        break;
    default:
        break;
    }

    renderSelectionMeta();
}

function ensureSelectedTasks(tasks) {
    if (tasks.length === 0) {
        alert("请先选择至少一个任务。");
        return false;
    }
    return true;
}

function formatBlockedTaskNames(tasks) {
    const names = tasks.slice(0, 5).map((task) => `- ${task.name || task.sourceName || task.id}（${translateStatus(task.status)}）`);
    if (tasks.length > 5) {
        names.push(`- 另外 ${tasks.length - 5} 个任务`);
    }
    return names.join("\n");
}

async function runBulkMutation(tasks, fn, emptyMessage = "没有可操作的任务。") {
    if (tasks.length === 0) {
        alert(emptyMessage);
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
    if (!silent && !window.confirm("确定删除该任务及其全部子任务、历史下载文件、TAR.GZ/MBTiles 和失败记录吗？此操作不可恢复。共享区域文件会保留至最后一个引用删除。")) {
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
        if (response.data.code === "baseline_missing") {
            const id = url.match(/^\/api\/tasks\/([^/]+)\/retry-failures$/)?.[1];
            if (id) await handleTaskAction("recreate", decodeURIComponent(id));
        }
        return;
    }
    if (!silent) {
        await loadTasks();
    }
}

async function fetchJSON(url, options = {}) {
    const generation = taskLoadGeneration;
    const config = {
        method: options.method || "GET",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin"
    };
    if(options.signal) config.signal = options.signal;

    if (options.body) {
        config.body = options.body;
    }

    try {
        const response = await fetch(url, config);
        let data = {};
        try {
            data = await response.json();
        } catch (_error) {
            data = {};
        }

        if (response.status === 401 && !options.allowUnauthorized && generation === taskLoadGeneration) {
            showLogin();
        }

        return { ok: response.ok, status: response.status, data };
    } catch (_error) {
        return { ok: false, status: 0, data: { error: "网络请求失败，请检查本地服务是否正在运行。" } };
    }
}

function translateStatus(status) {
    const map = {
        scheduled: "计划中",
        pending: "等待中",
        queued: "排队中",
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
    return status === "scheduled" || status === "queued" || status === "pending" || status === "running" || status === "paused";
}

function canDelete(status) {
    return status !== "running" && status !== "paused";
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
    const terrainIDs = new Set(tilemaps.filter(isTerrariumSource).map((item) => String(item.id)));
    const retainedTerrain = Array.from(selectedSourceIds).filter((id) => terrainIDs.has(id));
    if (!group) {
        selectedSourceIds = new Set(retainedTerrain);
        return;
    }

    const providerIds = new Set(group.items.map((item) => String(item.id)));
    const existing = Array.from(selectedSourceIds).filter((id) => providerIds.has(id));
    if (existing.length === 0) {
        selectedSourceIds = new Set([...defaultSelectedSourceIds(group), ...retainedTerrain]);
        return;
    }

    selectedSourceIds = new Set([...existing, ...retainedTerrain]);
}

function syncSelectedSourceIds() {
    selectedSourceIds = new Set([
        ...Array.from(document.querySelectorAll(".terrain-addon__input:checked")).map((element) => String(element.value)),
        ...Array.from(document.querySelectorAll(".tilemap-source-option:checked")).map((element) => String(element.value))
    ]);
}

function selectAllProviderItems(providerName) {
    const provider = getProviderGroups().find((group) => group.id === providerName);
    if (!provider) {
        return;
    }
    const terrain = Array.from(selectedSourceIds).filter((id) => tilemaps.some((item) => String(item.id) === id && isTerrariumSource(item)));
    selectedSourceIds = new Set([...provider.items.map((item) => String(item.id)), ...terrain]);
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
            return ["img", "cia", "vec"].includes(tiandituLayer(item));
        })
        .map((item) => String(item.id));
}

"use strict";

(async function () {
  const status = document.getElementById("terrain-status");
  const errorBox = document.getElementById("terrain-error");
  const fitButton = document.getElementById("terrain-fit");
  const worldButton = document.getElementById("terrain-world");
  const colorButton = document.getElementById("terrain-color");
  const task = new URLSearchParams(location.search).get("task");
  const showError = (message) => { errorBox.hidden = false; errorBox.textContent = message; };
  try {
    if (!task) throw new Error("缺少任务编号，请从任务的地形预览入口打开。");
    if (!window.Cesium) throw new Error("本地 Cesium 资源缺失，请运行 npm ci 或重新安装完整发布包。");
    const response = await fetch(`/api/tasks/${encodeURIComponent(task)}/terrain-info`, { credentials: "same-origin" });
    if (response.status === 401) throw new Error("登录已过期，请返回任务页登录后重试。");
    const info = await response.json();
    if (!response.ok) throw new Error(info.error || "地形产物尚未发布。");
    Cesium.Ion.defaultAccessToken = "";
    const provider = await Cesium.CesiumTerrainProvider.fromUrl(info.url);
    const viewer = new Cesium.Viewer("terrain-viewer", {
      terrainProvider: provider, baseLayer: false, baseLayerPicker: false,
      geocoder: false, animation: false, timeline: false, homeButton: false,
      sceneModePicker: false, navigationHelpButton: false, fullscreenButton: false,
      infoBox: false, selectionIndicator: false
    });
    viewer.scene.globe.baseColor = Cesium.Color.fromCssColorString("#7d967d");
    viewer.scene.globe.enableLighting = false;
    const rampCanvas = document.createElement("canvas");
    rampCanvas.width = 256; rampCanvas.height = 1;
    const rampContext = rampCanvas.getContext("2d");
    const gradient = rampContext.createLinearGradient(0, 0, 256, 0);
    for (const [height, color] of [[-1000,"#173d75"],[0,"#2f8658"],[1500,"#a88b54"],[5000,"#f4f4ed"]]) gradient.addColorStop((height + 1000) / 6000, color);
    rampContext.fillStyle = gradient; rampContext.fillRect(0, 0, 256, 1);
    const elevationMaterial = Cesium.Material.fromType("ElevationRamp", { image: rampCanvas, minimumHeight: -1000, maximumHeight: 5000 });
    viewer.scene.globe.material = elevationMaterial;
    colorButton.addEventListener("click", () => {
      const enabled = colorButton.getAttribute("aria-pressed") !== "true";
      viewer.scene.globe.material = enabled ? elevationMaterial : undefined;
      colorButton.setAttribute("aria-pressed", String(enabled));
      colorButton.textContent = `高程设色：${enabled ? "开" : "关"}`;
      document.getElementById("elevation-legend").hidden = !enabled;
    });
    viewer.scene.screenSpaceCameraController.minimumZoomDistance = 30;
    const [west, south, east, north] = info.bounds;
    const fit = () => {
      const rectangle = Cesium.Rectangle.fromDegrees(west, south, east, north);
      const sphere = Cesium.BoundingSphere.fromRectangle3D(rectangle);
      viewer.camera.flyToBoundingSphere(sphere, {
        duration: 0, offset: new Cesium.HeadingPitchRange(0, -0.85, Math.max(3000, sphere.radius * 2.8))
      });
    };
    fitButton.addEventListener("click", fit);
    worldButton.addEventListener("click", () => viewer.camera.flyTo({ destination: Cesium.Cartesian3.fromDegrees(0, 20, 24000000), duration: 0 }));
    fitButton.disabled = worldButton.disabled = colorButton.disabled = false;
    document.getElementById("terrain-source").textContent = `${info.source?.attribution || "自定义高程源"} ${info.source?.verticalDatum || "源数据垂直基准未声明"}`;
    status.textContent = `已发布 ${info.available}/${info.expected} 个地形瓦片 · 拖动旋转，滚轮缩放，Ctrl + 拖动调整视角`;
    provider.errorEvent.addEventListener((error) => showError(`本地地形读取失败：${error.message}`));
    fit();
    // Expose the actual viewer for local browser acceptance and diagnostics.
    window.terrainPreview = { viewer, provider, info, ready: true };
  } catch (error) {
    status.textContent = "地形预览未就绪";
    showError(error.message);
  }
})();

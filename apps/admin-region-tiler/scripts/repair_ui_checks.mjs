import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { execFileSync } from "node:child_process";

export async function runRepairUIChecks(page) {
  const screenshots=resolve("tmp/repair-ui");
  await mkdir(screenshots,{recursive:true});
  await page.evaluate(()=>stopTaskPolling());
  const task={id:"ui-task",kind:"single",name:'长名称'.repeat(30)+'<img src=x onerror="window.injected=1">',status:"completed",artifactStatus:"ready",downloadUrl:"/api/tasks/ui-task/download",total:4,current:4,successCount:4,integrity:{status:"complete",available:4,expected:4},effectiveWorkers:3,effectiveTimeDelay:80};
  if (process.env.TILER_REPAIR_BASELINE) {
    const oldCSS=execFileSync("git",["show",`${process.env.TILER_REPAIR_BASELINE}:apps/admin-region-tiler/static/styles.css`],{encoding:"utf8"});
    await page.setViewportSize({width:390,height:1000});
    const style=await page.addStyleTag({content:oldCSS});
    const oldWidth=await page.evaluate(()=>document.documentElement.scrollWidth);
    await style.evaluate(node=>node.remove());
    assert(oldWidth>392,`baseline did not reproduce mobile overflow: ${oldWidth}`);
    console.log(`Baseline mobile defect reproduced: 390px viewport, ${oldWidth}px document`);
  }
  for(const width of [390,768,1440]) {
    await page.setViewportSize({width,height:1000});
    for(const mode of ["region","bbox","tasks"]) {
      await page.evaluate(({mode,task})=>{setTaskMode(mode);if(mode==="tasks"){cachedTasks=[task];renderTaskList(cachedTasks);}}, {mode,task});
      await page.waitForTimeout(120);
      const overflow=await page.evaluate(()=>({width:innerWidth,document:document.documentElement.scrollWidth,items:[...document.querySelectorAll("body *")].filter(el=>{const b=el.getBoundingClientRect();return b.width>0&&b.right>innerWidth+2&&getComputedStyle(el).position!=="absolute";}).slice(0,12).map(el=>({tag:el.tagName,cls:el.className,width:el.getBoundingClientRect().width}))}));
      assert(overflow.document<=width+2,`${width}px ${mode} overflows: ${JSON.stringify(overflow)}`);
      assert.equal(await page.evaluate(()=>window.injected),undefined,"task name executed script");
      await page.screenshot({path:resolve(screenshots,`${width}-${mode}.png`),fullPage:true});
      if(mode==="tasks") {
        await page.locator('[data-task-menu-toggle="menu-ui-task"]').click();
        const menu=await page.locator("#menu-ui-task").boundingBox();
        assert(menu.x>=0&&menu.x+menu.width<=width+2,"task menu outside viewport");
        await page.evaluate(()=>closeAllTaskMenus());
      }
    }
  }
  const feature=(x)=>({type:"Feature",properties:{},geometry:{type:"Polygon",coordinates:[[[x,1],[x+1,1],[x+1,2],[x,2],[x,1]]]}});
  await page.route("**/api/tasks/*/area*",async route=>{
    const first=route.request().url().includes("slow-a");
    if(first) await new Promise(r=>setTimeout(r,160));
    await route.fulfill({json:{type:"FeatureCollection",features:[feature(first?1:10),feature(first?3:12)],levels:[{index:0,minZoom:1,maxZoom:3},{index:1,minZoom:4,maxZoom:5}],selectedLevel:1}}).catch(()=>{});
  });
  const preview=await page.evaluate(async()=>{
    const before=JSON.stringify(rangeClickPoints);
    const a=previewTaskArea({id:"slow-a",name:"A",area:{mode:"region"}});
    const b=previewTaskArea({id:"fast-b",name:"B",area:{mode:"region"}});
    await Promise.all([a,b]);
    const features=taskAreaPreviewLayer.toGeoJSON().features.length;
    const west=taskAreaPreviewLayer.getBounds().getWest();
    setTaskMode("bbox");
    return {features,west,cleared:taskAreaPreviewLayer===null,selectionUnchanged:before===JSON.stringify(rangeClickPoints)};
  });
  assert.deepEqual(preview,{features:2,west:10,cleared:true,selectionUnchanged:true});
  let registrations=0;
  await page.route("**/api/tile-preview/tianditu-token",async route=>{
    const now=await page.evaluate(()=>Date.now());
    registrations++;
    await route.fulfill({json:{id:`preview-${registrations}`,expiresAt:new Date(now+15*60000).toISOString()}});
  });
  await page.clock.install();
  await page.evaluate(async()=>{resetPreviewCredentials();await Promise.all([ensureTiandituPreviewToken("fixture-token"),ensureTiandituPreviewToken("fixture-token")]);});
  assert.equal(registrations,1,"registration was not coalesced");
  await page.clock.fastForward(16*60000);
  await page.waitForFunction(()=>rangeTiandituPreviewTokenId==="preview-2");
  assert.equal(registrations,2,"preview did not renew after 16 minutes");
  await page.evaluate(async()=>{await recoverPreviewCredentials("fixture-token");await recoverPreviewCredentials("fixture-token");});
  assert.equal(registrations,3,"invalid preview recovery must run once");
  await page.evaluate(async()=>{resetPreviewCredentials();await ensureTiandituPreviewToken("other-account-token");});
  assert.equal(registrations,4,"account cache was reused");
  let active=0,maxActive=0,taskRequests=0;
  await page.route("**/api/tasks",async route=>{
    taskRequests++;active++;maxActive=Math.max(maxActive,active);
    await new Promise(r=>setTimeout(r,50));
    active--;await route.fulfill({json:[]});
  });
  await page.evaluate(async()=>{await Promise.all(Array.from({length:8},()=>loadTasks()));});
  assert.equal(taskRequests,1,"overlapping task refreshes were not coalesced");
  assert.equal(maxActive,1);
  await page.route("**/api/tasks", route=>route.fulfill({status:401,json:{error:"session expired"}}));
  await page.evaluate(()=>loadTasks());
  assert(await page.evaluate(()=>!authenticated&&taskPollingTimer===null&&rangeTiandituPreviewTokenId===""),"401 retained polling or preview cache");
  await page.unroute("**/api/tasks");
  let pollRequests=0;
  await page.route("**/api/tasks",route=>{pollRequests++;return route.fulfill({json:[]});});
  await page.locator('#loginForm [name="username"]').fill("admin");
  await page.locator('#loginForm [name="password"]').fill("adminmap");
  await page.locator('#loginForm button[type="submit"]').click();
  await page.waitForFunction(()=>authenticated&&taskPollingTimer!==null);
  await page.waitForFunction(()=>taskLoadPromise===null);
  pollRequests=0;
  await page.clock.fastForward(5000);
  await page.waitForFunction(()=>taskLoadPromise===null);
  assert.equal(pollRequests,1,"re-login created multiple pollers");
  await page.evaluate(()=>logout());
  assert(await page.evaluate(()=>!authenticated&&taskPollingTimer===null),"logout left polling active");
  console.log("Repair UI checks passed: 390/768/1440px, preview ordering, full geometry, expiry, account reset, 401/re-login, polling");
}

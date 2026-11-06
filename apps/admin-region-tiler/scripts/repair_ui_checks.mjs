import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { execFileSync } from "node:child_process";

export async function runRepairUIChecks(page) {
  const screenshots=resolve("tmp/repair-ui");
  await mkdir(screenshots,{recursive:true});
  await page.evaluate(()=>stopTaskPolling());
  await page.evaluate(()=>{document.getElementById("buildVersion").textContent="dev+123456789abc.dirty";});
  const missingState = await page.evaluate(() => {
    const item = missingRegionCatalog.find(item => item.id === "659011");
    const select = document.createElement("select");
    select.innerHTML = renderRegionOptions({options:[{...item,maintained:false}],selectedRegionId:""});
    return {code:item.reasonCode,disabled:select.options[0].disabled,text:select.textContent};
  });
  assert.equal(missingState.code,"boundary_attachment_required");
  assert(missingState.disabled && missingState.text.includes("附图"),"missing boundary reason or disable state missing");
  const task={id:"ui-task",kind:"single",name:'长名称'.repeat(30)+'<img src=x onerror="window.injected=1">',status:"completed",artifactStatus:"ready",downloadUrl:"/api/tasks/ui-task/download",total:4,current:4,successCount:4,integrity:{status:"complete",available:4,expected:4},effectiveWorkers:3,effectiveTimeDelay:80};
  await page.route("**/api/tasks/page?*",route=>route.fulfill({json:{items:[task],total:1,counts:{all:1,completed:1}}}));
  if (process.env.TILER_REPAIR_BASELINE) {
    const oldCSS=execFileSync("git",["show",`${process.env.TILER_REPAIR_BASELINE}:apps/admin-region-tiler/static/styles.css`],{encoding:"utf8"});
    await page.setViewportSize({width:390,height:1000});
    const style=await page.addStyleTag({content:oldCSS});
    const oldWidth=await page.evaluate(()=>document.documentElement.scrollWidth);
    await style.evaluate(node=>node.remove());
    assert(oldWidth>392,`baseline did not reproduce mobile overflow: ${oldWidth}`);
    console.log(`Baseline mobile defect reproduced: 390px viewport, ${oldWidth}px document`);
  }
  await runTaskPaginationChecks(page);
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
    if (document.getElementById("taskPreviewControls").getBoundingClientRect().height > 160) throw new Error("Task preview controls consumed the map's available height");
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
  await page.route("**/api/tasks/page?*",async route=>{
    taskRequests++;active++;maxActive=Math.max(maxActive,active);
    await new Promise(r=>setTimeout(r,50));
    active--;await route.fulfill({json:{items:[],total:0,counts:{all:0}}});
  });
  await page.evaluate(async()=>{await Promise.all(Array.from({length:8},()=>loadTasks()));});
  assert.equal(taskRequests,1,"overlapping task refreshes were not coalesced");
  assert.equal(maxActive,1);
  await page.route("**/api/tasks/page?*", route=>route.fulfill({status:401,json:{error:"session expired"}}));
  await page.evaluate(()=>loadTasks());
  assert(await page.evaluate(()=>!authenticated&&taskPollingTimer===null&&rangeTiandituPreviewTokenId===""),"401 retained polling or preview cache");
  await page.unroute("**/api/tasks/page?*");
  let pollRequests=0;
  await page.route("**/api/tasks/page?*",route=>{pollRequests++;return route.fulfill({json:{items:[],total:0,counts:{all:0}}});});
  await page.locator('#loginForm [name="username"]').fill("admin");
  await page.locator('#loginForm [name="password"]').fill("adminmap");
  await page.locator('#loginForm button[type="submit"]').click();
  await page.waitForFunction(()=>authenticated);
  await page.evaluate(()=>setTaskMode("tasks"));
  await page.waitForFunction(()=>taskPollingTimer!==null);
  await page.waitForFunction(()=>taskLoadPromise===null);
  pollRequests=0;
  await page.clock.fastForward(5000);
  await page.waitForFunction(()=>taskLoadPromise===null);
  assert.equal(pollRequests,1,"re-login created multiple pollers");
  await runPasswordUIChecks(page, screenshots);
  await page.evaluate(()=>logout());
  assert(await page.evaluate(()=>!authenticated&&taskPollingTimer===null),"logout left polling active");
  console.log("Repair UI checks passed: 390/768/1440px, preview ordering, full geometry, expiry, account reset, 401/re-login, polling");
}

async function runTaskPaginationChecks(page) {
  const tasks = Array.from({length:63},(_,index)=>({id:`history-${index}`,kind:"single",name:`History ${index}`,status:"completed",current:10,total:10,phase:{name:"completed",current:10,total:10}}));
  tasks[0] = {...tasks[0],kind:"group",status:"running",totalChildren:1,children:[{id:"history-child",kind:"child",name:"Source",sourceName:"Source",status:"running",total:10,current:7,phase:{name:"downloading",current:7,total:10}}]};
  let requests=0;
  const handler=async route=>{
    requests++;
    const params=new URL(route.request().url()).searchParams;
    const status=params.get("status");
    const filtered=status==="all"?tasks:tasks.filter(task=>task.status===status);
    const offset=Number(params.get("offset"));
    await route.fulfill({json:{items:filtered.slice(offset,offset+Number(params.get("limit"))),total:filtered.length,counts:{all:tasks.length,running:1,completed:tasks.length-1}}});
  };
  await page.route("**/api/tasks/page?*",handler);
  try {
    await page.evaluate(()=>{currentTaskFilter="all";taskPageOffset=0;setTaskMode("tasks");});
    await page.waitForFunction(()=>document.querySelectorAll("#taskList > .task-card").length===50);
    assert((await page.locator("#taskPageInfo").innerText()).includes("1/2"));
    assert.equal(await page.locator('[data-task-filter="all"] small').innerText(),"63");
    await page.evaluate(()=>{
      const group=document.querySelector('[data-group-task-id="history-0"]');
      group.open=true;expandedGroupTasks.add("history-0");
      const child=group.querySelector("details[data-child-task-id]");child.open=true;expandedChildTasks.add("history-child");
      selectedTaskIds.add("history-0");group.querySelector(".task-select").checked=true;
      document.getElementById("menu-history-0").classList.remove("is-hidden");
      group.querySelector('[data-task-action="pause"]').focus();
      window.historyStableCard=document.querySelector('[data-group-task-id="history-1"]');
      window.historyFocus=document.activeElement;
    });
    await page.evaluate(()=>loadTasks());
    assert(await page.evaluate(()=>document.activeElement===window.historyFocus),"unchanged refresh replaced the focused node");
    tasks[0].children[0].phase.current=8;
    tasks[0].children[0].current=8;
    await page.evaluate(()=>loadTasks());
    const state=await page.evaluate(()=>({
      same:document.querySelector('[data-group-task-id="history-1"]')===window.historyStableCard,
      selected:document.querySelector('[data-group-task-id="history-0"] .task-select').checked,
      focused:document.activeElement.dataset.taskAction,
      menu:!document.getElementById("menu-history-0").classList.contains("is-hidden"),
      expanded:document.querySelector('[data-group-task-id="history-0"]').open&&document.querySelector('[data-child-task-id="history-child"]').open,
      phase:document.querySelector('[data-group-task-id="history-0"] .task-phase').textContent
    }));
    assert(state.same&&state.selected&&state.focused==="pause"&&state.menu&&state.expanded,"refresh lost task interaction state");
    assert(state.phase.includes("下载瓦片")&&state.phase.includes("8/10"),"phase progress did not update");
    await page.locator("#taskPageNext").click();
    await page.waitForFunction(()=>document.querySelectorAll("#taskList > .task-card").length===13);
    assert((await page.locator("#taskPageInfo").innerText()).includes("2/2"));
    tasks.splice(49);
    await page.evaluate(()=>loadTasks());
    assert.equal(await page.locator("#taskList > .task-card").count(),49,"deleted last page did not return to the remaining page");
    assert((await page.locator("#taskPageInfo").innerText()).includes("1/1"));
    await page.locator('[data-task-filter="running"]').click();
    await page.waitForFunction(()=>document.querySelectorAll("#taskList > .task-card").length===1);
    assert.equal(await page.locator('[data-task-filter="all"] small').innerText(),"49","filter changed global statistics");
    await page.evaluate(()=>setTaskMode("region"));
    assert(await page.evaluate(()=>taskPollingTimer===null),"creation tab kept history polling active");
    const before=requests;
    await page.evaluate(()=>setTaskMode("tasks"));
    await page.waitForFunction(()=>taskLoadPromise===null);
    assert(requests>before,"returning to history did not refresh immediately");
    await page.evaluate(()=>{
      Object.defineProperty(document,"hidden",{configurable:true,get:()=>true});
      document.dispatchEvent(new Event("visibilitychange"));
    });
    assert(await page.evaluate(()=>taskPollingTimer===null),"hidden page kept polling active");
    const hiddenRequests=requests;
    await page.evaluate(()=>{delete document.hidden;document.dispatchEvent(new Event("visibilitychange"));});
    await page.waitForFunction(()=>taskLoadPromise===null&&taskPollingTimer!==null);
    assert(requests>hiddenRequests,"visible history did not refresh immediately");
  } finally {
    await page.unroute("**/api/tasks/page?*",handler);
    await page.evaluate(()=>{currentTaskFilter="all";taskPageOffset=0;stopTaskPolling();delete window.historyStableCard;delete window.historyFocus;});
  }
  console.log("Task history passed: paging, global counts, deletion, keyed updates, focus/menu/selection/expansion, phases and visible-only polling");
}

async function runPasswordUIChecks(page, screenshots) {
  for (const width of [390,768,1440]) {
    await page.setViewportSize({width,height:900});
    await page.locator("#accountMenuBtn").click();
    await page.locator("#openPasswordDialogBtn").click();
    const bounds=await page.locator("#passwordDialog").boundingBox();
    assert(bounds.x>=0&&bounds.x+bounds.width<=width,"password dialog outside viewport");
    await page.screenshot({path:resolve(screenshots,`${width}-password.png`)});
    await page.locator("#cancelPasswordBtn").click();
  }
  await page.locator("#accountMenuBtn").click();
  await page.locator("#openPasswordDialogBtn").click();
  await page.locator('[name="currentPassword"]').fill("wrong password");
  await page.locator('[name="newPassword"]').fill("test replacement password");
  await page.locator('[name="confirmPassword"]').fill("test replacement password");
  await page.locator("#savePasswordBtn").click();
  await page.waitForFunction(()=>document.getElementById("passwordError").textContent.includes("原密码错误"));
  await page.locator('[name="currentPassword"]').fill("adminmap");
  let requests=0;
  await page.route("**/api/auth/password",async route=>{
    requests++;
    await new Promise(resolve=>setTimeout(resolve,150));
    await route.continue();
  });
  await page.locator("#savePasswordBtn").click();
  assert(await page.locator("#savePasswordBtn").isDisabled());
  await page.evaluate(()=>document.getElementById("passwordForm").requestSubmit());
  await page.waitForFunction(()=>!authenticated&&!document.getElementById("passwordDialog").open);
  assert.equal(requests,1,"duplicate password request");
  assert(await page.evaluate(()=>taskPollingTimer===null&&rangeTiandituPreviewTokenId===""),"password change kept cache or poller");
  await page.locator('#loginForm [name="username"]').fill("admin");
  await page.locator('#loginForm [name="password"]').fill("test replacement password");
  await page.locator('#loginForm button[type="submit"]').click();
  await page.waitForFunction(()=>authenticated&&taskPollingTimer!==null);
  console.log("Password UI passed: validation, real change, duplicate prevention, logout, new login and 390/768/1440px dialog");
}

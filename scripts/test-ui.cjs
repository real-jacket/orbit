const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '../Resources/src-ui/index.html'), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
new vm.Script(script);
// Every sandbox gets t() (Chinese until a language is chosen), as the page does.
const i18nSource = fs.readFileSync(path.join(__dirname, '../Resources/src-ui/i18n/core.js'), 'utf8');
const createContext = vm.createContext;
vm.createContext = sandbox => { const c = createContext(sandbox); vm.runInContext(i18nSource, c); return c; };
const context = vm.createContext({
  STATE: { uninstallMode: 'standard', cleanGroups: [], appsList: [], orphanLeftoversList: [] },
  persistState() {}, updateSidebarBadges() {}, renderModule() {},
  absolutePath(value) { return value; }, showToast() {},
  pendingTrashRequests: new Map(), pendingRestoreRequests: new Map(),
});
context.window = context;
function load(name, target = context) {
  const start = script.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `${name} exists`);
  const next = script.indexOf('\nfunction ', start + 1);
  vm.runInContext(script.slice(start, next), target);
}
function loadConst(name, target = context) {
  const m = script.match(new RegExp(`^const ${name} = .*;$`, 'm'));
  assert.ok(m, `${name} exists`);
  vm.runInContext(m[0].replace(/^const /, 'var '), target);
}
function loadCallback(name, target = context) {
  const start = script.indexOf(`window.${name} = function(`);
  assert.ok(start >= 0, `${name} exists`);
  vm.runInContext(script.slice(start, script.indexOf('\n};', start) + 4), target);
}
['selectedSubpaths', 'selectedSizeGb', 'toggleSubpathCheck', 'toggleAppSubpathCheck', 'toggleOrphanSubpathCheck', 'normalizeComparePath', 'scanResultsUsable', 'buildCleanEntries'].forEach(name => load(name));

const app = { id: 'example', appPath: '/Applications/Example.app', checked: true, keepPersonal: true, subpaths: [
  { path: '/Applications/Example.app', kind: 'app', checked: false, sizeGb: 2 },
  { path: '~/Library/Caches/Example', kind: 'cache', checked: true, sizeGb: 0 },
  { path: '~/Library/Application Support/Example', kind: 'config', checked: true, sizeGb: 0.5 },
] };
context.app = app;
assert.equal(vm.runInContext('selectedSubpaths(app, "standard").length', context), 1);
assert.equal(vm.runInContext('selectedSizeGb(app, "standard")', context), 0, 'zero bytes never invents a fallback');
assert.equal(vm.runInContext('selectedSubpaths(app, "cache_only").length', context), 1);
app.keepPersonal = false;
assert.equal(vm.runInContext('selectedSizeGb(app, "complete")', context), 0.5);
assert.equal(vm.runInContext('selectedSubpaths(app, "complete").some(sp => sp.kind === "app")', context), false, 'unchecked app bundle stays out');

const group = { checked: true, sizeGb: 3, subpaths: [
  { path: '/tmp/one', checked: true, sizeGb: 0 },
  { path: '/tmp/two', checked: true, sizeGb: 3 },
] };
context.STATE.cleanGroups = [group];
context.group = group;
vm.runInContext('toggleSubpathCheck(0, 1)', context);
assert.equal(group.sizeGb, 3, 'selection never rewrites measured size');
assert.equal(vm.runInContext('selectedSizeGb(group)', context), 0);
vm.runInContext('toggleSubpathCheck(0, 0)', context);
assert.equal(group.checked, false, 'no subpaths means no parent selection');

context.STATE.appsList = [app];
app.subpaths.forEach(sp => { sp.checked = false; });
vm.runInContext('toggleAppSubpathCheck(0, 1)', context);
assert.equal(app.checked, true, 'selecting a path selects its parent');
vm.runInContext('toggleAppSubpathCheck(0, 1)', context);
assert.equal(app.checked, false);
context.STATE.orphanLeftoversList = [group];
vm.runInContext('toggleOrphanSubpathCheck(0, 1)', context);
assert.equal(group.checked, true);
vm.runInContext('toggleOrphanSubpathCheck(0, 1)', context);
assert.equal(group.checked, false, 'deselecting every leftover cannot fall back to all paths');

context.STATE.module = 'software';
context.STATE.softwarePanelMode = 'apps';
assert.equal(vm.runInContext('buildCleanEntries()', context), undefined, 'persisted scan results cannot authorize removal');
context.STATE.scanRunning = true;
assert.equal(vm.runInContext('scanResultsUsable()', context), false, 'a first scan in progress cannot authorize removal');
context.STATE.scanRefresh = true;
assert.equal(vm.runInContext('scanResultsUsable()', context), true, 'the rescan after a Trash receipt keeps receipt-updated results usable');
context.STATE.scanRunning = false;
context.STATE.scanRefresh = false;
context.STATE.scanCompleted = true;
context.pendingTrashRequests.set('in-flight', {});
assert.equal(vm.runInContext('buildCleanEntries()', context), undefined, 'in-flight removal blocks another preview');
context.pendingTrashRequests.clear();
app.subpaths[1].checked = true;
app.checked = true;
vm.runInContext('let confirmedClean = buildCleanEntries()', context);
assert.equal(vm.runInContext('confirmedClean.entries[0].paths.length', context), 1);
assert.equal(vm.runInContext('confirmedClean.entries[0].paths[0]', context), '~/Library/Caches/Example');
assert.equal(vm.runInContext('confirmedClean.entries[0].regKeys.includes(app.appPath)', context), false, 'cache-only handling cannot hide the installed app');
context.closeConfirmSheet = () => vm.runInContext('confirmedClean = null', context);
let sent = null;
context.submitTrash = entries => { sent = entries; };
// A background rescan or navigation cannot enlarge the approved path set.
app.subpaths[0].checked = true;
context.STATE.module = 'purge';
vm.runInContext('window.executeConfirmedClean()', context);
assert.equal(sent.length, 1);
assert.equal(sent[0].paths.length, 1);
assert.equal(sent[0].paths[0], '~/Library/Caches/Example');
sent = null;
vm.runInContext('window.executeConfirmedClean()', context);
assert.equal(sent, null, 'confirmation cannot be submitted twice');
context.STATE.module = 'software';
context.STATE.uninstallMode = 'cache_only';
app.subpaths[2].checked = true;
assert.equal(vm.runInContext('buildCleanEntries().entries[0].paths.length', context), 1, 'cache mode cannot include app/config paths');
context.STATE.softwarePanelMode = 'leftovers';
group.checked = true;
assert.equal(vm.runInContext('buildCleanEntries()', context), undefined, 'all unchecked leftover paths cannot reappear');
context.STATE.softwarePanelMode = 'apps';
context.STATE.uninstallMode = 'complete';
app.keepPersonal = false;
app.cacheSizeGb = 1;
app.configSizeGb = 0.5;
app.subpaths[0].checked = false;
app.subpaths[1].checked = false;
vm.runInContext('buildCleanEntries().entries[0].commit()', context);
assert.equal(context.STATE.appsList.length, 1, 'configuration-only removal keeps the app installed');
assert.equal(app.cacheSizeGb, 1, 'unselected cache size is preserved');
assert.equal(app.configSizeGb, 0);

// 系统加速：诊断 → 勾选 → 执行 → 回执 → 撤销。每一步只认自己的 requestId。
const optSent = [];
const optToasts = [];
context.sendNativeBridge = payload => optSent.push(payload);
context.nextRequestId = kind => kind + '-test-' + (optSent.length + 1);
context.showToast = text => optToasts.push(text);
context.setTimeout = () => 0;
loadConst('OPTIMIZE_RUN_LIMIT_MS');
['optimizeBusy', 'optimizeSelectedIds', 'requestOptimizePlan', 'toggleOptimizeSel', 'runOptimizeSelected', 'undoOptimizeTask'].forEach(name => load(name));
['onNativeOptimizePlan', 'onNativeOptimizeResult', 'onNativeOptimizeUndo'].forEach(name => loadCallback(name));
context.STATE.optimize = { status: 'idle', planId: '', error: '', items: [], sel: {}, receipts: {}, running: false, runId: '', undoing: {} };
const optState = () => context.STATE.optimize;
vm.runInContext('requestOptimizePlan()', context);
assert.equal(optSent[0].action, 'requestOptimizePlan');
assert.equal(optState().status, 'loading');
context.onNativeOptimizePlan({ requestId: 'other', items: [] });
assert.equal(optState().status, 'loading', 'a plan for another request is ignored');
const planItems = [
  { id: 'a', name: 'A', detail: 'd', status: 'needed', reason: 'r', tier: 'safe', undo: 'u', checked: true, runnable: true },
  { id: 'b', name: 'B', detail: 'd', status: 'needed', reason: 'r', tier: 'review', undo: 'u', checked: false, runnable: true },
  { id: 'c', name: 'C', detail: 'd', status: 'unavailable', reason: '需要管理员权限，orbit 不执行', tier: 'protected', undo: 'u', checked: false, runnable: false },
  { id: 'd', name: 'D', detail: 'd', status: 'clear', reason: 'r', tier: 'safe', undo: 'u', checked: false, runnable: false },
];
context.onNativeOptimizePlan({ requestId: optSent[0].requestId, items: planItems });
assert.equal(optState().status, 'ready');
assert.deepEqual(JSON.parse(JSON.stringify(optState().sel)), { a: true, b: false }, 'only runnable tasks get a checkbox, and only safe ones start checked');
vm.runInContext("toggleOptimizeSel('c')", context);
assert.equal(optState().sel.c, undefined, 'an unavailable task cannot be selected');
vm.runInContext("toggleOptimizeSel('b')", context);
assert.deepEqual(JSON.parse(vm.runInContext('JSON.stringify(optimizeSelectedIds())', context)), ['a', 'b']);
vm.runInContext('runOptimizeSelected()', context);
const runMsg = optSent[optSent.length - 1];
assert.equal(runMsg.action, 'executeOptimize');
assert.deepEqual(JSON.parse(JSON.stringify(runMsg.tasks)), ['a', 'b'], 'the page sends ids only');
vm.runInContext('runOptimizeSelected()', context);
assert.equal(optSent.filter(m => m.action === 'executeOptimize').length, 1, 'a run in progress cannot be started twice');
context.onNativeOptimizeResult({ requestId: 'other', receipt: { id: 'a', ok: true, outcome: 'applied' } });
assert.equal(optState().receipts.a, undefined, 'a receipt for another run is ignored');
context.onNativeOptimizeResult({ requestId: runMsg.requestId, receipt: { id: 'a', ok: true, outcome: 'applied', summary: 's', canUndo: true } });
context.onNativeOptimizeResult({ requestId: runMsg.requestId, receipt: { id: 'b', ok: false, outcome: 'failed', summary: 's', error: 'e' } });
assert.equal(optState().running, true, 'still running until the done marker arrives');
context.onNativeOptimizeResult({ requestId: runMsg.requestId, done: true, ok: true });
assert.equal(optState().running, false);
assert.match(optToasts[optToasts.length - 1], /1 项没有成功/, 'the toast counts the failure instead of claiming success');
assert.equal(optSent[optSent.length - 1].action, 'requestOptimizePlan', 'the plan is re-read after a run');
context.onNativeOptimizePlan({ requestId: optSent[optSent.length - 1].requestId, items: planItems });
vm.runInContext("undoOptimizeTask('a')", context);
const undoMsg = optSent[optSent.length - 1];
assert.deepEqual(JSON.parse(JSON.stringify(undoMsg)), { action: 'undoOptimize', requestId: undoMsg.requestId, task: 'a' });
context.onNativeOptimizeUndo({ requestId: 'other', id: 'a', ok: true, receipt: { ok: true } });
assert.ok(optState().undoing.a, 'an undo receipt for another request is ignored');
context.onNativeOptimizeUndo({ requestId: undoMsg.requestId, id: 'a', ok: true, receipt: { id: 'a', ok: true, summary: 'done' } });
assert.equal(optState().receipts.a.isUndo, true);
assert.equal(optState().undoing.a, undefined);
delete context.sendNativeBridge; delete context.setTimeout;
context.showToast = () => {};

const elements = new Map();
context.document = { getElementById(id) {
  if (!elements.has(id)) elements.set(id, { style: {}, disabled: true, textContent: '' });
  return elements.get(id);
} };
context.clearTimeout = () => {};
context.renderDoctorChecksList = () => {};
context.startDoctorReDiagnosis = () => { throw new Error('receipt must not issue another diagnosis'); };
context.STATE.recheckCount = 0;
context._doctorWaitingForReport = true;
context._doctorRequestId = 'doc-1';
loadCallback('onNativeDoctorReport');
context.onNativeDoctorReport({ requestId: 'old', checks: [{ passed: true }] });
assert.equal(context._doctorWaitingForReport, true, 'stale diagnosis cannot finish the current request');
context.onNativeDoctorReport({ requestId: 'doc-1', checks: [{ passed: false }] });
assert.equal(context._doctorWaitingForReport, false);
assert.equal(context.STATE.doctorChecks[0].passed, false);
assert.equal(elements.get('btnDoctorReRun').disabled, false);

function luminance(hex) {
  const channels = hex.match(/[0-9a-f]{2}/gi).map(value => parseInt(value, 16) / 255);
  return rgbLuminance(channels);
}
function rgbLuminance(channels) {
  const linear = channels.map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
  return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722;
}
function tokens(block) {
  return Object.fromEntries([...block.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6})/gi)].map(match => [match[1], match[2]]));
}
const health = vm.createContext({
  STATE: { totalDiskGb: 100, freeDiskGb: 50, memUsedGb: 95, memTotalGb: 100, power: { thermal: 'nominal', memPressure: 'normal' } },
});
vm.runInContext(script.slice(script.indexOf('const THERMAL_TEXT ='), script.indexOf('const PROTECTED_PROCS =')), health);
['hasNum', 'powerInfo', 'thermalInfo', 'pressureInfo', 'memPercent', 'diskUsedPercent', 'healthScoreNow'].forEach(name => load(name, health));
assert.equal(vm.runInContext('healthScoreNow()', health), 100, 'high memory occupancy with normal pressure is not a health failure');
health.STATE.power.memPressure = 'warn';
assert.equal(vm.runInContext('healthScoreNow()', health), 94, 'native memory pressure still affects the reference score');
health.STATE.power = {};
health.STATE.totalDiskGb = null;
assert.equal(vm.runInContext('healthScoreNow()', health), null, 'memory occupancy alone cannot fabricate a health reference');
const dark = tokens(html.match(/:root\s*\{([^}]+)\}/)[1]);
const light = { ...dark, ...tokens(html.match(/html\[data-theme="light"\]\s*\{([^}]+)\}/)[1]) };
for (const theme of [dark, light]) {
  for (const background of ['accent-primary', 'accent-primary-hover', 'accent-rose']) {
    const values = [luminance(theme[background]), luminance(theme['accent-ink'])].sort((a, b) => a - b);
    assert.ok((values[1] + 0.05) / (values[0] + 0.05) >= 4.5, `${background} button passes AA`);
  }
}
const rootBlocks = html.match(/:root\s*\{([^}]+)\}/)[1];
const lightBlocks = html.match(/html\[data-theme="light"\]\s*\{([^}]+)\}/)[1];
for (const [theme, block] of [[dark, rootBlocks], [light, lightBlocks]]) {
  for (const signal of ['emerald', 'amber', 'rose']) {
    const soft = block.match(new RegExp(`--accent-${signal}-soft:\\s*rgba\\(([^)]+)\\)`))[1].split(',').map(Number);
    const base = theme['bg-card'].match(/[0-9a-f]{2}/gi).map(value => parseInt(value, 16));
    const mixed = base.map((value, i) => (value * (1 - soft[3]) + soft[i] * soft[3]) / 255);
    const background = rgbLuminance(mixed), text = luminance(theme[`accent-${signal}`]);
    assert.ok((Math.max(background, text) + 0.05) / (Math.min(background, text) + 0.05) >= 4.5, `${signal} badge passes AA on its actual tinted background`);
  }
}
load('matchesSearch');
load('handleSearch');
context.STATE.appsList = [
  { name: 'Alpha', appPath: '/Applications/Alpha.app', checked: true },
  { name: 'Editor', appPath: '/Applications/My Editor.app', checked: false },
];
context.STATE.module = 'software';
context.STATE.softwarePanelMode = 'apps';
context.STATE.selectedAppIdx = 0;
vm.runInContext('handleSearch("my editor")', context);
assert.equal(context.STATE.selectedAppIdx, 1, 'inspector follows the visible search match');
assert.equal(context.STATE.appsList[0].checked, true, 'search does not change authorization');
assert.equal(context.STATE.appsList[1].checked, false);
context.STATE.searchQuery = '8123';
assert.equal(vm.runInContext('matchesSearch({ pid: 8123 })', context), true);

const receipts = vm.createContext({
  STATE: { module: 'clean', uninstallMode: 'standard', scanCompleted: true, homePath: '/Users/qa', collected: [], historyLogs: [], deletedPathsRegistry: [] },
  pendingTrashRequests: new Map(), pendingRestoreRequests: new Map(), absolutePath: value => value, displayPath: value => value,
  persistState() {}, updateSidebarBadges() {}, renderModule() {}, renderMenuBarTrayContent() {},
  scheduleRefresh() {}, reconcileAfterScan() {}, showToast() {}, renderRuntimeBanner() {},
  document: { getElementById() { return { classList: { contains() { return false; } } }; } },
});
receipts.window = receipts;
['hasNum', 'fmtDiskSizeGb', 'selectedSubpaths', 'selectedSizeGb', 'normalizeComparePath', 'scanResultsUsable', 'markSubpathsMoved', 'markItemFailed', 'scanRowsWithFailures', 'buildRestoreData', 'buildCleanEntries', 'servicesHasContent', 'resolveTrash', 'busyTrash', 'rowMoving', 'movingAttr', 'lockStageForTrash', 'trashBusyCopy', 'pruneCleanedRows', 'unpruneSubpaths', 'unpruneRows', 'restoreSnapshotEntities'].forEach(name => load(name, receipts));
['rowKey', 'prunedRows', 'PRUNE_SELECTED_KEY', 'NATIVE_RESULT_TIMEOUT_MS'].forEach(name => loadConst(name, receipts));
receipts.recomputeDiskAnalyzeTotals = () => {};
const partialGroup = { id: 'cache', name: 'Cache', path: '/tmp/cache', tier: 'safe', checked: true, sizeGb: 3, subpaths: [
  { path: '/tmp/cache/one', sizeGb: 1, checked: true },
  { path: '/tmp/cache/two', sizeGb: 2, checked: false },
] };
receipts.STATE.cleanGroups = [partialGroup];
vm.runInContext('buildCleanEntries().entries[0].commit(["/tmp/cache/one"])', receipts);
assert.equal(partialGroup.sizeGb, 2, 'moving one selected subpath preserves the measured remainder');
assert.equal(partialGroup.cleaned, false, 'a partially selected category cannot claim fully cleaned');
partialGroup.subpaths.forEach(sp => { sp.checked = true; sp.cleaned = false; });
partialGroup.checked = true;
partialGroup.sizeGb = 3;
receipts.closeConfirmSheet = () => vm.runInContext('confirmedClean = null', receipts);
receipts.submitTrash = (entries, options) => receipts.pendingTrashRequests.set('partial', { entries, options, requested: 2 });
vm.runInContext('let confirmedClean = buildCleanEntries(); window.executeConfirmedClean()', receipts);
receipts.resolveTrash({ requestId: 'partial', moved: [{ original: '/tmp/cache/one', trashed: '/trash/one' }], failed: [{ path: '/tmp/cache/two', error: 'Permission denied' }], movedCount: 1 });
assert.equal(receipts.STATE.historyLogs.length, 1, 'every actual move has a restoration receipt, even on partial failure');
assert.equal(receipts.STATE.historyLogs[0].restoreData.paths.join(','), '/tmp/cache/one');
assert.equal(receipts.STATE.historyLogs[0].restoreData.trashedPaths['/tmp/cache/one'], '/trash/one', 'history retains the exact Trash target, not only the original basename');
assert.equal(receipts.STATE.historyLogs[0].sizeGb, 1, 'partial receipt uses only known moved-path bytes');
assert.equal(partialGroup.subpaths.length, 1, 'the moved subpath leaves the row at once');
assert.equal(partialGroup.subpaths[0].cleaned, false, 'the failed one stays selectable');
assert.equal(partialGroup.failed, true);
assert.equal(partialGroup.sizeGb, 2);
receipts.rows = [{ id: 'cache', path: '/tmp/cache', sizeGb: 2 }, { id: 'other', path: '/tmp/other', sizeGb: 4 }];
receipts.previous = [partialGroup];
const rescanned = vm.runInContext('scanRowsWithFailures(rows, previous)', receipts);
assert.equal(rescanned[0].failedReason, 'Permission denied', 'fresh scan keeps the previous failed-path reason');
assert.equal(rescanned[0].sizeGb, 2, 'failure preservation cannot replace the new measurement');
assert.equal(rescanned[1].failed, undefined, 'other paths never inherit failure');
receipts.rows = [{ id: 'installer_0', path: '/tmp/new.dmg' }, { id: 'installer_1', path: '/tmp/old.dmg' }];
receipts.previous = [{ id: 'installer_0', path: '/tmp/old.dmg', failed: true, failedReason: 'Permission denied' }];
const reordered = vm.runInContext('scanRowsWithFailures(rows, previous)', receipts);
assert.equal(reordered[0].failed, undefined, 'an index-based id cannot transfer an old error to a different path');
assert.equal(reordered[1].failedReason, 'Permission denied', 'reordered entries retain errors by their stable path');
receipts.pendingTrashRequests.set('late', { entries: [{ paths: ['/tmp/late'], sizeGb: 1 }], options: {}, requested: 1 });
receipts.resolveTrash({ requestId: 'late', timedOut: true });
assert.equal(receipts.pendingTrashRequests.has('late'), true, 'unknown timeout does not discard an in-flight removal');
receipts.resolveTrash({ requestId: 'late', moved: [{ original: '/tmp/late', trashed: '/trash/late' }], movedCount: 1 });
assert.equal(receipts.pendingTrashRequests.has('late'), false, 'late receipt still settles the request');


// --- 移入废纸篓：等待回执时本模块真锁定，回执一到同步从列表移除、重算合计 ---------------------------
{
  const R = receipts, run = code => vm.runInContext(code, R);
  const sub = (path, sizeGb, extra) => ({ path, sizeGb, checked: true, ...extra });
  const row = (id, subs, extra) => ({ id, name: id, path: '/t/' + id, tier: 'safe', checked: true, sizeGb: subs.reduce((n, x) => n + x.sizeGb, 0), subpaths: subs, ...extra });
  // 提交：登记一次待回执请求（与 submitTrash 同样记录 module / keys / sizeGb），返回 plan 方便结算。
  const submit = (module, id) => {
    R.STATE.module = module;
    const plan = run(`buildCleanEntries('${module}')`);
    assert.ok(plan && plan.entries.length, `${module} has entries to submit`);
    const requested = plan.entries.reduce((n, e) => n + e.paths.length, 0);
    R.pendingTrashRequests.set(id, { entries: plan.entries, options: {}, requested, module, keys: new Set(plan.entries.map(e => R.rowKey(e.entity))), sizeGb: plan.entries.reduce((n, e) => n + e.sizeGb, 0) });
    return plan;
  };
  const mv = (...paths) => paths.map(original => ({ original, trashed: '/trash' + original }));
  const ids = list => list.map(x => x.id || x.path || x.name).join(',');
  const fakeStage = () => {
    const el = () => ({ disabled: false, tabIndex: 0, attrs: {}, setAttribute(k, v) { this.attrs[k] = v; } });
    const buttons = [el(), el()], inputs = [el()], tabs = [el(), el()], right = { innerHTML: '<b>1.00 GB</b>' };
    return { dataset: {}, buttons, inputs, tabs, right, querySelectorAll(sel) { return sel.includes('[data-moving]') ? [{ querySelector: () => right }] : sel.includes('seg-tab') ? tabs : buttons.concat(inputs); } };
  };

  // Deep Clean：等待中锁定；回执后整行消失、部分行只剩没移走的子路径、失败行保留红色原因。
  R.STATE.scanCompleted = true;
  R.STATE.cleanGroups = [
    row('a', [sub('/t/a/1', 1)]),
    row('b', [sub('/t/b/1', 2)]),
    row('c', [sub('/t/c/1', 1), sub('/t/c/2', 3)]),
    row('d', [sub('/t/d/1', 5)], { checked: false }),
  ];
  R.STATE.selectedCleanIdx = 2;
  submit('clean', 'dc');
  assert.ok(run('busyTrash()'), 'the module that submitted is busy');
  assert.equal(run('rowMoving(STATE.cleanGroups[0])'), true, 'a submitted row is marked moving');
  assert.equal(run('rowMoving(STATE.cleanGroups[3])'), false, 'an unselected row is not moving');
  assert.match(run('movingAttr(STATE.cleanGroups[0])'), /data-moving aria-busy="true"/);
  R.STATE.module = 'software';
  assert.equal(run('busyTrash()'), null, 'other modules stay navigable');
  R.STATE.module = 'clean';
  const stage = fakeStage();
  R.stageArg = stage; run('lockStageForTrash(stageArg)');
  assert.ok(stage.buttons.concat(stage.inputs).every(x => x.disabled), 'buttons and checkboxes are really disabled while pending');
  assert.ok(stage.tabs.every(x => x.attrs['aria-disabled'] === 'true' && x.tabIndex === -1), 'tabs are aria-disabled and leave the tab order');
  assert.match(stage.right.innerHTML, /正在移入废纸篓…/, 'moving rows say what is happening instead of the size');
  assert.ok('trashBusy' in stage.dataset, 'the stage is marked busy');
  const copy = run('trashBusyCopy(busyTrash())');
  assert.match(copy.headline, /正在移入废纸篓：4 个路径/);
  assert.equal(copy.statValue, '7.00 GB', 'hero shows the batch being moved');
  R.pendingTrashRequests.get('dc').timedOut = true;
  const slow = run('trashBusyCopy(busyTrash())');
  assert.match(slow.badge, /等待系统回执/);
  assert.match(slow.desc, /45 秒/, 'the timeout state explains the wait');
  assert.match(slow.desc, /授权弹窗/);
  assert.equal(R.pendingTrashRequests.has('dc'), true, 'timeout keeps waiting for the real receipt');
  R.resolveTrash({ requestId: 'dc', moved: mv('/t/a/1', '/t/c/1'), failed: [{ path: '/t/b/1', error: 'Permission denied' }], movedCount: 2 });
  assert.equal(R.pendingTrashRequests.size, 0, 'receipt clears the lock');
  const cg = R.STATE.cleanGroups;
  assert.equal(ids(cg), 'b,c,d', 'the fully moved row is removed from the list at once');
  assert.equal(cg[1].subpaths.map(x => x.path).join(), '/t/c/2', 'a partially moved row drops only the moved subpath');
  assert.equal(cg[1].sizeGb, 3, 'its total is recomputed from what is left');
  assert.equal(cg[0].failed, true, 'a failed row stays');
  assert.equal(cg[0].failedReason, 'Permission denied', 'with its red reason');
  assert.equal(cg[0].subpaths.length, 1, 'and keeps its subpaths');
  assert.equal(R.STATE.selectedCleanIdx, 1, 'the inspector stays on the same row after the list shrinks');
  assert.equal(cg.filter(g => g.checked).reduce((n, g) => n + R.selectedSizeGb(g), 0), 5, 'selected total counts only what remains');
  // 立即还原：整行放回原位。
  R.restoreSnapshotEntities({ restoreData: { type: 'clean', ids: ['a', 'c'], paths: ['/t/a/1', '/t/c/1'] } });
  assert.equal(ids(R.STATE.cleanGroups), 'b,c,d,a', 'restoring puts a pruned row back');
  assert.equal(R.STATE.cleanGroups.find(g => g.id === 'c').subpaths.length, 2, 'and a pruned subpath');
  assert.equal(R.STATE.cleanGroups.find(g => g.id === 'a').sizeGb, 1);

  // AI 清理。
  R.STATE.aiMoonBands = [row('m1', [sub('/t/m1/1', 2)]), row('m2', [sub('/t/m2/1', 1)])];
  R.STATE.selectedMoonIdx = 1;
  submit('ai_moon', 'ai');
  R.resolveTrash({ requestId: 'ai', moved: mv('/t/m1/1'), failed: [{ path: '/t/m2/1', error: 'Operation not permitted' }], movedCount: 1 });
  assert.equal(ids(R.STATE.aiMoonBands), 'm2', 'AI cleanup row is removed, failed one stays');
  assert.equal(R.STATE.aiMoonBands[0].failedReason, 'Operation not permitted');
  assert.equal(R.STATE.selectedMoonIdx, 0);

  // 卸载：应用行消失；仅清缓存时应用留下、缓存体积与子路径同步减少；回执卡片仍渲染。
  const appRow = (id, subs, extra) => ({ id, name: id, appPath: `/Applications/${id}.app`, checked: true, version: '1', sizeGb: subs.reduce((n, x) => n + x.sizeGb, 0), appSizeGb: 2, cacheSizeGb: 1, configSizeGb: 0.5, leftoverSizeGb: 1.5, subpaths: subs, ...extra });
  const appSubs = id => [sub(`/Applications/${id}.app`, 2, { kind: 'app' }), sub(`/Library/Caches/${id}`, 1, { kind: 'cache' }), sub(`/Library/Preferences/${id}`, 0.5, { kind: 'config' })];
  R.STATE.softwarePanelMode = 'apps';
  R.STATE.uninstallMode = 'complete';
  R.STATE.appsList = [appRow('Gone', appSubs('Gone')), appRow('Stay', appSubs('Stay'))];
  R.STATE.appsList[1].subpaths.forEach(x => { x.checked = x.kind === 'cache'; });
  R.STATE.appsList[1].subpaths[0].checked = false;
  R.STATE.appsList[0].subpaths.forEach(x => { x.checked = true; });
  R.STATE.selectedAppIdx = 1;
  // 走真正的 executeConfirmedClean，回执卡片（服务 / 配置引用）由它的 onDone 写入。
  R.STATE.module = 'software';
  R.submitTrash = (entries, options) => R.pendingTrashRequests.set('sw', { entries, options, requested: 4, module: 'software', keys: new Set(entries.map(e => R.rowKey(e.entity))), sizeGb: 0 });
  run("confirmedClean = buildCleanEntries('software'); window.executeConfirmedClean()");
  assert.equal(R.pendingTrashRequests.get('sw').entries.length, 2);
  const services = { stopped: [{ label: 'com.gone.helper' }], casks: [], failed: [], reloaded: [] };
  R.resolveTrash({ requestId: 'sw', moved: mv('/Applications/Gone.app', '/Library/Caches/Gone', '/Library/Preferences/Gone', '/Library/Caches/Stay'), failed: [], movedCount: 4, services, configRefs: [{ file: '~/.zshrc', line: 3 }] });
  assert.equal(ids(R.STATE.appsList), 'Stay', 'the uninstalled app disappears from the list at once');
  const stay = R.STATE.appsList[0];
  assert.equal(stay.cacheSizeGb, 0, 'cache bytes leave the kept app');
  assert.equal(stay.subpaths.map(x => x.kind).join(), 'app,config', 'the moved cache subpath is gone from the inspector');
  assert.equal(R.STATE.selectedAppIdx, 0);
  assert.equal(vm.runInContext('servicesReceipt', R) && vm.runInContext('servicesHasContent(servicesReceipt)', R), 1, 'services receipt is kept for the uninstall card');
  assert.equal(vm.runInContext('configRefsReceipt.refs.length', R), 1, 'config references are kept for their card');
  // 卸载残留。
  R.STATE.softwarePanelMode = 'leftovers';
  R.STATE.orphanLeftoversList = [row('o1', [sub('/t/o1/1', 1)]), row('o2', [sub('/t/o2/1', 1), sub('/t/o2/2', 2)]), row('o3', [sub('/t/o3/1', 1)])];
  R.STATE.selectedOrphanIdx = 2;
  submit('software', 'or');
  R.resolveTrash({ requestId: 'or', moved: mv('/t/o1/1', '/t/o2/1'), failed: [{ path: '/t/o3/1', error: 'Permission denied' }], movedCount: 2 });
  assert.equal(ids(R.STATE.orphanLeftoversList), 'o2,o3', 'moved leftovers leave the list, partial ones keep the rest');
  assert.equal(R.STATE.orphanLeftoversList[0].sizeGb, 2);
  assert.equal(R.STATE.orphanLeftoversList[1].failedReason, 'Permission denied');
  assert.equal(R.STATE.selectedOrphanIdx, 1);

  // 构建清理：目录与安装包。
  R.STATE.purgeItems = [{ id: 'p0', project: 'web', artifact: 'node_modules', path: '/t/web/node_modules', sizeGb: 2, checked: true, tier: 'safe' }, { id: 'p1', project: 'api', artifact: 'target', path: '/t/api/target', sizeGb: 4, checked: true, tier: 'safe' }];
  R.STATE.installerFiles = [{ id: 'i0', name: 'x.dmg', path: '/t/x.dmg', sizeGb: 1, checked: true, tier: 'safe' }, { id: 'i1', name: 'y.pkg', path: '/t/y.pkg', sizeGb: 1, checked: true, tier: 'safe' }];
  R.STATE.selectedPurgeIdx = 1;
  submit('purge', 'pg');
  R.resolveTrash({ requestId: 'pg', moved: mv('/t/web/node_modules', '/t/x.dmg'), failed: [{ path: '/t/api/target', error: 'In use' }, { path: '/t/y.pkg', error: 'In use' }], movedCount: 2 });
  assert.equal(ids(R.STATE.purgeItems), 'p1', 'moved build artifact leaves the list');
  assert.equal(R.STATE.purgeItems[0].failedReason, 'In use', 'failed artifact keeps its reason');
  assert.equal(R.STATE.selectedPurgeIdx, 0);
  assert.equal(ids(R.STATE.installerFiles), 'i1', 'moved installer leaves the list');
  assert.equal(R.STATE.installerFiles[0].failedReason, 'In use');
  assert.equal(R.STATE.purgeItems.filter(p => p.checked).reduce((n, p) => n + p.sizeGb, 0) + R.STATE.installerFiles.filter(i => i.checked).reduce((n, i) => n + i.sizeGb, 0), 5, 'purge total counts what is left');
  assert.equal(run('prunedRows.length') > 0, true, 'pruned rows are remembered for an immediate restore');
}

receipts.pendingRestoreRequests = new Map();
receipts.restoreSnapshotEntities = () => {};
['firstFailureText', 'resolveRestore'].forEach(name => load(name, receipts));
const restore = { id: 'restore', restoreData: { paths: ['/tmp/one', '/tmp/two'] } };
receipts.STATE.historyLogs = [restore];
receipts.pendingRestoreRequests.set('restore-1', { snap: restore, paths: restore.restoreData.paths.slice() });
receipts.resolveRestore({ requestId: 'restore-1', restored: ['/tmp/one'], failed: [{ path: '/tmp/two', error: 'Permission denied' }] });
assert.equal(restore.restoreData.paths.join(','), '/tmp/two', 'retry restores only the remaining paths');
receipts.pendingRestoreRequests.set('restore-2', { snap: restore, paths: restore.restoreData.paths.slice() });
receipts.resolveRestore({ requestId: 'restore-2', timedOut: true });
assert.equal(receipts.pendingRestoreRequests.has('restore-2'), true);
receipts.resolveRestore({ requestId: 'restore-2', restored: ['/tmp/two'], failed: [] });
assert.equal(receipts.STATE.historyLogs.length, 0, 'successful retry can finish a partially restored record');

// --- 破坏性操作后的刷新：本地结算、单次兜底重扫、迟到扫描不复活已移走路径 ---------------------------
{
  const timers = [];
  const rescans = [];
  const refresh = vm.createContext({
    STATE: { scanRunning: false, homePath: '/Users/qa', module: 'clean', cleanGroups: [{ id: 'c', path: '/tmp/c', sizeGb: 3, checked: true, tier: 'safe', subpaths: [
      { path: '/tmp/a', sizeGb: 1, checked: true }, { path: '/tmp/b', sizeGb: 2, checked: true }] }] },
    renderModule() {}, requestDiskAnalyze() {}, sendNativeBridge() {},
    triggerNativeRescan(quiet) { rescans.push(quiet); },
    setTimeout(fn) { timers.push(fn); return timers.length; }, clearTimeout(id) { if (id) timers[id - 1] = null; },
  });
  refresh.window = refresh;
  ['hasNum', 'selectedSubpaths', 'normalizeComparePath', 'scanResultsUsable', 'markSubpathsMoved', 'endRefreshing', 'armRefreshFallback', 'scheduleRefresh', 'noteNativeScanStarted', 'reconcileAfterScan', 'dropMovedFromLists', 'pruneCleanedRows'].forEach(name => load(name, refresh));
  vm.runInContext('var recentMoves = new Map(), REFRESH_DEBOUNCE_MS = 2500, refreshing = false, refreshTimer = 0, refreshGuard = 0, scanStartedAt = 0, lastMoveAt = 0', refresh);
  const fire = () => timers.splice(0).forEach(fn => fn && fn());
  vm.runInContext('scheduleRefresh(["/tmp/a"]); scheduleRefresh(["/tmp/b"])', refresh);
  assert.equal(vm.runInContext('refreshing', refresh), true, 'a move enters the quiet refreshing state');
  fire();
  assert.equal(rescans.length, 1, 'back-to-back operations cause exactly one fallback rescan');
  assert.equal(rescans[0], true, 'the fallback rescan is quiet');
  // Go 的扫描晚于最后一次移动开始：页面兜底作废，不会二次重扫。
  rescans.length = 0;
  vm.runInContext('scheduleRefresh(["/tmp/x"]); scanStartedAt = lastMoveAt + 1; noteNativeScanStarted()', refresh);
  fire();
  assert.equal(rescans.length, 0, 'a native scan that started after the move owns the refresh');
  // 扫描在移动之前开始、之后才回来：陈旧结果里的已移走路径要再剔掉一次。
  vm.runInContext('recentMoves.clear(); scanStartedAt = 0; scheduleRefresh(["/tmp/a"])', refresh);
  vm.runInContext('STATE.cleanGroups = [{ id: "c", path: "/tmp/c", sizeGb: 3, checked: true, tier: "safe", subpaths: [{ path: "/tmp/a", sizeGb: 1, checked: true }, { path: "/tmp/b", sizeGb: 2, checked: true }] }]; reconcileAfterScan()', refresh);
  const ghost = vm.runInContext('STATE.cleanGroups[0]', refresh);
  assert.equal(ghost.subpaths.map(sp => sp.path).join(), '/tmp/b', 'a late scan cannot resurrect a path moved after it began');
  assert.equal(ghost.sizeGb, 2, 'remaining size is recomputed from what is left');
  vm.runInContext('STATE.cleanGroups = [{ id: "z", path: "/tmp/z", sizeGb: 1, checked: true, tier: "safe", subpaths: [{ path: "/tmp/a", sizeGb: 1, checked: true }] }]; reconcileAfterScan()', refresh);
  assert.equal(vm.runInContext('STATE.cleanGroups.length', refresh), 0, 'a stale row whose every path was moved is removed, not left as a cleaned shell');
  assert.equal(vm.runInContext('refreshing', refresh), true, 'still refreshing until a scan that started after the last move returns');
  // 开始于最后一次移动之后的扫描是权威的：结果原样采用，刷新态结束。
  vm.runInContext('scanStartedAt = lastMoveAt + 1; STATE.cleanGroups = [{ id: "c", path: "/tmp/c", sizeGb: 3, checked: true, tier: "safe", subpaths: [{ path: "/tmp/a", sizeGb: 1, checked: true }] }]; reconcileAfterScan()', refresh);
  assert.equal(vm.runInContext('STATE.cleanGroups[0].subpaths[0].cleaned', refresh), undefined, 'a fresh scan is authoritative');
  assert.equal(vm.runInContext('refreshing', refresh), false, 'refreshing ends with the authoritative scan');
}

// --- 分级汇总、≥ 下限、按 kind 分组的测量告警 ---------------------------------------------------------
{
  const view = vm.createContext({ STATE: { scanCompleted: true, scanReadWarnings: [] }, UI_ICON: () => '' });
  view.window = view;
  loadConst('WARNING_KIND_ORDER', view);
  ['hasNum', 'fmtDiskSizeGb', 'tierSummary', 'warningKind', 'warningSummary', 'warningHeadline', 'warningBadge'].forEach(name => load(name, view));
  vm.runInContext(script.slice(script.indexOf('const fmtPartialGb'), script.indexOf('const spSizeText')), view);
  assert.equal(vm.runInContext('fmtPartialGb(1.5, true)', view), '≥ 1.50 GB', 'a partial size is a lower bound');
  assert.equal(vm.runInContext('fmtPartialGb(0, true)', view), '未读取', 'nothing read shows no invented number');
  assert.equal(vm.runInContext('fmtPartialGb(1.5, false)', view), '1.50 GB');
  vm.runInContext('var TIERS = ["safe", "review", "protected"]', view);
  view.rows = [
    { tier: 'safe', checked: true, sizeGb: 3, subpaths: [{ path: '/a', sizeGb: 1, tier: 'safe', partial: true }, { path: '/b', sizeGb: 2 }] },
    { tier: 'review', sizeGb: 4, subpaths: [{ path: '/c', sizeGb: 4, tier: 'review' }] },
    { tier: 'protected', sizeGb: 5, subpaths: [{ path: '/d', sizeGb: 5, tier: 'safe' }] },
    { tier: 'review', sizeGb: 9, cleaned: true, subpaths: [] },
    { sizeGb: 7 },
  ];
  const sum = vm.runInContext('tierSummary(rows)', view);
  assert.deepEqual([sum.safe.gb, sum.safe.n, sum.safe.partial], [3, 2, true], 'safe sums payload sizes per subpath and flags partial');
  assert.deepEqual([sum.review.gb, sum.protected.gb], [4, 5], 'a protected row keeps all its subpaths protected');
  assert.equal(sum.review.n + sum.protected.n + sum.safe.n, 4, 'cleaned and untiered rows are not classified');
  const only = kind => { view.STATE.scanReadWarnings = [{ path: '/p', reason: 'r', kind }, { path: '/q', reason: 'r', kind }]; return vm.runInContext('warningSummary()', view); };
  assert.equal(vm.runInContext('warningHeadline(warningSummary())', view), '', 'no warnings, no headline');
  assert.match(vm.runInContext('warningHeadline(arg)', Object.assign(view, { arg: only('limit') }) && view), /权限正常；2 个较大目录只测量了部分体积（不影响清理）/);
  assert.equal(vm.runInContext('warningHeadline(arg)', Object.assign(view, { arg: only('system') }) && view), '', 'system locations never reach the headline');
  assert.equal(only('system').actionable, 0, 'system is not actionable');
  assert.match(vm.runInContext('warningHeadline(arg)', Object.assign(view, { arg: only('permission') }) && view), /2 个路径读取被拒绝，请按路径核对权限/);
  assert.equal(only('weird').legacy, 2, 'unknown kinds fall back to the legacy summary');
  assert.equal(vm.runInContext('warningHeadline(arg)', Object.assign(view, { arg: only(undefined) }) && view), '部分路径未完成测量，请先查看原因', 'payloads without kind keep the old headline');
}

receipts.recomputeDiskAnalyzeTotals = () => {};
receipts.STATE.appsList = [];
receipts.STATE.orphanLeftoversList = [];
receipts.STATE.whitelist = ['old-protection'];
receipts.STATE.deletedPathsRegistry = ['cache', '/tmp/cache'];
load('applyAppSort', receipts);
loadCallback('onNativeRealScanComplete', receipts);
receipts.onNativeRealScanComplete({ cleanGroups: null, aiMoonBands: null, appsList: null, orphanLeftoversList: null, purgeItems: null, installerFiles: null, largeFiles: null, whitelist: null });
for (const key of ['cleanGroups', 'aiMoonBands', 'appsList', 'orphanLeftoversList', 'purgeItems', 'installerFiles', 'largeFiles', 'whitelist']) {
  assert.equal(receipts.STATE[key].length, 0, `native null ${key} replaces stale results`);
}
receipts.STATE.deletedPathsRegistry = ['cache', '/tmp/cache'];
receipts.onNativeRealScanComplete({ cleanGroups: [{ ...partialGroup, sizeGb: 3, cleaned: false }] });
assert.equal(receipts.STATE.cleanGroups[0].sizeGb, 3, 'fresh measurements override the old deletion registry');
assert.equal(receipts.STATE.cleanGroups[0].cleaned, false);
receipts.STATE.softwarePanelMode = 'leftovers';
receipts.STATE.uninstallMode = 'cache_only';
receipts.frozenAppPlan = { module: 'software', mode: 'standard', panelMode: 'apps' };
receipts.movedApp = { entity: { id: 'example', appPath: '/Applications/Example.app' }, paths: ['/Applications/Example.app'] };
const frozenRestore = vm.runInContext('buildRestoreData(frozenAppPlan, [movedApp], movedApp.paths)', receipts);
assert.equal(frozenRestore.type, 'software', 'navigation during execution cannot reclassify the restoration record');
assert.equal(frozenRestore.uninstallMode, 'standard', 'restore metadata uses the approved mode, not the current one');
load('escHtml');
load('jsq');
const unusualPath = "/tmp/Owner's &quot;cache\nfile\\name";
context.unusualPath = unusualPath;
const encodedPath = vm.runInContext('jsq(unusualPath)', context);
const decodedPath = encodedPath.replace(/&(amp|lt|gt|quot|#39);/g, (_, entity) => ({ amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'" }[entity]));
assert.equal(vm.runInContext("'" + decodedPath + "'", context), unusualPath, 'HTML handler quoting preserves quotes, entities, newlines and backslashes');

['toggleMoonCheck', 'toggleMoonSubpathCheck'].forEach(name => load(name));
context.STATE.module = 'ai_moon';
context.STATE.aiMoonBands = [{ id: 'mixed', name: 'Mixed', path: '/tmp/mixed', tier: 'review', checked: false, sizeGb: 3, subpaths: [
  { path: '/tmp/protected', sizeGb: 2, tier: 'protected', checked: false },
  { path: '/tmp/allowed', sizeGb: 1, tier: 'review', checked: false },
] }];
vm.runInContext('toggleMoonCheck(0); toggleMoonSubpathCheck(0,0)', context);
assert.equal(context.STATE.aiMoonBands[0].subpaths[0].checked, false, 'group selection never authorizes protected AI data');
assert.equal(vm.runInContext('buildCleanEntries().entries[0].paths.join(",")', context), '/tmp/allowed', 'execution uses the same protected-subpath filter as the preview');
context.STATE.aiMoonBands[0].subpaths[0].checked = true;
assert.equal(vm.runInContext('selectedSizeGb(STATE.aiMoonBands[0])', context), 1, 'a corrupted check flag cannot count protected bytes');

const analysis = vm.createContext({
  STATE: { homePath: '/Users/me', analyzeRequestId: 'root-1', analyzeLoading: true, treemap2D: [], drillStack: [], analyzeLargeFiles: [],
    analyzeAggregated: null, searchQuery: '', analyzeSort: 'size', collected: [{ path: '~/draft' }, { path: '~/failed', failed: true }],
    totalDiskGb: 100, freeDiskGb: 50 },
  document: { getElementById() { return { style: {} }; }, querySelector() { return { clientWidth: 720, clientHeight: 300 }; } },
  PROTECTED_PATHS: new Set(['~', '/']),
  showToast() {}, renderModule() {}, updateSidebarBadges() {}, renderAnalyzeProgress() {}, console,
});
analysis.window = analysis;
['hasNum', 'escHtml', 'displayPath', 'diskGbOf', 'fmtDiskSizeGb', 'uiLocale', 'fmtModified', 'normalizeComparePath', 'recomputeDiskAnalyzeTotals', 'findTileByPath',
  'currentTiles', 'currentAggregated', 'analyzeScopeTotal', 'matchesSearch', 'analysisSelectedSmallItem', 'analysisVisibleTiles', 'restoreAnalysisSelection', 'currentLargeFiles', 'scanRowsWithFailures',
  'normalizeAggregated', 'squarifyRects', 'foldedTreemapTile', 'layoutTreemap', 'treemapPreviewHTML', 'absolutePath', 'permissionPaneForPath',
  'warningKind', 'groupRestrictedPaths', 'protectionReason'].forEach(name => load(name, analysis));
loadConst('WARNING_KIND_ORDER', analysis);
loadCallback('onNativeDiskAnalyze', analysis);
loadCallback('onNativeAnalyzeProgress', analysis);
analysis.onNativeDiskAnalyze({ requestId: 'root-1', root: '~', scanStatus: 'ok', totalGb: 8, totalSizeGb: 8, totalLogicalGb: 10, fileCount: 23,
  aggregated: { count: 3, sizeGb: 1, items: [{ name: 'archive.zip', path: '~/Downloads/archive.zip', sizeGb: 0.8, isDir: false, modified: 1700000000 },
    { name: 'notes', path: '~/Downloads/notes', sizeGb: 0.2, isDir: true, fileCount: 4 }] },
  largeFiles: [{ path: '~/Projects/a.bin', sizeGb: 2 }, { path: '~/Projects-other/b.bin', sizeGb: 1 }],
  restricted: [{ path: '~/Downloads/private', reason: 'denied' }],
  treemap2D: [{ name: 'Projects', path: '~/Projects', isDir: true, diskGb: 5, sizeGb: 5, children: [] }, { name: 'Downloads', path: '~/Downloads', diskGb: 2, sizeGb: 2, children: [] }],
});
assert.equal(analysis.STATE.totalAnalyzedGb, 8);
assert.equal(analysis.STATE.treemap2D[0].pct, 62.5, 'percentages include the backend aggregation, not only visible tiles');
assert.equal(analysis.STATE.collected.length, 1, 'refresh preserves failed receipt paths, not unconfirmed draft selections');
assert.equal(analysis.STATE.analyzeAggregated.items.length, 2, 'aggregated receipts retain a bounded inspectable sample');
analysis.STATE.searchQuery = 'archive';
let searchRows = vm.runInContext('analysisVisibleTiles()', analysis);
assert.equal(searchRows.length, 1, 'search can reach an item folded into the aggregate');
assert.equal(searchRows[0].path, '~/Downloads/archive.zip');
assert.equal(searchRows[0].fromOther, true);
analysis.STATE.searchQuery = '';
analysis.STATE.analyzeSort = 'name';
assert.deepEqual(Array.from(vm.runInContext('analysisVisibleTiles().map(x => x.name)', analysis)), ['Downloads', 'Projects']);
analysis.STATE.analyzeSort = 'size';
['pushTrail', 'goBack', 'goForward', 'setAnalyzeSort', 'drillTile', 'enterAnalyzeTile', 'drillUp', 'jumpToDepth', 'resetAnalysisScope'].forEach(name => load(name, analysis));
analysis.STATE.analyzeSel = 0;
analysis.STATE.treemap2D[0].childrenLoaded = true;
analysis.STATE.treemap2D[0].children = [{ name: 'Canvas', path: '~/Projects/Canvas', diskGb: 2, isDir: true, childrenLoaded: true }];
vm.runInContext('setAnalyzeSort("name")', analysis);
assert.equal(vm.runInContext('analysisVisibleTiles()[STATE.analyzeSel].name', analysis), 'Projects', 'sorting preserves the inspected path');
analysis.STATE.searchQuery = 'Projects';
analysis.STATE.analyzeSel = 0;
vm.runInContext('drillTile(0)', analysis);
assert.equal(analysis.STATE.searchQuery, '', 'a parent filter does not hide a child directory');
assert.equal(vm.runInContext('analysisVisibleTiles()[0].name', analysis), 'Canvas');
vm.runInContext('drillUp()', analysis);
assert.equal(vm.runInContext('analysisVisibleTiles()[STATE.analyzeSel].name', analysis), 'Projects', 'going up selects the sorted directory just left');
vm.runInContext('goBack()', analysis);
assert.equal(analysis.STATE.drillStack[0].name, 'Projects');
analysis.STATE.searchQuery = 'no match';
vm.runInContext('goForward()', analysis);
assert.equal(analysis.STATE.drillStack.length, 0);
assert.equal(analysis.STATE.searchQuery, '', 'history navigation resets the old scope filter');
analysis.STATE.analyzeSort = 'size';
analysis.STATE.analyzeSel = 0;
assert.match(vm.runInContext('fmtModified(1700000000)', analysis), /\d{2}\/\d{2}/, 'modification time is rendered from the captured timestamp');
const graph = vm.runInContext('layoutTreemap(STATE.treemap2D, STATE.analyzeAggregated)', analysis);
const others = graph.find(t => t.__others);
assert.ok(others && Math.abs(others.w * others.h / 100 - 12.5) < 0.02, 'aggregated bytes keep their actual share of treemap area');
assert.equal(vm.runInContext('layoutTreemap([{sizeGb:0}], null).length', analysis), 0, 'zero allocation cannot invent a visible minimum area');
const slimGraph = vm.runInContext('layoutTreemap([{name:"pkg",path:"~/go/pkg",sizeGb:94.9},{name:"bin",path:"~/go/bin",sizeGb:5.1}], null)', analysis);
assert.equal(slimGraph.length, 2);
assert.equal(slimGraph[1].name, 'bin', 'a few slim siblings keep their path instead of becoming synthetic Others');
assert.ok(Math.abs(slimGraph[1].w * slimGraph[1].h / 100 - 5.1) < 0.02, 'slim siblings retain their exact share, without a minimum-area boost');
analysis.slimGraph = slimGraph;
assert.ok(!slimGraph.some(t => t.__others), 'slim siblings are not folded, so the folded tab has nothing to list');
assert.equal(slimGraph[1].path, '~/go/bin', 'the slim tile keeps its original path');
const tinyGraph = vm.runInContext('layoutTreemap([{name:"main",path:"~/main",sizeGb:10},{name:"tiny",path:"~/tiny",sizeGb:0.000001}],null)', analysis);
analysis.tinyGraph = tinyGraph;
assert.equal(tinyGraph.length, 2, 'extremely small siblings keep an identity even when they cannot carry a map label');
assert.ok(!tinyGraph.some(t => t.__others) && tinyGraph[1].path === '~/tiny', 'a tiny sibling keeps its own path instead of folding');
analysis.denseTiles = [{ name: 'Main', path: '~/Main', diskGb: 90, isDir: true }, ...Array.from({ length: 20 }, (_, i) => ({ name: 'small-' + i, path: '~/small-' + i, diskGb: 0.2, isDir: true }))];
let denseGraph = vm.runInContext('layoutTreemap(denseTiles, null)', analysis);
let denseOther = denseGraph.find(tile => tile.__others);
assert.ok(denseOther && denseOther.items.length > 0, 'front-end pixel folding retains inspectable identities without a backend aggregate');
assert.equal(denseOther.knownCount, denseOther.items.length);
assert.equal(denseOther.count, denseOther.items.length);
assert.ok(denseOther.items.every(tile => tile.path.startsWith('~/small-')));
analysis.mixedAggregate = { count: 30, sizeGb: 2, items: [{ name: 'backend.bin', path: '~/backend.bin', sizeGb: 1, isDir: false }] };
denseGraph = vm.runInContext('layoutTreemap(denseTiles, mixedAggregate)', analysis);
denseOther = denseGraph.find(tile => tile.__others);
assert.equal(denseOther.count, denseOther.knownCount + 30, 'folded count includes known nodes and backend children exactly once');
assert.equal(denseOther.sampleCount, 1);
assert.equal(denseOther.unreported, 29);
assert.equal(denseOther.items.at(-1).path, '~/backend.bin');
assert.ok(Math.abs(denseOther.w * denseOther.h / 100 - denseOther.sizeGb / 96 * 100) < 0.03, 'mixed folding preserves the sum represented by its area');
assert.equal(denseOther.items.length, denseOther.knownCount + denseOther.sampleCount, 'the folded tile lists its folded and sampled identities exactly once');
assert.equal(denseOther.unreported, 29, 'the folded group does not pretend bounded samples are a complete directory');
analysis.STATE.analyzeMapFolded = denseOther;
analysis.STATE.collected = [];
analysis.clearInterval = () => {};
['isCollected', 'cancelTrash', 'toggleCollectedObject', 'toggleOtherCollect'].forEach(name => load(name, analysis));
vm.runInContext('toggleOtherCollect(0)', analysis);
assert.equal(analysis.STATE.collected[0].path, denseOther.items[0].path, 'folded panel indices resolve the displayed node, not a backend sample index');
assert.equal(analysis.STATE.collected[0].sizeGb, denseOther.items[0].diskGb, 'collection retains allocated size even when sizeGb is absent');
analysis.previewTile = {
  name: 'Projects', path: '~/Projects', w: 52, h: 72,
  preview: [
    { name: 'Canvas', path: '~/Projects/Canvas', diskGb: 5, isDir: true },
    { name: 'archive.bin', path: '~/Projects/archive.bin', diskGb: 3, isDir: false },
  ]
};
const previewHTML = vm.runInContext('treemapPreviewHTML(previewTile)', analysis);
assert.equal((previewHTML.match(/tt-preview-cell/g) || []).length, 2, 'large directory tiles carry a nested child map');
assert.ok(previewHTML.includes('tt-preview-label') && previewHTML.includes('Canvas'), 'large nested cells keep readable child labels');
analysis.previewTile.w = 4;
assert.equal(vm.runInContext('treemapPreviewHTML(previewTile)', analysis), '', 'tiny tiles skip unreadable nested geometry');
analysis.restrictedFixture = [
  { path: '/Users/me/Library/Containers/com.a', reason: '权限不足' },
  { path: '/Users/me/Library/Containers/com.b', reason: '权限不足' },
  { path: '/Users/me/Documents/private', reason: '需要文件访问权限' },
];
const groups = vm.runInContext('groupRestrictedPaths(restrictedFixture)', analysis);
assert.equal(groups.length, 2, 'restricted paths are grouped by the directory to inspect');
assert.equal(groups[0].label, '~/Library/Containers');
assert.equal(groups[0].count, 2);
assert.equal(groups[0].pane, 'fullDiskAccess');
assert.equal(groups[1].pane, 'filesAndFolders', 'documents permissions point at Files and Folders');
analysis.limitWarning = [{ path: '~/Projects/Example/target', reason: '达到文件测量上限；只计入已读取部分' }];
assert.equal(vm.runInContext('groupRestrictedPaths(limitWarning)[0].pane', analysis), '', 'a file budget limit is not mislabeled as a missing OS permission');
analysis.timeoutWarning = [{ path: '~/Library/Containers/com.example', kind: 'timeout', reason: '目录读取超时，可能正在等待 macOS 授权；未计入本次扫描' }];
assert.equal(vm.runInContext('groupRestrictedPaths(timeoutWarning)[0].pane', analysis), '', 'a timeout that mentions authorization cannot claim Full Disk Access is missing');
analysis.projectDenial = [{ path: '~/Projects/Example/build', kind: 'permission', reason: 'open: permission denied' }];
assert.equal(vm.runInContext('groupRestrictedPaths(projectDenial)[0].pane', analysis), '', 'an ordinary project access-control failure cannot prescribe Full Disk Access');
assert.equal(vm.runInContext('groupRestrictedPaths(projectDenial)[0].label', analysis), '~/Projects/Example/build', 'one denied path must show the exact location, not only its parent');
assert.equal(vm.runInContext('groupRestrictedPaths(projectDenial)[0].path', analysis), '/Users/me/Projects/Example/build', 'revealing a single warning opens its exact path');
analysis.STATE.purgeRoots = { custom: true, paths: ['~/Work'], roots: [{ path: '~/Work', status: 'readable' }, { path: '~/old', status: 'missing' }], warnings: [] };
analysis.STATE.scanReadWarnings = [
  { path: '~/Library/Containers/com.example', kind: 'permission', reason: 'operation not permitted', modules: ['software'] },
  { path: '~/Work/Example/target', kind: 'limit', reason: '达到文件测量上限', modules: ['purge'] },
];
load('scanWarningsForModule', analysis);
assert.equal(vm.runInContext('scanWarningsForModule("purge").length', analysis), 1, 'build cleanup only shows warnings for its actual scan scope');
assert.equal(vm.runInContext('scanWarningsForModule("purge")[0].kind', analysis), 'limit', 'an unrelated app denial cannot contaminate the build scan');
analysis.STATE.scanReadWarnings = [{ path: '~/Library/Containers/com.example', kind: 'permission', reason: 'operation not permitted' }];
assert.equal(vm.runInContext('scanWarningsForModule("purge").length', analysis), 0, 'legacy app warnings also stay outside a readable custom build scope');
analysis.STATE.scanReadWarnings = [{ path: '~/Projects/Example/target', kind: 'limit', reason: '达到文件测量上限' }];
assert.equal(vm.runInContext('scanWarningsForModule("purge").length', analysis), 0, 'custom build paths cannot silently inherit default project roots');
analysis.STATE.scanReadWarnings = [{ path: '~/Downloads/Example.dmg', kind: 'permission', reason: 'operation not permitted' }];
assert.equal(vm.runInContext('scanWarningsForModule("purge").length', analysis), 1, 'independently scanned installers retain their own reading warning');
analysis.STATE.scanReadWarnings = [];
assert.equal(vm.runInContext('scanWarningsForModule("purge").length', analysis), 0, 'missing build roots do not become permission warnings');
assert.equal(vm.runInContext('fmtDiskSizeGb(0.000004096)', analysis), '4.1 KB', 'small real allocations never display as fictional GB');
assert.equal(vm.runInContext('fmtDiskSizeGb(0.04, 1)', analysis), '40.0 MB', 'one-decimal labels do not round a nonzero allocation to zero GB');
assert.equal(vm.runInContext('fmtDiskSizeGb(1, 1)', analysis), '1.0 GB', 'gigabyte allocations retain their display unit');
load('scanResultsUsable', analysis);
load('appCacheSummary', analysis);
analysis.STATE.scanCompleted = true;
assert.equal(vm.runInContext('appCacheSummary({subpaths:[]}).state', analysis), '未发现已识别缓存', 'missing recognized paths are not proof of no app cache');
assert.equal(vm.runInContext('appCacheSummary({subpaths:[{kind:"cache",sizeGb:0.04,tier:"protected",reason:"正在运行"}]}).state', analysis), '安全保留');
assert.equal(vm.runInContext('appCacheSummary({subpaths:[{kind:"cache",sizeGb:null,partial:true}]}).size', analysis), '未读取', 'unread cache bytes do not turn into zero');
assert.equal(vm.runInContext('appCacheSummary({cachePartial:true,subpaths:[]}).state', analysis), '测量未完成', 'failed cache discovery is different from no known cache');
assert.equal(vm.runInContext('appCacheSummary({cachePartial:true,subpaths:[]}).size', analysis), '未读取', 'failed discovery never claims a zero-byte cache');
assert.equal(vm.runInContext('appCacheSummary({cachePartial:true,subpaths:[{kind:"cache",sizeGb:0.04}]}).state', analysis), '测量未完成', 'a known cache cannot hide another unread discovery root');
['svgSparkline', 'statusTrend', 'fmtBytesPerSec', 'numOrNull', 'fmtNum'].forEach(name => load(name, analysis));
assert.equal(vm.runInContext('statusTrend([], "#d97757", "CPU")', analysis), '', 'missing measurements have no invented chart');
assert.ok(!vm.runInContext('statusTrend([24], "#d97757", "CPU")', analysis).includes('<svg'), 'one measurement cannot invent a history');
assert.ok(vm.runInContext('statusTrend([0,100], "#d97757", "CPU")', analysis).includes('100%'));
const thermalTrend = vm.runInContext('statusTrend([52,61], "#d97757", "温度", "°C")', analysis);
assert.ok(thermalTrend.includes('70°C</span>') && !thermalTrend.includes('100%</span>'), 'temperature uses degrees, not a percentage scale');
const idleTrend = vm.runInContext('statusTrend([0,0], "#d97757", "下载", "B/s")', analysis);
assert.ok(idleTrend.includes('<svg') && !/NaN|Infinity/.test(idleTrend), 'measured idle throughput has a valid zero line');
analysis.siblingPath = analysis.STATE.homePath + '-other/file';
assert.equal(vm.runInContext('displayPath(siblingPath)', analysis), analysis.siblingPath);
analysis.STATE.analyzeRequestId = 'child-2';
analysis.STATE.analyzeLoading = true;
analysis.onNativeDiskAnalyze({ requestId: 'root-1', totalGb: 100, treemap2D: [] });
assert.equal(analysis.STATE.analyzeLoading, true, 'an old root receipt cannot end the new request');
assert.equal(analysis.STATE.totalAnalyzedGb, 8);
analysis.onNativeAnalyzeProgress({ requestId: 'root-1', files: 9999 });
assert.equal(analysis.STATE.analyzeProgress, null, 'old scan progress is ignored');
analysis.STATE.drillStack = [analysis.STATE.treemap2D[0]];
analysis.onNativeDiskAnalyze({ requestId: 'child-2', path: '~/Projects', scanStatus: 'ok', totalSizeGb: 3, totalLogicalGb: 4, fileCount: 6,
  treemap2D: [], largeFiles: [{ path: '~/Projects/c.bin', sizeGb: 1, protectionReason: 'runtime protected' }], restricted: [], aggregated: { count: 6, sizeGb: 3 } });
assert.equal(analysis.STATE.analyzeAllocGb, 8, 'drill facts cannot replace the root snapshot');
assert.equal(analysis.STATE.drillStack[0].diskGb, 3, 'the current directory gets its own totals');
assert.equal(analysis.STATE.drillStack[0].files, 6);
assert.equal(vm.runInContext('currentLargeFiles().length', analysis), 1);
assert.equal(vm.runInContext('protectionReason(currentLargeFiles()[0])', analysis), 'runtime protected');
assert.equal(analysis.STATE.analyzeRestricted.length, 1, 'a complete child does not erase a root restriction');
analysis.STATE.analyzeRequestId = 'folded-3';
analysis.STATE.drillStack = [{ name: 'notes', path: '~/Downloads/notes', sizeGb: 0.2, fromOther: true }];
analysis.onNativeDiskAnalyze({ requestId: 'folded-3', path: '~/Downloads/notes', scanStatus: 'ok', totalSizeGb: 0.2,
  treemap2D: [{ name: 'note.txt', path: '~/Downloads/notes/note.txt', diskGb: 0.2, isDir: false }] });
assert.equal(analysis.STATE.drillStack[0].children[0].name, 'note.txt', 'a folded search result receives its requested children even outside the main tile array');
analysis.STATE.analyzeRequestId = 'tiny-new-root';
analysis.STATE.analyzePath = '~/tiny';
analysis.STATE.analyzeRequestChildPath = '~/tiny';
analysis.STATE.drillStack = [{name:'parent',path:'~/parent',children:[]},{name:'tiny',path:'~/tiny',children:[]}];
analysis.onNativeDiskAnalyze({requestId:'tiny-new-root',root:'~/tiny',scanStatus:'ok',totalSizeGb:0.000004096,treemap2D:[{name:'small.txt',path:'~/tiny/small.txt',diskGb:0.000004096,isDir:false}]});
assert.equal(analysis.STATE.drillStack.length, 2, 'a new native scan root must not discard the requested child navigation');
assert.equal(analysis.STATE.drillStack.at(-1).children[0].name, 'small.txt', 'a result without path still belongs to the requested small directory');
const rootPathsBefore = analysis.STATE.treemap2D.map(tile => tile.path).join('|');
const rootTotalBefore = analysis.STATE.totalAnalyzedGb;
analysis.STATE.analyzePath = '';
analysis.STATE.drillStack = [];
analysis.onNativeDiskAnalyze({requestId:'tiny-new-root',root:'~/tiny',scanStatus:'ok',totalSizeGb:0.000004096,treemap2D:[{name:'small.txt',path:'~/tiny/small.txt',diskGb:0.000004096,isDir:false}]});
assert.equal(analysis.STATE.treemap2D.map(tile => tile.path).join('|'), rootPathsBefore, 'navigation changes cannot reclassify a late result as the root');
assert.equal(analysis.STATE.totalAnalyzedGb, rootTotalBefore, 'a late child receipt cannot replace the root size');

const quits = vm.createContext({
  STATE: { appsList: [], processes: [{ pid: 8123, name: 'Canvas', path: '/Applications/Canvas.app/Contents/MacOS/Canvas' }] },
  PROTECTED_PROCS: new Set(['Finder']), pendingProcessQuits: new Map(),
  requestSeq: 0, sent: [], nextRequestId: () => 'quit-' + ++quits.requestSeq,
  sendNativeBridge: payload => quits.sent.push(payload), showToast() {}, renderModule() {}, renderMenuBarTrayContent() {},
});
quits.window = quits;
load('killProcessByPid', quits);
vm.runInContext('killProcessByPid(8123)', quits);
assert.equal(quits.STATE.processes.length, 1, 'requesting termination does not optimistically erase a real process');
assert.equal(quits.sent[0].path, quits.STATE.processes[0].path, 'native termination carries the identity the user confirmed');
quits.onNativeProcessQuitResult({ requestId: 'other', pid: 8123, ok: true });
assert.equal(quits.pendingProcessQuits.size, 1, 'an unrelated native receipt cannot settle a process request');
quits.onNativeProcessQuitResult({ requestId: quits.sent[0].requestId, pid: 8123, ok: false, error: 'identity changed' });
assert.equal(quits.STATE.processes[0].quitError, 'identity changed');
assert.equal(quits.STATE.processes[0].quitRequested, false);
vm.runInContext('killProcessByPid(8123)', quits);
quits.onNativeProcessQuitResult({ requestId: quits.sent[1].requestId, pid: 8123, ok: true });
assert.equal(quits.STATE.processes.length, 1, 'SIGTERM acknowledgement is not proof that the process exited');
assert.equal(quits.STATE.processes[0].quitRequested, true);
quits.setTimeout = () => 0;
quits.clearTimeout = () => {};
quits.pendingProcessQuits.clear();
quits.STATE.processes[0].quitRequested = false;
load('armKill', quits);
vm.runInContext('armKill(8123)', quits);
const quitCount = quits.sent.length;
assert.equal(quits.STATE.killArmedIdentity.path, '/Applications/Canvas.app/Contents/MacOS/Canvas', 'the first confirmation records the executable identity');
quits.STATE.processes[0].path = '/Applications/Other.app/Contents/MacOS/Canvas';
vm.runInContext('armKill(8123)', quits);
assert.equal(quits.sent.length, quitCount, 'a reused PID or changed executable cannot inherit an armed confirmation');
assert.equal(quits.STATE.killArmed, null, 'a mismatched process identity cancels the confirmation');
quits.STATE.processes[0].path = 'Canvas';
vm.runInContext('armKill(8123); killProcessByPid(8123)', quits);
assert.equal(quits.sent.length, quitCount, 'a command name alone cannot authorize termination');
assert.equal(quits.STATE.killArmed, null, 'a missing executable path cannot arm confirmation');
const saved = vm.createContext({});
load('dropSavedFailures', saved);
const savedState = { cleanGroups: [{ failed: true, failedReason: 'old' }], aiMoonBands: [], appsList: [{ failed: true, failedReason: 'macOS 拒绝移动应用', needsAdmin: true }, null],
  orphanLeftoversList: [], largeFiles: [], purgeItems: [], installerFiles: [] };
saved.dropSavedFailures(savedState);
assert.equal(savedState.appsList[0].failed, false, 'a failure saved by an earlier run does not survive a relaunch');
assert.equal(savedState.cleanGroups[0].failedReason, '');
assert.equal(savedState.appsList[0].needsAdmin, true, 'only the failure marks are dropped');
console.log('UI regression checks passed: frozen selections, protected AI paths, scoped scan receipts, real treemap areas and dual-theme contrast; inline JavaScript parses.');

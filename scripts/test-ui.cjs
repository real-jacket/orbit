const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const html = fs.readFileSync(path.join(__dirname, '../Resources/src-ui/index.html'), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
new vm.Script(script);
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
function loadCallback(name, target = context) {
  const start = script.indexOf(`window.${name} = function(`);
  assert.ok(start >= 0, `${name} exists`);
  vm.runInContext(script.slice(start, script.indexOf('\n};', start) + 4), target);
}
['selectedSubpaths', 'selectedSizeGb', 'toggleSubpathCheck', 'toggleAppSubpathCheck', 'toggleOrphanSubpathCheck', 'normalizeComparePath', 'buildCleanEntries'].forEach(name => load(name));

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

context.STATE.optimizeTasks = [{ requestId: 'opt-1', requested: true, done: false }];
loadCallback('onNativeOptimizeResult');
context.onNativeOptimizeResult({ requestId: 'other', ok: true });
assert.equal(context.STATE.optimizeTasks[0].done, false, 'unrelated maintenance receipt is ignored');
context.onNativeOptimizeResult({ requestId: 'opt-1', ok: false, error: 'Permission denied' });
assert.equal(context.STATE.optimizeTasks[0].done, false);
assert.equal(context.STATE.optimizeTasks[0].error, 'Permission denied');
context.STATE.optimizeTasks[0].requested = true;
context.onNativeOptimizeResult({ requestId: 'opt-1', ok: true });
assert.equal(context.STATE.optimizeTasks[0].done, true);

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
  refreshAfterDestructive() {}, showToast() {}, renderRuntimeBanner() {},
  document: { getElementById() { return { classList: { contains() { return false; } } }; } },
});
receipts.window = receipts;
['hasNum', 'fmtDiskSizeGb', 'selectedSubpaths', 'selectedSizeGb', 'normalizeComparePath', 'markSubpathsMoved', 'markItemFailed', 'scanRowsWithFailures', 'buildRestoreData', 'buildCleanEntries', 'resolveTrash'].forEach(name => load(name, receipts));
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
assert.equal(partialGroup.subpaths[1].cleaned, false);
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

receipts.recomputeDiskAnalyzeTotals = () => {};
receipts.STATE.appsList = [];
receipts.STATE.orphanLeftoversList = [];
receipts.STATE.whitelist = ['old-protection'];
receipts.STATE.deletedPathsRegistry = ['cache', '/tmp/cache'];
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
['hasNum', 'escHtml', 'displayPath', 'diskGbOf', 'fmtDiskSizeGb', 'fmtModified', 'normalizeComparePath', 'recomputeDiskAnalyzeTotals', 'findTileByPath',
  'currentTiles', 'currentAggregated', 'analyzeScopeTotal', 'matchesSearch', 'analysisSelectedSmallItem', 'analysisVisibleTiles', 'restoreAnalysisSelection', 'currentLargeFiles', 'scanRowsWithFailures',
  'normalizeAggregated', 'squarifyRects', 'foldedTreemapTile', 'smallTreemapItems', 'layoutTreemap', 'treemapPreviewHTML', 'absolutePath', 'permissionPaneForPath',
  'groupRestrictedPaths', 'protectionReason'].forEach(name => load(name, analysis));
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
const smallItems = vm.runInContext('smallTreemapItems(slimGraph)', analysis);
assert.equal(smallItems.items[0].path, '~/go/bin', 'readable small-item details keep the original path');
assert.equal(smallItems.sizeGb, 5.1, 'details report measured size rather than a boosted tile weight');
const tinyGraph = vm.runInContext('layoutTreemap([{name:"main",path:"~/main",sizeGb:10},{name:"tiny",path:"~/tiny",sizeGb:0.000001}],null)', analysis);
analysis.tinyGraph = tinyGraph;
assert.equal(tinyGraph.length, 2, 'extremely small siblings keep an identity even when they cannot carry a map label');
assert.equal(vm.runInContext('smallTreemapItems(tinyGraph).items[0].path', analysis), '~/tiny');
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
analysis.denseGraph = denseGraph;
const denseDetails = vm.runInContext('smallTreemapItems(denseGraph)', analysis);
assert.equal(denseDetails.count, denseOther.count);
assert.equal(denseDetails.items.length, denseOther.items.length, 'side details include the same folded and sampled identities exactly once');
assert.equal(denseDetails.unreported, 29, 'small details do not pretend bounded samples are a complete directory');
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
assert.equal(vm.runInContext('fmtDiskSizeGb(0.000004096)', analysis), '4.1 KB', 'small real allocations never display as fictional GB');
assert.equal(vm.runInContext('fmtDiskSizeGb(0.04, 1)', analysis), '40.0 MB', 'one-decimal labels do not round a nonzero allocation to zero GB');
assert.equal(vm.runInContext('fmtDiskSizeGb(1, 1)', analysis), '1.0 GB', 'gigabyte allocations retain their display unit');
load('appCacheSummary', analysis);
analysis.STATE.scanCompleted = true;
assert.equal(vm.runInContext('appCacheSummary({subpaths:[]}).state', analysis), '未发现已识别缓存', 'missing recognized paths are not proof of no app cache');
assert.equal(vm.runInContext('appCacheSummary({subpaths:[{kind:"cache",sizeGb:0.04,tier:"protected",reason:"正在运行"}]}).state', analysis), '安全保留');
assert.equal(vm.runInContext('appCacheSummary({subpaths:[{kind:"cache",sizeGb:null,partial:true}]}).size', analysis), '未读取', 'unread cache bytes do not turn into zero');
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
  STATE: { processes: [{ pid: 8123, name: 'Canvas', path: '/Applications/Canvas.app/Contents/MacOS/Canvas' }] },
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
console.log('UI regression checks passed: frozen selections, protected AI paths, scoped scan receipts, real treemap areas and dual-theme contrast; inline JavaScript parses.');

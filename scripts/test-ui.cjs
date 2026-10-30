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
  pendingTrashRequests: new Map(),
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
  pendingTrashRequests: new Map(), absolutePath: value => value, displayPath: value => value,
  persistState() {}, updateSidebarBadges() {}, renderModule() {}, renderMenuBarTrayContent() {},
  refreshAfterDestructive() {}, showToast() {},
});
receipts.window = receipts;
['selectedSubpaths', 'selectedSizeGb', 'normalizeComparePath', 'markSubpathsMoved', 'markItemFailed', 'scanRowsWithFailures', 'buildRestoreData', 'buildCleanEntries', 'resolveTrash'].forEach(name => load(name, receipts));
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
console.log('UI regression checks passed: selection, frozen paths, mode boundaries, receipts and dual-theme button contrast; inline JavaScript parses.');

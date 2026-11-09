const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const dir = path.resolve(process.argv[2] || path.join(root, '.review/webkit'));
fs.mkdirSync(dir, { recursive: true });
const icon = 'data:image/png;base64,' + fs.readFileSync(path.join(root, 'assets/AppIcon-256.png')).toString('base64');
const group = (id, name, path, size, tier = 'safe') => ({ id, name, path, sizeGb: size, origSizeGb: size, items: 2, filesCount: '12 个文件 · 2 个路径', category: '测试目录', lastMod: '测试时间', rebuildTime: '可重新生成', impact: '隔离测试目录，仅用于页面验收。', tier, checked: tier === 'safe', subpaths: [{ path: path + '/one', sizeGb: size / 3, size: (size / 3).toFixed(2) + ' GB', checked: tier === 'safe', tier, isDir: true }, { path: path + '/two', sizeGb: size * 2 / 3, size: (size * 2 / 3).toFixed(2) + ' GB', checked: tier === 'safe', tier, isDir: true }] });
const version = (id, name, path, size, tier, reason) => ({ ...group(id, name, path, size, tier), band: '程序版本', safetyReason: reason, impact: reason || '隔离测试：不是当前或备用版本，执行前再次核验。', items: 1, filesCount: '1 个程序文件 · 1 个路径', subpaths: [{ path, sizeGb: size, size: size.toFixed(2) + ' GB', tier, checked: false, isDir: false, reason }] });
const fixture = {
  homePath: '/Users/orbit-fixture', freeDiskGb: 128, totalDiskGb: 512, memTotalGb: 32, cpuModel: '隔离测试处理器', whitelist: ['~/Documents/Protected'], whitelistWarnings: [],
  cleanGroups: [group('cache', '应用运行缓存', '~/Library/Caches/Canvas', 3), group('logs', '应用日志', '~/Library/Logs/Canvas', 1), group('review', '需要检查的项目缓存', '~/Projects/Canvas/target', 2, 'review')],
  aiMoonBands: [{ ...group('ai-runtime', 'AI 运行缓存与日志', '~/Library/Caches/Assistant', 1), band: '运行缓存' },
    version('ai-old', 'Claude Code · 2.0.0', '~/.local/share/claude/versions/2.0.0', 0.2, 'review', ''),
    version('ai-current', 'Claude Code · 2.1.0', '~/.local/share/claude/versions/2.1.0', 0.3, 'protected', '当前启动器指向的版本，保持原状'),
    version('ai-spare', 'Claude Code · 2.2.0', '~/.local/share/claude/versions/2.2.0', 0.3, 'protected', '保留最近的非当前版本作为备用'),
    { ...group('ai-worktree', 'Codex · 未提交的工作成果', '~/.codex/worktrees/fixture', 2, 'protected'), band: '工作成果（仅检查）', gitState: '存在未提交或未跟踪内容，保留工作成果' },
    { ...group('ai-models', '共享模型缓存（仅检查）', '~/.cache/huggingface/hub', 3, 'protected'), band: '模型资源', safetyReason: '共享模型与离线资源不能整目录清理' }],
  appsList: Array.from({ length: 12 }, (_, i) => ({ id: 'fixture-app-' + i, bundleId: 'org.example.canvas.' + i, name: i ? '测试编辑工具 ' + i : 'Canvas Editor', appPath: '/Applications/' + (i ? 'Test Tool ' : 'Canvas Editor ') + i + '.app', version: '1.2.0', category: 'Mac 应用', icon, lastUsedAt: i % 3 === 1 ? null : Math.floor(Date.now() / 1000) - (i * 23 + 3) * 86400, lastUsed: i % 3 === 1 ? '未记录' : '2026-01-01', running: i === 2, runningPid: i === 2 ? 4242 : undefined, runningPath: i === 2 ? '/Applications/Test Tool 2.app/Contents/MacOS/Tool' : undefined, appSizeGb: 1.5, cacheSizeGb: 0.3, configSizeGb: 0.2, leftoverSizeGb: 0.5, checked: false, keepPersonal: true, subpaths: [{ path: '/Applications/' + (i ? 'Test Tool ' : 'Canvas Editor ') + i + '.app', kind: 'app', sizeGb: 1.5, size: '1.50 GB', checked: true }, { path: '~/Library/Caches/Canvas ' + i, kind: 'cache', sizeGb: 0.3, size: '0.30 GB', checked: true }, { path: '~/Library/Application Support/Canvas ' + i, kind: 'config', sizeGb: 0.2, size: '0.20 GB', checked: false }].concat(i === 1 ? [{ path: '~/Library/LaunchAgents/org.example.canvas.1.plist', kind: 'launch', sizeGb: 0, size: '1 KB', checked: true }, { path: '/Library/LaunchDaemons/org.example.canvas.1.helper.plist', kind: 'launch', sizeGb: 0, size: '1 KB', checked: false, tier: 'protected', reason: '需要管理员权限，仅列出' }] : []) })),
  orphanLeftoversList: [{ ...group('orphan-fixture', '遗留编辑工具', '~/Library/Caches/Legacy Canvas', 1), leftoverSizeGb: 1, statusText: '未发现主程序', bundleId: 'org.example.legacy', checked: false }],
  purgeItems: [{ id: 'build', project: 'Canvas', path: '~/Projects/Canvas/node_modules', artifact: 'node_modules', sizeGb: 2, origSizeGb: 2, checked: true, kind: 'Node.js 依赖', age: '测试时间', gitClean: '仅用于测试', rebuildNote: '重新安装依赖' }],
  installerFiles: [{ id: 'installer', name: 'Canvas Setup.dmg', path: '~/Downloads/Canvas Setup.dmg', location: '~/Downloads', sizeGb: 1, age: '测试时间', checked: false }],
  largeFiles: [{ id: 'large', name: '测试视频.mov', path: '~/Downloads/测试视频.mov', sizeGb: 3, kind: '视频', age: '测试时间' }],
};
const tiles = [{ name: 'Projects', path: '~/Projects', diskGb: 8, sizeGb: 8, pct: 40, isDir: true, files: 12, modified: 1761739200, preview: [{ name: 'Canvas', path: '~/Projects/Canvas', diskGb: 5, isDir: true }, { name: 'Shared', path: '~/Projects/Shared', diskGb: 2, isDir: true }, { name: 'notes.bin', path: '~/Projects/notes.bin', diskGb: 1, isDir: false }], children: [{ name: 'Canvas', path: '~/Projects/Canvas', diskGb: 8, sizeGb: 8, pct: 100, isDir: true, files: 6, modified: 1761739200, children: [{ name: 'target', path: '~/Projects/Canvas/target', diskGb: 2, sizeGb: 2, pct: 100, isDir: true, files: 2 }] }] }, { name: 'Downloads', path: '~/Downloads', diskGb: 5, sizeGb: 5, pct: 25, isDir: true, files: 7, modified: 1761652800, preview: [{ name: 'Video.mov', path: '~/Downloads/Video.mov', diskGb: 3, isDir: false }, { name: 'Archive', path: '~/Downloads/Archive', diskGb: 2, isDir: true }], children: [{ name: 'Video.mov', path: '~/Downloads/Video.mov', diskGb: 5, sizeGb: 5, pct: 100, isDir: false, files: 1 }] }, { name: 'Library', path: '~/Library', diskGb: 7, sizeGb: 7, pct: 35, isDir: true, files: 5, modified: 1761566400, preview: [{ name: 'Caches', path: '~/Library/Caches', diskGb: 4, isDir: true }, { name: 'Application Support', path: '~/Library/Application Support', diskGb: 2, isDir: true }, { name: 'Mail', path: '~/Library/Mail', diskGb: 1, isDir: true }], children: [] }];
const dominantTiles = [
  { name: 'Documents', path: '~/Documents', diskGb: 527.1, pct: 73.5, isDir: true, files: 108668, modified: 1793435520, preview: [
    { name: '资料', path: '~/Documents/资料', diskGb: 412.2, isDir: true },
    { name: '项目', path: '~/Documents/项目', diskGb: 72.5, isDir: true },
    { name: '归档', path: '~/Documents/归档', diskGb: 28.4, isDir: true },
    { name: 'design.fig', path: '~/Documents/design.fig', diskGb: 14, isDir: false },
  ] },
  { name: 'Library', path: '~/Library', diskGb: 49.5, pct: 6.9, isDir: true, files: 466487, preview: [{ name: 'Containers', path: '~/Library/Containers', diskGb: 20, isDir: true }, { name: 'Application Support', path: '~/Library/Application Support', diskGb: 15, isDir: true }, { name: 'Caches', path: '~/Library/Caches', diskGb: 14.5, isDir: true }] },
  { name: 'code', path: '~/code', diskGb: 49.1, pct: 6.8, isDir: true, files: 433753, preview: [{ name: 'github', path: '~/code/github', diskGb: 31, isDir: true }, { name: 'worktrees', path: '~/code/worktrees', diskGb: 18.1, isDir: true }] },
  { name: 'Pictures', path: '~/Pictures', diskGb: 21, pct: 2.9, isDir: true, files: 1972 },
  { name: '.hermes', path: '~/.hermes', diskGb: 15.3, pct: 2.1, isDir: true, files: 220001 },
  { name: '.volta', path: '~/.volta', diskGb: 8.5, pct: 1.2, isDir: true, files: 339903 },
];
const telemetry = { cpu: 24, freeDiskGb: 128, memUsedGb: 16, memTotalGb: 32, processes: [{ pid: 8123, name: 'Canvas Editor', cpu: 12, memory_mb: 384, path: '/Applications/Canvas Editor.app', power: null }, { pid: 8234, name: 'Canvas Helper', cpu: 4, memory_mb: 128, path: '/Applications/Canvas Helper.app', power: null }], power: { thermal: 'nominal', memPressure: 'normal', battery: { percent: 80, healthPct: 84, cycles: 439, charging: false, source: 'battery' } }, net: { rxBytesPerSec: 1048576, txBytesPerSec: 131072 }, diskIo: { readBytesPerSec: 2097152, writeBytesPerSec: 1048576 }, gpuPct: 20, cpuTempC: 52, fans: [1200] };
const setup = `const style=document.createElement('style');style.textContent='*,*::before,*::after{transition:none!important;animation:none!important}';document.head.appendChild(style);window.qaFixture=${JSON.stringify(fixture)}; window.qaTelemetry=${JSON.stringify(telemetry)}; window.qaTiles=${JSON.stringify(tiles)}; window.qaDominantTiles=${JSON.stringify(dominantTiles)};
window.qaAssert=(condition,message)=>{if(!condition)throw new Error(message)};
window.qaReset=()=>{ I18N.setLang('zh',false); document.querySelector('.permission-toast')?.remove();document.getElementById('qa-refold-style')?.remove();document.body.classList.remove('menubar-mode');closeCommandPalette();closeConfirmSheet();closeDoctorModal();closeRuntimePanel();clearTimeout(window._runtimeTimer); clearTimeout(window._toastTimer); document.getElementById('toastBanner').style.display='none'; document.getElementById('searchInput').value=''; STATE=structuredClone(DEFAULT_STATE); STATE.softwarePanelMode='apps'; window._doctorWaitingForReport=false;clearTimeout(window._docTimer);clearInterval(window._basketTick);pendingTrashRequests.clear();pendingRestoreRequests.clear();pendingProcessQuits.clear();recentMoves.clear();clearTimeout(refreshTimer);clearTimeout(refreshGuard);refreshTimer=refreshGuard=0;refreshing=false;scanStartedAt=lastMoveAt=0;clearTimeout(window._killTimer); window.__qa.actions=[]; window._analyzeFocusSelector=""; sendNativeBridge=message=>window.__qa.actions.push(structuredClone(message)); onNativeRealScanComplete(structuredClone(qaFixture)); onNativeRealTelemetry(structuredClone(qaTelemetry)); Object.assign(STATE,{analyzeAllocGb:20,analyzeLogicalGb:22,totalAnalyzedGb:20,analyzeFileCount:24,treemap2D:structuredClone(qaTiles),analyzeSource:'隔离测试',analyzeStatus:'ok',analyzeRoot:'~',analyzeLargeFiles:structuredClone(qaFixture.largeFiles)}); STATE.historyLogs=[{id:'fixture-1',action:'测试缓存清理',time:'测试时间',sizeGb:1,items:1,status:'已移入废纸篓',restoreData:{type:'clean',paths:[qaFixture.homePath+'/Library/Caches/Canvas/one'],trashedPaths:{[qaFixture.homePath+'/Library/Caches/Canvas/one']:qaFixture.homePath+'/.Trash/one'}}}]; switchModule('clean');renderModule();};`;
// English walk helpers (installed into the page). The fixture is Chinese user data plus Chinese backend
// sentences; backend sentences without a dictionary entry get a placeholder so only the page's own text is judged.
function installEnglishWalk() {
  const CJK = /[㐀-鿿]/;
  const userData = /测试编辑工具 \d+|测试视频\.mov/g;   // fixture file and app names are user data, never translated
  const fixtureStrings = () => {
    const out = new Set();
    const visit = v => { if (typeof v === 'string') { if (CJK.test(v.replace(userData, ''))) out.add(v); } else if (v && typeof v === 'object') Object.values(v).forEach(visit); };
    [qaFixture, qaTelemetry, qaTiles].forEach(visit);
    ['隔离测试', '测试缓存清理', '测试时间', '测试拒绝结果', '隔离测试结果', '已移入废纸篓'].forEach(x => out.add(x));
    return out;
  };
  window.qaEnglishFixture = () => {
    const add = {};
    fixtureStrings().forEach(x => { if (t(x) === x && CJK.test(x.replace(userData, ''))) add[x] = 'Fixture text'; });
    I18N.add(add);
  };
  const visible = el => { const st = getComputedStyle(el); return st.display !== 'none' && st.visibility !== 'hidden' && el.getClientRects().length > 0; };
  window.qaLeaks = () => {
    const leaks = [];
    const bad = (where, text) => { const clean = String(text).replace(userData, ''); if (CJK.test(clean)) leaks.push(where + ': ' + String(text).trim().slice(0, 60)); };
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const el = n.parentElement;
      if (!el || el.closest('script,style,#btnLang') || !visible(el)) continue;
      bad(el.tagName.toLowerCase() + (el.className ? '.' + String(el.className).split(' ')[0] : ''), n.nodeValue);
    }
    document.querySelectorAll('[title],[placeholder],[aria-label],[alt]').forEach(el => {
      if (el.closest('#btnLang') || !visible(el)) return;
      ['title', 'placeholder', 'aria-label', 'alt'].forEach(a => { if (el.hasAttribute(a)) bad('@' + a, el.getAttribute(a)); });
    });
    document.querySelectorAll('input,textarea').forEach(el => { if (visible(el) && el.value) bad('value', el.value); });
    return leaks;
  };
  const settle = () => new Promise(r => setTimeout(r, 80));
  // Switch live to English, run the scenario step, and fail on any CJK left on screen.
  window.qaEn = async (label, step) => {
    qaEnglishFixture();
    I18N.setLang('en', false);
    qaAssert(document.documentElement.lang === 'en', 'html lang follows the language');
    if (step) await step();
    await settle();
    const leaks = qaLeaks();
    qaAssert(leaks.length === 0, label + ' leaks Chinese in English mode: ' + leaks.slice(0, 6).join(' | '));
  };
  window.qaStageText = () => document.getElementById('moduleStage').textContent + '|' + document.querySelector('.top-header').textContent;
  window.qaSettle = settle;
}
const setupEn = '(' + installEnglishWalk.toString() + ')();';
fs.writeFileSync(dir + '/fixtures.js', setup + setupEn);
const scenarios = [];
const modules = ['clean', 'ai_moon', 'software', 'optimize', 'analyze', 'status', 'purge', 'history'];
for (const width of [1380, 1060, 880]) for (const theme of ['dark', 'light']) for (const mod of modules) scenarios.push({ name: `${width}-${theme}-${mod}`, width, height: width === 880 ? 620 : 860, script: `qaReset(); document.documentElement.dataset.theme='${theme}'; switchModule('${mod}');` });
const add = (name, script) => scenarios.push({ name, width: 1380, height: 860, script: 'qaReset();window.isNativeMacApp=false;closeMemoryMaintenance();clearTimeout(window._memoryMaintenanceTimer);await Promise.resolve(); ' + script });
add('apps-search', `switchModule('software'); handleSearch('canvas editor'); qaAssert(document.querySelectorAll('#moduleStage .list-row').length===1,'app search filters rows'); qaAssert(STATE.appsList.every(a=>!a.checked),'search does not authorize deletion');`);
add('apps-selection-confirm', `switchModule('software'); toggleAppCheck(0); openConfirmSheet(); await new Promise(r=>setTimeout(r,80)); qaAssert(document.querySelector('.app-shell').inert,'modal isolates background'); qaAssert(document.activeElement.textContent.includes('取消'),'cancel is initial focus'); qaAssert(document.getElementById('confirmModalItemsList').textContent.includes('Canvas Editor 0.app'),'exact app path is previewed');`);
add('apps-no-bundle-confirm', `switchModule('software'); toggleAppCheck(0); toggleAppSubpathCheck(0,0); openConfirmSheet(); qaAssert(!document.getElementById('confirmModalItemsList').textContent.includes('/Applications/'),'unchecked bundle not previewed');qaAssert(document.getElementById('confirmModalTitle').textContent.includes('缓存与配置'),'preview cannot claim the app will be uninstalled');qaAssert(document.getElementById('heroStatSub').textContent.includes('保留应用'),'selection summary describes the actual selected paths');`);
add('apps-cache-only', `switchModule('software'); setUninstallMode('cache_only'); toggleAppCheck(0); qaAssert(selectedSubpaths(STATE.appsList[0],STATE.uninstallMode).every(p=>p.kind==='cache'),'cache mode preserves config and app'); openConfirmSheet();`);
add('clean-empty', `onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[]}); switchModule('clean'); qaAssert(document.querySelector('.empty-title').textContent.includes('未发现'),'empty scan has truthful state'); qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'empty scan cannot remove');`);
add('clean-filter', `switchModule('clean'); setCleanFilter('review'); qaAssert(document.querySelectorAll('#moduleStage .list-row').length===1,'tier filter works');`);
add('purge-installers-only', `onNativeRealScanComplete({...structuredClone(qaFixture),purgeItems:[]}); switchModule('purge'); qaAssert(document.getElementById('moduleStage').textContent.includes('Canvas Setup.dmg'),'installers remain selectable when no build artifacts exist');`);
add('analyze-drill', `switchModule('analyze'); drillTile(0); qaAssert(STATE.drillStack.length===1,'directory drill works'); qaAssert(currentTiles()[0].name==='Canvas','drill shows children'); drillUp(); qaAssert(STATE.drillStack.length===0,'drill-up works');`);
add('status-search', `switchModule('status'); handleSearch('8123'); qaAssert(document.querySelectorAll('#procTbody tr').length===1,'PID search works');`);
add('doctor-wait', `openDoctorModal(true); qaAssert(document.getElementById('btnDoctorReRun').disabled,'diagnosis awaits receipt'); qaAssert(__qa.actions.some(a=>a.action==='runDoctorReCheck'&&a.requestId),'request is correlated');`);
add('doctor-result', `openDoctorModal(true); onNativeDoctorReport({requestId:_doctorRequestId,checks:[{id:'disk',title:'磁盘剩余空间',detail:'隔离测试结果',passed:true},{id:'permission',title:'完全磁盘访问',detail:'隔离测试结果',passed:false}]}); qaAssert(!document.getElementById('btnDoctorReRun').disabled,'receipt ends wait');`);
add('menu-tray', `document.body.classList.add('menubar-mode'); renderMenuBarTrayContent(); qaAssert(document.getElementById('menubarTrayShell').textContent.length>20,'tray renders');`);
scenarios[scenarios.length-1].width=356;scenarios[scenarios.length-1].height=480;
add('tray-doctor-single-request', `document.body.classList.add('menubar-mode');renderMenuBarTrayContent();document.querySelector('#menubarTrayShell button[onclick*="menubarOpenDoctor"]').click();qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='menubarOpenDoctor','tray opens diagnosis only in the main window');qaAssert(!document.getElementById('doctorModal').classList.contains('open'),'tray does not create a second diagnosis modal');`);
scenarios[scenarios.length-1].width=356;scenarios[scenarios.length-1].height=480;
add('modal-keyboard', `switchModule('software'); toggleAppCheck(0); const opener=document.getElementById('primaryBtnText').closest('button');opener.focus();openConfirmSheet();await new Promise(r=>setTimeout(r,80));const cancel=document.activeElement;cancel.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',shiftKey:true,bubbles:true}));qaAssert(document.activeElement.id==='confirmModalConfirmBtn','Shift-Tab stays inside confirmation');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true}));qaAssert(document.activeElement===cancel,'Tab cycles to cancel');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'3',metaKey:true,bubbles:true}));qaAssert(STATE.module==='software','modal blocks module shortcut');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));await new Promise(r=>setTimeout(r,50));qaAssert(!document.querySelector('.app-shell').inert,'Escape releases background');qaAssert(document.activeElement===opener,'Escape restores opener focus');`);
add('palette-navigation', `openCommandPalette('系统加速');qaAssert(document.querySelectorAll('#cmdPaletteList .list-row').length===1,'palette filters');document.querySelector('#cmdPaletteList .list-row').click();qaAssert(STATE.module==='optimize','palette reaches maintenance');`);
add('keyboard-module-doctor', `document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'6',metaKey:true,bubbles:true}));qaAssert(STATE.module==='status','module shortcut works');document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'d',metaKey:true,bubbles:true}));qaAssert(document.getElementById('doctorModal').classList.contains('open'),'doctor shortcut works');`);
add('partial-trash-receipt', `switchModule('clean');openConfirmSheet();executeConfirmedClean();const request=__qa.actions.find(a=>a.action==='trashRealPaths');qaAssert(request,'confirmation posts request');onNativeTrashResult({requestId:request.requestId,moved:[{original:request.paths[0],trashed:'/trash/one'}],movedCount:1,failed:request.paths.slice(1).map(path=>({path,error:'Permission denied',pane:'fullDiskAccess'}))});qaAssert(STATE.historyLogs[0].restoreData.paths.length===1,'partial success recorded');qaAssert(document.getElementById('permHint'),'permission failure has recovery action');`);
add('apps-no-matches', `switchModule('software');handleSearch('unmatched-keyword');qaAssert(document.querySelectorAll('#moduleStage .list-row').length===0,'no-match list is empty');qaAssert(getComputedStyle(document.querySelector('.master-detail-grid > .panel-card:nth-child(2)')).display==='none','unmatched inspector is hidden');`);
add('apps-sort-keeps-selection', `switchModule('software');selectAppRow(4);const id=STATE.appsList[4].id;
  const rows=()=>[...document.querySelectorAll('#moduleStage .list-row .row-name')].map(n=>n.textContent);
  document.querySelector('.analysis-sort[onclick*="recent"]').click();
  qaAssert(STATE.appSort==='recent'&&STATE.appsList[STATE.selectedAppIdx].id===id,'sorting keeps the selected app by id, not index');
  const used=STATE.appsList.map(a=>Number.isFinite(a.lastUsedAt)?a.lastUsedAt:null);const firstUnknown=used.indexOf(null);
  qaAssert(firstUnknown>0&&used.slice(0,firstUnknown).every((v,k,all)=>v!==null&&(k===0||all[k-1]>=v))&&used.slice(firstUnknown).every(v=>v===null),'recent sort is newest first with unknown last');
  qaAssert(document.querySelector('.analysis-sort[onclick*="recent"]').getAttribute('aria-pressed')==='true','active sort is exposed');
  const subs=[...document.querySelectorAll('#moduleStage .list-row .row-sub')].map(n=>n.textContent);qaAssert(subs.some(t=>/^最近使用 [0-9]+ 个月前$/.test(t)),'rows show relative time');qaAssert([...document.querySelectorAll('#moduleStage .list-row .row-sub')].some(n=>n.textContent==='最近使用 未记录'),'rows show relative time, unknown as 未记录');
  setAppSort('name');const names=STATE.appsList.map(a=>a.name);qaAssert(names.every((n,k)=>k===0||names[k-1].localeCompare(n,'zh')<=0)&&STATE.appsList[STATE.selectedAppIdx].id===id,'name sort keeps selection');
  const big=STATE.appsList.find(a=>a.id==='fixture-app-5');big.appSizeGb=2.5;big.subpaths[0].sizeGb=2.5;setAppSort('size');qaAssert(STATE.appsList[0].id==='fixture-app-5'&&STATE.appsList[STATE.selectedAppIdx].id===id,'size sort puts the biggest app first');
  qaAssert(!__qa.actions.some(a=>['trashRealPaths','killProcess'].includes(a.action)),'sorting is read only');`);
add('apps-running-blocked', `switchModule('software');const idx=STATE.appsList.findIndex(a=>a.running);selectAppRow(idx);
  const row=document.querySelectorAll('#moduleStage .list-row')[idx];
  qaAssert(row.textContent.includes('运行中')&&document.querySelectorAll('#moduleStage .list-row .badge').length>=1,'running app shows a badge');
  qaAssert(row.querySelector('input[type=checkbox]').disabled,'a running app cannot be selected for removal');
  toggleAppCheck(idx);qaAssert(!STATE.appsList[idx].checked,'direct toggles cannot select a running app');
  const quit=document.querySelector('[data-quit-app]');qaAssert(quit&&!quit.disabled,'the inspector offers 退出应用');
  qaAssert(!STATE.appsList.some((a,k)=>k!==idx&&a.running),'only the running app is flagged');
  quit.click();const request=__qa.actions.find(a=>a.action==='killProcess');
  qaAssert(request&&request.pid===4242&&request.path.endsWith('/Contents/MacOS/Tool')&&request.name==='Tool','quit reuses the killProcess bridge with the main executable');
  qaAssert(document.querySelector('[data-quit-app]').disabled,'a pending quit cannot be repeated');
  onNativeProcessQuitResult({requestId:request.requestId,pid:4242,ok:true});
  qaAssert(STATE.appsList[idx].running&&document.querySelector('[data-quit-app]').textContent.includes('已请求退出'),'the page does not claim the app quit before a rescan');
  qaAssert(!__qa.actions.some(a=>a.action==='trashRealPaths'),'quitting never trashes anything');`);
add('apps-launch-items', `switchModule('software');const idx=STATE.appsList.findIndex(a=>a.subpaths.some(sp=>sp.kind==='launch'));selectAppRow(idx);
  const text=document.getElementById('moduleStage').textContent;
  qaAssert(text.includes('启动项')&&text.includes('需要管理员权限，仅列出'),'user launch item and the admin-only reason are listed');
  const boxes=[...document.querySelectorAll('#moduleStage .subpath-item input[type=checkbox]')];const sys=STATE.appsList[idx].subpaths.findIndex(sp=>sp.tier==='protected');
  qaAssert(boxes[sys].disabled&&!boxes[sys].checked,'root-owned launch item is not selectable');
  toggleAppCheck(idx);openConfirmSheet();const preview=document.getElementById('confirmModalItemsList').textContent;
  qaAssert(preview.includes('LaunchAgents/org.example.canvas.1.plist')&&!preview.includes('/Library/LaunchDaemons'),'only the user-level launch item joins the removal');`);
add('clean-no-matches', `switchModule('clean');handleSearch('unmatched-keyword');qaAssert(getComputedStyle(document.querySelector('.master-detail-grid > .panel-card:nth-child(2)')).display==='none','unmatched cache inspector is hidden');`);
add('purge-installer-search', `switchModule('purge');handleSearch('Canvas Setup');qaAssert(document.getElementById('moduleStage').textContent.includes('Canvas Setup.dmg'),'installer-only search remains visible');qaAssert(document.querySelector('.inspector-content').querySelectorAll('.bento-cell').length===0,'unmatched build metadata is absent');`);
add('status-missing-sensors', `switchModule('status');STATE.hw={};STATE.power={thermal:'nominal',memPressure:'normal'};renderModule();qaAssert(document.getElementById('statusCardNet').textContent.includes('本机不提供'),'missing network sensor is not zero');qaAssert(document.getElementById('statusCardGpu').textContent.includes('本机不提供'),'missing GPU sensor is not invented');`);
add('frozen-restore-metadata', `switchModule('software');toggleAppCheck(0);openConfirmSheet();executeConfirmedClean();const request=__qa.actions.find(a=>a.action==='trashRealPaths');setSoftwarePanelMode('leftovers');onNativeTrashResult({requestId:request.requestId,moved:request.paths.map(original=>({original,trashed:'/trash/fixture'})),movedCount:request.paths.length,failed:[]});qaAssert(STATE.historyLogs[0].restoreData.type==='software','receipt keeps approved panel context');qaAssert(STATE.historyLogs[0].restoreData.uninstallMode==='standard','receipt keeps approved mode');`);
add('doctor-copy-report', `openDoctorModal(true);onNativeDoctorReport({requestId:_doctorRequestId,checks:[{id:'permission',title:'权限',detail:'测试拒绝结果',passed:false}]});document.querySelector('#doctorModal button[onclick^="copyTextToClipboard"]').click();const request=__qa.actions.find(a=>a.action==='copyText');qaAssert(request&&JSON.parse(request.text).checks[0].passed===false,'copy contains the actual diagnostic results');`);
const unusualPath = "~/Library/Caches/Owner's &quot;cache\nfile\\name";
add('measured-volume-bars', `switchModule('clean');qaAssert(document.querySelector('.prop-bar-fill').style.width==='50%','bar uses the real scan denominator');const item=structuredClone(qaFixture.cleanGroups[0]);item.sizeGb=0.01;onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[item]});qaAssert(document.querySelector('.prop-bar-fill').style.width==='100%','small measured totals are not clamped to a fictional baseline');`);
add('quoted-path-actions', `const item=structuredClone(qaFixture.cleanGroups[0]);item.name="Owner's <cache> &quot;";item.path=${JSON.stringify(unusualPath)};item.subpaths[0].path=${JSON.stringify(unusualPath)};onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[item]});switchModule('clean');qaAssert(!document.querySelector('cache'),'directory names cannot create markup');document.querySelector('.inspector-content .subpath-left span[onclick]').click();const request=__qa.actions.find(a=>a.action==='revealInFinder');qaAssert(request.path===${JSON.stringify(unusualPath)},'Finder receives the unmodified path');`);
add('whitelist-awaits-receipt', `switchModule('clean');const count=STATE.cleanGroups.length;addPathToWhitelist(STATE.cleanGroups[0].path);qaAssert(STATE.cleanGroups.length===count,'adding protection cannot hide paths before receipt');onNativeWhitelistChanged({ok:false,action:'addWhitelist',path:STATE.cleanGroups[0].path,error:'测试拒绝',whitelist:qaFixture.whitelist});qaAssert(STATE.cleanGroups.length===count,'failed whitelist update keeps candidate visible');`);
add('sensors-become-unavailable', `switchModule('status');const data=structuredClone(qaTelemetry);delete data.net;delete data.diskIo;delete data.gpuPct;delete data.cpuTempC;delete data.fans;data.processes[0].memory_mb=null;onNativeRealTelemetry(data);qaAssert(document.getElementById('statusCardNet').textContent.includes('本机不提供'),'missing sensor replaces old reading');qaAssert(document.getElementById('statusCardGpu').textContent.includes('本机不提供'),'missing GPU does not keep stale value');qaAssert(STATE.processes[0].memory_mb===null,'unreported process memory cannot become zero');`);
add('status-live-preserves-page', `switchModule('status');const table=document.querySelector('.proc-table');const row=document.querySelector('#procTbody tr');const data=structuredClone(qaTelemetry);data.processes[0].cpu=16;onNativeRealTelemetry(data);qaAssert(document.querySelector('.proc-table')===table,'telemetry does not rebuild the process panel');qaAssert(document.querySelector('#procTbody tr')===row,'unchanged process set preserves row identity');qaAssert(row.querySelector('.p-cpu').textContent==='16.0%','current CPU is patched');`);
add('startup-missing-data', `STATE=structuredClone(DEFAULT_STATE);switchModule('status');qaAssert(memPercent()===null&&diskUsedPercent()===null,'unmeasured capacities are unknown');qaAssert(document.getElementById('sidebarFreeDisk').textContent.includes('未知'),'sidebar cannot show fabricated free space');qaAssert(!buildMenuBarTrayHTML().includes('已用 0.0 / 0 GB'),'tray never invents zero memory');`);
add('scan-permission-partial', `onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],readWarnings:[{path:qaFixture.homePath+'/Desktop',reason:'目录读取超时，未计入本次扫描'}]});switchModule('clean');qaAssert(document.querySelector('.empty-title').textContent.includes('未能读取'),'restricted scan cannot claim an empty directory');qaAssert(document.getElementById('scanSourceBadge').textContent.includes('受限'),'summary discloses partial data');const detail=document.querySelector('#moduleStage details');qaAssert(detail&&detail.textContent.includes('Desktop'),'restricted paths are inspectable');detail.open=true;`);
for (const module of ['clean', 'ai_moon', 'software', 'purge']) {
  add(`${module}-failure-state`, `switchModule('${module}');${module === 'software' ? 'toggleAppCheck(0);' : ''}openConfirmSheet();executeConfirmedClean();const request=__qa.actions.find(a=>a.action==='trashRealPaths');qaAssert(request,'selected paths produce a request');onNativeTrashResult({requestId:request.requestId,moved:[],movedCount:0,failed:request.paths.map(path=>({path,error:'隔离测试：读取被拒绝'}))});onNativeRealScanComplete(structuredClone(qaFixture));qaAssert(document.getElementById('moduleStage').textContent.includes('隔离测试：读取被拒绝'),'automatic rescan preserves the failed path reason in its source module');`);
}
add('ai-protected-inspector', `switchModule('ai_moon');selectMoonRow(4);qaAssert(document.querySelector('.inspector-content').textContent.includes('未提交'),'worktree state is explicit');qaAssert(!document.querySelector('.inspector-content input[type=checkbox]'),'worktree inspection has no removal checkbox');const before=STATE.aiMoonBands[4].checked;toggleMoonCheck(4);qaAssert(STATE.aiMoonBands[4].checked===before,'protected group cannot be selected');qaAssert(document.getElementById('heroDesc').textContent.includes('不是 CLI'),'UI distinguishes local scanner from CLI');`);
add('ai-version-filter', `switchModule('ai_moon');setAIFilter('review');qaAssert(document.querySelectorAll('#moduleStage .list-row').length===1,'only eligible old version is reviewable');qaAssert(document.querySelector('.inspector-content').textContent.includes('2.0.0'),'inspector follows filter');toggleMoonCheck(1);openConfirmSheet();const text=document.getElementById('confirmModalItemsList').textContent;qaAssert(text.includes('versions/2.0.0')&&!text.includes('versions/2.1.0'),'preview never expands to the current version');`);
add('ai-current-version-small-window', `document.documentElement.dataset.theme='light';switchModule('ai_moon');setAIFilter('protected');selectMoonRow(2);qaAssert(document.querySelector('.inspector-content').textContent.includes('当前启动器'),'current version has a keep reason');qaAssert(!document.querySelector('.inspector-content input'),'protected version has no checkbox');`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('ai-mixed-subpath-selection', `const row=structuredClone(qaFixture.aiMoonBands[0]);row.tier='review';row.checked=false;row.subpaths.forEach(sp=>sp.checked=false);row.subpaths[0].tier='protected';row.subpaths[0].checked=false;row.subpaths[0].reason='隔离测试：工具正在运行';onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:[row]});switchModule('ai_moon');toggleMoonCheck(0);qaAssert(!STATE.aiMoonBands[0].subpaths[0].checked,'select-all preserves the protected cache');openConfirmSheet();qaAssert(!document.getElementById('confirmModalItemsList').textContent.includes('/one'),'frozen preview excludes protected paths');`);
add('clean-unknown-small-allocation', `const row=structuredClone(qaFixture.cleanGroups[0]);row.tier='review';row.checked=false;row.sizeGb=0.000004096;row.items=1;row.filesCount='1 个文件 · 1 个路径';row.lastMod='2026-10-30 12:34';row.subpaths=[{path:row.path+'/small',sizeGb:row.sizeGb,size:'4.1 KB',tier:'review',checked:false}];onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[row]});switchModule('clean');qaAssert(document.querySelector('.size-cell').textContent==='4.1 KB','small cache is not inflated to 0.01 GB');qaAssert(document.querySelector('.inspector-content').textContent.includes('1 个文件'),'actual file facts are displayed');qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'unknown cache needs explicit selection');toggleSubpathCheck(0,0);openConfirmSheet();qaAssert(document.getElementById('confirmModalItemsList').textContent.includes('/small'),'selection previews the real child');`);
add('clean-partial-path-protected', `const row=structuredClone(qaFixture.cleanGroups[0]);row.tier='protected';row.checked=false;row.partial=true;row.subpaths.forEach(sp=>{sp.tier='protected';sp.checked=false;sp.partial=true;sp.reason='达到文件测量上限；只计入已读取部分'});onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[row],readWarnings:[{path:row.path,reason:row.subpaths[0].reason}]});switchModule('clean');qaAssert(!document.querySelector('.inspector-content input'),'partial inventory cannot be selected');qaAssert(document.getElementById('moduleStage').textContent.includes('未完成测量'),'partial state does not claim complete totals');`);
add('analyze-correlated-refresh', `switchModule('analyze');handlePrimaryAction();const request=__qa.actions.find(a=>a.action==='requestDiskAnalyze');qaAssert(request&&request.refresh===true&&request.requestId,'rescan explicitly requests a fresh correlated scan');const oldTotal=STATE.analyzeAllocGb;onNativeDiskAnalyze({requestId:'old',totalGb:999,treemap2D:[]});qaAssert(STATE.analyzeLoading&&STATE.analyzeAllocGb===oldTotal,'old result cannot replace the latest request');onNativeDiskAnalyze({requestId:request.requestId,root:'~',scanStatus:'ok',totalGb:2,totalSizeGb:2,totalLogicalGb:3,fileCount:4,treemap2D:[{name:'Fresh',path:'~/Fresh',sizeGb:2,diskGb:2,isDir:true}],largeFiles:[]});qaAssert(!STATE.analyzeLoading&&STATE.totalAnalyzedGb===2,'latest receipt supplies fresh totals');qaAssert(!document.getElementById('heroHeadline').textContent.includes('全机'),'home scope is not labeled whole machine');`);
add('analyze-scoped-large-files', `switchModule('analyze');const parent=STATE.treemap2D[0];parent.logicalGb=9;parent.files=6;parent.childrenLoaded=true;parent.aggregated={count:3,sizeGb:1};parent.largeFiles=[{name:'Project archive',path:'~/Projects/a.zip',sizeGb:1},{name:'Outside',path:'~/Downloads/b.zip',sizeGb:2}];drillTile(0);qaAssert(document.querySelector('.facts-bar').textContent.includes('文件 6'),'drill-down reports its own file count');qaAssert(document.querySelector('.facts-bar').textContent.includes('9.00 GB'),'drill-down reports its own logical size');qaAssert(currentLargeFiles().length===1,'outside large files are not mixed into the current directory');qaAssert(!document.getElementById('moduleStage').textContent.includes('Outside'),'outside file is not displayed');`);
add('analyze-aggregated-only', `switchModule('analyze');STATE.treemap2D=[];STATE.analyzeAllocGb=2;STATE.analyzeAggregated={count:14,sizeGb:2,items:[{name:'cache-a.bin',path:'~/Library/Caches/cache-a.bin',sizeGb:0.8,fileCount:1,isDir:false,modified:1761739200},{name:'logs',path:'~/Library/Logs',sizeGb:0.6,fileCount:4,isDir:true}]};STATE.analyzeFileCount=14;renderModule();qaAssert(document.querySelector('.treemap-tile.tt-others'),'backend aggregation remains in the area map');qaAssert(document.querySelector('.analysis-aggregate-only'),'aggregate-only layer has a specific state');qaAssert(!document.getElementById('moduleStage').textContent.includes('未发现可清理项目'),'space analysis never claims there are no cleanable items');toggleAnalyzeOtherItems(true);qaAssert(document.querySelectorAll('.analysis-other-row').length===2,'aggregate exposes its bounded inspectable sample');qaAssert(document.querySelector('.crumb-sel .insp-kind').textContent==='折叠项'&&!document.querySelector('.crumb-actions button[onclick^="revealInFinder"]'),'synthetic aggregate cannot reveal a fake path: the inspector describes the folded group only');`);
add('analyze-search-aggregated', `switchModule('analyze');STATE.analyzeAggregated={count:2,sizeGb:1.4,items:[{name:'cache-a.bin',path:'~/Library/Caches/cache-a.bin',sizeGb:0.8,fileCount:1,isDir:false,reason:''},{name:'other.bin',path:'~/Library/Caches/other.bin',sizeGb:0.6,fileCount:1,isDir:false}]};STATE.searchQuery='cache-a';const input=document.getElementById('searchInput');input.value='cache-a';renderModule();qaAssert(document.querySelectorAll('.analysis-children .list-row').length===1,'search filters the current analysis layer');qaAssert(document.querySelector('.treemap-stage').textContent.includes('cache-a.bin'),'search can surface an aggregated child');qaAssert(document.querySelector('.tt-size').textContent==='800.0MB','matched child keeps its measured size');clearAnalysisSearch();qaAssert(STATE.searchQuery==='','search can be cleared without rescanning');input.value='cache-a';handleSearch('cache-a');`);
add('analyze-no-match', `switchModule('analyze');handleSearch('no-such-path');qaAssert(document.querySelector('.analysis-no-match'),'empty search has a specific result state');qaAssert(document.querySelector('.analysis-no-match').textContent.includes('只覆盖这一层'),'search scope is explicit');document.querySelector('.analysis-no-match button[onclick="clearAnalysisSearch()"]').click();qaAssert(document.querySelectorAll('.analysis-children .list-row').length===3,'clearing search restores the current layer');document.getElementById('searchInput').value='no-such-path';handleSearch('no-such-path');`);
add('analyze-modified-metadata', `switchModule('analyze');selectTile(0);qaAssert(document.querySelector('.crumb-sel small').textContent.includes('12 个文件'),'selection exposes its file count');qaAssert(document.querySelector('.crumb-sel small').textContent.includes('修改于'),'selection exposes its latest modification');qaAssert(!document.querySelector('.treemap-split .rank-dot'),'removed duplicate decorative rank dots');`);
add('analyze-pointer-selection', `switchModule('analyze');const idx=analysisVisibleTiles().findIndex(t=>t.name==='Downloads');const tile=document.querySelector('.treemap-tile[data-idx="'+idx+'"]');tile.click();qaAssert(STATE.analyzeSel===idx,'single click inspects the clicked map item');qaAssert(tile.isConnected,'inspection preserves the click target for a native double click');qaAssert(document.querySelector('.crumb-sel').textContent.includes('Downloads'),'inspector matches the clicked tile');qaAssert(!STATE.collected.length,'inspection never adds a cleanup target');tile.dispatchEvent(new MouseEvent('dblclick',{bubbles:true}));qaAssert(STATE.drillStack[0].path==='~/Downloads','double click enters the same directory');drillUp();qaAssert(analysisVisibleTiles()[STATE.analyzeSel].path==='~/Downloads','going up selects the directory just left');`);
add('analyze-sorted-keyboard', `switchModule('analyze');setAnalyzeSort('name');const map=document.querySelector('.treemap-stage');let tile=map.querySelector('[data-idx="0"]');tile.focus();tile.dispatchEvent(new KeyboardEvent('keydown',{key:'f',bubbles:true,cancelable:true}));qaAssert(__qa.actions.some(a=>a.action==='revealInFinder'&&a.path.endsWith('/Downloads')),'Finder uses the sorted visible item, not the unsorted array');tile.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true,cancelable:true}));qaAssert(!STATE.drillStack.length,'map arrows move instead of entering a directory');qaAssert(document.activeElement.classList.contains('treemap-tile')&&document.activeElement!==tile,'map arrows move focus');const current=document.activeElement;current.dispatchEvent(new KeyboardEvent('keydown',{key:' ',bubbles:true,cancelable:true}));qaAssert(current.isConnected&&document.activeElement===current,'Space inspects without rebuilding the focused tile');qaAssert(document.querySelector('.crumb-sel').textContent.includes(analysisVisibleTiles()[STATE.analyzeSel].name),'keyboard inspection and toolbar agree');current.dispatchEvent(new KeyboardEvent('keydown',{key:'End',bubbles:true,cancelable:true}));qaAssert(document.activeElement===map.querySelector('.treemap-tile:last-child'),'End reaches the last visible tile');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true,cancelable:true}));qaAssert(document.activeElement===map.querySelector('.treemap-tile'),'Home reaches the first visible tile');`);
add('analyze-filtered-navigation', `switchModule('analyze');setAnalyzeSort('name');handleSearch('Projects');drillTile(0);qaAssert(STATE.searchQuery===''&&document.getElementById('searchInput').value==='','changing directories clears the current-layer filter');qaAssert(currentTiles()[0].name==='Canvas'&&document.querySelector('.treemap-tile'),'a parent filter cannot hide its children');drillUp();qaAssert(analysisVisibleTiles()[STATE.analyzeSel].name==='Projects','sorted parent restores the directory just left');setAnalyzeSort('size');qaAssert(analysisVisibleTiles()[STATE.analyzeSel].name==='Projects','sorting keeps the inspected path');handleSearch('Projects');drillTile(0);goBack();qaAssert(STATE.searchQuery===''&&!STATE.drillStack.length,'history navigation resets the old scope filter');goForward();qaAssert(STATE.searchQuery===''&&STATE.drillStack.length===1,'forward navigation returns to the child scope');qaAssert(document.querySelector('.crumb-left button.crumb-item'),'ancestor breadcrumbs are keyboard buttons');`);
add('analyze-keyboard-controls', `switchModule('analyze');qaAssert(!document.querySelector('.treemap-tile button'),'tiles carry no competing action buttons');const button=document.querySelector('.analysis-inspector [aria-label^="在访达中显示"]');button.focus();const key=new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true});button.dispatchEvent(key);qaAssert(!key.defaultPrevented&&!STATE.drillStack.length,'an inspector action keeps its own native keyboard activation');button.click();qaAssert(__qa.actions.some(a=>a.action==='revealInFinder'&&a.path.endsWith('/Projects')),'Finder button still opens its own path');`);
add('analyze-folded-keyboard', `switchModule('analyze');STATE.analyzeAggregated={count:2,sizeGb:1,items:[{name:'small.bin',path:'~/small.bin',sizeGb:0.5,isDir:false}]};renderModule();const tile=document.querySelector('.tt-others');tile.focus();tile.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));qaAssert(STATE.analyzeOtherOpen,'Enter opens the folded sample');qaAssert(document.activeElement.classList.contains('analysis-other-toggle'),'folded-item keyboard navigation retains focus');`);
add('analyze-small-file-units', `switchModule('analyze');STATE.treemap2D=[{name:'small.bin',path:'~/small.bin',sizeGb:0.000004096,isDir:false}];STATE.analyzeAllocGb=0.000004096;STATE.analyzeAggregated=null;renderModule();qaAssert(document.querySelector('.tt-size').textContent==='4.1KB','map uses real units instead of rounding small files to zero GB');qaAssert(document.querySelector('.treemap-tile').getAttribute('aria-label').includes('4.1 KB'),'accessible size agrees with the visible size');STATE.treemap2D[0].sizeGb=0.04;STATE.analyzeAllocGb=0.04;renderModule();qaAssert(document.querySelector('.tt-size').textContent==='40.0MB','sub-GB map labels use MB');qaAssert(document.querySelector('.size-cell').textContent==='40.0 MB','one-decimal list labels do not show zero GB');qaAssert(document.querySelector('.crumb-sel').textContent.includes('40.0 MB'),'inspection agrees with the list and map size');`);
add('analyze-empty-directory-return', `switchModule('analyze');STATE.treemap2D[0].children=[];STATE.treemap2D[0].childrenLoaded=true;STATE.treemap2D[0].aggregated=null;drillTile(0);const back=document.querySelector('#moduleStage button[onclick="drillUp()"]');qaAssert(back,'empty directories keep a mouse and keyboard return path');back.click();qaAssert(!STATE.drillStack.length&&document.querySelector('.treemap-stage'),'return restores the parent map');`);
add('analyze-async-keyboard-focus', `switchModule('analyze');const parent=STATE.treemap2D[0];parent.children=[];parent.childrenLoaded=false;const tile=document.querySelector('.treemap-tile[data-idx="0"]');tile.focus();tile.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));const request=__qa.actions.find(a=>a.action==='requestDiskAnalyze');qaAssert(request&&STATE.analyzeLoading,'keyboard entry requests an unloaded child');onNativeDiskAnalyze({requestId:request.requestId,path:parent.path,scanStatus:'ok',totalSizeGb:2,treemap2D:[{name:'loaded.bin',path:parent.path+'/loaded.bin',diskGb:2,isDir:false}],largeFiles:[]});qaAssert(document.activeElement.classList.contains('treemap-tile')&&document.activeElement.textContent.includes('loaded.bin'),'child receipt restores focus inside the loaded map');`);
add('analyze-list-keyboard', `switchModule('analyze');setAnalyzeSort('name');let row=document.querySelector('.analysis-children .list-row[data-idx="0"]');row.focus();row.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true,cancelable:true}));qaAssert(document.activeElement.dataset.idx==='1'&&STATE.analyzeSel===1,'list arrows inspect the same sorted row they focus');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true,cancelable:true}));qaAssert(STATE.drillStack[0].name==='Library','list Right enters the focused directory');onNativeDiskAnalyze({requestId:STATE.analyzeRequestId,path:'~/Library',scanStatus:'ok',totalSizeGb:7,treemap2D:[{name:'Caches',path:'~/Library/Caches',diskGb:7,isDir:true,childrenLoaded:true}],largeFiles:[]});document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowLeft',bubbles:true,cancelable:true}));qaAssert(!STATE.drillStack.length&&analysisVisibleTiles()[STATE.analyzeSel].name==='Library','list Left restores the sorted parent');qaAssert(document.activeElement.classList.contains('selected')&&document.activeElement.dataset.idx===String(STATE.analyzeSel),'return retains focus on the inspected list row');`);
add('analyze-hover-details', `switchModule('analyze');const tile=document.querySelector('.treemap-tile[data-idx="1"]');tile.style.width='100px';const bar=document.querySelector('.breadcrumb-bar');const before=bar.getBoundingClientRect();tile.dispatchEvent(new MouseEvent('pointerover',{bubbles:true}));await new Promise(r=>setTimeout(r,140));const preview=document.getElementById('tilePreview');qaAssert(preview.classList.contains('show')&&preview.textContent.includes('Library'),'small-tile hover details remain readable below the map');qaAssert(preview.closest('.analysis-map-note'),'hover does not reserve breadcrumb space');const after=bar.getBoundingClientRect();qaAssert(before.width===after.width&&before.height===after.height,'hover does not reflow the context bar');tile.dispatchEvent(new MouseEvent('pointerout',{bubbles:true,relatedTarget:tile.querySelector('.tt-head')}));qaAssert(preview.classList.contains('show'),'moving within the tile does not flicker the preview');tile.dispatchEvent(new MouseEvent('pointerout',{bubbles:true}));qaAssert(!preview.classList.contains('show'),'leaving the tile clears its preview');`);
add('analyze-resize-focus', `switchModule('analyze');const tile=document.querySelector('.treemap-tile[data-idx="1"]');tile.focus();window.dispatchEvent(new Event('resize'));await new Promise(r=>setTimeout(r,180));qaAssert(!tile.isConnected,'resize recalculates map geometry against the current viewport');qaAssert(document.activeElement.classList.contains('treemap-tile')&&document.activeElement.dataset.idx==='1','resizing preserves the focused map item');qaAssert(!__qa.actions.some(a=>a.action==='requestDiskAnalyze'),'resizing never rescans the disk');`);
add('analyze-narrow-pointer', `switchModule('analyze');const map=document.querySelector('.treemap-stage');const before=map.getBoundingClientRect().top;const tile=map.querySelector('.treemap-tile[data-idx="1"]');tile.click();qaAssert(tile.isConnected&&map.getBoundingClientRect().top===before,'narrow-window inspection does not scroll the map out from under the pointer');qaAssert(map.clientHeight>=260,'narrow-window map remains usable');tile.dispatchEvent(new MouseEvent('dblclick',{bubbles:true}));qaAssert(STATE.drillStack[0].name==='Library','double click remains usable in the narrow window');`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('analyze-countdown-escape', `switchModule('analyze');toggleCollect(0);startTrash();document.querySelector('.crumb-basket-btn').focus();document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));qaAssert(!STATE.trashAt&&STATE.collected.length===1,'Escape cancels the countdown from a toolbar control without losing the draft');qaAssert(!__qa.actions.some(a=>a.action==='trashRealPaths'),'cancel never submits a removal');`);
for(const theme of ['light','dark'])scenarios.push({name:'1060-'+theme+'-analyze',width:1060,height:760,script:`qaReset();document.documentElement.dataset.theme='${theme}';switchModule('analyze');qaAssert(document.querySelector('.treemap-stage').clientHeight>=260,'intermediate width keeps the map readable');`});
add('analyze-nested-tiles', `switchModule('analyze');const tile=document.querySelector('.treemap-tile[data-idx="0"]');qaAssert(tile.querySelectorAll('.tt-preview-cell').length===3,'large directory tiles show a nested child map');qaAssert(getComputedStyle(tile.querySelector('.tt-preview')).display!=='none','nested map is visible at the normal window size');qaAssert([...document.querySelectorAll('.crumb-icon-btn')].every(b=>b.textContent.trim()||b.getAttribute('aria-label')),'compact path actions keep accessible names');`);
add('analyze-dominant-directory', `switchModule('analyze');STATE.analyzeAllocGb=717.61;STATE.totalAnalyzedGb=717.61;STATE.analyzeFileCount=2423088;STATE.treemap2D=structuredClone(qaDominantTiles);STATE.analyzeAggregated={count:183,sizeGb:47.1,items:[]};STATE.analyzeStatus='ok';STATE.analyzeSel=0;renderModule();const tile=document.querySelector('.treemap-tile[data-idx="0"]');qaAssert(tile.querySelector('.tt-preview-label b').textContent==='资料','dominant child stays labelled inside the hierarchy');qaAssert(tile.querySelector('.tt-preview-label small').textContent.includes('GB'),'nested label carries its measured size');qaAssert(getComputedStyle(tile.querySelector('.tt-preview')).top==='46px','nested map sits below the parent header');qaAssert(getComputedStyle(document.querySelector('.treemap-tile.tt-others')).opacity==='1','folded items do not look disabled');`);
add('analyze-permission-recovery', `switchModule('analyze');STATE.analyzeStatus='partial';STATE.analyzeRestricted=[{path:qaFixture.homePath+'/Library/Containers/com.example.a',reason:'权限不足'},{path:qaFixture.homePath+'/Library/Containers/com.example.b',reason:'权限不足'},{path:qaFixture.homePath+'/Documents/private',reason:'需要文件访问权限'}];STATE.analyzeRestrictedOpen=true;renderModule();qaAssert(document.querySelector('.permission-recovery'),'restricted paths open a recovery panel');qaAssert(document.querySelectorAll('.permission-group-row').length===2,'restricted paths are grouped by directory');document.querySelector('.permission-recovery-actions [onclick="revealCurrentApp()"]').click();qaAssert(__qa.actions.some(a=>a.action==='revealAppBundle'),'recovery panel can reveal the exact running app');const paneButton=document.querySelector('.permission-group-row .permission-group-pane');paneButton.click();qaAssert(__qa.actions.some(a=>a.action==='openPrivacySettings'&&a.pane==='fullDiskAccess'),'each path group opens its recommended permissions pane');`);
add('analyze-coverage', `switchModule('analyze');STATE.analyzeVolume={totalGb:512,usedGb:384,freeGb:128};renderModule();qaAssert(document.querySelector('.facts-bar').textContent.includes('范围外约 364.00 GB'),'unscanned volume space is disclosed as an estimate');qaAssert(document.getElementById('heroStatSub').textContent.includes('卷容量 512.0 GB'),'volume capacity remains separate from scan scope');`);
for(const [width,height] of [[1380,860],[1060,760],[880,620]]){
  add('analyze-fits-viewport-'+width,`
    switchModule('analyze');
    const body=document.querySelector('.workspace-body'),stage=document.querySelector('.treemap-stage'),panel=document.querySelector('.analysis-inspector');
    const stageBox=stage.getBoundingClientRect(),panelBox=panel.getBoundingClientRect();
    __qa.motionEvidence={map:Math.round(stageBox.width)+'x'+Math.round(stageBox.height),inspector:Math.round(panelBox.width)+'x'+Math.round(panelBox.height)};
    if(innerWidth>960){
      qaAssert(body.scrollHeight<=body.clientHeight+1,'map plus inspector fit the window without page scroll; '+body.scrollHeight+'/'+body.clientHeight);
      const gridWidth=document.querySelector('.treemap-split').getBoundingClientRect().width,expected=innerWidth<=1100?300:Math.min(500,Math.max(340,gridWidth*0.3));
      qaAssert(Math.abs(panelBox.width-expected)<2&&stageBox.height>=360,'map takes the remaining space beside an inspector that scales with the window; '+__qa.motionEvidence.map+' '+__qa.motionEvidence.inspector);
    }else qaAssert(stageBox.height>=360&&stageBox.bottom<=panelBox.top,'narrow windows keep a useful map above the inspector');
    qaAssert(document.querySelectorAll('.analysis-inspector [data-tab]').length===3&&document.querySelectorAll('[onclick*="toggleAnalyzeOtherItems"]').length<=1,'tabs exist and the folded-items intent is not duplicated');
  `);
  Object.assign(scenarios.at(-1),{width,height});
}
add('analyze-narrow-priority', `switchModule('analyze');const map=document.querySelector('.treemap-stage').getBoundingClientRect();const list=document.querySelector('.analysis-inspector').getBoundingClientRect();qaAssert(map.left<list.left||map.top<list.top,'the map comes before the inspector');qaAssert(document.getElementById('searchInput').disabled===false,'space analysis supports current-layer search');qaAssert(map.top<innerHeight,'map starts within the first viewport');qaAssert(map.height>=260,'minimum window keeps a useful map height');`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('analyze-protected-large-file', `switchModule('analyze');STATE.analyzeLargeFiles=[{name:'Active model',path:'~/.cache/huggingface/hub/model.bin',sizeGb:2,protectionReason:'共享模型保持原状'}];renderModule();const button=document.querySelector('#moduleStage button[onclick="removeLargeFile(0)"]');qaAssert(button.disabled&&button.textContent==='保留','protected large file cannot bypass the safety path');removeLargeFile(0);qaAssert(!__qa.actions.some(a=>a.action==='trashRealPaths'),'direct large-file action also respects protection');`);
add('permission-toast-path-actions', `switchModule('clean');const requestId='qa-permission-toast';const deniedPath=qaFixture.homePath+'/Documents/protected';pendingTrashRequests.set(requestId,{entries:[{label:'protected',sizeGb:1,paths:[deniedPath],basketOnFail:false}],options:{title:'隔离权限回执'},requested:1});onNativeTrashResult({requestId,moved:[],movedCount:0,failed:[{path:deniedPath,error:'权限不足',pane:'filesAndFolders'}]});qaAssert(document.querySelector('.permission-toast'),'permission failure exposes a recovery surface');qaAssert(document.querySelector('.permission-toast-path').textContent.includes('Documents/protected'),'the failed path stays visible');document.querySelector('[data-perm-reveal]').click();qaAssert(__qa.actions.some(a=>a.action==='revealAppBundle'),'permission toast can reveal the exact running app');`);
add('apps-cache-not-found', `
  const app=structuredClone(qaFixture.appsList[0]);app.cacheSizeGb=0;app.subpaths=app.subpaths.filter(sp=>sp.kind!=='cache');
  onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[app],readWarnings:[{path:qaFixture.homePath+'/Projects/Canvas/target',reason:'达到文件测量上限；只计入已读取部分'}]});
  switchModule('software');setUninstallMode('cache_only');
  qaAssert(document.querySelector('.app-cache-note').textContent.includes('不代表应用没有缓存'),'unrecognized private cache is not presented as zero');
  qaAssert(document.querySelector('#moduleStage details summary').textContent.includes('本机扫描'),'read warnings identify their full-scan scope');
  qaAssert(document.querySelector('#moduleStage details').textContent.includes('不代表同等数量的应用卸载失败'),'a global warning is not an uninstall failure');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'no recognized cache cannot authorize a removal');
`);
add('apps-cache-protected', `
  const app=structuredClone(qaFixture.appsList[0]);const cache=app.subpaths.find(sp=>sp.kind==='cache');
  Object.assign(cache,{checked:false,tier:'protected',reason:'应用正在运行，暂缓清理'});
  onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[app]});switchModule('software');setUninstallMode('cache_only');
  qaAssert(document.querySelector('.app-cache-note').textContent.includes('正在运行'),'protected cache explains why its measured bytes are retained');
  qaAssert(document.querySelector('.bento-4').textContent.includes('300.0 MB'),'protected cache measurement remains visible');
  const idx=app.subpaths.findIndex(sp=>sp.kind==='cache');toggleAppSubpathCheck(0,idx);
  qaAssert(!STATE.appsList[0].subpaths[idx].checked,'direct toggles cannot authorize protected cache');
  qaAssert(document.querySelectorAll('.app-path-entry input:not(:disabled)').length===0,'cache-only keeps protected cache, application and config unselectable');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'protected cache is not removable');
`);
add('apps-cache-unread', `
  const app=structuredClone(qaFixture.appsList[0]);app.cacheSizeGb=0;
  Object.assign(app.subpaths.find(sp=>sp.kind==='cache'),{sizeGb:null,size:'未读取',partial:true,tier:'protected',checked:false,reason:'目录读取权限不足'});
  onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[app]});switchModule('software');setUninstallMode('cache_only');
  qaAssert(document.querySelector('.bento-4').textContent.includes('未读取'),'unread cache is unknown rather than zero');
  qaAssert(document.querySelector('.app-cache-note').textContent.includes('未读取不等于没有缓存'),'partial measurement explains its lower bound');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'incomplete cache cannot be removed');
`);
const slimDirectoryScript=`
  switchModule('analyze');
  const children=[{name:'pkg',path:'~/go/pkg',diskGb:2.154,sizeGb:2.154,pct:94.9,isDir:true,childrenLoaded:true,files:96690,preview:[{name:'mod',path:'~/go/pkg/mod',diskGb:2.154,isDir:true}]},{name:'bin',path:'~/go/bin',diskGb:0.1163,sizeGb:0.1163,pct:5.1,isDir:true,childrenLoaded:true,children:[],files:12}];
  STATE.treemap2D=[{name:'go',path:'~/go',diskGb:2.2703,sizeGb:2.2703,isDir:true,childrenLoaded:true,children,aggregated:{count:0,sizeGb:0,items:[]}}];
  STATE.analyzeAllocGb=2.2703;STATE.analyzeLargeFiles=[];drillTile(0);
  const tiles=[...document.querySelectorAll('.treemap-tile')];const bin=tiles.find(t=>t.getAttribute('aria-label').startsWith('bin '));
  qaAssert(tiles.length===2&&bin,'few slim directories preserve their actual identity');
  qaAssert(!document.querySelector('.tt-others'),'empty backend aggregation does not create a false Others tile');
  setAnalyzeTab('children');
  const detail=document.querySelector('.analysis-children .list-row[data-path="~/go/bin"]');
  qaAssert(detail&&detail.querySelector('.row-name').textContent.trim()==='bin'&&detail.textContent.includes('116.3 MB'),'small directory has a visible full-size name and measured size beside the map');
  qaAssert(detail.getBoundingClientRect().height>=28,'small directory has a usable target regardless of its area');
  qaAssert(document.querySelector('.insp-tab[data-tab="folded"]').getAttribute('aria-disabled')==='true','nothing folded: the folded tab is disabled instead of listing real tiles');
  const sizes=tiles.map(t=>{const r=t.getBoundingClientRect();return r.width*r.height});
  qaAssert(Math.abs(sizes[1]/(sizes[0]+sizes[1])-0.1163/2.2703)<0.001,'tile proportions match allocated bytes');
  qaAssert(detail.querySelector('.row-name').scrollWidth<=detail.querySelector('.row-name').clientWidth+1,'bin label remains readable in its side detail');
  {const m=document.querySelector('.analysis-map').getBoundingClientRect(),i=document.querySelector('.analysis-inspector').getBoundingClientRect();qaAssert(innerWidth>960?m.right<=i.left+1&&Math.abs(m.top-i.top)<2:m.bottom<=i.top+1,'inspector sits beside the map above 960px and below it on narrow windows');}
  qaAssert([...document.querySelectorAll('.breadcrumb-bar .btn')].every(b=>b.getBoundingClientRect().height===28),'navigation and inspection actions share one height');
  bin.click();qaAssert(document.querySelector('.crumb-sel').textContent.includes('bin')&&document.querySelector('.crumb-sel').textContent.includes('116.3 MB'),'slim directory has complete inspectable details');
  qaAssert(detail.classList.contains('selected'),'map selection highlights the matching small-item detail');
  detail.click();
  qaAssert(detail.isConnected,'small-item selection preserves its double-click target');
`;
for(const width of [880,1060,1380])for(const theme of ['light','dark'])scenarios.push({name:width+'-'+theme+'-analyze-slim',width,height:width===880?620:860,script:'qaReset();document.documentElement.dataset.theme="'+theme+'";'+slimDirectoryScript});
add('analyze-long-breadcrumb', `
  switchModule('analyze');STATE.drillStack=[{name:'很长的工作目录名称-Archive',path:'~/Archive'},{name:'嵌套工程目录名称-Projects',path:'~/Archive/Projects'},{name:'当前子目录-Current',path:'~/Archive/Projects/Current',diskGb:20,children:structuredClone(qaTiles),childrenLoaded:true}];renderModule();
  const location=document.querySelector('.crumb-location').getBoundingClientRect();const current=document.querySelector('.crumb-item.active').getBoundingClientRect();
  qaAssert(current.left>=location.left-1&&current.right<=location.right+1,'deep navigation keeps the current directory visible');
  document.querySelector('.crumb-left [onclick="drillUp()"]').click();qaAssert(STATE.drillStack.length===2,'icon-only up action still returns one level');
`);
const statusTrendScript=`
  switchModule('status');
  for(const cpu of [12,38,27,54,18,32]){const sample=structuredClone(qaTelemetry);sample.cpu=cpu;sample.memUsedGb=14+cpu/10;sample.gpuPct=cpu/2;sample.cpuTempC=50+cpu/5;sample.net.rxBytesPerSec=1048576+cpu*32768;sample.net.txBytesPerSec=cpu*8192;sample.diskIo.readBytesPerSec=2097152+cpu*65536;sample.diskIo.writeBytesPerSec=cpu*16384;onNativeRealTelemetry(sample);}
  for(const id of ['statusCardCpu','statusCardMem','statusCardGpu'])qaAssert(document.querySelector('#'+id+' .status-trend-scale').textContent.includes('100%'),'utilization charts share a 0-100% scale');
  qaAssert(document.querySelectorAll('#statusCardNet .status-trend-lines svg').length===2,'network plots real download and upload histories');
  qaAssert(document.querySelectorAll('#statusCardDiskIo .status-trend-lines svg').length===2,'disk I/O plots real read and write histories');
  qaAssert(document.querySelector('#statusCardThermal .status-trend-scale').textContent.includes('°C')&&!document.querySelector('#statusCardThermal .status-trend-scale').textContent.includes('%'),'temperature is not represented as utilization');
  qaAssert(document.querySelector('.status-capacity-track i').style.width==='75%','disk bar matches measured free and total capacity');
  qaAssert([...document.querySelectorAll('.status-trend')].every(el=>el.getAttribute('aria-label').includes('趋势')),'graphs keep accessible trend descriptions');
`;
for(const width of [880,1060,1380])for(const theme of ['light','dark'])scenarios.push({name:width+'-'+theme+'-status-trends',width,height:width===880?620:860,script:'qaReset();document.documentElement.dataset.theme="'+theme+'";'+statusTrendScript});
add('status-live-graphs-and-controls', statusTrendScript+`
  const button=document.querySelector('#statusCardPower button');button.focus();const table=document.querySelector('.proc-table');const row=document.querySelector('#procTbody tr');
  const sample=structuredClone(qaTelemetry);sample.power.battery.percent=61;onNativeRealTelemetry(sample);
  qaAssert(document.activeElement===button&&button.isConnected,'telemetry leaves the focused power action intact');
  qaAssert(document.querySelector('#statusCardBattery .tc-big').textContent==='61%','battery reading updates with the same live sample');
  qaAssert(document.querySelector('.proc-table')===table&&document.querySelector('#procTbody tr')===row,'new charts do not rebuild the process panel');
  delete sample.net;delete sample.diskIo;delete sample.gpuPct;delete sample.cpuTempC;onNativeRealTelemetry(sample);
  qaAssert(!document.querySelector('#statusCardNet .status-trend-lines svg')&&!document.querySelector('#statusCardThermal .status-trend-lines svg'),'missing sensors do not keep stale histories');
  qaAssert(STATE.hw.netRxSeries.length===0&&STATE.hw.tempSeries.length===0,'sensor absence resets the corresponding chart history');
`);
add('restore-occupied-target', `
  switchModule('history');
  const snapshot=STATE.historyLogs[0];
  restoreHistorySnapshot(0);
  const request=__qa.actions.find(action=>action.action==='restoreTrashSnapshot');
  qaAssert(request&&request.trashedPaths[request.paths[0]],'restore sends the exact saved receipt');
  onNativeRestoreResult({requestId:request.requestId,restored:[],failed:[{path:request.paths[0],error:'还原目标已存在，原文件与废纸篓条目均保留'}]});
  qaAssert(STATE.historyLogs.includes(snapshot),'an occupied target keeps the retry record');
  qaAssert(document.getElementById('toastBanner').textContent.includes('还原目标已存在'),'restore failure explains the occupied destination');
  qaAssert(document.querySelector('#moduleStage button[onclick="restoreHistorySnapshot(0)"]').disabled===false,'the saved receipt remains available for retry');
`);
add('status-memory-pressure-reference', `
  switchModule('status');
  const sample=structuredClone(qaTelemetry);sample.memUsedGb=31;
  onNativeRealTelemetry(sample);
  qaAssert(STATE.healthScore===100,'high memory occupancy with normal native pressure does not reduce the reference score');
  qaAssert(document.getElementById('statusCardMem').textContent.includes('内存压力正常'),'memory occupancy and native pressure remain distinct');
  sample.power.memPressure='warn';onNativeRealTelemetry(sample);
  qaAssert(STATE.healthScore===94&&document.getElementById('statusCardMem').textContent.includes('内存压力偏高'),'the real pressure state still changes the reference');
`);
const leftoverIconsScript=`
  const names=['Docker','Android Studio','com.example.long-name.helper'];
  const orphans=names.map((name,index)=>({...structuredClone(qaFixture.orphanLeftoversList[0]),id:'icon-fixture-'+index,name,bundleId:index===0?'com.docker.docker':'com.example.'+index}));
  onNativeRealScanComplete({...structuredClone(qaFixture),orphanLeftoversList:orphans});
  switchModule('software');setSoftwarePanelMode('leftovers');
  for(const icon of document.querySelectorAll('.list-row .app-squircle-icon')){
    const bounds=icon.getBoundingClientRect();
    qaAssert(Math.abs(bounds.width-38)<1&&Math.abs(bounds.height-38)<1,'leftover icon stays 38px square and cannot grow with row text');
    qaAssert(getComputedStyle(icon).boxShadow==='none','residual icon uses the same quiet surface treatment');
  }
  const inspectorIcon=document.querySelector('.inspector-content .app-squircle-icon').getBoundingClientRect();
  qaAssert(inspectorIcon.width===46&&inspectorIcon.height===46,'inspector icon keeps its existing 46px size');
  handleSearch('com.example.long-name');
  qaAssert(document.querySelectorAll('#moduleStage .list-row').length===1,'icon correction preserves residual filtering');
  handleSearch('');
`;
for(const width of [880,1060,1380])for(const theme of ['light','dark'])scenarios.push({name:width+'-'+theme+'-leftover-icons',width,height:width===880?620:860,script:'qaReset();document.documentElement.dataset.theme="'+theme+'";'+leftoverIconsScript});
const tinyDirectoryScript=`
  switchModule('analyze');
  STATE.treemap2D=[{name:'主目录',path:'~/main',diskGb:10,isDir:true,childrenLoaded:true,children:[]},{name:'很小的缓存目录',path:'~/tiny',diskGb:0.000002048,pct:0,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAllocGb=10.000002048;STATE.totalAnalyzedGb=10.000002048;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('children');
  const detail=document.querySelector('.analysis-children .list-row[data-path="~/tiny"]');
  qaAssert(detail&&detail.textContent.includes('很小的缓存目录')&&detail.textContent.includes('2.0 KB'),'tiny directories remain visible without a minimum-area lie');
  qaAssert(detail.getBoundingClientRect().height>=28,'tiny directory has a usable row target');
  detail.click();qaAssert(document.querySelector('.crumb-sel').textContent.includes('很小的缓存目录'),'tiny item selects the actual directory');
  detail.dispatchEvent(new MouseEvent('dblclick',{bubbles:true}));qaAssert(STATE.drillStack.at(-1).path==='~/tiny'&&!STATE.analyzeLoading,'tiny directory enters with the existing scan tree');
  drillUp();
`;
for(const width of [880,1060,1380])for(const theme of ['light','dark'])scenarios.push({name:width+'-'+theme+'-analyze-tiny',width,height:width===880?620:860,script:'qaReset();document.documentElement.dataset.theme="'+theme+'";'+tinyDirectoryScript});
add('analyze-navigation-motion', `
  Object.defineProperty(document,'visibilityState',{value:'visible',configurable:true});
  document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));
  switchModule('analyze');drillTile(0);
  const first=document.getAnimations().find(animation=>animation.id==='analysis-navigation');
  qaAssert(first&&first.effect.getTiming().duration===160,'directory navigation has one short native animation');
  first.pause();first.currentTime=80;
  const intermediate=getComputedStyle(document.querySelector('.analysis-map'));
  qaAssert(intermediate.transform!=='none'&&Number(intermediate.opacity)<1,'the rendered midpoint has real movement and opacity feedback');
  __qa.motionEvidence={duration:first.effect.getTiming().duration,midpointTransform:intermediate.transform,midpointOpacity:intermediate.opacity};
  const map=document.querySelector('.treemap-stage');const child=map.querySelector('.treemap-tile');
  child.click();qaAssert(child.isConnected&&map===document.querySelector('.treemap-stage'),'selection during motion preserves the native double-click target');
  drillUp();qaAssert(first.playState==='idle','rapid reverse cancels the old animation');
  qaAssert(document.getAnimations().filter(animation=>animation.id==='analysis-navigation').length===1,'rapid navigation cannot stack transitions');
  document.getAnimations().find(animation=>animation.id==='analysis-navigation').finish();
  qaAssert(!document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'motion settles without persistent transforms');
  document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true}));drillTile(0);
  qaAssert(!document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'keyboard navigation stays immediate');
  delete document.visibilityState;
`);
add('analyze-reduced-motion', `
  Object.defineProperty(document,'visibilityState',{value:'visible',configurable:true});
  for(const media of __qa.motionQueries)Object.defineProperty(media,'matches',{value:true,configurable:true});
  document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));
  switchModule('analyze');drillTile(0);
  qaAssert(!document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'reduced motion disables spatial directory movement');
  __qa.motionEvidence={reduced:true,navigationAnimations:0};
  for(const media of __qa.motionQueries)delete media.matches;
  delete document.visibilityState;
`);
for(const time of [0,80,160])add('analyze-motion-frame-'+time, `
  Object.defineProperty(document,'visibilityState',{value:'visible',configurable:true});
  document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));switchModule('analyze');drillTile(0);
  const animation=document.getAnimations().find(item=>item.id==='analysis-navigation');
  qaAssert(animation,'directory transition is available for frame inspection');animation.pause();animation.currentTime=${time};
  __qa.motionEvidence={frameTime:${time},transform:getComputedStyle(document.querySelector('.analysis-map')).transform};
  delete document.visibilityState;
`);
add('analyze-hidden-window-motion', `
  Object.defineProperty(document,'visibilityState',{value:'hidden',configurable:true});
  document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));switchModule('analyze');drillTile(0);
  qaAssert(!document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'hidden windows show the final directory without a suspended animation');
  Object.defineProperty(document,'visibilityState',{value:'visible',configurable:true});drillUp();
  qaAssert(document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'visible navigation can animate');
  Object.defineProperty(document,'visibilityState',{value:'hidden',configurable:true});document.dispatchEvent(new Event('visibilitychange'));
  qaAssert(!document.getAnimations().some(animation=>animation.id==='analysis-navigation'),'hiding the window cancels the in-flight spatial transition');
  __qa.motionEvidence={hidden:true,canceled:true};
  delete document.visibilityState;
`);
add('analyze-small-items-preserve-scroll', `
  switchModule('analyze');
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:90,isDir:true},...Array.from({length:30},(_,index)=>({name:'Small '+index,path:'~/small-'+index,diskGb:0.002,isDir:true,childrenLoaded:true,children:[]}))];
  STATE.analyzeAllocGb=90.06;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
  const body=document.querySelector('.analysis-other-body');
  body.scrollTop=body.scrollHeight;
  const before=body.scrollTop;
  qaAssert(before>100,'dense small items have a real scroll position');
  const target=body.querySelectorAll('.analysis-other-row')[27];
  const path=target.dataset.path;
  target.querySelector('.analysis-other-actions .btn:last-child').focus({preventScroll:true});
  target.querySelector('.analysis-other-actions .btn:last-child').click();
  qaAssert(STATE.collected.some(item=>item.path===path),'scroll preservation keeps the actual selected path');
  qaAssert(Math.abs(document.querySelector('.analysis-other-body').scrollTop-before)<2,'adding a small item preserves its list scroll');
  window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,170));
  qaAssert(Math.abs(document.querySelector('.analysis-other-body').scrollTop-before)<2,'resize does not jump the small-item list to the top');
`);
add('analyze-small-directory-new-scan-root', `
  switchModule('analyze');
  const tiny={name:'Small Directory',path:'~/small-new-root',diskGb:0.000004096,isDir:true};
  STATE.analyzeAggregated={count:1,sizeGb:tiny.diskGb,items:[{...tiny,sizeGb:tiny.diskGb}]};renderModule();
  const row=document.querySelector('.analysis-other-row[data-path="~/small-new-root"]');
  row.querySelector('.analysis-other-actions .btn').click();
  const request=__qa.actions.filter(action=>action.action==='requestDiskAnalyze').at(-1);
  qaAssert(request?.path===tiny.path,'small directory requests its actual scan path');
  onNativeDiskAnalyze({requestId:request.requestId,root:tiny.path,scanStatus:'ok',totalSizeGb:tiny.diskGb,treemap2D:[{name:'tiny.txt',path:tiny.path+'/tiny.txt',diskGb:tiny.diskGb,isDir:false}],restricted:[],aggregated:{count:0,sizeGb:0,items:[]}});
  qaAssert(STATE.drillStack.at(-1)?.path===tiny.path,'an omitted native result path does not replace the navigation root');
  qaAssert(currentTiles()[0].name==='tiny.txt','the new scan result is attached to the requested small directory');
  drillUp();qaAssert(STATE.drillStack.length===0&&STATE.treemap2D.length===3,'return preserves the previous parent snapshot');
`);
add('analyze-small-directory-late-result-after-return', `
  switchModule('analyze');
  const parentPaths=STATE.treemap2D.map(tile=>tile.path).join('|');
  const beforeTotal=STATE.totalAnalyzedGb;
  STATE.analyzeAggregated={count:1,sizeGb:0.000004096,items:[{name:'Small Directory',path:'~/small-late-result',sizeGb:0.000004096,isDir:true}]};renderModule();
  document.querySelector('.analysis-other-row[data-path="~/small-late-result"] .analysis-other-actions .btn').click();
  const request=__qa.actions.filter(action=>action.action==='requestDiskAnalyze').at(-1);
  drillUp();
  onNativeDiskAnalyze({requestId:request.requestId,root:request.path,scanStatus:'ok',totalSizeGb:0.000004096,treemap2D:[{name:'tiny.txt',path:request.path+'/tiny.txt',diskGb:0.000004096,isDir:false}],restricted:[]});
  qaAssert(STATE.drillStack.length===0&&STATE.treemap2D.map(tile=>tile.path).join('|')===parentPaths,'a late path-less small directory result cannot replace the parent map');
  qaAssert(STATE.totalAnalyzedGb===beforeTotal&&!STATE.analyzeLoading,'late small result settles its request without replacing the root total');
`);
add('analyze-small-directory-scroll-scope', `
  switchModule('analyze');
  const children=Array.from({length:40},(_,index)=>({name:'Child '+index,path:'~/small-29/child-'+index,diskGb:0.00005,isDir:false}));
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:90,isDir:true},...Array.from({length:30},(_,index)=>({name:'Small '+index,path:'~/small-'+index,diskGb:0.002,isDir:true,childrenLoaded:true,children:index===29?children:[]}))];
  STATE.analyzeAllocGb=90.06;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
  const body=document.querySelector('.analysis-other-body');body.scrollTop=body.scrollHeight;
  qaAssert(body.scrollTop>100,'parent has a scrolled small-item list');
  const row=document.querySelector('.analysis-other-row[data-path="~/small-29"]');
  row.querySelector('.analysis-other-actions .btn').click();
  qaAssert(STATE.drillStack.at(-1)?.path==='~/small-29'&&currentTiles().length===40,'small directory enters the child scope');
  {const folded=document.querySelector('.analysis-other-body');qaAssert(!folded||folded.scrollTop===0,'a new directory starts its own small-item list at the top');}
  qaAssert(document.querySelector('.analysis-children .list-body').scrollTop===0,'parent detail scrolling cannot move the new directory list');
`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('analyze-small-items-keyboard-navigation', `
  switchModule('analyze');
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:90,isDir:true},...Array.from({length:12},(_,index)=>({name:'Small '+index,path:'~/small-'+index,diskGb:0.002,isDir:true,childrenLoaded:true,children:[]}))];
  STATE.analyzeAllocGb=90.024;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
  const names=[...document.querySelectorAll('.analysis-other-path')];names[0].focus();
  names[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===names[1],'Down moves focus between small-item names');
  document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'End',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===names.at(-1),'End reaches the last small item');
  document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===names[0],'Home returns to the first small item');
  const path=names[0].closest('.analysis-other-row').dataset.path;
  names[0].dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  qaAssert(STATE.drillStack.at(-1)?.path===path,'Enter enters the focused small directory');
  qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder'),'arrow navigation does not open Finder');
  qaAssert(!STATE.collected.length,'keyboard navigation cannot authorize cleanup');
`);
const aggregateInspectionSetup=`
  switchModule('analyze');
  STATE.analyzeAggregated={count:2,sizeGb:0.007,items:[
    {name:'Sample Directory',path:'~/aggregate-sample-directory',sizeGb:0.004,isDir:true,fileCount:2},
    {name:'sample-notes.txt',path:'~/aggregate-sample-notes.txt',sizeGb:0.003,isDir:false,fileCount:1}
  ]};
  STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
`;
add('analyze-aggregate-sample-click-inspection', aggregateInspectionSetup+`
  const path='~/aggregate-sample-directory';
  const row=document.querySelector('.analysis-other-row[data-path="'+path+'"]');
  row.querySelector('.analysis-other-path').click();
  qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder'),'single-clicking an aggregate sample inspects in the app instead of opening Finder');
  qaAssert(row.classList.contains('selected')&&document.querySelector('.crumb-sel').textContent.includes('Sample Directory'),'aggregate sample click updates its selected row and directory details');
  qaAssert(row.isConnected&&!STATE.collected.length,'aggregate inspection keeps its double-click target and never authorizes cleanup');
  renderModule();
  qaAssert(document.querySelector('.analysis-other-row[data-path="'+path+'"]').classList.contains('selected')&&document.querySelector('.crumb-sel').textContent.includes('Sample Directory'),'a normal rerender retains aggregate inspection by path');
`);
add('analyze-aggregate-sample-space-inspection', aggregateInspectionSetup+`
  const names=[...document.querySelectorAll('.analysis-other-path')];
  const file=names.find(button=>button.closest('.analysis-other-row').dataset.path==='~/aggregate-sample-notes.txt');
  const index=names.indexOf(file);names[Math.max(0,index-1)].focus();
  if(index>0)document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===file,'arrow movement reaches the intended aggregate file');
  const key=new KeyboardEvent('keydown',{key:' ',bubbles:true,cancelable:true});file.dispatchEvent(key);
  if(!key.defaultPrevented)file.click();
  qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder'),'Space on the focused aggregate sample inspects in the app instead of opening Finder');
  qaAssert(file.closest('.analysis-other-row').classList.contains('selected')&&document.querySelector('.crumb-sel').textContent.includes('sample-notes.txt'),'Space inspects the same aggregate file reached by keyboard');
  qaAssert(!STATE.collected.length,'keyboard aggregate inspection cannot authorize cleanup');
`);
add('analyze-aggregate-sample-double-click-entry', aggregateInspectionSetup+`
  const path='~/aggregate-sample-directory';
  const name=document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path');
  name.click();name.click();name.dispatchEvent(new MouseEvent('dblclick',{bubbles:true,cancelable:true}));
  qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder'),'double-clicking an aggregate directory never opens Finder before entry');
  qaAssert(STATE.drillStack.at(-1)?.path===path&&__qa.actions.some(action=>action.action==='requestDiskAnalyze'&&action.path===path),'aggregate directory double-click enters and requests the exact sample path');
`);
add('analyze-aggregate-sample-toolbar-collect', aggregateInspectionSetup+`
  const path='~/aggregate-sample-directory';
  const name=document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path');
  name.click();
  qaAssert(name.isConnected,'sample selection preserves the exact name button for double click');
  document.querySelector('.crumb-actions .crumb-basket-btn').click();
  qaAssert(STATE.collected.length===1&&STATE.collected[0].path===path,'toolbar collection authorizes the inspected aggregate sample instead of the main map selection');
  qaAssert(document.querySelector('.analysis-other-row[data-path="'+path+'"]').classList.contains('selected'),'collection rerender preserves aggregate sample inspection');
`);
add('analyze-aggregate-sample-toolbar-entry', aggregateInspectionSetup+`
  const path='~/aggregate-sample-directory';
  document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path').click();
  document.querySelector('.crumb-actions .btn-primary').click();
  qaAssert(STATE.drillStack.at(-1)?.path===path&&__qa.actions.some(action=>action.action==='requestDiskAnalyze'&&action.path===path),'toolbar entry opens the inspected aggregate sample instead of the main map directory');
  qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder'),'toolbar inspection and entry stay inside the app');
`);
const smallItemRefoldSetup=`
  document.getElementById('qa-refold-style')?.remove();
  switchModule('analyze');
  { const base=document.createElement('style');base.id='qa-refold-style';base.textContent='#moduleStage .analysis-map{width:720px;height:312px}';document.head.appendChild(base); }
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:50,isDir:true},
    ...[12,10,8,7,5,3,2,1].map((diskGb,index)=>({name:'Medium '+index,path:'~/medium-'+index,diskGb,isDir:true,childrenLoaded:true,children:[]})),
    {name:'Tiny Focus Target',path:'~/tiny-focus-target',diskGb:0.01,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAllocGb=98.01;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
`;
for(const role of ['name','collect'])add('analyze-small-items-refold-focus-'+role, smallItemRefoldSetup+`
  const path='~/tiny-focus-target';
  const selector=${JSON.stringify(role==='name'?'.analysis-other-path':'.analysis-other-actions .btn:last-child')};
  const row=document.querySelector('.analysis-other-row[data-path="'+path+'"]');
  qaAssert(row,'the stable tiny target is represented in small-item details');
  row.querySelector(selector).focus({preventScroll:true});
  ${role==='collect'?`row.querySelector(selector).click();qaAssert(STATE.collected.some(item=>item.path===path),'the focused action collects its actual path');`:''}
  const beforeCount=document.querySelectorAll('.treemap-tile').length;
  const resizeStyle=document.createElement('style');resizeStyle.id='qa-refold-style';resizeStyle.textContent='#moduleStage .analysis-map{width:420px;max-width:100%}';document.head.appendChild(resizeStyle);
  try{
  window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
  const afterCount=document.querySelectorAll('.treemap-tile').length;
  qaAssert(beforeCount!==afterCount,'resize fixture actually changes the set of folded map nodes: '+beforeCount+' -> '+afterCount);
  const expected=document.querySelector('.analysis-other-row[data-path="'+path+'"] '+selector);
  qaAssert(document.activeElement===expected,'refolding preserves the same small-item path and ${role} role; before/after map nodes '+beforeCount+'/'+afterCount+', focused path '+document.activeElement.closest('.analysis-other-row')?.dataset.path+', action '+document.activeElement.getAttribute('onclick'));
  setAnalyzeSort('name');
  qaAssert(document.activeElement===document.querySelector('.analysis-other-row[data-path="'+path+'"] '+selector),'sorting retains the same small-item path and ${role} role');
  }finally{resizeStyle.remove();}
`);
add('analyze-aggregate-sample-return-selection', aggregateInspectionSetup+`
  const path='~/aggregate-sample-directory';
  document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path').click();
  document.querySelector('.crumb-actions .btn-primary').click();
  const request=__qa.actions.filter(action=>action.action==='requestDiskAnalyze').at(-1);
  qaAssert(request?.path===path,'toolbar entry requests the selected sample directory');
  onNativeDiskAnalyze({requestId:request.requestId,path,scanStatus:'ok',totalSizeGb:0.004,treemap2D:[{name:'child.txt',path:path+'/child.txt',diskGb:0.004,isDir:false}],restricted:[],aggregated:{count:0,sizeGb:0,items:[]}});
  qaAssert(currentTiles()[0]?.name==='child.txt','correct child receipt populates the sample directory');
  drillUp();
  const returned=document.querySelector('.analysis-other-row[data-path="'+path+'"]');
  qaAssert(STATE.drillStack.length===0&&returned?.classList.contains('selected')&&returned.querySelector('.analysis-other-path').getAttribute('aria-pressed')==='true'&&document.querySelector('.crumb-sel').textContent.includes('Sample Directory'),'returning from an aggregate sample keeps the departed sample selected with matching details and aria-pressed; actual details '+document.querySelector('.crumb-sel').textContent);
`);
add('analyze-small-items-refold-focus-visible', smallItemRefoldSetup+`
  const path='~/tiny-focus-target';
  const selector='.analysis-other-actions .btn:last-child';
  const button=document.querySelector('.analysis-other-row[data-path="'+path+'"] '+selector);
  button.focus();button.scrollIntoView({block:'nearest'});
  const viewport=document.querySelector('.analysis-other-body').getBoundingClientRect(),before=button.getBoundingClientRect();
  qaAssert(before.top>=viewport.top-1&&before.bottom<=viewport.bottom+1,'the focused action starts visible inside the small-item viewport');
  const beforeCount=document.querySelectorAll('.treemap-tile').length;
  const style=document.createElement('style');style.textContent='#moduleStage .analysis-map{width:420px;max-width:100%}';document.head.appendChild(style);
  try{
    window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
    const afterCount=document.querySelectorAll('.treemap-tile').length;
    qaAssert(beforeCount===9&&afterCount===7,'width change refolds 9 map nodes into 7; actual '+beforeCount+'/'+afterCount);
    const expected=document.querySelector('.analysis-other-row[data-path="'+path+'"] '+selector);
    qaAssert(document.activeElement===expected,'refolding preserves the focused path and collect role');
    const bounds=document.querySelector('.analysis-other-body').getBoundingClientRect(),rect=expected.getBoundingClientRect();
    qaAssert(rect.top>=bounds.top-1&&rect.bottom<=bounds.bottom+1,'restored focus remains visible in the small-item viewport; button '+rect.top+'..'+rect.bottom+', viewport '+bounds.top+'..'+bounds.bottom);
  }finally{style.remove();}
`);
for(const width of [1380,880]){
add('analyze-small-items-collapse-space-'+width, `
  qaReset();switchModule('analyze');
  STATE.treemap2D=[{name:'主目录',path:'~/main',diskGb:10,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAggregated={count:1,sizeGb:0.000002048,items:[{name:'很小的缓存目录',path:'~/tiny',sizeGb:0.000002048,isDir:true}]};
  STATE.analyzeAllocGb=10.000002048;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
  const mapBefore=document.querySelector('.treemap-stage').getBoundingClientRect();
  document.querySelector('.analysis-other-toggle').click();
  const map=document.querySelector('.treemap-stage').getBoundingClientRect();
  const region=document.getElementById('analysisOtherHost').getBoundingClientRect();
  qaAssert(document.querySelector('.analysis-other-toggle').getAttribute('aria-expanded')==='false','details collapse');
  __qa.motionEvidence={beforeWidth:mapBefore.width,afterWidth:map.width,collapsedDetailsWidth:region.width,collapsedDetailsHeight:region.height};
  qaAssert(Math.abs(map.width-mapBefore.width)<2,'collapsing the folded list never reflows the map; actual '+mapBefore.width+'/'+map.width);
  qaAssert(document.querySelector('.analysis-other-toggle').getBoundingClientRect().height<=48,'collapsed details remain a compact, reachable disclosure');
  document.querySelector('.analysis-other-toggle').click();
  qaAssert(document.querySelector('.analysis-other-row[data-path="~/tiny"]'),'reopening retains the actual tiny directory');
`);
Object.assign(scenarios.at(-1),{width,height:width===1380?860:620});
}
const smallItemMotionInterruptSetup=`
  Object.defineProperty(document,'visibilityState',{value:'visible',configurable:true});
  document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));
  switchModule('analyze');
  STATE.analyzeAggregated={count:1,sizeGb:0.005,items:[{name:'小目录',path:'~/small-motion',sizeGb:0.005,isDir:true}]};
  renderModule();toggleAnalyzeOtherItems(false);toggleAnalyzeOtherItems(true);
  const animation=document.getAnimations().find(animation=>animation.id==='analysis-small-items');
  qaAssert(animation,'small details have their purposeful opening motion');
  animation.pause();animation.currentTime=60;
  const panel=document.querySelector('.analysis-other-body');
  qaAssert(getComputedStyle(panel).transform!=='none','opening motion is rendered before interruption');
`;
add('analyze-small-items-motion-keyboard-interrupt',smallItemMotionInterruptSetup+`
  document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true}));
  qaAssert(animation.playState==='idle'&&!document.getAnimations().some(animation=>animation.id==='analysis-small-items'),'keyboard input ends in-flight details movement immediately');
  qaAssert(getComputedStyle(panel).transform==='none','keyboard users see final detail positions');
  delete document.visibilityState;
`);
add('analyze-small-items-motion-reduced-change',smallItemMotionInterruptSetup+`
  for(const media of __qa.motionQueries){Object.defineProperty(media,'matches',{value:true,configurable:true});media.dispatchEvent(new Event('change'));}
  qaAssert(animation.playState==='idle'&&!document.getAnimations().some(animation=>animation.id==='analysis-small-items'),'enabling reduced motion cancels ongoing detail movement');
  qaAssert(getComputedStyle(panel).transform==='none','reduced-motion users see final detail positions');
  for(const media of __qa.motionQueries)delete media.matches;
  delete document.visibilityState;
`);
for(const kind of ['zero','unknown'])add('analyze-aggregate-'+kind+'-sample-inspection',`
  switchModule('analyze');STATE.treemap2D=[];STATE.analyzeLargeFiles=[];
  STATE.analyzeAggregated={count:1,${kind==='zero'?'sizeGb:0,':''}items:[{name:'${kind} sample',path:'~/${kind}-sample',${kind==='zero'?'sizeGb:0,':''}isDir:true,childrenLoaded:true,children:[]}]};
  renderModule();
  const row=document.querySelector('.analysis-other-row[data-path="~/${kind}-sample"]');
  qaAssert(row,'the reported ${kind} allocation sample remains inspectable without invented map area');
  qaAssert(!document.querySelector('.treemap-stage .treemap-tile'),'${kind} allocation never receives invented map area');
  qaAssert(!document.querySelector('.treemap-stage').textContent.includes('未发现可清理项目'),'the analysis map does not confuse unplottable ${kind} allocation with cleanup candidates');
  qaAssert(row.querySelector('.analysis-other-meta').textContent.includes('${kind==='zero'?'0 B':'未知'}'),'the ${kind} sample size keeps its actual zero or unknown meaning');
  qaAssert(document.querySelector('.analysis-other-note').textContent.includes('共 ${kind==='zero'?'0 B':'未知'}'),'the ${kind} aggregate total keeps its actual zero or unknown meaning');
  row.querySelector('.analysis-other-path').click();
  qaAssert(document.querySelector('.crumb-sel').textContent.includes('${kind} sample'),'sample inspection preserves its identity');
  qaAssert(document.querySelector('.crumb-sel').textContent.includes('${kind==='zero'?'0 B':'未知'}'),'the inspection toolbar preserves the ${kind} sample size');
  qaAssert(!document.querySelector('.treemap-stage .treemap-tile'),'inspecting a ${kind} allocation sample adds no fabricated rectangle');
`);
for(const surface of ['map','list'])add('analyze-'+surface+'-sort-focus-path',`
  switchModule('analyze');
  STATE.treemap2D=[{name:'Zulu',path:'~/zulu',diskGb:8,isDir:true,childrenLoaded:true,children:[]},{name:'Alpha',path:'~/alpha',diskGb:5,isDir:true,childrenLoaded:true,children:[]},{name:'Middle',path:'~/middle',diskGb:2,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeLargeFiles=[];STATE.analyzeAggregated=null;renderModule();
  const selector='${surface==='map'?'.treemap-stage .treemap-tile':'.analysis-children .list-row'}';
  document.querySelector(selector+'[data-idx="0"]').focus({preventScroll:true});
  setAnalyzeSort('name');
  const focused=document.activeElement.closest('[data-idx]');
  const path=focused&&analysisVisibleTiles()[Number(focused.dataset.idx)]?.path;
  qaAssert(path==='~/zulu','sorting preserves the focused ${surface} path, actual '+path);
  for(const action of ${JSON.stringify(surface==='map'?['tile']:['toggleCollect'])}){
    const index=analysisVisibleTiles().findIndex(item=>item.path==='~/zulu');
    const row=document.querySelector(selector+'[data-idx="'+index+'"]');
    const button=action==='tile'?row:row.querySelector('[onclick*="'+action+'("]');
    qaAssert(button,'the intended ${surface} action is present before sorting');
    button.focus({preventScroll:true});
    setAnalyzeSort(STATE.analyzeSort==='name'?'size':'name');
    const active=document.activeElement,owner=active.closest('[data-idx]');
    const actual=owner&&analysisVisibleTiles()[Number(owner.dataset.idx)]?.path;
    qaAssert(actual==='~/zulu'&&(action==='tile'?active.classList.contains('treemap-tile'):active.getAttribute('onclick')?.includes(action+'(')),'sorting preserves the ${surface} '+action+' role for the same path, actual '+actual);
  }
`);
add('analyze-map-refold-focus-path',smallItemRefoldSetup+`
  const path='~/medium-6';
  const index=analysisVisibleTiles().findIndex(item=>item.path===path);
  const initial=document.querySelector('.treemap-stage .treemap-tile[data-idx="'+index+'"]');
  qaAssert(initial,'the intended path starts as a visible map tile');
  initial.focus({preventScroll:true});
  const style=document.createElement('style');style.textContent='#moduleStage .analysis-map{width:420px;max-width:100%}';document.head.appendChild(style);
  try{
    window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
    qaAssert(!document.querySelector('.treemap-stage .treemap-tile[data-idx="'+index+'"]')&&document.querySelector('.analysis-other-row[data-path="'+path+'"]'),'resize actually moves the focused path into small-item details');
    const focus=document.activeElement,small=focus.closest('.analysis-other-row[data-path]'),row=focus.closest('[data-idx]');
    const actual=small?.dataset.path||(row&&analysisVisibleTiles()[Number(row.dataset.idx)]?.path);
    qaAssert(actual===path,'refolding preserves focused map path, actual '+actual);
  }finally{style.remove();}
`);
add('analyze-visible-map-reveal-refold-inspect',`
  switchModule('analyze');
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:10,isDir:true,childrenLoaded:true,children:[]},...Array.from({length:20},(_,index)=>({name:'Medium '+index,path:'~/medium-'+index,diskGb:1,isDir:true,childrenLoaded:true,children:[]}))];
  STATE.analyzeAllocGb=30;STATE.analyzeAggregated=null;STATE.analyzeLargeFiles=[];renderModule();
  const path='~/medium-9';
  const button=document.querySelector('.treemap-tile[data-path="'+path+'"]');
  qaAssert(button&&button.getClientRects().length,'the map tile starts rendered; '+(document.querySelector('.treemap-tile[data-path="'+path+'"]')?.outerHTML||'no tile').slice(0,700)+' tiles '+document.querySelectorAll('.treemap-tile').length);
  button.click();button.focus({preventScroll:true});
  qaAssert(document.activeElement===button,'the visible map tile really receives focus');
  const rect=button.getBoundingClientRect();
  qaAssert(rect.width>0&&rect.height>0&&rect.left>=0&&rect.right<=innerWidth&&rect.top>=0&&rect.bottom<=innerHeight,'the focused map tile lies inside the viewport');
  const style=document.createElement('style');style.textContent='#moduleStage .analysis-map{width:300px;height:156px;max-width:100%}';document.head.appendChild(style);
  try{
    window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
    qaAssert(!document.querySelector('.treemap-tile[data-path="'+path+'"]'),'the original map path really becomes folded');
    const small=document.querySelector('.analysis-other-row[data-path="'+path+'"]');
    qaAssert(small&&small.querySelector('.analysis-other-path'),'the same small path has a readable inspect target');
    const focused=document.activeElement,owner=focused.closest('[data-path]');
    qaAssert(owner?.dataset.path===path&&focused.dataset.analyzeAction==='inspect','a tile that folds away falls back to inspecting the same path, actual '+focused.outerHTML.slice(0,200));
    for(const intent of ['new-focus','body-pointer']){
      style.remove();window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
      const next=document.querySelector('.treemap-tile[data-path="'+path+'"]');
      qaAssert(next&&next.getClientRects().length,'the same visible map tile is restored before the '+intent+' check');
      next.click();next.focus({preventScroll:true});
      qaAssert(document.activeElement===next,'the map tile is really visible and focused before '+intent);
      document.head.appendChild(style);
      let intended;
      if(intent==='new-focus'){
        intended=document.querySelector('.analysis-sort');intended.focus({preventScroll:true});
        qaAssert(document.activeElement===intended,'the user deliberately places focus on the sort control');
      }else{document.body.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}));document.activeElement.blur();}
      window.dispatchEvent(new Event('resize'));await new Promise(resolve=>setTimeout(resolve,180));
      qaAssert(intent==='new-focus'?document.activeElement.matches('.analysis-sort')&&document.activeElement.getAttribute('onclick')===intended.getAttribute('onclick'):document.activeElement===document.body,'cached map focus does not override '+intent+' after reflow; actual '+document.activeElement.outerHTML.slice(0,180));
    }
  }finally{style.remove();}
`);
Object.assign(scenarios.at(-1),{width:1800,height:1000});
const renderedTinyAreaSetup=`
  switchModule('analyze');
  STATE.treemap2D=[{name:'主目录',path:'~/main-rendered-area',diskGb:90,isDir:true,childrenLoaded:true,children:[]},{name:'小目录',path:'~/tiny-rendered-area',diskGb:0.001,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAllocGb=90.001;STATE.analyzeAggregated=null;STATE.analyzeLargeFiles=[];renderModule();
`;
const renderedTinyAreaCheck=`
  const map=document.querySelector('.treemap-stage'),tile=map.querySelector('[data-path="~/tiny-rendered-area"]');
  qaAssert(tile,'tiny map item preserves its actual identity');
  const bounds=tile.getBoundingClientRect();
  const expected={width:map.clientWidth*parseFloat(tile.style.width)/100,height:map.clientHeight*parseFloat(tile.style.height)/100};
  __qa.motionEvidence={actual:{width:bounds.width,height:bounds.height},expected};
  qaAssert(Math.abs(bounds.width-expected.width)<1&&Math.abs(bounds.height-expected.height)<1,'rendered tile respects measured dimensions, not padding minima: actual '+bounds.width+'x'+bounds.height+', expected '+expected.width+'x'+expected.height);
  qaAssert(document.querySelector('.analysis-children .list-row[data-path="~/tiny-rendered-area"] .row-name'),'a readable details target complements tiny honest map area');
`;
for(const width of [1380,880]){
  add(width===1380?'analyze-rendered-tiny-area':'analyze-rendered-narrow-area',renderedTinyAreaSetup+renderedTinyAreaCheck);
  Object.assign(scenarios.at(-1),{width,height:width===1380?860:620});
}
add('analyze-rendered-horizontal-tiny-area',renderedTinyAreaSetup+`
  const style=document.createElement('style');style.textContent='#moduleStage .analysis-map{width:450px;max-width:100%}';document.head.appendChild(style);
  try{
    renderModule();
    ${renderedTinyAreaCheck}
    qaAssert(expected.height<0.01&&expected.width>100,'the fixture actually creates a horizontal tiny strip');
  }finally{style.remove();}
`);
const smallNavigationSetup=`
  switchModule('analyze');
  STATE.treemap2D=[{name:'Main',path:'~/main',diskGb:10,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeLargeFiles=[];
  STATE.analyzeAggregated={count:2,sizeGb:0.009,items:[{name:'Sample Directory',path:'~/sample-directory',sizeGb:0.005,isDir:true},{name:'sample-notes.txt',path:'~/sample-notes.txt',sizeGb:0.004,isDir:false}]};renderModule();setAnalyzeTab('folded');
`;
add('analyze-aggregate-search-selection-roundtrip',smallNavigationSetup+`
  const path='~/sample-directory';
  document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path').click();
  handleSearch('Sample Directory');
  qaAssert(document.querySelector('.crumb-sel').title===path,'searching for the inspected sample keeps its toolbar identity');
  const index=analysisVisibleTiles().findIndex(item=>item.path===path);drillTile(index);
  const request=__qa.actions.filter(action=>action.action==='requestDiskAnalyze').at(-1);
  qaAssert(request?.path===path,'entering a searched aggregate directory requests that exact path');
  onNativeDiskAnalyze({requestId:request.requestId,path,scanStatus:'ok',totalSizeGb:0.005,treemap2D:[{name:'child.txt',path:path+'/child.txt',diskGb:0.005,isDir:false}],restricted:[],aggregated:{count:0,sizeGb:0,items:[]}});
  drillUp();
  qaAssert(document.querySelector('.crumb-sel').title===path&&document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path').getAttribute('aria-pressed')==='true','returning from the searched sample retains its identity');
  handleSearch('Sample Directory');handleSearch('');const clearedInput=document.querySelector('.crumb-sel').title;
  document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path').click();
  handleSearch('Sample Directory');clearAnalysisSearch();const clearedButton=document.querySelector('.crumb-sel').title;
  qaAssert(clearedInput===path&&clearedButton===path,'clearing a matching query preserves the inspected aggregate path; input '+clearedInput+', clear action '+clearedButton);
`);
add('analyze-small-keyboard-inspection-agreement',smallNavigationSetup+`
  const directory=document.querySelector('.analysis-other-row[data-path="~/sample-directory"] .analysis-other-path');
  const file=document.querySelector('.analysis-other-row[data-path="~/sample-notes.txt"] .analysis-other-path');
  directory.click();directory.focus();directory.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===file,'Down reaches the aggregate file');
  const downMatches=document.querySelector('.crumb-sel').title==='~/sample-notes.txt'&&file.getAttribute('aria-pressed')==='true';
  for(const key of [' ','Enter']){
    const event=new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true});file.dispatchEvent(event);if(!event.defaultPrevented)file.click();
    qaAssert(!__qa.actions.some(action=>action.action==='revealInFinder')&&document.querySelector('.crumb-sel').title==='~/sample-notes.txt','file '+key+' only inspects the focused path without Finder');
  }
  file.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowUp',bubbles:true,cancelable:true}));
  const upMatches=document.activeElement===directory&&document.querySelector('.crumb-sel').title==='~/sample-directory'&&directory.getAttribute('aria-pressed')==='true';
  qaAssert(downMatches&&upMatches,'arrow focus and inspection toolbar agree for each sample; Down '+downMatches+', Up '+upMatches);
`);
add('analyze-repeated-small-disclosure-focus',smallNavigationSetup+`
  STATE.treemap2D=[];renderModule();
  const name=document.querySelector('.analysis-other-row[data-path="~/sample-notes.txt"] .analysis-other-path');
  name.click();name.focus();name.scrollIntoView({block:'nearest'});
  qaAssert(document.activeElement===name,'a visible expanded row begins with focus');
  for(let repeat=0;repeat<2;repeat++){
    document.querySelector('.analysis-aggregate-only button').click();
    const current=document.querySelector('.analysis-other-row[data-path="~/sample-notes.txt"] .analysis-other-path');
    qaAssert(document.activeElement===current,'requesting already-open details preserves the current row focus; actual '+document.activeElement.outerHTML.slice(0,200));
  }
`);
add('analyze-tiny-basket-units',`
  switchModule('analyze');
  STATE.treemap2D=[{name:'小缓存',path:'~/tiny-basket',diskGb:0.000004096,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAllocGb=0.000004096;STATE.analyzeAggregated=null;STATE.analyzeLargeFiles=[];renderModule();
  toggleCollect(0);STATE.basketOpen=true;renderBasket();
  const basket=document.getElementById('basketBar');
  qaAssert(basket.querySelector('.basket-item b').textContent==='4.1 KB','a tiny basket item retains readable measured units');
  qaAssert(basket.querySelector('summary').textContent.includes('4.1 KB'),'basket total agrees with the tiny measured item');
  qaAssert(!basket.textContent.includes('0.00 GB'),'tiny nonzero measurements cannot round to zero GB');
  qaAssert(!__qa.actions.some(action=>action.action==='trashRealPaths'),'adding a draft never submits cleanup');
  basket.querySelector('.basket-item button').click();
  qaAssert(STATE.collected.length===0,'removal from the draft still works');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('analyze-unknown-basket-size',`
  switchModule('analyze');
  STATE.analyzeAggregated={count:1,items:[{name:'未测量目录',path:'~/unknown-basket',isDir:true}]};renderModule();
  const row=document.querySelector('.analysis-other-row[data-path="~/unknown-basket"]');
  qaAssert(row,'an unmeasured sample has a real draft target');
  row.querySelector('[data-analyze-action="collect"]').click();STATE.basketOpen=true;renderBasket();
  const basket=document.getElementById('basketBar');
  qaAssert(basket.querySelector('.basket-item b').textContent==='未知','unmeasured draft size must remain unknown');
  qaAssert(basket.querySelector('summary').textContent.includes('总量未知'),'an entirely unmeasured draft cannot show a measured total');
  qaAssert(!basket.textContent.includes('0.00 GB'),'missing measurements cannot masquerade as zero');
  qaAssert(!__qa.actions.some(action=>action.action==='trashRealPaths'),'an unmeasured draft is not cleanup authorization');
`);
add('analyze-mixed-basket-measurements',`
  switchModule('analyze');
  STATE.treemap2D=[{name:'已测缓存',path:'~/known-basket',diskGb:0.000004096,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAllocGb=0.000004096;STATE.analyzeAggregated={count:1,items:[{name:'未测量目录',path:'~/unknown-basket',isDir:true}]};
  STATE.analyzeLargeFiles=[];renderModule();toggleCollect(0);
  document.querySelector('.analysis-other-row[data-path="~/unknown-basket"] [data-analyze-action="collect"]').click();STATE.basketOpen=true;renderBasket();
  const basket=document.getElementById('basketBar'),summary=basket.querySelector('summary').textContent;
  qaAssert(summary.includes('已测 4.1 KB')&&summary.includes('1 项未测量'),'a mixed draft separates known allocation from unknown paths');
  qaAssert([...basket.querySelectorAll('.basket-item b')].map(item=>item.textContent).join('|')==='4.1 KB|未知','per-path measurements stay exact');
  qaAssert(!__qa.actions.some(action=>action.action==='trashRealPaths'),'draft totals do not submit cleanup');
`);
const microKeyboardSetup=`
  switchModule('analyze');
  STATE.treemap2D=[{name:'Main',path:'~/micro-main',diskGb:90,isDir:true,childrenLoaded:true,children:[]},{name:'Tiny',path:'~/micro-tiny',diskGb:0.000004096,isDir:true,childrenLoaded:true,children:[{name:'child.txt',path:'~/micro-tiny/child.txt',diskGb:0.000004096,isDir:false}]}];
  STATE.analyzeAllocGb=90.000004096;STATE.analyzeAggregated=null;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('children');
  const micro=document.querySelector('.treemap-tile[data-path="~/micro-tiny"]');
  const rect=micro.getBoundingClientRect();
  qaAssert(rect.width===0||rect.height===0,'the fixture really renders a zero-pixel micro map tile');
`;
add('analyze-zero-pixel-map-keyboard-target',microKeyboardSetup+`
  const main=document.querySelector('.treemap-tile[data-path="~/micro-main"]');
  const targets=[];
  for(const key of ['End','ArrowRight']){
    main.focus();qaAssert(document.activeElement===main,'the readable map starts with actual focus');
    main.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true}));
    const focused=document.activeElement,bounds=focused.getBoundingClientRect();
    targets.push({key,path:focused.closest('[data-path]')?.dataset.path,width:bounds.width,height:bounds.height});
  }
  __qa.motionEvidence={microTabIndex:micro.tabIndex,targets};
  qaAssert(micro.tabIndex<0&&targets.every(target=>target.path==='~/micro-tiny'&&target.width>=24&&target.height>=24),'zero-pixel map nodes leave Tab order and map keys reach the same readable path; tabindex '+micro.tabIndex+', actual '+JSON.stringify(targets));
`);
for(const mode of ['command-up','backspace'])add('analyze-micro-directory-return-'+mode,microKeyboardSetup+`
  const button=document.querySelector('.analysis-children .list-row[data-path="~/micro-tiny"]');
  button.focus();
  qaAssert(document.activeElement===button&&button.getBoundingClientRect().width>=24,'the readable tiny name really receives focus');
  button.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  qaAssert(STATE.drillStack.at(-1)?.path==='~/micro-tiny'&&document.activeElement.closest('[data-path]')?.dataset.path==='~/micro-tiny/child.txt','keyboard entry focuses the loaded child');
  document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'${mode==='command-up'?'ArrowUp':'Backspace'}',metaKey:${mode==='command-up'},bubbles:true,cancelable:true}));
  const focused=document.activeElement,bounds=focused.getBoundingClientRect();
  __qa.motionEvidence={path:focused.closest('[data-path]')?.dataset.path,action:focused.dataset.analyzeAction,width:bounds.width,height:bounds.height};
  qaAssert(!STATE.drillStack.length,'the parent scope is restored');
  qaAssert(focused.closest('[data-path]')?.dataset.path==='~/micro-tiny'&&bounds.width>=24&&bounds.height>=24,'return focuses the same readable tiny path instead of a zero-pixel tile; actual '+focused.outerHTML.slice(0,180)+', size '+bounds.width+'x'+bounds.height);
`);
add('analyze-aggregate-only-child-keyboard-focus',`
  switchModule('analyze');
  const path='~/SmallDirectory';
  STATE.treemap2D=[{name:'Main',path:'~/Main',diskGb:90,isDir:true,childrenLoaded:true,children:[]}];
  STATE.analyzeAggregated={count:1,sizeGb:0.000004096,items:[{name:'SmallDirectory',path,sizeGb:0.000004096,isDir:true}]};
  STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('folded');
  const name=document.querySelector('.analysis-other-row[data-path="'+path+'"] .analysis-other-path');
  name.focus();qaAssert(document.activeElement===name&&name.getBoundingClientRect().width>=24,'the unloaded small directory starts with readable keyboard focus');
  name.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  const request=__qa.actions.filter(action=>action.action==='requestDiskAnalyze').at(-1);
  qaAssert(request?.path===path&&STATE.drillStack.at(-1)?.path===path,'keyboard entry requests the intended unloaded directory');
  onNativeDiskAnalyze({requestId:request.requestId,path,scanStatus:'ok',totalSizeGb:0.000004096,treemap2D:[],restricted:[],aggregated:{count:1,sizeGb:0.000004096,items:[{name:'fixture.bin',path:path+'/fixture.bin',sizeGb:0.000004096,isDir:false}]}});
  const child=STATE.drillStack.at(-1),focused=document.activeElement,bounds=focused.getBoundingClientRect();
  qaAssert(child?.childrenLoaded&&child.children.length===0&&document.querySelector('.analysis-other-row[data-path="'+path+'/fixture.bin"]'),'the loaded child really contains only aggregate samples');
  __qa.motionEvidence={focused:focused.outerHTML.slice(0,200),width:bounds.width,height:bounds.height};
  qaAssert(!!focused.closest('.treemap-stage,.analysis-small-items')&&bounds.width>=24&&bounds.height>=24,'aggregate-only child entry focuses readable child content instead of an earlier breadcrumb; actual '+focused.outerHTML.slice(0,200));
`);
const smallReturnKeyboardSetup=`
  switchModule('analyze');
  const path='~/return-small';
  const children=[{name:'Large child',path:path+'/large.bin',diskGb:0.004,isDir:false},{name:'Tiny note',path:path+'/note.txt',diskGb:0.000004096,isDir:false}];
  STATE.treemap2D=[{name:'Main',path:'~/return-main',diskGb:90,isDir:true,childrenLoaded:true,children:[]},{name:'Small parent',path,diskGb:0.004004096,isDir:true,childrenLoaded:true,children}];
  STATE.analyzeAllocGb=90.004004096;STATE.analyzeAggregated=null;STATE.analyzeLargeFiles=[];renderModule();setAnalyzeTab('children');
  const parent=document.querySelector('.analysis-children .list-row[data-path="'+path+'"]');
  parent.focus();qaAssert(document.activeElement===parent&&parent.getBoundingClientRect().width>=24,'the parent small directory has actual readable focus');
  parent.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));
  qaAssert(STATE.drillStack.at(-1)?.path===path&&currentTiles().length===2,'keyboard entry loads the intended small directory');
  const input=document.getElementById('searchInput');input.focus();
  const edit=new KeyboardEvent('keydown',{key:'Backspace',bubbles:true,cancelable:true});input.dispatchEvent(edit);
  qaAssert(!edit.defaultPrevented&&STATE.drillStack.at(-1)?.path===path,'Backspace inside the search input stays native editing');
  const note=document.querySelector('.analysis-children .list-row[data-path="'+path+'/note.txt"]');
  note.click();note.focus();
  qaAssert(document.activeElement===note&&note.getBoundingClientRect().height>=24,'the loaded child small-item name really receives readable focus');
  for(const key of [' ','Enter']){
    const event=new KeyboardEvent('keydown',{key,bubbles:true,cancelable:true});note.dispatchEvent(event);if(!event.defaultPrevented)note.click();
    qaAssert(document.querySelector('.crumb-sel').title===path+'/note.txt'&&STATE.drillStack.at(-1)?.path===path,'file '+key+' inspects without changing the directory');
  }
`;
for(const key of ['Backspace','ArrowLeft'])add('analyze-small-item-return-'+(key==='Backspace'?'backspace':'left'),smallReturnKeyboardSetup+`
  const event=new KeyboardEvent('keydown',{key:'${key}',bubbles:true,cancelable:true});note.dispatchEvent(event);
  const focused=document.activeElement,bounds=focused.getBoundingClientRect();
  qaAssert(event.defaultPrevented&&!STATE.drillStack.length,'${key} from small-item details returns to the parent layer');
  qaAssert(focused.closest('[data-path]')?.dataset.path===path&&bounds.width>=24&&bounds.height>=24,'return keeps focus on the same readable parent path');
  qaAssert(document.querySelector('.crumb-sel').title===path,'parent inspection matches the directory just left');
  qaAssert(!STATE.collected.length&&!__qa.actions.some(action=>['revealInFinder','trashRealPaths'].includes(action.action)),'return navigation cannot open Finder or authorize cleanup');
  __qa.motionEvidence={key:'${key}',path:focused.closest('[data-path]')?.dataset.path,width:bounds.width,height:bounds.height};
`);
const compactTraySetup=`
  document.body.classList.add('menubar-mode');renderMenuBarTrayContent();
  const tray=document.getElementById('menubarTrayShell');
`;
const trayScenarioStart=scenarios.length;
const trayGraphSetup=compactTraySetup+`
  STATE.cpuSeries=[4,24,65,20];
  STATE.memUsedGb=12;STATE.memTotalGb=32;
  STATE.memory={appGb:8,wiredGb:3,compressedGb:1,cacheGb:5};
  STATE.hw.netRxSeries=[0,1048576,524288,2097152];STATE.hw.netTxSeries=[0,131072,262144,131072];
  renderMenuBarTrayContent();
`;
for(const theme of ['dark','light'])add('tray-graphs-'+theme,trayGraphSetup+`
  document.documentElement.dataset.theme='${theme}';
  const cpu=tray.querySelector('[data-tray-metric="cpu"] .tray-chart');
  qaAssert(cpu?.querySelector('svg')&&cpu.getAttribute('aria-label').includes('刻度 0 至 100%'),'CPU keeps a fixed 0–100% chart scale');
  qaAssert(cpu.textContent.includes('峰值 65.0%')&&cpu.textContent.includes('4 次采样'),'CPU caption shows the real peak and sample count');
  qaAssert(cpu.getBoundingClientRect().width>120&&cpu.getAttribute('aria-label').includes('最近 4 次采样'),'the graph uses the card width and discloses real sample count');
  qaAssert([...tray.querySelectorAll('.tray-chart-caption')].every(caption=>caption.scrollWidth<=caption.clientWidth+1),'chart captions fit their card without clipping');
  const network=tray.querySelector('[data-tray-metric="network"] .tray-chart');
  qaAssert(network?.querySelectorAll('svg').length===2&&network.textContent.includes('峰值 2.0 MB/s'),'upload and download share one honest throughput scale labelled by its real peak');
  const memory=tray.querySelector('[data-tray-metric="memory"] .tray-capacity');
  qaAssert(memory?.querySelectorAll('[data-memory-part]').length===4,'memory displays app, wired, compressed and remaining capacity without adding cached files twice');
  const widths=[...memory.querySelectorAll('[data-memory-part]')].map(part=>parseFloat(part.style.width));
  qaAssert(Math.abs(widths.reduce((sum,value)=>sum+value,0)-100)<0.01&&Math.abs(widths[0]-25)<0.01,'memory segments preserve their native proportions');
  const disk=tray.querySelector('[data-tray-metric="disk"] .tray-capacity');
  qaAssert(disk?.getAttribute('aria-label').includes('已用')&&disk.querySelector('[data-capacity-used]'),'disk capacity has a measured usage bar');
  qaAssert(tray.scrollHeight<=tray.clientHeight+1,'graphs and cleaning tools fit the shipped popup');
`);
add('tray-graph-first-and-missing-samples',compactTraySetup+`
  STATE.cpuSeries=[24];STATE.hw.netRxSeries=[1024];STATE.hw.netTxSeries=[0];renderMenuBarTrayContent();
  qaAssert(!tray.querySelector('[data-tray-metric="cpu"] .tray-chart svg')&&tray.querySelector('[data-tray-metric="cpu"] .tray-chart').textContent.includes('下一次'),'one reading is not stretched into a fabricated history');
  qaAssert(!tray.querySelector('[data-tray-metric="network"] .tray-chart svg'),'one network reading is not a trend');
  onNativeRealTelemetry({...structuredClone(qaTelemetry),memory:null,net:null,cpu:null});
  qaAssert(!STATE.memory&&!tray.querySelector('[data-memory-part]'),'unavailable breakdown removes stale memory segments');
  qaAssert(!tray.querySelector('[data-tray-metric="network"] .tray-chart svg'),'a disappearing source removes its old graph');
`);
add('tray-telemetry-disk-capacity-before-scan',compactTraySetup+`
  STATE.totalDiskGb=null;STATE.scanCompleted=false;
  onNativeRealTelemetry({...structuredClone(qaTelemetry),freeDiskGb:25,totalDiskGb:100});
  const disk=tray.querySelector('[data-tray-metric="disk"]');
  qaAssert(disk.textContent.includes('100.0 GB')&&Math.abs(parseFloat(disk.querySelector('[data-capacity-used]').style.width)-75)<0.01,'disk visualization is available before a slow full file scan');
`);
add('tray-quick-clean-frozen-preview',`
  switchModule('status');__qa.actions=[];openQuickClean();
  qaAssert(STATE.module==='clean'&&document.getElementById('confirmModal').classList.contains('open'),'one quick entry opens the existing exact-path review in the main window');
  qaAssert(!__qa.actions.some(action=>action.action==='trashRealPaths'),'quick entry never deletes before confirmation');
  const preview=document.getElementById('confirmModalItemsList').textContent;
  qaAssert(preview.includes('/one')&&!preview.includes('SessionStore'),'quick cleaning reuses selected cache paths and excludes protected assets');
  STATE.cleanGroups[0].subpaths.push({...structuredClone(STATE.cleanGroups[0].subpaths[0]),path:'~/Library/Caches/AddedAfterPreview',checked:true,tier:'safe',sizeGb:1});
  executeConfirmedClean();
  qaAssert(__qa.actions.find(action=>action.action==='trashRealPaths').paths.every(path=>!path.includes('AddedAfterPreview')),'background changes cannot enlarge the frozen quick-clean plan');
`);
add('memory-maintenance-consent-and-receipt',`
  window.isNativeMacApp=true;
  __qa.actions=[];openMemoryMaintenance();
  const modal=document.getElementById('memoryModal');
  qaAssert(modal.classList.contains('open')&&modal.textContent.includes('不会清空应用')&&modal.textContent.includes('变慢'),'file-cache scope and performance risk appear before authorization');
  qaAssert(!__qa.actions.some(action=>action.action==='releaseFileCache'),'opening memory tools is read-only');
  releaseFileCache();releaseFileCache();
  const requests=__qa.actions.filter(action=>action.action==='releaseFileCache');
  qaAssert(requests.length===1&&requests[0].confirmed===true&&requests[0].requestId,'explicit confirmation posts one correlated native request');
  qaAssert(document.getElementById('memoryReleaseBtn').disabled&&STATE.memoryMaintenance.pending,'the command waits for a real receipt');
  onNativeCachePurgeResult({requestId:'obsolete',ok:true});
  qaAssert(STATE.memoryMaintenance.pending,'an old receipt cannot complete a new request');
  onNativeCachePurgeResult({requestId:requests[0].requestId,ok:true});
  qaAssert(!STATE.memoryMaintenance.pending&&document.getElementById('memoryMaintenanceStatus').textContent.includes('命令已完成'),'only a correlated native success completes the command');
  qaAssert(!/已释放.*(?:MB|GB)|加速.*%/.test(modal.textContent),'completion does not fabricate freed application memory or speed gains');
`);
for(const outcome of ['cancelled','failed','unknown'])add('memory-maintenance-'+outcome,`
  window.isNativeMacApp=true;
  openMemoryMaintenance();__qa.actions=[];releaseFileCache();
  const request=__qa.actions.find(action=>action.action==='releaseFileCache');
  onNativeCachePurgeResult({requestId:request.requestId,ok:false,cancelled:${outcome==='cancelled'},unknown:${outcome==='unknown'},error:'隔离测试：管理员未授权'});
  const status=document.getElementById('memoryMaintenanceStatus').textContent;
  qaAssert(!STATE.memoryMaintenance.pending&&!status.includes('命令已完成'),'${outcome} cannot be reported as success');
  qaAssert(${outcome==='unknown'}===document.getElementById('memoryReleaseBtn').disabled,'only an unknown running outcome refuses another purge');
  ${outcome==='cancelled'?"qaAssert(status.includes('已取消'),'native authorization cancellation is distinct from failure');":outcome==='unknown'?"qaAssert(status.includes('状态未知'),'timeout cannot claim the privileged command stopped');":"qaAssert(status.includes('管理员未授权'),'the actual native failure stays visible');"}
`);
add('memory-maintenance-lost-receipt',`
  window.isNativeMacApp=true;
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===100000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    openMemoryMaintenance();__qa.actions=[];releaseFileCache();
    const request=__qa.actions.find(action=>action.action==='releaseFileCache');
    qaAssert(typeof timeout==='function','transport receipt waiting is bounded');timeout();releaseFileCache();
    qaAssert(STATE.memoryMaintenance.unknown&&document.getElementById('memoryReleaseBtn').disabled&&__qa.actions.filter(action=>action.action==='releaseFileCache').length===1,'missing receipt neither invents failure nor retries a possibly running privileged command');
    onNativeCachePurgeResult({requestId:request.requestId,ok:false,cancelled:true});
    qaAssert(!STATE.memoryMaintenance.pending&&!STATE.memoryMaintenance.unknown&&!document.getElementById('memoryReleaseBtn').disabled,'a late authoritative cancellation releases the uncertain UI state');
  }finally{window.setTimeout=original;clearTimeout(window._memoryMaintenanceTimer);}
`);
add('memory-maintenance-web-preview',`
  __qa.actions=[];openMemoryMaintenance();releaseFileCache();
  qaAssert(document.getElementById('memoryReleaseBtn').disabled&&!__qa.actions.some(action=>action.action==='releaseFileCache'),'a standalone web preview cannot request privileged maintenance');
  qaAssert(document.getElementById('memoryMaintenanceStatus').textContent.includes('网页预览不会运行系统命令'),'preview limitation is explained rather than pretending to clean RAM');
`);
add('memory-maintenance-keyboard',`
  window.isNativeMacApp=true;
  const opener=document.querySelector('.header-controls button');opener.focus();openMemoryMaintenance();
  await new Promise(resolve=>setTimeout(resolve,60));
  qaAssert(document.activeElement===document.getElementById('memoryCloseBtn')&&document.querySelector('.app-shell').inert,'confirmation defaults to Cancel and isolates the background');
  document.getElementById('memoryReleaseBtn').focus();document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===document.querySelector('#memoryModal .memory-panel-footer > button'),'Tab stays inside the protected confirmation dialog');
  document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));
  await new Promise(resolve=>setTimeout(resolve,60));
  qaAssert(!document.getElementById('memoryModal').classList.contains('open')&&!document.querySelector('.app-shell').inert&&document.activeElement===opener,'Escape closes the dialog and restores the original focus without executing');
`);
add('modal-hidden-native-handoff',`
  const style=[...document.querySelectorAll('style')].find(node=>node.textContent.includes('*,*::before,*::after{transition:none!important;animation:none!important}'));
  const originalStyle=style.textContent;style.textContent=originalStyle.replace('animation:none!important','');
  Object.defineProperty(document,'visibilityState',{configurable:true,get:()=> 'hidden'});document.documentElement.dataset.inputMode='pointer';
  try{
    openMemoryMaintenance();await Promise.resolve();
    const sheet=document.querySelector('#memoryModal .modal-sheet');
    qaAssert(getComputedStyle(sheet).opacity==='1'&&!sheet.getAnimations().length,'hidden native handoff must present the final dialog, not freeze at zero opacity');
    qaAssert(document.getElementById('memoryModal').classList.contains('open'),'skipping unavailable motion still opens the confirmation');
  }finally{delete document.visibilityState;style.textContent=originalStyle;closeMemoryMaintenance();}
`);
add('modal-hidden-motion-cancellation',`
  const style=[...document.querySelectorAll('style')].find(node=>node.textContent.includes('*,*::before,*::after{transition:none!important;animation:none!important}'));
  const originalStyle=style.textContent;style.textContent=originalStyle.replace('animation:none!important','');
  let visibility='visible';Object.defineProperty(document,'visibilityState',{configurable:true,get:()=>visibility});document.documentElement.dataset.inputMode='pointer';
  try{
    openMemoryMaintenance();await Promise.resolve();
    const sheet=document.querySelector('#memoryModal .modal-sheet');getComputedStyle(sheet).opacity;
    const animation=sheet.getAnimations()[0];qaAssert(animation,'a visible pointer entry keeps its authored motion');animation.pause();animation.currentTime=40;
    visibility='hidden';document.dispatchEvent(new Event('visibilitychange'));
    qaAssert(!sheet.getAnimations().length&&getComputedStyle(sheet).opacity==='1','hiding a dialog cancels its entrance into the visible final state');
    closeMemoryMaintenance();await Promise.resolve();visibility='visible';openMemoryMaintenance();await Promise.resolve();getComputedStyle(sheet).opacity;
    qaAssert(sheet.getAnimations().length===1,'a later visible entry can animate again without replaying a hidden entrance');
  }finally{delete document.visibilityState;style.textContent=originalStyle;closeMemoryMaintenance();}
`);
for(const theme of ['light','dark'])add('tray-essentials-'+theme,compactTraySetup+`
  document.documentElement.dataset.theme='${theme}';
  qaAssert(tray.querySelectorAll('[data-tray-metric]').length===4,'the compact tray contains only four core metric groups');
  qaAssert(tray.querySelector('[data-tray-metric="cpu"]').textContent.includes('24.0%'),'CPU uses the real sample');
  qaAssert(tray.querySelector('[data-tray-metric="memory"]').textContent.includes('内存压力')&&tray.querySelector('[data-tray-metric="memory"]').textContent.includes('正常'),'native memory pressure is explicit');
  qaAssert(tray.querySelector('[data-tray-metric="disk"]').textContent.includes('128.0 GB'),'disk shows available allocation');
  qaAssert(tray.querySelector('[data-tray-metric="network"]').textContent.includes('1.0 MB/s')&&tray.querySelector('[data-tray-metric="network"]').textContent.includes('128.0 KB/s'),'both network rates have their units');
  qaAssert(tray.querySelector('.tray-battery').textContent.includes('80%'),'battery is a small conditional summary');
  qaAssert(!/最大容量|循环.*次|状态 \d+.*100|已勾选清理量/.test(tray.textContent),'diagnostics and synthetic scores do not crowd the overview');
  const controls=[...tray.querySelectorAll('button')];
  qaAssert(controls.every(button=>{const rect=button.getBoundingClientRect();return rect.width>=24&&rect.height>=24&&rect.left>=0&&rect.right<=innerWidth+1;}),'tools are reachable and fit the popup');
  qaAssert(tray.scrollHeight<=tray.clientHeight+1,'the regular overview fits without clipping controls');
`);
add('tray-desktop-and-unavailable',compactTraySetup+`
  STATE.power={thermal:'nominal',memPressure:'normal'};renderMenuBarTrayContent();
  qaAssert(!tray.querySelector('.tray-battery')&&!tray.textContent.includes('已勾选清理量'),'desktop hardware does not get a filler battery card');
  STATE=structuredClone(DEFAULT_STATE);STATE.hw={};STATE.power={};renderMenuBarTrayContent();
  qaAssert([...tray.querySelectorAll('[data-tray-metric]')].every(metric=>metric.textContent.includes('未知')),'missing readings remain unknown');
  qaAssert(tray.querySelector('.tray-updated').textContent==='等待采样','an empty startup does not claim a live update');
  qaAssert(tray.querySelector('.tray-empty')&&!tray.textContent.includes('0.0%'),'no process sample is distinct from idle utilization');
  qaAssert(!tray.textContent.includes('undefined'),'the compact menu also handles absent CPU');
`);
add('tray-idle-rates-and-pressure',compactTraySetup+`
  const idle=structuredClone(qaTelemetry);idle.cpu=0;idle.net={rxBytesPerSec:0,txBytesPerSec:0};idle.memUsedGb=30;idle.memTotalGb=32;
  onNativeRealTelemetry(idle);
  qaAssert(tray.querySelector('[data-tray-metric="cpu"]').textContent.includes('0.0%'),'measured idle CPU remains zero');
  qaAssert(tray.querySelector('[data-tray-metric="network"]').textContent.split('0.0 B/s').length===3,'measured idle network is not an unsupported sensor');
  qaAssert(tray.querySelector('[data-tray-metric="memory"] .tray-metric-value').textContent==='正常','high memory occupancy does not invent pressure');
  idle.power.memPressure='warn';onNativeRealTelemetry(idle);
  qaAssert(tray.querySelector('[data-tray-metric="memory"] .tray-metric-value').textContent==='偏高','native pressure warnings have priority');
  delete idle.net;onNativeRealTelemetry(idle);
  qaAssert(tray.querySelector('[data-tray-metric="network"]').textContent.includes('本机不提供速率'),'a disappearing network source replaces its old reading');
`);
add('tray-safe-detail-routes',compactTraySetup+`
  const routes={cpu:'menubarOpenStatus',memory:'menubarOpenStatus',disk:'menubarOpenAnalyze',network:'menubarOpenStatus',status:'menubarOpenStatus',clean:'menubarOpenClean','memory-tools':'menubarOpenMemory',doctor:'menubarOpenDoctor',main:'menubarShowMain',quit:'menubarQuit'};
  for(const [key,action] of Object.entries(routes)){
    __qa.actions=[];tray.querySelector('[data-tray-key="'+key+'"]').click();
    qaAssert(__qa.actions.length===1&&__qa.actions[0].action===action,'each '+key+' tool posts one intended route');
  }
  __qa.actions=[];tray.querySelector('[data-tray-key="process:8123"]').click();
  qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='menubarOpenStatus'&&__qa.actions[0].query==='8123','a process opens its PID in the main status view');
  qaAssert(!tray.querySelector('[onclick*="armKill"]')&&!tray.querySelector('[onclick*="trashRealPaths"]'),'the compact tray has no destructive shortcuts');
  qaAssert(!document.getElementById('confirmModal').classList.contains('open')&&!document.getElementById('doctorModal').classList.contains('open'),'detail routes do not create dialogs inside the tray');
`);
add('tray-refresh-tool-focus',compactTraySetup+`
  const doctor=tray.querySelector('[data-tray-key="doctor"]');doctor.focus();
  qaAssert(document.activeElement===doctor,'the tool starts with actual keyboard focus');
  const update=structuredClone(qaTelemetry);update.cpu=42;onNativeRealTelemetry(update);
  qaAssert(tray.querySelector('[data-tray-key="doctor"]')===doctor&&document.activeElement===doctor,'sampling preserves the focused tool and its node identity');
  qaAssert(tray.querySelector('[data-tray-metric="cpu"]').textContent.includes('42.0%'),'preserving focus does not freeze readings');
`);
add('tray-refresh-process-identity',compactTraySetup+`
  const process=tray.querySelector('[data-tray-key="process:8123"]');process.focus();
  const update=structuredClone(qaTelemetry);update.processes.reverse();update.processes[0].cpu=50;onNativeRealTelemetry(update);
  qaAssert(tray.querySelector('[data-tray-key="process:8123"]')===process&&document.activeElement===process,'process reorder keeps the same PID button and focus');
  __qa.actions=[];process.click();qaAssert(__qa.actions[0].query==='8123','the retained process button cannot act on the new row index');
  update.processes=update.processes.filter(item=>item.pid!==8123);onNativeRealTelemetry(update);
  qaAssert(document.activeElement===tray.querySelector('[data-tray-key="status"]'),'a removed process falls back to all processes, not a different PID');
`);
add('tray-preventsleep-native-receipt',compactTraySetup+`
  STATE.caffeinateActive=false;STATE.caffeinatePending=null;renderMenuBarTrayContent();__qa.actions=[];
  tray.querySelector('[data-tray-key="caffeinate"]').click();
  qaAssert(!STATE.caffeinateActive&&STATE.caffeinatePending===true,'a request does not masquerade as native activation');
  qaAssert(tray.querySelector('[data-tray-key="caffeinate"]').disabled&&tray.querySelector('[data-tray-key="caffeinate"]').getAttribute('aria-checked')==='false','pending state stays unchecked and prevents repeated requests');
  toggleCaffeinateState();qaAssert(__qa.actions.length===1,'rapid toggles cannot overlap native requests');
  onNativeCaffeinate({active:false});qaAssert(!STATE.caffeinateActive&&STATE.caffeinatePending===null&&!tray.querySelector('[data-tray-key="caffeinate"]').disabled,'failure keeps the confirmed off state and allows retry');
  toggleCaffeinateState();onNativeCaffeinate({active:true});
  qaAssert(STATE.caffeinateActive&&tray.querySelector('[data-tray-key="caffeinate"]').getAttribute('aria-checked')==='true','only the native receipt turns the switch on');
  onNativeCaffeinate({active:false});qaAssert(!STATE.caffeinateActive&&tray.querySelector('[data-tray-key="caffeinate"]').getAttribute('aria-checked')==='false','automatic native expiry turns the switch off');
  qaAssert(__qa.actions.every(action=>action.action==='toggleCaffeinate'),'switch interaction cannot issue cleanup or process termination');
`);
add('tray-preventsleep-timeout',compactTraySetup+`
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===15000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    STATE.caffeinateActive=false;STATE.caffeinatePending=null;toggleCaffeinateState();
    qaAssert(typeof timeout==='function','native confirmation has a bounded wait');timeout();
    qaAssert(!STATE.caffeinateActive&&STATE.caffeinatePending===null&&!tray.querySelector('[data-tray-key="caffeinate"]').disabled,'timeout keeps confirmed state and releases the pending control');
  }finally{window.setTimeout=original;clearTimeout(window._caffeinateTimer);}
`);
add('tray-compact-height-scroll',compactTraySetup+`
  const update=structuredClone(qaTelemetry);update.processes.push({pid:8333,name:'Very long application process title for compact layout',cpu:8,memory_mb:2048,path:'/Applications/Third.app'});onNativeRealTelemetry(update);
  tray.scrollTop=tray.scrollHeight;const before=tray.scrollTop;
  const quit=tray.querySelector('[data-tray-key="quit"]');quit.focus({preventScroll:true});
  onNativeRealTelemetry(update);
  qaAssert(Math.abs(tray.scrollTop-before)<2&&document.activeElement===quit,'a shorter popup preserves scrolling and tool focus');
  const bounds=quit.getBoundingClientRect();qaAssert(bounds.top>=0&&bounds.bottom<=innerHeight+1,'footer tools stay reachable inside the smaller height');
`);
const trayHeight=588;
for(const scenario of scenarios.slice(trayScenarioStart))Object.assign(scenario,{width:380,height:trayHeight});
Object.assign(scenarios.at(-1),{width:356,height:480});
for(const scenario of scenarios.slice(trayScenarioStart).filter(item=>item.name.startsWith('memory-')||item.name.startsWith('modal-hidden-')||item.name==='tray-quick-clean-frozen-preview'))Object.assign(scenario,{width:880,height:620});
const newTrayStart=scenarios.length;
const trayThreeProcesses=compactTraySetup+`
  const sample=structuredClone(qaTelemetry);sample.processes.push({pid:8333,name:'Canvas Renderer',cpu:8,memory_mb:2048,path:'/Applications/Canvas Renderer.app'});onNativeRealTelemetry(sample);
`;
for(const theme of ['light','dark'])add('tray-top-processes-'+theme,trayThreeProcesses+`
  document.documentElement.dataset.theme='${theme}';
  const head=tray.querySelector('.tray-proc-head span');
  qaAssert(head.textContent==='CPU 占用最高的进程'&&!tray.textContent.includes('单核 100%')&&head.title.includes('单核 100%'),'the header is plain and keeps the per-core note as a hint');
  qaAssert(tray.querySelector('.tray-proc-list').getAttribute('aria-label').includes('单核 100%'),'the per-core note is also available to assistive technology');
  const rows=[...tray.querySelectorAll('.tray-proc')];
  qaAssert(rows.length===3&&rows.every(row=>row.querySelector('.tray-proc-chev')),'three processes are listed and each row has a chevron');
  const lastBottom=tray.lastElementChild.getBoundingClientRect().bottom;qaAssert(tray.scrollHeight<=tray.clientHeight+1&&lastBottom<=innerHeight,'three processes fit the shipped popup without scrolling; '+tray.scrollHeight+'/'+tray.clientHeight+' last bottom '+lastBottom);
  qaAssert(rows[0].textContent.includes('384 MiB')&&rows[1].textContent.includes('2.0 GiB')&&rows[2].textContent.includes('128 MiB'),'process memory is labelled MiB, or GiB above 1024');
  switchModule('status');
  qaAssert(document.querySelector('#procTbody').textContent.includes('384 MiB')&&!document.querySelector('#procTbody').textContent.includes(' MB'),'the status table uses the same unit label as the tray');
`);
add('tray-memory-legend-swatch',compactTraySetup+`
  STATE.memUsedGb=12;STATE.memTotalGb=32;STATE.memory={appGb:8,wiredGb:3,compressedGb:1,cacheGb:5};renderMenuBarTrayContent();
  const legend=[...tray.querySelectorAll('[data-tray-metric="memory"] .tray-memory-legend')];
  qaAssert(legend.length===4&&legend.every(item=>item.querySelector('i')),'every memory legend entry has a swatch');
  qaAssert(legend.at(-1).textContent==='剩余'&&legend.at(-1).querySelector('i').style.background.includes('--border-strong'),'the remaining swatch uses the empty track colour');
`);
add('tray-follows-app-theme',compactTraySetup+`
  const original={theme:document.documentElement.dataset.theme,popover:window.isMenuBarPopover,render:window.renderMenuBarTrayContent};
  let renders=0;window.renderMenuBarTrayContent=()=>{renders++;original.render();};
  try{
    window.isMenuBarPopover=true;
    onNativeTheme('dark');
    qaAssert(document.documentElement.dataset.theme==='dark'&&renders===1,'a pushed dark theme restyles and re-renders the popover');
    const dark=getComputedStyle(tray).backgroundColor;
    onNativeTheme('light');
    qaAssert(document.documentElement.dataset.theme==='light'&&renders===2&&getComputedStyle(tray).backgroundColor!==dark,'a pushed light theme follows even after dark');
    onNativeTheme('system');
    qaAssert(document.documentElement.dataset.theme===(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'),'system falls back to the media query');
    window.isMenuBarPopover=false;renders=0;onNativeTheme('dark');
    qaAssert(renders===0,'the main window ignores pushes meant for the popover');
  }finally{window.renderMenuBarTrayContent=original.render;window.isMenuBarPopover=original.popover;onNativeTheme('system');document.documentElement.dataset.theme=original.theme;}
`);
add('tray-releases-focus-when-hidden',compactTraySetup+`
  const card=tray.querySelector('[data-tray-key="cpu"]');card.focus();
  qaAssert(document.activeElement===card,'the clicked card holds focus');
  window.dispatchEvent(new Event('blur'));
  qaAssert(!tray.contains(document.activeElement),'losing key status releases the highlighted card');
  card.focus();document.body.classList.remove('menubar-mode');window.dispatchEvent(new Event('blur'));
  qaAssert(document.activeElement===card,'the main window keeps focus when it blurs');
  card.blur();document.body.classList.add('menubar-mode');
`);
add('quick-clean-nothing-selected-hint',`
  onNativeRealScanComplete(structuredClone(qaFixture));
  STATE.cleanGroups.forEach(group=>{group.checked=false;});
  switchModule('status');openQuickClean();
  const toast=document.getElementById('toastBanner');
  qaAssert(STATE.module==='clean'&&!document.getElementById('confirmModal').classList.contains('open'),'quick clean opens the list without a review sheet when nothing is selected');
  qaAssert(toast.style.display==='block'&&toast.textContent.includes('尚未勾选清理项，请先在列表中勾选要清理的项目'),'the user gets a calm in-page hint instead of silence');
`);
for(const scenario of scenarios.slice(newTrayStart).filter(item=>item.name.startsWith('tray-')))Object.assign(scenario,{width:380,height:trayHeight});
add('runtime-raw-launch-guidance',`
  openRuntimePanel();
  const request=__qa.actions.find(action=>action.action==='requestRuntimeStatus');
  qaAssert(request?.requestId,'the runtime panel asks the native process for evidence');
  onNativeRuntimeStatus({requestId:request.requestId,packaged:false,appPath:'/tmp/go-build123/b001/exe/mole-studio',executablePath:'/tmp/go-build123/b001/exe/mole-studio',identifier:'',version:'dev',menuBarRegistered:true,readAccess:{passed:null,detail:'本机没有可判定目录'}});
  qaAssert(!document.getElementById('runtimeBanner').hidden,'a raw launch has a visible diagnostic banner');
  qaAssert(document.getElementById('runtimePanelBody').textContent.includes('Ctrl-C')&&document.getElementById('runtimePanelBody').textContent.includes('未绑定 Bundle ID'),'raw identity has a concrete app-bundle recovery path');
  qaAssert(document.getElementById('runtimePanelBody').textContent.includes('尚不能判定'),'unknown read access cannot claim permission');
  qaAssert(!__qa.actions.some(action=>['trashRealPaths','killProcess','toggleCaffeinate'].includes(action.action)),'diagnostics do not mutate user resources');
  closeRuntimePanel();
`);
add('runtime-packaged-permission-guidance',`
  openRuntimePanel();const request=__qa.actions.find(action=>action.action==='requestRuntimeStatus');
  onNativeRuntimeStatus({requestId:request.requestId,packaged:true,appPath:'/Applications/orbit.app',identifier:'com.mymoles.mole-studio',version:'0.1.0',menuBarRegistered:true,readAccess:{passed:false,detail:'受保护目录读取被拒绝'}});
  const body=document.getElementById('runtimePanelBody');
  qaAssert(body.textContent.includes('/Applications/orbit.app')&&body.textContent.includes('读取被拒绝'),'the guide identifies the exact app and actual access failure');
  qaAssert(body.textContent.includes('程序不能自动授予')&&body.textContent.includes('仅接入 Quick Look'),'permissions and implemented maintenance limits are honest');
  for(const pane of ['fullDiskAccess','filesAndFolders','appManagement']){
    __qa.actions=[];[...body.querySelectorAll('button')].find(button=>button.getAttribute('onclick').includes(pane)).click();
    qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='openPrivacySettings'&&__qa.actions[0].pane===pane,'each permission opens its exact settings pane');
  }
  __qa.actions=[];body.querySelector('[onclick="revealCurrentApp()"]').click();qaAssert(__qa.actions[0].action==='revealAppBundle','the guide can locate the exact native application');
  requestRuntimeCheck();onNativeRuntimeStatus({requestId:STATE.runtimeRequestId,packaged:true,appPath:'/Applications/orbit.app',identifier:'com.mymoles.mole-studio',version:'0.1.0',menuBarRegistered:true,readAccess:{passed:true,detail:'可读取受保护目录'}});
  qaAssert(document.getElementById('runtimeBanner').hidden,'verified read access clears the warning without altering grants');
  closeRuntimePanel();
`);
add('runtime-header-tools-reachable',`
  document.querySelector('.header-controls [onclick="openRuntimePanel()"]').click();
  qaAssert(document.getElementById('runtimeModal').classList.contains('open'),'permissions are discoverable before a failed cleanup');
  document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
  qaAssert(!document.getElementById('runtimeModal').classList.contains('open'),'Escape closes the read-only runtime guide');
  __qa.actions=[];document.querySelector('.header-controls [aria-label="打开菜单栏工具"]').click();
  qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='menubarShowTools','the main header opens the real native tray');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-late-receipt-safety',`
  openRuntimePanel();const first=__qa.actions.at(-1);requestRuntimeCheck();const second=__qa.actions.at(-1);
  onNativeRuntimeStatus({requestId:first.requestId,packaged:false,readAccess:{passed:false}});
  qaAssert(STATE.runtimeStatus===null,'a stale diagnostic response cannot replace a newer request');
  onNativeRuntimeStatus({requestId:second.requestId,packaged:true,appPath:"/Applications/Owner's <app>.app",identifier:'com.example.app',menuBarRegistered:true,readAccess:{passed:null,detail:'未知'}});
  qaAssert(!document.getElementById('runtimePanelBody').querySelector('app'),'runtime paths cannot create markup');
  qaAssert(document.getElementById('runtimePanelBody').textContent.includes("Owner's <app>.app"),'the displayed path is preserved');
  closeRuntimePanel();
`);
add('system-scan-native-startup', `
  STATE=structuredClone(DEFAULT_STATE);document.documentElement.dataset.theme='light';switchModule('clean');
  qaAssert(typeof window.onNativeRealScanStarted==='function','automatic native scans publish their running state');
  onNativeRealScanStarted();
  for(const module of ['clean','ai_moon','software','purge']){
    switchModule(module);
    qaAssert(document.getElementById('moduleStage').getAttribute('aria-busy')==='true','native startup marks '+module+' as scanning');
    qaAssert(document.querySelector('#moduleStage .skel-row'),'native startup shows loading rows instead of an empty '+module+' feature');
    qaAssert(document.getElementById('primaryBtnText').textContent==='预览所选'&&document.getElementById('primaryBtnText').closest('button').disabled,'native startup cannot offer a second scan');
    qaAssert(document.getElementById('btnRescanText').closest('button').disabled,'native startup disables repeated refresh requests');
    qaAssert(document.getElementById('scanSourceBadge').textContent==='正在读取本机','native startup names the actual operation');
    if(module==='software'){
      setSoftwarePanelMode('apps');
      qaAssert(document.querySelector('#moduleStage .skel-row'),'installed applications also show a loader before their native result');
      qaAssert(!document.getElementById('moduleStage').textContent.includes('当前没有可显示的应用'),'pending application results cannot claim an empty catalog');
    }
  }
  qaAssert(!__qa.actions.length,'a native scan notification does not request another scan');
  switchModule('clean');
`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('system-scan-long-wait', `
  switchModule('clean');const previous=STATE.cleanGroups;const nativeSetTimeout=window.setTimeout;let slowNotice;
  window.setTimeout=(callback,delay,...args)=>delay===20000?(slowNotice=callback,0):nativeSetTimeout(callback,delay,...args);
  try{
    triggerNativeRescan();
    qaAssert(typeof slowNotice==='function','a long scan has a bounded waiting notice');slowNotice();
    qaAssert(document.getElementById('moduleStage').getAttribute('aria-busy')==='true','20 seconds does not pretend that the native scan stopped');
    qaAssert(document.getElementById('btnRescanText').closest('button').disabled,'a slow scan still blocks duplicate refresh requests');
    qaAssert(!document.getElementById('toastBanner').textContent.includes('重试'),'waiting is not reported as a failed scan');
    qaAssert(!STATE.scanCompleted&&STATE.cleanGroups===previous,'old measurements remain inspectable without authorizing a fresh cleanup');
    openConfirmSheet();qaAssert(!document.getElementById('confirmModal').classList.contains('open'),'a pending scan cannot authorize the previous selection');
    triggerNativeRescan();qaAssert(__qa.actions.filter(action=>action.action==='requestRealSystemScan').length===1,'a slow scan is submitted only once');
    onNativeRealScanComplete(structuredClone(qaFixture));
    qaAssert(STATE.scanCompleted&&!document.getElementById('btnRescanText').closest('button').disabled,'a late native completion ends the wait');
  }finally{window.setTimeout=nativeSetTimeout;}
`);
add('system-scan-invalid-completion', `
  STATE=structuredClone(DEFAULT_STATE);switchModule('clean');triggerNativeRescan();
  for(const payload of [null,undefined]){
    onNativeRealScanComplete(payload);
    qaAssert(document.getElementById('moduleStage').getAttribute('aria-busy')==='true','an empty receipt cannot end a running scan');
    qaAssert(!STATE.scanCompleted&&document.getElementById('btnRescanText').closest('button').disabled,'an empty receipt cannot authorize cleanup or retry');
  }
  onNativeRealScanComplete(structuredClone(qaFixture));
  qaAssert(STATE.scanCompleted&&document.getElementById('moduleStage').getAttribute('aria-busy')==='false','the actual receipt restores the settled state');
`);
add('system-scan-queued-native-start', `
  switchModule('clean');qaAssert(typeof window.onNativeRealScanStarted==='function','queued scans have a native start receipt');
  onNativeRealScanStarted();onNativeRealScanComplete(structuredClone(qaFixture));
  qaAssert(STATE.scanCompleted,'the first native scan completes');const previous=STATE.cleanGroups;
  onNativeRealScanStarted();onNativeRealScanStarted();
  qaAssert(!STATE.scanCompleted&&STATE.cleanGroups===previous,'a follow-up scan retains the prior list for inspection');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'a follow-up scan does not authorize stale measurements');
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[]});
  qaAssert(STATE.scanCompleted&&!STATE.cleanGroups.length,'the newest completion remains authoritative, including an empty result');
  qaAssert(document.getElementById('moduleStage').getAttribute('aria-busy')==='false'&&!document.getElementById('btnRescanText').closest('button').disabled,'native completion releases the loading controls');
  qaAssert(!__qa.actions.length,'native queue receipts never submit a redundant scan');
`);
add('uninstaller-default-installed-apps', `
  STATE=structuredClone(DEFAULT_STATE);document.documentElement.dataset.theme='light';
  onNativeRealScanComplete({...structuredClone(qaFixture),orphanLeftoversList:[]});
  document.querySelector('.nav-item[data-mod="software"]').click();
  qaAssert(STATE.softwarePanelMode==='apps','the uninstall entry opens installed applications by default');
  qaAssert(document.querySelectorAll('#moduleStage .list-row').length===qaFixture.appsList.length,'no residuals cannot hide the installed application catalog');
  qaAssert(!document.getElementById('moduleStage').textContent.includes('当前没有残留候选'),'the primary uninstall entry is not an empty residual panel');
  qaAssert(STATE.appsList.every(app=>!app.checked)&&document.getElementById('primaryBtnText').closest('button').disabled,'opening the application catalog never selects an uninstall');
  setSoftwarePanelMode('leftovers');
  qaAssert(document.getElementById('moduleStage').textContent.includes('当前没有残留候选'),'the separate residual panel remains available');
  setSoftwarePanelMode('apps');qaAssert(document.querySelectorAll('#moduleStage .list-row').length===qaFixture.appsList.length,'returning from residuals restores the installed catalog');
  qaAssert(!__qa.actions.length,'switching uninstall panels does not submit file operations or another scan');
`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('uninstaller-missing-panel-mode', `
  delete STATE.softwarePanelMode;switchModule('software');
  qaAssert(document.querySelectorAll('#moduleStage .list-row').length===qaFixture.appsList.length,'a missing panel preference falls back to the installed application catalog');
  qaAssert(document.querySelector('.app-path-entry'),'the fallback inspector matches the installed-app cleanup path');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled,'a missing preference never selects an application');
`);
for(const panel of ['apps','leftovers']){
  add('uninstaller-start-scan-'+panel, `
    STATE=structuredClone(DEFAULT_STATE);STATE.softwarePanelMode='${panel}';switchModule('software');
    const primary=document.getElementById('primaryBtnText').closest('button');
    qaAssert(!primary.disabled&&document.getElementById('primaryBtnText').textContent==='开始扫描','the first-use ${panel} action clearly offers a scan');
    primary.click();
    qaAssert(__qa.actions.filter(action=>action.action==='requestRealSystemScan').length===1,'the ${panel} first-use action requests one real scan');
    qaAssert(!document.getElementById('confirmModal').classList.contains('open')&&!pendingTrashRequests.size,'first-use scanning never enters an uninstall confirmation');
    qaAssert(primary.disabled&&document.getElementById('moduleStage').getAttribute('aria-busy')==='true','the ${panel} first-use action enters the shared pending state');
    primary.click();qaAssert(__qa.actions.length===1,'repeated ${panel} clicks cannot submit another scan');
    onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[],orphanLeftoversList:[]});
    qaAssert(primary.disabled&&document.getElementById('primaryBtnText').textContent==='预览所选','a completed empty scan cannot authorize uninstall');
    handlePrimaryAction();
    qaAssert(__qa.actions.length===1,'a completed empty catalog is not treated as an unstarted scan');
  `);
}
add('uninstaller-sidebar-unmeasured', `
  STATE=structuredClone(DEFAULT_STATE);switchModule('software');
  qaAssert(document.getElementById('navMetaSoftware').textContent==='待扫描','unmeasured installed applications are not presented as zero');
  onNativeRealScanStarted();
  qaAssert(document.getElementById('navMetaSoftware').textContent==='扫描中','native startup keeps the application count unknown until its result');
  onNativeRealScanComplete(structuredClone(qaFixture));
  qaAssert(document.getElementById('navMetaSoftware').textContent===qaFixture.appsList.length+' 应用','the actual result supplies the installed application count');
`);
add('analyze-folded-single-source', `switchModule('analyze');STATE.analyzeAggregated={count:14,sizeGb:2,items:[{name:'cache-a.bin',path:'~/Library/Caches/cache-a.bin',sizeGb:0.8,fileCount:1,isDir:false},{name:'logs',path:'~/Library/Logs',sizeGb:0.6,fileCount:4,isDir:true}]};renderModule();const folded=STATE.analyzeMapFolded;qaAssert(folded&&folded.count>=14,'folded tile exists');const n=fmtCount(folded.count);const tab=document.querySelector('.insp-tab[data-tab="folded"]');qaAssert(tab.textContent.replace(/\\s/g,'')==='折叠项'+n,'folded tab count is the folded tile count: '+tab.textContent);qaAssert(document.querySelector('.tt-others').dataset.tip.includes(n+' 个小项'),'tile label uses the same count');qaAssert(document.querySelector('.facts-bar').textContent.includes('合并 '+n+' 个小项'),'summary uses the same count');qaAssert(document.querySelectorAll('#analysisOtherHost .analysis-other-row').length===folded.items.length,'folded tab lists exactly the folded items');qaAssert(!document.getElementById('analysisOtherHost').textContent.includes('窄方块'),'no narrow-tile copy remains');qaAssert(document.querySelector('.facts-bar').scrollWidth<=document.querySelector('.facts-bar').clientWidth+1,'summary does not truncate');`);
add('analyze-others-inspector', `switchModule('analyze');STATE.analyzeAggregated={count:14,sizeGb:2,items:[{name:'cache-a.bin',path:'~/Library/Caches/cache-a.bin',sizeGb:0.8,fileCount:1,isDir:false}]};renderModule();const folded=STATE.analyzeMapFolded;document.querySelector('.tt-others').click();const head=document.querySelector('.crumb-sel').textContent;qaAssert(head.includes('其它')&&head.includes(fmtCount(folded.count)+' 个小项')&&head.includes(fmtDiskSizeGb(folded.sizeGb,1)),'inspector header describes the folded group: '+head);qaAssert(STATE.analyzeGroupSel&&document.querySelector('.tt-others').classList.contains('active-tile'),'the others tile is the active selection');qaAssert(document.querySelector('.insp-tab[data-tab="folded"]').classList.contains('active')||document.querySelector('.insp-tab[data-tab="folded"]').getAttribute('aria-selected')==='true','folded tab opens');renderModule();qaAssert(document.querySelector('.crumb-sel').textContent.includes('其它'),'group selection survives a re-render');selectTile(0);qaAssert(!STATE.analyzeGroupSel&&!document.querySelector('.crumb-sel').textContent.includes('折叠项'),'selecting a tile leaves the group');`);
add('ai-age-and-boundary-copy', `const row=structuredClone(qaFixture.aiMoonBands[2]);row.ageText='354–354 天前最后修改';const range=structuredClone(qaFixture.aiMoonBands[1]);range.ageText='3–9 天前最后修改';onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:[row,range]});switchModule('ai_moon');const text=document.getElementById('moduleStage').textContent;qaAssert(text.includes('354 天前最后修改')&&!/354\\s*[–-]\\s*354/.test(text),'equal bounds collapse to one number');qaAssert(text.includes('3–9 天前最后修改'),'real ranges are untouched');const box=document.querySelector('.inspector-box');qaAssert(box.textContent.split('当前启动器指向的版本，保持原状').length-1===1,'the boundary box says the reason once');const head=document.querySelector('.panel-header span:last-child').textContent.trim();qaAssert(document.querySelector('.inspector-content .subpath-right').textContent.trim()===fmtDiskSizeGb(row.subpaths[0].sizeGb),'path rows use the shared size formatter');qaAssert(/(MB|GB|KB| B)$/.test(head),'header keeps its size');`);
add('uninstaller-sort-follows-preset', `const mk=(id,app,cache,config,running)=>({...structuredClone(qaFixture.appsList[0]),id,name:id,appPath:'/Applications/'+id+'.app',running:!!running,appSizeGb:app,leftoverSizeGb:cache+config,cacheSizeGb:cache,configSizeGb:config,checked:false,keepPersonal:true,subpaths:[Object.assign({path:'/Applications/'+id+'.app',kind:'app',sizeGb:app,size:app+' GB',checked:true},running?{tier:'protected',reason:'应用正在运行'}:{}),{path:'~/Library/Caches/'+id,kind:'cache',sizeGb:cache,size:cache+' GB',checked:true},{path:'~/Library/Application Support/'+id,kind:'config',sizeGb:config,size:config+' GB',checked:false}]});onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[mk('A',3,0.1,0),mk('B',1,0.2,4),mk('C',10,0.00008,0,true)]});switchModule('software');setSoftwarePanelMode('apps');const order=()=>STATE.appsList.map(a=>a.id).join();qaAssert(order()==='C,A,B','standard sorts by the displayed removal size: '+order());selectAppRow(STATE.appsList.findIndex(a=>a.id==='B'));setUninstallMode('complete');qaAssert(order()==='C,B,A','complete re-sorts: '+order());qaAssert(STATE.appsList[STATE.selectedAppIdx].id==='B','selection follows the app id');setUninstallMode('cache_only');qaAssert(order()==='B,A,C','cache only re-sorts: '+order());setUninstallMode('standard');const first=document.querySelector('#moduleStage .list-row');qaAssert(first.textContent.includes('退出后可卸载')&&first.textContent.includes('应用 10.00 GB')&&!/(^|\\s)0 B|81\\.9 KB|80 B/.test(first.querySelector('.size-cell').textContent),'a running app is not shown with its leftover size: '+first.querySelector('.size-cell').textContent);`);
add('uninstaller-caption-follows-preset', `onNativeRealScanComplete(structuredClone(qaFixture));switchModule('software');setSoftwarePanelMode('apps');const sub=()=>document.getElementById('heroStatSub').textContent;qaAssert(sub()==='移除应用 · 保留个人配置','standard caption: '+sub());setUninstallMode('cache_only');qaAssert(sub()==='只清理所选缓存，保留应用','cache caption: '+sub());setUninstallMode('complete');qaAssert(sub()==='移除应用及所选数据','complete caption: '+sub());const toast=document.getElementById('toastBanner');toast.style.display='none';setSoftwarePanelMode('leftovers');qaAssert(toast.style.display!=='block','switching tabs does not toast');qaAssert(sub().includes('移入废纸篓'),'leftover caption: '+sub());`);
add('scan-state-single-message', `STATE=structuredClone(DEFAULT_STATE);switchModule('clean');onNativeRealScanStarted();const stage=document.getElementById('moduleStage');qaAssert(document.getElementById('primaryBtnText').textContent==='预览所选'&&document.getElementById('primaryBtnText').closest('button').disabled,'the CTA keeps the module action but is disabled');const refresh=document.getElementById('btnRescanText').closest('button');qaAssert(refresh.disabled&&refresh.classList.contains('busy')&&refresh.getAttribute('aria-busy')==='true','refresh shows a busy state');qaAssert(document.getElementById('navMetaClean').textContent==='扫描中','sidebar says scanning');qaAssert(!/\\(0\\)/.test(stage.textContent),'no zero counts while scanning');const visible=[...document.querySelectorAll('button,.summary-headline,.summary-eyebrow,#moduleStage')].map(e=>e.innerText||'').join('\\n');qaAssert((visible.match(/正在扫描/g)||[]).length===1,'one visible scanning message: '+(visible.match(/正在扫描/g)||[]).length);switchModule('history');qaAssert(document.getElementById('scanSourceBadge').parentElement.hidden,'history has no scan-source kicker');`);
add('purge-rows-use-home-paths', `onNativeRealScanComplete({...structuredClone(qaFixture),purgeItems:[{...structuredClone(qaFixture.purgeItems[0]),project:'/Users/orbit-fixture/Projects/Canvas'}],installerFiles:[{...structuredClone(qaFixture.installerFiles[0]),location:'/Users/orbit-fixture/Downloads'}]});switchModule('purge');const title=document.querySelector('.row-title');qaAssert(title.textContent==='~/Projects/Canvas/node_modules','purge row shows a home-relative path: '+title.textContent);qaAssert(title.querySelector('span').textContent==='node_modules','artifact suffix keeps its own highlight');qaAssert(document.getElementById('moduleStage').textContent.includes('~/Downloads/Canvas Setup.dmg'),'installer row is home-relative');`);
const largeFileScrollSetup=`
  STATE.analyzeLargeFiles=Array.from({length:50},(_,index)=>({id:'large-scroll-'+index,name:'大文件 '+String(index+1).padStart(2,'0')+'.bin',path:'~/Library/ScrollFixture/'+String(index+1).padStart(2,'0')+'.bin',sizeGb:0.25,kind:'文件',protectionReason:'隔离验收保留，不执行移除'}));
  switchModule('analyze');setAnalyzeTab('large');
  const panel=document.querySelector('.analysis-large-files');
  const body=panel.querySelector('.list-body');
`;
for(const width of [1380,1424,880])for(const theme of ['dark','light']){
  add('analyze-large-files-scroll-'+width+'-'+theme,largeFileScrollSetup+`
    document.documentElement.dataset.theme='${theme}';
    qaAssert(body.querySelectorAll('.list-row').length===50,'all measured large files remain available');
    body.scrollTop=body.scrollHeight;
    qaAssert(body.scrollTop>0&&body.clientHeight<body.scrollHeight,'the bottom list scrolls instead of clipping rows inside its panel');
    qaAssert(getComputedStyle(body).overflowY==='auto'&&body.getBoundingClientRect().bottom<=panel.closest('.analysis-inspector').getBoundingClientRect().bottom+1,'the file list has a bounded independent viewport inside the inspector');
    qaAssert(body.getAttribute('role')==='region'&&body.getAttribute('tabindex')==='0'&&body.getAttribute('aria-label').includes('大文件'),'keyboard users can focus the scrollable region');
    const workspace=document.querySelector('.workspace-body');workspace.scrollTop=workspace.scrollHeight;
    const header=panel.querySelector('.insp-note').getBoundingClientRect(),last=body.lastElementChild.getBoundingClientRect(),bounds=body.getBoundingClientRect();
    qaAssert(last.bottom<=bounds.bottom+1&&last.top>=bounds.top-1&&last.bottom<=innerHeight+1,'the fiftieth file is visible after scrolling to the end');
    qaAssert(header.top>=0&&header.bottom<=bounds.top+1,'the count header stays visible while rows scroll');
    {const bar=document.getElementById('basketBar');qaAssert(!bar.getClientRects().length||last.bottom<=bar.getBoundingClientRect().top+1,'the cleanup bar does not cover the last file at the end of scrolling');}
    qaAssert(!__qa.actions.some(action=>['trashRealPaths','executeClean','killProcess'].includes(action.action)),'scrolling never submits cleanup');
    __qa.motionEvidence={rows:50,bodyHeight:body.clientHeight,scrollHeight:body.scrollHeight,scrollTop:body.scrollTop,lastRow:last.top,header:header.top};
  `);
  Object.assign(scenarios.at(-1),{width,height:width===880?620:width===1424?1053:860});
}
add('analyze-large-files-scroll-retained',largeFileScrollSetup+`
  body.scrollTop=360;body.focus({preventScroll:true});
  const before=body.scrollTop;
  renderModule();
  const next=document.querySelector('.analysis-large-files .list-body');
  qaAssert(next&&before>0&&Math.abs(next.scrollTop-before)<1,'same-layer re-render preserves the large-file scroll offset');
  qaAssert(document.activeElement===next,'same-layer refresh preserves focus on the named scroll region');
  document.querySelector('.analysis-sort').click();
  qaAssert(Math.abs(document.querySelector('.analysis-large-files .list-body').scrollTop-before)<1,'changing directory sort does not reset the independent file list');
`);
const runtimePermissionFixture=`
  const permissionStatus={requestId:STATE.runtimeRequestId,packaged:true,appPath:'/Applications/orbit.app',executablePath:'/Applications/orbit.app/Contents/MacOS/orbit',identifier:'com.mymoles.mole-studio',version:'0.1.0',menuBarRegistered:true,checkedAt:Date.now(),readAccess:{passed:false,status:'denied',detail:'~/Library/Safari 读取被拒绝',checkedAt:Date.now(),checks:[{path:'~/Library/Mail',passed:true,status:'readable',detail:'已读取目录'},{path:'~/Library/Safari',passed:false,status:'denied',detail:'读取被拒绝'}]},folderReadAccess:[{id:'desktop',title:'桌面',path:'~/Desktop',passed:true,status:'readable',detail:'可读取目录（2 项）',checkedAt:Date.now()},{id:'documents',title:'文稿',path:'~/Documents',passed:false,status:'denied',detail:'目录读取被拒绝',checkedAt:Date.now()},{id:'downloads',title:'下载',path:'~/Downloads',passed:null,status:'missing',detail:'未找到该目录',checkedAt:Date.now()}]};
`;
for(const width of [1380,880])for(const theme of ['dark','light']){
  add('runtime-permission-status-'+width+'-'+theme,`
    document.documentElement.dataset.theme='${theme}';openRuntimePanel();
    ${runtimePermissionFixture}
    onNativeRuntimeStatus(permissionStatus);
    const body=document.getElementById('runtimePanelBody');
    const row=key=>body.querySelector('[data-runtime-permission="'+key+'"]');
    qaAssert(row('fullDiskAccess')?.textContent.includes('读取被拒绝'),'Full Disk Access has a visible evidence-based state');
    qaAssert(row('desktop')?.textContent.includes('已验证可读取')&&row('documents')?.textContent.includes('读取被拒绝'),'Desktop and Documents have independent outcomes rather than one generic paragraph');
    qaAssert(row('downloads')?.textContent.includes('目录不存在')&&!row('downloads').textContent.includes('已授权'),'a missing directory is not a permission denial or grant');
    qaAssert(row('appManagement')?.textContent.includes('需在设置确认')&&row('administrator')?.textContent.includes('操作时授权'),'permissions that are not safely queried cannot invent a grant');
    qaAssert(body.textContent.includes('不等于系统开关')&&!body.textContent.includes('全部已授权'),'readability and the system authorization switch remain distinct');
    qaAssert(document.getElementById('runtimeCheckStatus').textContent.includes('检查于'),'a permission result names its observation time');
    const close=document.getElementById('runtimeCloseBtn').getBoundingClientRect();
    qaAssert(close.top>=0&&close.bottom<=innerHeight,'Close remains visible even when permission rows scroll');
    qaAssert(!__qa.actions.some(action=>['trashRealPaths','killProcess','releaseFileCache'].includes(action.action)),'checking permission presentation cannot mutate user resources');
  `);
  Object.assign(scenarios.at(-1),{width,height:width===880?620:860});
}
add('runtime-folder-check-explicit',`
  openRuntimePanel();
  const initial=__qa.actions.at(-1);
  qaAssert(initial.action==='requestRuntimeStatus'&&initial.checkFolders===false,'opening the panel does not unexpectedly probe Desktop/Documents/Downloads');
  ${runtimePermissionFixture}
  delete permissionStatus.folderReadAccess;onNativeRuntimeStatus(permissionStatus);
  qaAssert(document.querySelector('[data-runtime-permission="desktop"]').textContent.includes('尚未检查'),'not checked is not represented as denied');
  __qa.actions=[];document.getElementById('runtimeFolderCheckBtn').click();requestRuntimeCheck(true);
  const requests=__qa.actions.filter(action=>action.action==='requestRuntimeStatus');
  qaAssert(requests.length===1&&requests[0].checkFolders===true&&requests[0].requestId,'only an explicit check can request folder readability and rapid clicks cannot overlap it');
  qaAssert(document.getElementById('runtimeFolderCheckBtn').disabled&&document.querySelector('[data-runtime-permission="desktop"]').textContent.includes('正在检查'),'folder probing has visible pending state');
  permissionStatus.requestId=requests[0].requestId;permissionStatus.folderReadAccess=[{id:'desktop',title:'桌面',path:'~/Desktop',passed:true,status:'readable',detail:'可读取目录',checkedAt:Date.now()},{id:'documents',title:'文稿',path:'~/Documents',passed:null,status:'unknown',detail:'读取超时，可能正在等待系统授权',checkedAt:Date.now()},{id:'downloads',title:'下载',path:'~/Downloads',passed:false,status:'denied',detail:'读取被拒绝',checkedAt:Date.now()}];
  onNativeRuntimeStatus(permissionStatus);
  qaAssert(!document.getElementById('runtimeFolderCheckBtn').disabled&&document.querySelector('[data-runtime-permission="documents"]').textContent.includes('尚不能判定'),'an authorization wait is unknown rather than falsely granted');
`);
add('runtime-permission-refresh-preserves-focus',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const body=document.getElementById('runtimePanelBody');body.scrollTop=body.scrollHeight;
  const button=body.querySelector('[data-runtime-action="appManagement"]');button.focus({preventScroll:true});const before=body.scrollTop;
  requestRuntimeCheck();const requestId=STATE.runtimeRequestId;
  onNativeRuntimeStatus({...permissionStatus,requestId,checkedAt:Date.now()});
  qaAssert(STATE.runtimeStatus.requestId===requestId,'the new correlated permission receipt was actually applied');
  qaAssert(document.activeElement.dataset.runtimeAction==='appManagement'&&Math.abs(body.scrollTop-before)<1,'permission receipts keep the same focused settings action and scroll position');
  __qa.actions=[];document.activeElement.click();qaAssert(__qa.actions.length===1&&__qa.actions[0].pane==='appManagement','retained focus cannot point to a different permission');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-refresh-timeout',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===10000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    requestRuntimeCheck(true);timeout();
    qaAssert(document.getElementById('runtimeCheckStatus').textContent.includes('未收到')&&document.getElementById('runtimePanelBody').textContent.includes('上次结果'),'a missing receipt cannot present an old permission snapshot as freshly verified');
    qaAssert(document.querySelector('[data-runtime-permission="documents"]').textContent.includes('读取被拒绝'),'refresh timeout does not turn a known denial into a grant');
  }finally{window.setTimeout=original;clearTimeout(window._runtimeTimer);}
`);
add('runtime-permission-late-completed-result',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const first=permissionStatus.requestId;
  requestRuntimeCheck();const latest=STATE.runtimeRequestId;
  onNativeRuntimeStatus({...permissionStatus,requestId:latest,readAccess:{passed:true,status:'readable',detail:'本次可读取'}});
  onNativeRuntimeStatus({...permissionStatus,requestId:first,readAccess:{passed:false,status:'denied',detail:'旧请求拒绝'}});
  onNativeRuntimeStatus({...permissionStatus,requestId:'',readAccess:{passed:false,status:'denied',detail:'启动检查迟到'}});
  qaAssert(STATE.runtimeStatus.requestId===latest&&document.querySelector('[data-runtime-permission="fullDiskAccess"]').textContent.includes('已验证可读取'),'a completed latest permission result cannot be replaced by an older request or startup receipt');
`);
add('runtime-permission-hidden-theme-state',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const style=[...document.querySelectorAll('style')].find(node=>node.textContent.includes('*,*::before,*::after{transition:none!important;animation:none!important}'));
  const original=style.textContent;style.textContent=original.replace('transition:none!important;','');
  Object.defineProperty(document,'visibilityState',{configurable:true,get:()=> 'hidden'});
  try{
    const button=document.querySelector('[data-runtime-action="fullDiskAccess"]');
    document.documentElement.dataset.theme='light';getComputedStyle(button).backgroundColor;
    document.documentElement.dataset.theme='dark';
    qaAssert(getComputedStyle(button).transitionDuration==='0s'&&getComputedStyle(button).backgroundColor==='rgb(33, 32, 29)','permission buttons apply their final theme and state immediately, even when native WebKit pauses hidden-page transitions');
  }finally{style.textContent=original;delete document.visibilityState;}
`);
add('runtime-permission-disclosure-refresh',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const body=document.getElementById('runtimePanelBody');
  const detail=label=>[...body.querySelectorAll('details')].find(node=>node.querySelector('summary').textContent.includes(label));
  detail('个目录').open=true;detail('如何确认').open=true;
  const summary=detail('个目录').querySelector('summary');summary.focus({preventScroll:true});body.scrollTop=180;
  const before=body.scrollTop;requestRuntimeCheck();const requestId=STATE.runtimeRequestId;
  qaAssert(detail('个目录').open&&detail('如何确认').open,'starting refresh must not collapse the read evidence or permission instructions the user is reading');
  qaAssert(document.activeElement===detail('个目录').querySelector('summary')&&Math.abs(body.scrollTop-before)<1,'pending refresh preserves the exact disclosure focus and scroll');
  onNativeRuntimeStatus({...permissionStatus,requestId,checkedAt:Date.now()});
  qaAssert(detail('个目录').open&&detail('如何确认').open&&document.activeElement===detail('个目录').querySelector('summary'),'the native receipt preserves the same expanded content and focused summary');
  detail('如何确认').open=false;renderRuntimePanel();
  qaAssert(!detail('如何确认').open&&detail('个目录').open,'an intentionally closed disclosure stays closed independently of the other evidence');
  qaAssert(!__qa.actions.some(action=>['trashRealPaths','releaseFileCache','killProcess'].includes(action.action)),'preserving disclosure state never submits a mutation');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-disclosure-keyboard-wrap',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  await new Promise(resolve=>setTimeout(resolve,60));
  const first=document.querySelector('#runtimePanelBody details > summary');
  const close=document.getElementById('runtimeCloseBtn');close.focus();
  close.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===first,'wrapping Tab from Close reaches the first native evidence disclosure instead of skipping it');
  first.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',shiftKey:true,bubbles:true,cancelable:true}));
  qaAssert(document.activeElement===close,'Shift-Tab from the first disclosure stays inside the permission dialog');
`);
add('runtime-permission-identity-reset',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const body=document.getElementById('runtimePanelBody');
  requestRuntimeCheck();const requestId=STATE.runtimeRequestId;
  body.querySelectorAll('details').forEach(detail=>detail.open=true);body.scrollTop=body.scrollHeight;
  body.querySelector('[data-runtime-action="appManagement"]').focus({preventScroll:true});
  qaAssert(body.scrollTop>0,'the old application is scrolled away from its identity before replacement');
  onNativeRuntimeStatus({...permissionStatus,requestId,appPath:'/Applications/Another Build.app',identifier:'com.example.another',folderReadAccess:undefined});
  qaAssert(body.scrollTop===0&&!body.querySelector('details[open]'),'a different running bundle resets old disclosures and shows the new identity at the top');
  qaAssert(document.activeElement===document.getElementById('runtimeCloseBtn'),'a new bundle does not preserve an old settings action as current focus');
  qaAssert(body.textContent.includes('/Applications/Another Build.app')&&body.querySelector('[data-runtime-permission="desktop"]').textContent.includes('尚未检查'),'folder permission results from the previous bundle cannot apply to another identity');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-no-verifiable-probes',`
  openRuntimePanel();${runtimePermissionFixture}
  onNativeRuntimeStatus({...permissionStatus,readAccess:{passed:null,status:'missing',detail:'未找到 Mail 或 Safari 核对目录',checks:[]}});
  const row=document.querySelector('[data-runtime-permission="fullDiskAccess"]');
  qaAssert(row.querySelector('.runtime-permission-state').textContent==='无可核验目录','missing FDA probes cannot describe the permission itself as a nonexistent directory');
  qaAssert(!row.querySelector('details[data-runtime-detail="read-evidence"]')&&!row.textContent.includes('查看 0 个'),'an empty evidence payload does not present a fabricated check collection');
`);
add('runtime-permission-disclosure-removed-evidence',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const body=document.getElementById('runtimePanelBody');
  const evidence=body.querySelector('[data-runtime-detail="read-evidence"]');evidence.open=true;
  body.querySelector('[data-runtime-detail="permission-guide"]').open=true;
  evidence.querySelector('summary').focus();requestRuntimeCheck();
  onNativeRuntimeStatus({...permissionStatus,requestId:STATE.runtimeRequestId,readAccess:{passed:null,status:'missing',detail:'本次未找到可核验目录',checks:[]}});
  qaAssert(!body.querySelector('[data-runtime-detail="read-evidence"]')&&body.querySelector('[data-runtime-detail="permission-guide"]').open,'removed evidence does not retain stale check rows or collapse the still-relevant authorization guide');
  qaAssert(document.activeElement===document.getElementById('runtimeCloseBtn'),'focus safely falls back to Close when the exact evidence disclosure no longer exists');
`);
const runtimeScanWarningFixture=`
  const scanWarnings=[{path:qaFixture.homePath+'/Library/Containers/Denied App/Data',reason:'读取被拒绝（permission denied）'},{path:qaFixture.homePath+'/Projects/Very Large Build',reason:'达到目录探索上限，未完成测量'}];
  const readableStatus={...permissionStatus,readAccess:{passed:true,status:'readable',detail:'本次 Mail / Safari 探测目录可读取',checks:[{path:'~/Library/Mail',passed:true,status:'readable'},{path:'~/Library/Safari',passed:true,status:'readable'}]},folderReadAccess:[]};
`;
for(const theme of ['dark','light']){
  add('runtime-permission-scan-evidence-'+theme,`
    document.documentElement.dataset.theme='${theme}';openRuntimePanel();${runtimePermissionFixture}${runtimeScanWarningFixture}
    onNativeRuntimeStatus(readableStatus);onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:scanWarnings});
    const body=document.getElementById('runtimePanelBody');
    qaAssert(body.querySelector('[data-runtime-permission="fullDiskAccess"] .runtime-permission-state').textContent==='已验证可读取','scan limits do not overwrite a measured successful probe with a guessed permission denial');
    const detail=body.querySelector('[data-runtime-detail="scan-evidence"]');
    qaAssert(detail?.textContent.includes('2 个路径未完成测量'),'a new scan receipt immediately exposes the still-unread paths inside the open permission panel');
    qaAssert(!document.getElementById('runtimeBanner').hidden&&document.getElementById('runtimeBannerText').textContent.includes('未完成测量'),'successful probes cannot hide the actual incomplete scan warning');
    detail.open=true;
    qaAssert(detail.querySelectorAll('.permission-group-row').length===2&&detail.textContent.includes('读取被拒绝')&&detail.textContent.includes('探索上限'),'the permission panel separates both exact scan reasons rather than calling every warning an ungranted permission');
    const limit=[...detail.querySelectorAll('.permission-group-row')].find(row=>row.textContent.includes('探索上限'));
    qaAssert(!limit.querySelector('[onclick*="openPrivacySettings"]'),'a directory cap does not falsely ask for a system permission');
    __qa.actions=[];
    detail.querySelector('[onclick*="revealInFinder"]').click();
    qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='revealInFinder'&&__qa.actions[0].path.startsWith(qaFixture.homePath),'an unread group reveals its real native path');
    detail.querySelector('[onclick*="openPrivacySettings"]').click();
    qaAssert(__qa.actions.at(-1).action==='openPrivacySettings'&&__qa.actions.at(-1).pane==='fullDiskAccess','permission-related scan evidence links to its matching settings pane');
    qaAssert(!__qa.actions.some(action=>['trashRealPaths','killProcess','releaseFileCache','requestRealSystemScan'].includes(action.action)),'viewing scan evidence neither mutates files nor starts an extra scan');
  `);
  Object.assign(scenarios.at(-1),{width:880,height:620});
}
add('runtime-permission-scan-receipt-refresh',`
  openRuntimePanel();${runtimePermissionFixture}${runtimeScanWarningFixture}
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:scanWarnings});onNativeRuntimeStatus(readableStatus);
  const body=document.getElementById('runtimePanelBody');
  const detail=body.querySelector('[data-runtime-detail="scan-evidence"]');
  qaAssert(detail,'scan evidence must also exist when the runtime receipt arrives after the scan');detail.open=true;
  const path=detail.querySelector('.permission-group-path');path.focus({preventScroll:true});body.scrollTop=body.scrollHeight;const before=body.scrollTop;
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[...scanWarnings,{path:qaFixture.homePath+'/Library/Containers/Denied App/Cache',reason:'读取被拒绝（permission denied）'}]});
  const next=body.querySelector('[data-runtime-detail="scan-evidence"]');
  qaAssert(next.open&&next.textContent.includes('3 个路径未完成测量'),'native scan updates keep the evidence expanded and update its count');
  qaAssert(document.activeElement.dataset.runtimeAction===path.dataset.runtimeAction&&Math.abs(body.scrollTop-before)<1,'scan updates preserve the same directory action and panel scroll');
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[]});
  qaAssert(!body.querySelector('[data-runtime-detail="scan-evidence"]')&&document.getElementById('runtimeBanner').hidden,'a fresh completed scan clears obsolete unread paths and their banner');
  qaAssert(document.activeElement===document.getElementById('runtimeCloseBtn'),'removed scan evidence falls back to Close without pointing at another path');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-scan-nonpermission-warning',`
  openRuntimePanel();${runtimePermissionFixture}${runtimeScanWarningFixture}onNativeRuntimeStatus(readableStatus);
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[scanWarnings[1]]});
  const body=document.getElementById('runtimePanelBody'),detail=body.querySelector('[data-runtime-detail="scan-evidence"]');
  qaAssert(detail&&detail.textContent.includes('不等于系统权限开关'),'scan evidence explicitly remains separate from authorization state');
  qaAssert(!document.getElementById('runtimeBannerText').textContent.includes('读取被拒绝'),'a measurement cap does not masquerade as denied permission');
  qaAssert(body.querySelector('[data-runtime-permission="fullDiskAccess"]').textContent.includes('已验证可读取'),'limits never change the measured access verdict');
  requestRuntimeCheck();onNativeRuntimeStatus({...permissionStatus,requestId:STATE.runtimeRequestId});
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[]});
  qaAssert(!document.getElementById('runtimeBanner').hidden&&document.getElementById('runtimeBannerText').textContent.includes('读取被拒绝'),'clearing scan limits cannot clear an independently confirmed read denial');
`);
add('runtime-permission-scan-invalid-records',`
  openRuntimePanel();${runtimePermissionFixture}${runtimeScanWarningFixture}onNativeRuntimeStatus(readableStatus);
  const unusualPath="/Volumes/Owner's <Disk>&quot;/Folder/item";
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[null,{},'invalid',{path:12},{path:'  '},{path:unusualPath,reason:'读取失败 <details> &quot;'}]});
  const detail=document.querySelector('[data-runtime-detail="scan-evidence"]');
  qaAssert(STATE.scanReadWarnings.length===1&&detail.textContent.includes('1 个路径未完成测量'),'only well-formed native warning paths enter the shared scan record');
  qaAssert(!detail.querySelector('disk')&&detail.querySelectorAll('details').length===0,'unread paths and reasons cannot create markup in the permission panel');
  detail.open=true;__qa.actions=[];detail.querySelector('.permission-group-path').click();
  qaAssert(__qa.actions[0].action==='revealInFinder'&&__qa.actions[0].path==="/Volumes/Owner's <Disk>&quot;/Folder",'a grouped path preserves quotes and entities across the HTML and native message boundary');
`);
add('runtime-permission-scan-directory-access-control',`
  openRuntimePanel();${runtimePermissionFixture}${runtimeScanWarningFixture}onNativeRuntimeStatus(readableStatus);
  const path=qaFixture.homePath+'/Projects/Directory With Local Access Control';
  onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:[{path,reason:'open '+path+': permission denied'}]});
  const detail=document.querySelector('[data-runtime-detail="scan-evidence"]');detail.open=true;
  qaAssert(!detail.querySelector('[onclick*="openPrivacySettings"]'),'ordinary project-directory access control is not falsely mapped to Full Disk Access');
  const reason=detail.querySelector('.permission-group-path small');
  qaAssert(getComputedStyle(reason).whiteSpace==='normal'&&reason.textContent.includes('permission denied'),'long native read errors wrap so their actual failure reason is visible rather than ellipsized away');
  qaAssert(detail.textContent.includes('访问控制'),'permission guidance distinguishes local access control from a system grant');
`);
add('runtime-permission-current-check-pending',`
  openRuntimePanel();${runtimePermissionFixture}
  permissionStatus.readAccess={passed:true,status:'readable',detail:'上次目录可读取',checkedAt:Date.now()-3600000,checks:[]};onNativeRuntimeStatus(permissionStatus);
  const previous=STATE.runtimeStatus.readAccess;
  requestRuntimeCheck();const requestId=STATE.runtimeRequestId;
  const row=document.querySelector('[data-runtime-permission="fullDiskAccess"]');
  qaAssert(row.querySelector('.runtime-permission-state').textContent==='正在检查…'&&row.querySelector('.runtime-permission-state').dataset.state==='unknown','a pending current check cannot present the last readable result as currently verified');
  qaAssert(row.querySelector('[data-runtime-last-read]').textContent.includes('已验证可读取')&&row.querySelector('[data-runtime-last-read]').textContent.includes(runtimeCheckTime(previous.checkedAt)),'the previous observation remains visible with its actual time');
  qaAssert(STATE.runtimeStatus.readAccess===previous,'pending UI must not mutate the authoritative observed access result');
  onNativeRuntimeStatus({...permissionStatus,requestId,readAccess:{passed:false,status:'denied',detail:'本次读取被拒绝',checkedAt:Date.now(),checks:[]}});
  qaAssert(document.querySelector('[data-runtime-permission="fullDiskAccess"] .runtime-permission-state').textContent==='读取被拒绝'&&!document.querySelector('[data-runtime-last-read]'),'the new native receipt replaces pending state and removes obsolete last-observation copy');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-current-check-timeout',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const previous=STATE.runtimeStatus.readAccess;
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===10000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    requestRuntimeCheck();const requestId=STATE.runtimeRequestId;timeout();
    const row=document.querySelector('[data-runtime-permission="fullDiskAccess"]');
    qaAssert(row.querySelector('.runtime-permission-state').textContent==='本次未确认'&&row.querySelector('.runtime-permission-state').dataset.state==='unknown','a missing current receipt is not a freshly confirmed denied or granted permission');
    qaAssert(row.querySelector('[data-runtime-last-read]').textContent.includes('读取被拒绝')&&STATE.runtimeStatus.readAccess===previous,'timeout preserves the old diagnostic as a last observation, not a new failure');
    qaAssert(!document.getElementById('runtimeRecheckBtn').disabled,'an unknown diagnostic can be refreshed without running cleanup');
    onNativeRuntimeStatus({...permissionStatus,requestId,readAccess:{passed:true,status:'readable',detail:'迟到的本次真实读取通过',checkedAt:Date.now(),checks:[]}});
    qaAssert(document.querySelector('[data-runtime-permission="fullDiskAccess"] .runtime-permission-state').textContent==='已验证可读取'&&!document.querySelector('[data-runtime-last-read]'),'a late correlated diagnostic can still resolve the unknown state truthfully');
  }finally{window.setTimeout=original;clearTimeout(window._runtimeTimer);}
`);
add('runtime-permission-folder-pending-history',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const previous=STATE.runtimeStatus.folderReadAccess;
  requestRuntimeCheck(true);const requestId=STATE.runtimeRequestId;
  for(const check of previous){
    const row=document.querySelector('[data-runtime-permission="'+check.id+'"]');
    qaAssert(row.querySelector('.runtime-permission-state').textContent==='正在检查…','an explicit folder check presents its current pending state');
    const last=row.querySelector('[data-runtime-folder-last-read]');
    qaAssert(last&&last.textContent.includes(runtimeReadState(check)[0])&&row.textContent.includes(runtimeCheckTime(check.checkedAt)),'pending folder checks keep their previous verdict and observation time separately');
  }
  qaAssert(STATE.runtimeStatus.folderReadAccess===previous,'pending presentation never alters native folder evidence');
  onNativeRuntimeStatus({...permissionStatus,requestId,folderReadAccess:previous.map(check=>({...check,checkedAt:Date.now()}))});
  qaAssert(!document.querySelector('[data-runtime-folder-last-read]'),'the correlated folder receipt replaces the pending history annotation');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-folder-timeout-scope',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const previous=STATE.runtimeStatus.folderReadAccess;
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===10000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    requestRuntimeCheck(true);timeout();
    const row=()=>document.querySelector('[data-runtime-permission="desktop"]');
    qaAssert(row().querySelector('.runtime-permission-state').textContent==='本次未确认'&&row().querySelector('.runtime-permission-state').dataset.state==='unknown','a missing folder receipt cannot revert to an old currently verified grant');
    qaAssert(row().querySelector('[data-runtime-folder-last-read]').textContent.includes('已验证可读取')&&STATE.runtimeStatus.folderReadAccess===previous,'the previous folder observation remains separate from the unconfirmed current check');
    requestRuntimeCheck();const basicRequest=STATE.runtimeRequestId;
    onNativeRuntimeStatus({...permissionStatus,requestId:basicRequest,folderReadAccess:undefined});
    qaAssert(row().querySelector('.runtime-permission-state').textContent==='本次未确认','a basic refresh that does not check folders cannot resolve a failed folder check');
    requestRuntimeCheck(true);const folderRequest=STATE.runtimeRequestId;
    onNativeRuntimeStatus({...permissionStatus,requestId:folderRequest,folderReadAccess:previous.map(check=>({...check,checkedAt:Date.now()}))});
    qaAssert(row().querySelector('.runtime-permission-state').textContent==='已验证可读取'&&!row().querySelector('[data-runtime-folder-last-read]'),'only a new correlated folder result resolves the unconfirmed folder state');
    qaAssert(!__qa.actions.some(action=>['trashRealPaths','releaseFileCache','requestRealSystemScan'].includes(action.action)),'diagnostic retries do not perform maintenance or scan user files');
  }finally{window.setTimeout=original;clearTimeout(window._runtimeTimer);}
`);
add('runtime-permission-folder-missing-result',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const previous=STATE.runtimeStatus.folderReadAccess;
  requestRuntimeCheck(true);const requestId=STATE.runtimeRequestId;
  onNativeRuntimeStatus({...permissionStatus,requestId,folderReadAccess:undefined});
  const row=document.querySelector('[data-runtime-permission="documents"]');
  qaAssert(row.querySelector('.runtime-permission-state').textContent==='本次未确认'&&STATE.runtimeStatus.folderReadAccess===previous,'a receipt without the requested folder data cannot relabel the old evidence as a completed folder check');
  qaAssert(document.getElementById('runtimePanelBody').textContent.includes('未返回目录核对结果')&&!document.getElementById('runtimeFolderCheckBtn').disabled,'the omitted scope is explained and can be explicitly retried');
`);
add('runtime-permission-observation-date',`
  openRuntimePanel();${runtimePermissionFixture}
  const previous=new Date();previous.setDate(previous.getDate()-1);
  const date=previous.toLocaleDateString('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit'});
  permissionStatus.folderReadAccess[0].checkedAt=previous.getTime();onNativeRuntimeStatus(permissionStatus);
  qaAssert(runtimeCheckTime(previous.getTime()).includes(date)&&document.querySelector('[data-runtime-permission="desktop"]').textContent.includes(date),'a previous-day permission observation includes its date rather than looking like today’s time');
  qaAssert(runtimeCheckTime(1e20)==='时间未知'&&runtimeCheckTime(null)==='时间未知','out-of-range or absent native timestamps cannot display Invalid Date as evidence');
`);
for(const theme of ['dark','light']){
  add('runtime-permission-readable-reasons-'+theme,`
    document.documentElement.dataset.theme='${theme}';openRuntimePanel();${runtimePermissionFixture}
    const reason='读取未完成：~/Documents/'+('long-path-'.repeat(30))+'/<img src=x onerror="window.__permissionReasonInjected=true">/not-a-directory';
    permissionStatus.folderReadAccess[1]={...permissionStatus.folderReadAccess[1],passed:null,status:'unknown',detail:reason};
    permissionStatus.readAccess={...permissionStatus.readAccess,passed:null,status:'unknown',detail:'部分受保护目录未完成读取'};
    permissionStatus.readAccess.checks[1]={...permissionStatus.readAccess.checks[1],passed:null,status:'unknown',detail:reason};
    onNativeRuntimeStatus(permissionStatus);
    const body=document.getElementById('runtimePanelBody');
    const folder=body.querySelector('[data-runtime-permission="documents"]');
    const detail=folder.querySelector('.runtime-read-detail');
    qaAssert(detail&&detail.textContent===reason,'the actual folder failure reason is readable text, not a hover-only tooltip');
    qaAssert(folder.querySelector('.runtime-permission-state').textContent==='尚不能判定','a failed non-permission read is not mislabeled as a missing grant');
    const evidence=body.querySelector('[data-runtime-detail="read-evidence"]');evidence.open=true;
    const probe=evidence.querySelectorAll('.runtime-evidence-row')[1];
    qaAssert(probe.querySelector('.runtime-read-detail')?.textContent===reason,'each Full Disk Access probe exposes its own diagnostic rather than only the aggregate verdict');
    qaAssert(!body.querySelector('img')&&!window.__permissionReasonInjected,'native diagnostic text cannot create markup or execute a path fragment');
    for(const target of [detail,probe.querySelector('.runtime-read-detail')])qaAssert(target.scrollWidth<=target.clientWidth+1&&getComputedStyle(target).overflowWrap==='anywhere','long diagnostic paths wrap inside their permission row');
    const close=document.getElementById('runtimeCloseBtn').getBoundingClientRect();
    qaAssert(close.top>=0&&close.bottom<=innerHeight,'expanded failure reasons cannot hide the fixed Close action');
  `);
  Object.assign(scenarios.at(-1),{width:880,height:620});
}
add('runtime-permission-reasons-history',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const body=document.getElementById('runtimePanelBody');
  const evidence=body.querySelector('[data-runtime-detail="read-evidence"]');evidence.open=true;
  evidence.querySelector('summary').focus({preventScroll:true});body.scrollTop=150;const before=body.scrollTop;
  requestRuntimeCheck(true);
  const folder=body.querySelector('[data-runtime-permission="documents"] .runtime-read-detail');
  const probe=body.querySelectorAll('.runtime-evidence-row')[1].querySelector('.runtime-read-detail');
  qaAssert(folder&&probe&&folder.textContent.startsWith('上次读取说明：')&&probe.textContent.startsWith('上次读取说明：'),'pending diagnostics explicitly label each old failure reason as historical evidence');
  qaAssert(body.querySelector('[data-runtime-detail="read-evidence"]').open&&document.activeElement.dataset.runtimeAction==='read-evidence'&&Math.abs(body.scrollTop-before)<1,'readable reasons retain existing disclosure focus and scrolling');
  const requestId=STATE.runtimeRequestId;
  permissionStatus.folderReadAccess[1]={...permissionStatus.folderReadAccess[1],passed:true,status:'readable',detail:'本次目录可读取',checkedAt:Date.now()};
  permissionStatus.readAccess.checks[1]={...permissionStatus.readAccess.checks[1],passed:true,status:'readable',detail:'本次探测可读取'};
  onNativeRuntimeStatus({...permissionStatus,requestId});
  qaAssert(!body.querySelector('[data-runtime-permission="documents"] .runtime-read-detail'),'a newly readable folder removes its stale failure explanation');
  qaAssert(body.querySelectorAll('.runtime-evidence-row')[1].querySelector('.runtime-read-detail').textContent==='本次探测可读取','a corresponding probe receipt replaces its historical reason');
  qaAssert(!__qa.actions.some(action=>['trashRealPaths','releaseFileCache','killProcess','requestRealSystemScan'].includes(action.action)),'diagnostic presentation cannot initiate cleanup or another scan');
`);
add('runtime-permission-pending-reopen',`
  const trigger=document.querySelector('.header-controls [onclick="openRuntimePanel()"]');trigger.focus();trigger.click();
  const requestId=STATE.runtimeRequestId;
  ${runtimePermissionFixture}
  const timer=window._runtimeTimer;
  await new Promise(resolve=>setTimeout(resolve,60));
  qaAssert(document.getElementById('runtimeModal').contains(document.activeElement),'the pending guide receives focus before the close/reopen interaction');
  closeRuntimePanel();await new Promise(resolve=>setTimeout(resolve,60));
  qaAssert(document.activeElement===trigger,'closing the pending guide restores its existing opener');
  trigger.click();openRuntimePanel();
  qaAssert(STATE.runtimeRequestId===requestId&&window._runtimeTimer===timer,'reopening a pending guide retains the original request and wait deadline');
  qaAssert(__qa.actions.filter(action=>action.action==='requestRuntimeStatus').length===1,'repeated entry cannot start overlapping native directory probes');
  qaAssert(document.getElementById('runtimeModal').getAttribute('aria-busy')==='true'&&document.getElementById('runtimeRecheckBtn').disabled,'reopened pending state remains explicit without offering another check');
  onNativeRuntimeStatus(permissionStatus);
  qaAssert(STATE.runtimeStatus.requestId===requestId&&!STATE.runtimeRequestId&&!document.getElementById('runtimeRecheckBtn').disabled,'the original corresponding receipt resolves the reopened panel instead of being discarded');
`);
Object.assign(scenarios.at(-1),{width:880,height:620});
add('runtime-permission-timeout-reopen-retry',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  const original=window.setTimeout;let timeout;
  window.setTimeout=(callback,delay,...args)=>{if(delay===10000){timeout=callback;return 0;}return original(callback,delay,...args);};
  try{
    requestRuntimeCheck();const expired=STATE.runtimeRequestId;timeout();closeRuntimePanel();openRuntimePanel();
    const retry=STATE.runtimeRequestId;
    qaAssert(retry&&retry!==expired&&!STATE.runtimeCheckError,'reopening a timed-out guide can start an explicit fresh wait');
    onNativeRuntimeStatus({...permissionStatus,requestId:expired,readAccess:{passed:true,status:'readable',detail:'expired result'}});
    qaAssert(STATE.runtimeRequestId===retry,'an expired receipt cannot finish the reopened retry');
    onNativeRuntimeStatus({...permissionStatus,requestId:retry});
    qaAssert(!STATE.runtimeRequestId&&STATE.runtimeStatus.requestId===retry,'the reopened retry resolves only from its corresponding receipt');
  }finally{window.setTimeout=original;clearTimeout(window._runtimeTimer);}
`);
add('runtime-permission-folder-result-while-closed',`
  openRuntimePanel();${runtimePermissionFixture}onNativeRuntimeStatus(permissionStatus);
  requestRuntimeCheck(true);const requestId=STATE.runtimeRequestId;const previous=STATE.runtimeStatus.folderReadAccess;
  closeRuntimePanel();
  const updated=previous.map(check=>({...check,passed:true,status:'readable',detail:'本次目录可读取',checkedAt:Date.now()}));
  onNativeRuntimeStatus({...permissionStatus,requestId,folderReadAccess:updated});
  qaAssert(!document.getElementById('runtimeModal').classList.contains('open')&&!STATE.runtimeRequestId,'a background diagnostic receipt cannot reopen a dismissed guide');
  openRuntimePanel();
  qaAssert(STATE.runtimeStatus.folderReadAccess===updated&&!STATE.runtimeFolderChecking,'a later basic entry retains the completed explicit folder observation without re-probing folders');
  qaAssert(document.querySelector('[data-runtime-permission="documents"] .runtime-permission-state').textContent==='已验证可读取','the completed folder result remains inspectable after closing and reopening');
  qaAssert(!__qa.actions.some(action=>['trashRealPaths','releaseFileCache','killProcess','requestRealSystemScan'].includes(action.action)),'guide reopening and background results do not mutate user resources');
`);
add('clean-coverage-not-selection',`
  const rows=${JSON.stringify([group('known-safe','闲置缓存','~/Library/Caches/Idle',2),group('known-review','需检查缓存','~/.cache/uv',4,'review'),group('known-kept','运行中缓存','~/Library/Caches/Active',4,'protected')])};
  rows.forEach(row=>{row.checked=false;row.subpaths.forEach(path=>{path.checked=false;});});
  rows[1].subpaths=[{path:'~/.cache/uv/ready',sizeGb:3,size:'3 GB',tier:'review',checked:false},{path:'~/.cache/uv/incomplete',sizeGb:1,size:'1 GB',tier:'protected',partial:true,checked:false}];rows[1].partial=true;
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:rows,cleanExcludedRoots:[],readWarnings:[]});switchModule('clean');
  const summary=document.querySelector('.clean-scan-coverage');
  qaAssert(summary&&summary.querySelector('[data-clean-measured]').textContent.includes('10.00 GB'),'cleanup reports measured space independently of the zero selected amount');
  qaAssert(summary.querySelector('[data-clean-reviewable]').textContent.includes('5.00 GB')&&summary.querySelector('[data-clean-retained]').textContent.includes('5.00 GB'),'complete reviewable paths remain separate from protected or incomplete measurements');
  qaAssert(document.getElementById('heroStatValue').textContent==='0 B'&&document.getElementById('primaryBtnText').closest('button').disabled,'showing reviewable space never silently selects paths');
  qaAssert(!__qa.actions.some(action=>['trashRealPaths','addWhitelist','removeWhitelist'].includes(action.action)),'coverage information cannot change cleanup permissions');
`);
add('clean-excluded-roots-empty',`
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],readWarnings:[],cleanExcludedRoots:[{path:'~/Library/Caches',name:'用户缓存与诊断日志',reason:'白名单排除：~/Library/Caches'},{path:'~/Library/Logs',name:'用户缓存与诊断日志',reason:'白名单排除：~/Library/Logs'}]});switchModule('clean');
  const detail=document.querySelector('.clean-exclusions');
  qaAssert(detail&&detail.querySelector('summary').textContent.includes('2 个')&&detail.textContent.includes('体积未测量'),'excluded known roots are disclosed without assigning zero bytes');
  qaAssert(document.querySelector('.empty-title').textContent.includes('排除')&&document.getElementById('heroHeadline').textContent.includes('排除'),'an excluded scan scope is not mistaken for a clean disk or an unstarted scan');
  qaAssert(!detail.querySelector('input')&&document.getElementById('primaryBtnText').closest('button').disabled,'excluded roots can never enter the selection list');
  __qa.actions=[];detail.querySelector('[data-clean-whitelist]').click();
  qaAssert(STATE.module==='history'&&!__qa.actions.some(action=>['addWhitelist','removeWhitelist','trashRealPaths','requestRealSystemScan'].includes(action.action)),'the whitelist entrance reviews existing rules rather than disabling them');
`);
for(const theme of ['dark','light']){
  add('clean-exclusions-long-'+theme,`
    document.documentElement.dataset.theme='${theme}';
    const roots=Array.from({length:30},(_,index)=>({path:'~/Library/Caches/Excluded-'+index+'-'+('LongName'.repeat(12)),name:'受保护缓存',reason:'白名单排除：<img src=x onerror="window.__cleanScopeInjected=true">'+('读取边界保持原状。'.repeat(10))}));
    onNativeRealScanComplete({...structuredClone(qaFixture),cleanExcludedRoots:[null,{path:3},...roots],readWarnings:[]});switchModule('clean');
    const detail=document.querySelector('.clean-exclusions');
    qaAssert(detail,'the cleanup scope disclosure is rendered');detail.open=true;await new Promise(resolve=>setTimeout(resolve,30));
    const body=detail.querySelector('.permission-group-list');body.scrollTop=body.scrollHeight;
    qaAssert(body.scrollTop>0&&getComputedStyle(body).overflowY==='auto','many excluded roots remain reachable inside a bounded scroll region');
    qaAssert(body.querySelectorAll('.permission-group-row').length===30&&!detail.querySelector('img')&&!window.__cleanScopeInjected,'invalid roots are discarded and native reasons remain escaped text');
    for(const label of body.querySelectorAll('b,small'))qaAssert(label.scrollWidth<=label.clientWidth+1&&getComputedStyle(label).overflowWrap==='anywhere','long excluded paths and causes wrap instead of overflowing');
    const last=body.lastElementChild.querySelector('button');window.isNativeMacApp=true;__qa.actions=[];last.click();
    qaAssert(__qa.actions.length===1&&__qa.actions[0].action==='revealInFinder'&&__qa.actions[0].path.includes('Excluded-29'),'the path action keeps the exact excluded directory, not its parent');
  `);
  Object.assign(scenarios.at(-1),{width:880,height:620});
}
add('clean-exclusions-refresh',`
  const payload={...structuredClone(qaFixture),cleanExcludedRoots:[{path:'~/Library/Caches',name:'缓存',reason:'白名单排除'}],readWarnings:[]};
  onNativeRealScanComplete(payload);switchModule('clean');
  const detail=document.querySelector('.clean-exclusions');qaAssert(detail,'excluded scope is visible before refresh');detail.open=true;await new Promise(resolve=>setTimeout(resolve,30));
  const summary=detail.querySelector('summary');summary.focus({preventScroll:true});renderModule();
  qaAssert(document.querySelector('.clean-exclusions').open&&document.activeElement.matches('.clean-exclusions > summary'),'same-module updates preserve scope disclosure and keyboard focus');
  onNativeRealScanComplete({...payload,cleanExcludedRoots:undefined});
  qaAssert(!document.querySelector('.clean-exclusions')&&STATE.cleanExcludedRoots.length===0,'a new receipt without excluded-root evidence does not retain stale scope exclusions');
`);
const agentDataFixture=[
  {...group('claude-code-cache','Claude Desktop · 可重建缓存','~/Library/Application Support/Claude/Code Cache',1,'review'),tool:'Claude Desktop',band:'可重建缓存'},
  {...group('copilot-diagnostics','GitHub Copilot CLI · 诊断日志','~/.copilot/logs',2,'review'),tool:'GitHub Copilot CLI',band:'诊断日志',impact:'保留最近 30 天、最新一份和仍被打开的日志；移除后无法用于历史排障。',subpaths:[{path:'~/.copilot/logs/process-old.log',sizeGb:1,size:'1 GB',tier:'review',checked:false,isDir:false},{path:'~/.copilot/logs/process-current.log',sizeGb:1,size:'1 GB',tier:'protected',checked:false,isDir:false,reason:'最新诊断日志保持原状'}]},
  {...group('gemini-data','Gemini CLI · 会话资料','~/.gemini/tmp',3,'protected'),tool:'Gemini CLI',band:'会话与配置（仅检查）',safetyReason:'会话与凭据保持原状'}
];
for(const theme of ['dark','light']){
  add('ai-agent-data-overview-'+theme,`
    document.documentElement.dataset.theme='${theme}';onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}});switchModule('ai_moon');
    const summary=document.querySelector('.ai-scan-coverage');
    qaAssert(summary&&summary.querySelector('[data-ai-tools]').textContent.includes('3'),'AI cleanup shows its detected tool coverage rather than a generic cache group');
    qaAssert(summary.querySelector('[data-ai-reviewable]').textContent.includes('2.00 GB')&&summary.querySelector('[data-ai-retained]').textContent.includes('4.00 GB'),'rebuildable and diagnostic data stay separate from retained resources');
    qaAssert(document.getElementById('heroStatValue').textContent==='0 B'&&document.getElementById('primaryBtnText').closest('button').disabled,'new AI data categories remain unchecked until explicit selection');
    const filter=document.querySelector('.ai-tool-filter select');
    qaAssert(filter&&filter.getAttribute('aria-label')&&filter.options.length===4,'tool selection has a native accessible control and only detected tools');
    qaAssert(document.getElementById('heroDesc').textContent.includes('不是 CLI')&&!document.getElementById('heroDesc').textContent.includes('删除会话'),'the page does not overpromise deleting protected histories');
  `);
  Object.assign(scenarios.at(-1),{width:880,height:620});
}
add('ai-agent-tool-filter-and-log-preview',`
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}});switchModule('ai_moon');
  const filter=document.querySelector('.ai-tool-filter select');qaAssert(filter,'AI tools have an explicit filter');filter.value='GitHub Copilot CLI';filter.dispatchEvent(new Event('change',{bubbles:true}));
  qaAssert(STATE.aiToolFilter==='GitHub Copilot CLI'&&document.querySelectorAll('#moduleStage .list-row').length===1,'the selected tool scopes both its rows and inspector');
  const inspector=document.querySelector('.inspector-content');
  qaAssert(inspector.textContent.includes('30 天')&&inspector.textContent.includes('process-current.log')&&inspector.querySelectorAll('input').length===1,'diagnostic retention is inspectable and current logs have no removal control');
  toggleMoonCheck(1);openConfirmSheet();
  qaAssert(confirmedClean.entries.length===1&&confirmedClean.entries[0].paths.length===1&&confirmedClean.entries[0].paths[0].endsWith('/process-old.log'),'selecting diagnostic cleanup freezes only the old closed file, never its directory');
  qaAssert(!pendingTrashRequests.size,'viewing the data preview never executes real cleanup');closeConfirmSheet();
  const retained=document.querySelector('.ai-tool-filter select');retained.value='Gemini CLI';retained.dispatchEvent(new Event('change',{bubbles:true}));
  qaAssert(!document.querySelector('.inspector-content input')&&document.querySelector('.inspector-content').textContent.includes('会话与凭据'),'retained session data has no removal checkbox');
`);
add('ai-agent-tool-filter-receipt-and-focus',`
  const payload={...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}};
  onNativeRealScanComplete(payload);switchModule('ai_moon');const select=document.querySelector('.ai-tool-filter select');qaAssert(select,'tool filter exists before the native refresh');
  select.value='Claude Desktop';select.dispatchEvent(new Event('change',{bubbles:true}));
  document.querySelector('.ai-tool-filter select').focus({preventScroll:true});renderModule();
  qaAssert(document.activeElement.matches('.ai-tool-filter select')&&document.activeElement.value==='Claude Desktop','same-module rendering preserves native filter focus and its current tool');
  onNativeRealScanComplete({...payload,aiMoonBands:payload.aiMoonBands.filter(item=>item.tool!=='Claude Desktop')});
  qaAssert(STATE.aiToolFilter==='all'&&document.querySelectorAll('#moduleStage .list-row').length===2,'a receipt removing a tool resets only its obsolete filter rather than hiding all results');
`);
add('ai-agent-excluded-only-tool',`
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:[],readWarnings:[],aiExcludedRoots:[{tool:'Claude Desktop',path:'~/Library/Application Support/Claude/Code Cache',reason:'白名单排除：~/Library/Application Support'}]});switchModule('ai_moon');
  const summary=document.querySelector('.ai-scan-coverage'),filter=document.querySelector('.ai-tool-filter select');
  qaAssert(summary.querySelector('[data-ai-tools]').textContent.includes('1')&&filter.options.length===2,'an excluded but recognized cache still counts as a detected tool');
  filter.value='Claude Desktop';filter.dispatchEvent(new Event('change',{bubbles:true}));
  qaAssert(STATE.aiToolFilter==='Claude Desktop'&&document.querySelector('.ai-exclusions').textContent.includes('体积未测量'),'the excluded tool remains filterable without inventing cleanup bytes');
  qaAssert(document.getElementById('heroHeadline').textContent.includes('排除')&&document.querySelector('.empty-title').textContent.includes('排除'),'an excluded AI tool is not reported as an absent tool or a cleaned disk');
  qaAssert(!document.querySelector('#moduleStage input')&&document.getElementById('primaryBtnText').closest('button').disabled,'a detected excluded tool never becomes a selectable cache');
`);
const expiredSessionFixture=[
  {...group('codex-expired-2025-09','Codex · 过期会话 · 2025-09','~/.codex/sessions/2025/09/26',0.5,'review'),tool:'Codex',band:'过期会话',ageText:'402–407 天前最后修改',impact:'这些是超过 90 天未修改的 Codex 对话历史，不能重建；会移入废纸篓，可在“历史”中还原。',checked:false,subpaths:[{path:'~/.codex/sessions/2025/09/26/rollout-a.jsonl',sizeGb:0.2,size:'0.20 GB',tier:'review',checked:false,isDir:false},{path:'~/.codex/sessions/2025/09/26/rollout-b.jsonl',sizeGb:0.3,size:'0.30 GB',tier:'review',checked:false,isDir:false}]},
  {...group('copilot-expired-logs','GitHub Copilot CLI · 诊断日志','~/.copilot/logs',0.1,'review'),tool:'GitHub Copilot CLI',band:'诊断日志',uninstalled:true,checked:false,subpaths:[{path:'~/.copilot/logs/old.log',sizeGb:0.1,size:'0.10 GB',tier:'review',checked:false,isDir:false}]}
];
add('ai-expired-sessions-and-uninstalled-tool',`
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(expiredSessionFixture)}});switchModule('ai_moon');
  qaAssert(STATE.aiMoonBands.every(item=>!item.checked)&&document.getElementById('heroStatValue').textContent==='0 B','expired conversations are never preselected');
  const options=[...document.querySelectorAll('.ai-tool-filter select option')].map(option=>option.textContent);
  qaAssert(options.includes('GitHub Copilot CLI · 已卸载')&&options.includes('Codex'),'only a tool detected as uninstalled carries the marker');
  const rows=[...document.querySelectorAll('#moduleStage .list-row .row-sub')].map(row=>row.textContent);
  qaAssert(rows.some(text=>text.includes('过期会话')&&text.includes('402–407 天前'))&&rows.some(text=>text.includes('诊断日志 · 已卸载')),'the band header shows age text and the uninstalled marker');
  selectMoonRow(0);
  qaAssert(document.querySelector('.inspector-content').textContent.includes('不能重建'),'expired history says it cannot be rebuilt');
  setAIToolFilter('GitHub Copilot CLI');selectMoonRow(1);
  qaAssert(document.querySelector('.inspector-content').textContent.includes('凭据')&&document.querySelector('.inspector-content').textContent.includes('已卸载'),'uninstalled copy says credentials stay protected');
  setAIToolFilter('all');toggleMoonSubpathCheck(0,1);openConfirmSheet();
  qaAssert(confirmedClean.entries.length===1&&confirmedClean.entries[0].paths.length===1&&confirmedClean.entries[0].paths[0].endsWith('/rollout-b.jsonl')&&!pendingTrashRequests.size,'review freezes only the explicitly selected expired session and never executes cleanup');closeConfirmSheet();
`);
Object.assign(scenarios.at(-1),{width:1380,height:760});
add('ai-tool-scope-statistics',`
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}});switchModule('ai_moon');setAIToolFilter('GitHub Copilot CLI');
  const coverage=()=>document.querySelector('.ai-scan-coverage');
  qaAssert(coverage().querySelector('[data-ai-measured]').textContent.includes('2.00 GB')&&coverage().querySelector('[data-ai-reviewable]').textContent.includes('1.00 GB')&&coverage().querySelector('[data-ai-retained]').textContent.includes('1.00 GB'),'tool-filtered measurements must not include another tool’s caches or sessions');
  const tabs=()=>[...document.querySelectorAll('#moduleStage .seg-tab')].map(tab=>tab.textContent.trim());
  qaAssert(tabs()[0]==='全部 (1)'&&tabs()[2]==='需确认 (1)'&&tabs()[3]==='受保护 (0)','safety-category counts follow the current tool rather than the full AI inventory');
  qaAssert(coverage().querySelector('[data-ai-scope]').textContent.includes('当前筛选'),'filtered capacity explicitly names its scope');
  document.getElementById('searchInput').value='Gemini';handleSearch('Gemini');
  qaAssert(coverage().querySelector('[data-ai-measured]').textContent.includes('0 B')&&tabs()[0]==='全部 (0)'&&!document.querySelector('#moduleStage .list-row'),'search and tool scope compose without silently reusing global totals');
  setAIToolFilter('all');
  qaAssert(coverage().querySelector('[data-ai-measured]').textContent.includes('3.00 GB')&&tabs()[3]==='受保护 (1)','keyword scope reports the measured session row without claiming it is removable');
  qaAssert(STATE.aiMoonBands.every(item=>!item.checked),'scope and capacity changes do not authorize cleanup');
`);
add('ai-tool-scope-exclusions',`
  const excluded=[{tool:'Claude Desktop',path:'~/Library/Application Support/Claude/Cache',reason:'白名单排除'},{tool:'Windsurf',path:'~/Library/Caches/Windsurf',reason:'白名单排除'}];
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)},aiExcludedRoots:excluded,readWarnings:[]});switchModule('ai_moon');setAIToolFilter('Gemini CLI');
  qaAssert(!document.querySelector('.ai-exclusions'),'an unrelated tool’s exclusions cannot appear to be part of the current tool');
  setAIToolFilter('Windsurf');
  const detail=document.querySelector('.ai-exclusions');
  qaAssert(detail&&detail.querySelector('summary').textContent.includes('1 个')&&detail.querySelector('.permission-group-list').textContent.includes('Windsurf')&&!detail.querySelector('.permission-group-list').textContent.includes('Claude'),'excluded roots follow the same tool scope as the list and counts');
  qaAssert(document.querySelector('.empty-title').textContent.includes('排除')&&!document.querySelector('#moduleStage .list-row'),'an excluded-only tool explains its own empty results even when other tools have candidates');
  qaAssert(document.getElementById('primaryBtnText').closest('button').disabled&&!__qa.actions.some(action=>['addWhitelist','removeWhitelist','trashRealPaths'].includes(action.action)),'scope disclosure does not remove a whitelist rule or enable cleanup');
`);
for(const theme of ['dark','light']){
  add('ai-hidden-selection-tool-'+theme,`
    document.documentElement.dataset.theme='${theme}';onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}});switchModule('ai_moon');
    toggleMoonCheck(0);toggleMoonCheck(1);setAIToolFilter('Gemini CLI');
    const notice=document.querySelector('.ai-selection-notice');
    qaAssert(notice&&notice.getAttribute('role')==='status'&&notice.textContent.includes('3 个已选路径')&&notice.textContent.includes('2.00 GB'),'the filtered-out selected cache and old log remain explicitly counted before review');
    qaAssert(notice.textContent.includes('全部已选')&&document.getElementById('heroStatLabel').textContent.includes('全部已选'),'filtering is not presented as limiting the frozen selection to the current tool');
    qaAssert(!document.querySelector('.inspector-content input')&&!document.getElementById('primaryBtnText').closest('button').disabled,'protected current-tool rows and the prior global selection remain separate intents');
    openConfirmSheet();
    const paths=confirmedClean.entries.flatMap(entry=>entry.paths);
    qaAssert(paths.length===3&&paths.some(path=>path.endsWith('/process-old.log'))&&paths.every(path=>!path.includes('/.gemini/')),'review preserves all explicitly selected paths and never includes the inspected protected tool');
    qaAssert(!pendingTrashRequests.size,'showing hidden selections or their review never submits cleanup');closeConfirmSheet();
  `);
  Object.assign(scenarios.at(-1),{width:880,height:620});
}
add('ai-hidden-selection-search-tier-recovery',`
  onNativeRealScanComplete({...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)}});switchModule('ai_moon');toggleMoonCheck(0);
  setAIToolFilter('GitHub Copilot CLI');setAIFilter('protected');document.getElementById('searchInput').value='Gemini';handleSearch('Gemini');
  const notice=document.querySelector('.ai-selection-notice');
  qaAssert(notice&&notice.textContent.includes('2 个已选路径')&&notice.textContent.includes('1.00 GB'),'keyword and safety filtering retain the hidden selection warning');
  const restore=notice.querySelector('button');qaAssert(restore,'hidden selections have an explicit review recovery action');restore.focus({preventScroll:true});__qa.actions=[];restore.click();
  qaAssert(STATE.aiToolFilter==='all'&&STATE.aiFilter==='all'&&!STATE.searchQuery&&!document.getElementById('searchInput').value,'showing the previous selection clears the tool, safety and keyword display filters');
  qaAssert(document.querySelectorAll('#moduleStage .list-row').length===3&&!document.querySelector('.ai-selection-notice'),'restored scope contains the chosen cache without duplicate hidden-selection messages');
  qaAssert(document.activeElement.matches('.ai-tool-filter select'),'focus returns to a surviving native filter rather than a vanished notice');
  qaAssert(STATE.aiMoonBands[0].checked&&!STATE.aiMoonBands[1].checked&&!STATE.aiMoonBands[2].checked&&!__qa.actions.length,'display recovery preserves the exact previous selection and does not send native actions');
`);
add('ai-tool-scope-filter-focus',`
  const payload={...structuredClone(qaFixture),aiMoonBands:${JSON.stringify(agentDataFixture)},aiExcludedRoots:[{tool:'Claude Desktop',path:'~/Library/Application Support/Claude/Cache',reason:'白名单排除'}],readWarnings:[]};
  onNativeRealScanComplete(payload);switchModule('ai_moon');
  const disclosure=document.querySelector('.ai-exclusions');disclosure.open=true;await new Promise(resolve=>setTimeout(resolve,30));
  const filter=document.querySelector('.ai-tool-filter select');filter.focus({preventScroll:true});filter.value='Gemini CLI';filter.dispatchEvent(new Event('change',{bubbles:true}));
  qaAssert(document.activeElement.matches('.ai-tool-filter select')&&document.activeElement.value==='Gemini CLI','removing unrelated exclusion controls keeps focus on the same native tool filter');
  onNativeRealScanComplete({...payload,aiExcludedRoots:[...payload.aiExcludedRoots,{tool:'Gemini CLI',path:'~/.gemini/cache',reason:'白名单排除'}]});
  qaAssert(document.activeElement.matches('.ai-tool-filter select')&&document.activeElement.value==='Gemini CLI','new corresponding exclusion evidence cannot shift filter focus to another action');
`);
const partialReason = '测量未完成，体积只是已读取部分；重新扫描完整后再处理';
add('measure-full-orphan', `
  const reason=${JSON.stringify(partialReason)};
  const sub={path:'~/Library/Application Support/Legacy Canvas',kind:'config',size:'0.40 GB',sizeGb:0.4,partial:true,checked:false,tier:'protected',reason};
  onNativeRealScanComplete({...structuredClone(qaFixture),orphanLeftoversList:[{...structuredClone(qaFixture.orphanLeftoversList[0]),partial:true,leftoverSizeGb:0.4,tier:'protected',safetyReason:reason,subpaths:[sub]}]});
  switchModule('software');setSoftwarePanelMode('leftovers');
  const button=()=>document.querySelector('[data-measure-path]');
  const stage=()=>document.getElementById('moduleStage');
  const post=()=>__qa.actions.filter(a=>a.action==='measurePathFully');
  qaAssert(button()&&button().textContent.trim()==='完整测量'&&!button().disabled,'a partial leftover offers a full measurement');
  qaAssert(stage().querySelector('input[type=checkbox][onchange*="toggleOrphanSubpathCheck"]').disabled,'a partial leftover stays unselectable before measuring');
  button().click();
  qaAssert(post().length===1&&post()[0].path===sub.path&&post()[0].requestId,'the click posts one correlated request');
  qaAssert(button().disabled&&button().getAttribute('aria-busy')==='true'&&button().textContent.includes('测量中'),'the button shows a busy state');
  button().click();qaAssert(post().length===1,'a busy button cannot post a second request');
  onNativeMeasureResult({requestId:'stale',path:sub.path,ok:true,bytes:1,sizeGb:1,partial:false});
  qaAssert(button().disabled&&STATE.orphanLeftoversList[0].subpaths[0].partial,'an unknown receipt settles nothing');
  onNativeMeasureResult({requestId:post()[0].requestId,path:sub.path,ok:false,reason:'受保护路径，不能测量'});
  qaAssert(!button().disabled&&stage().textContent.includes('受保护路径，不能测量'),'a refusal shows its reason and frees the button');
  qaAssert(STATE.orphanLeftoversList[0].subpaths[0].partial&&STATE.orphanLeftoversList[0].tier==='protected','a refusal leaves the row protected');
  button().click();
  onNativeMeasureResult({requestId:post()[1].requestId,path:sub.path,ok:true,bytes:1500000000,sizeGb:1.5,files:3,partial:true,reason:'超过 2 分钟仍未读完，体积只是已读取部分'});
  qaAssert(stage().textContent.includes('超过 2 分钟仍未读完')&&button(),'a still-partial result keeps the honest reason and the button');
  qaAssert(STATE.orphanLeftoversList[0].subpaths[0].partial&&STATE.orphanLeftoversList[0].subpaths[0].sizeGb===1.5,'a still-partial result only raises the measured lower bound');
  button().click();
  onNativeMeasureResult({requestId:post()[2].requestId,path:sub.path,ok:true,bytes:5830000000,sizeGb:5.83,files:90000,partial:false});
  const row=STATE.orphanLeftoversList[0];
  qaAssert(!button()&&!row.partial&&!row.subpaths[0].partial&&row.tier!=='protected'&&!row.subpaths[0].reason,'a complete measurement clears the partial state');
  qaAssert(row.leftoverSizeGb===5.83&&row.subpaths[0].size==='5.83 GB'&&stage().textContent.includes('5.83 GB'),'the row shows the measured size');
  const box=stage().querySelector('input[type=checkbox][onchange*="toggleOrphanSubpathCheck"]');
  qaAssert(!box.disabled&&!box.checked&&!row.subpaths[0].checked,'a measured path becomes selectable but is never pre-selected');
  toggleOrphanSubpathCheck(0,0);
  qaAssert(selectedSubpaths(row).length===1,'selection follows the normal rules afterwards');
  qaAssert(__qa.actions.every(a=>a.action==='measurePathFully'),'measuring never submits a move');
`);
add('measure-full-app-subpath', `
  const reason=${JSON.stringify(partialReason)};
  const app=structuredClone(qaFixture.appsList[0]);
  app.subpaths=[{path:app.appPath,kind:'app',size:'0.10 GB',sizeGb:0.1,checked:false},{path:'~/Library/Caches/org.example.canvas.0',kind:'cache',size:'2.00 GB',sizeGb:2,checked:false,partial:true,tier:'protected',reason}];
  Object.assign(app,{appSizeGb:0.1,cacheSizeGb:2,configSizeGb:0,leftoverSizeGb:2,partial:false});
  onNativeRealScanComplete({...structuredClone(qaFixture),appsList:[app,...structuredClone(qaFixture.appsList.slice(1))]});
  switchModule('software');setSoftwarePanelMode('apps');selectAppRow(STATE.appsList.findIndex(a=>a.id===app.id));
  const button=()=>document.querySelector('.app-path-entry [data-measure-path]');
  qaAssert(button()&&document.querySelectorAll('[data-measure-path]').length===1,'only the partial app path offers a full measurement');
  button().click();
  const request=__qa.actions.find(a=>a.action==='measurePathFully');
  qaAssert(request&&request.path==='~/Library/Caches/org.example.canvas.0'&&button().disabled,'the app subpath request is correlated and busy');
  onNativeMeasureResult({requestId:request.requestId,path:request.path,ok:true,bytes:6580000000,sizeGb:6.58,files:120000,partial:false});
  const row=STATE.appsList.find(a=>a.id===app.id);
  qaAssert(!button()&&row.cacheSizeGb===6.58&&row.leftoverSizeGb===6.58&&row.appSizeGb===0.1,'the app totals are recomputed from measured subpaths');
  qaAssert(document.getElementById('moduleStage').textContent.includes('6.58 GB')&&!row.subpaths[1].partial&&!row.subpaths[1].reason,'the cache subpath shows its measured size');
  qaAssert(!row.subpaths[1].checked&&!selectedSubpaths(row,'standard').includes(row.subpaths[1]),'the measured cache is not pre-selected');
`);
add('measure-full-purge', `
  const reason=${JSON.stringify(partialReason)};
  const p='~/code/deepseek-harness/node_modules';
  const item={...structuredClone(qaFixture.purgeItems[0]),id:'partial-build',path:p,project:'deepseek-harness',partial:true,checked:false,tier:'protected',safetyReason:reason,sizeGb:0.5,origSizeGb:0.5,subpaths:[{path:p,sizeGb:0.5,size:'0.50 GB',partial:true,tier:'protected',checked:false,reason}]};
  onNativeRealScanComplete({...structuredClone(qaFixture),purgeItems:[item]});
  switchModule('purge');
  const button=()=>document.querySelector('[data-measure-path]');
  const post=()=>__qa.actions.filter(a=>a.action==='measurePathFully');
  qaAssert(button()&&button().textContent.trim()==='完整测量','a partial build folder offers a full measurement');
  button().click();
  qaAssert(post().length===1&&post()[0].path===p,'the click posts the build folder path');
  onNativeMeasureResult({requestId:post()[0].requestId,path:p,ok:true,bytes:4200000000,sizeGb:4.2,files:120000,partial:false});
  const row=STATE.purgeItems[0];
  qaAssert(!button()&&!row.partial&&!row.subpaths[0].partial&&row.tier!=='protected'&&!row.safetyReason&&!row.subpaths[0].reason,'a complete measurement clears the partial state');
  qaAssert(row.sizeGb===4.2&&row.origSizeGb===4.2,'the build folder keeps the measured size');
  qaAssert(!row.checked&&!row.subpaths[0].checked&&document.getElementById('navMetaPurge').textContent==='未勾选','a measured build folder is never pre-selected');
  togglePurgeCheck(0);
  qaAssert(row.checked&&document.getElementById('navMetaPurge').textContent==='4.2 GB','selecting the measured folder updates the sidebar badge');
  qaAssert(__qa.actions.every(a=>a.action==='measurePathFully'),'measuring never submits a move');
`);
add('measure-full-clean-and-ai', `
  const reason=${JSON.stringify(partialReason)};
  const mk=(id,baseTier)=>({...structuredClone(qaFixture.cleanGroups[0]),id,tier:'protected',baseTier,checked:false,partial:true,safetyReason:reason,sizeGb:1.5,origSizeGb:1.5,subpaths:[
    {path:'~/Library/Caches/'+id+'/done',sizeGb:1,size:'1.00 GB',tier:'protected',checked:false,reason:'当前正在使用'},
    {path:'~/Library/Caches/'+id+'/pnpm',sizeGb:0.5,size:'0.50 GB',partial:true,tier:'protected',checked:false,reason}]});
  const group=mk('pnpm','review'), locked=mk('keep','protected'), band={...mk('ai-cache','review'),band:'运行缓存'};
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[group,locked],aiMoonBands:[band]});
  switchModule('clean');
  const buttons=()=>[...document.querySelectorAll('[data-measure-path]')].map(b=>b.dataset.measurePath);
  const post=()=>__qa.actions.filter(a=>a.action==='measurePathFully');
  selectCleanRow(0);
  qaAssert(buttons().length===1&&buttons()[0]===group.subpaths[1].path,'only the partial path of a non-protected group offers a measurement');
  selectCleanRow(1);qaAssert(!buttons().length,'a group protected by design never offers a measurement');
  selectCleanRow(0);
  document.querySelector('[data-measure-path]').click();
  onNativeMeasureResult({requestId:post()[0].requestId,path:group.subpaths[1].path,ok:true,bytes:3000000000,sizeGb:3,files:900,partial:false});
  const g=STATE.cleanGroups[0];
  qaAssert(!g.partial&&!g.subpaths[1].partial&&!g.subpaths[1].reason&&!g.subpaths[1].checked,'the measured subpath is cleared but never pre-checked');
  qaAssert(g.sizeGb===4&&g.origSizeGb===4&&!g.safetyReason&&g.tier==='review'&&!g.checked,'the group total follows its subpaths and stays unchecked');
  qaAssert(STATE.cleanGroups[1].partial&&STATE.cleanGroups[1].tier==='protected','a group protected by design stays untouched');
  switchModule('ai_moon');
  qaAssert(buttons().length===1,'a partial AI band path offers a measurement');
  document.querySelector('[data-measure-path]').click();
  onNativeMeasureResult({requestId:post()[1].requestId,path:band.subpaths[1].path,ok:true,bytes:2000000000,sizeGb:2,files:10,partial:false});
  qaAssert(STATE.aiMoonBands[0].sizeGb===3&&!STATE.aiMoonBands[0].partial,'the AI band total is recomputed');
  qaAssert(__qa.actions.every(a=>a.action==='measurePathFully'),'measuring never submits a move');
`);
add('purge-installer-empty-state', `
  onNativeRealScanComplete({...structuredClone(qaFixture),installerFiles:[]});switchModule('purge');
  const note=document.querySelector('[data-installer-empty]');
  qaAssert(note&&note.textContent.includes('下载、桌面、文稿')&&note.textContent.includes('mpkg'),'an empty installer list says what was checked');
  onNativeRealScanComplete({...structuredClone(qaFixture)});switchModule('purge');
  qaAssert(!document.querySelector('[data-installer-empty]'),'the empty state disappears when installers exist');
`);
add('analyze-hover-readout-outside-map', `
  switchModule('analyze');
  const stage=document.querySelector('.treemap-stage'), preview=document.getElementById('tilePreview');
  const tile=document.querySelector('.treemap-tile[data-idx="1"]');tile.style.width='100px';
  tile.dispatchEvent(new MouseEvent('pointerover',{bubbles:true}));await new Promise(r=>setTimeout(r,140));
  qaAssert(preview.classList.contains('show'),'hovering a small tile fills the readout');
  const pr=preview.getBoundingClientRect(), sr=stage.getBoundingClientRect(), before=sr.height;
  qaAssert(pr.top>=sr.bottom-0.5,'the readout sits below the map, not over a tile');
  qaAssert(pr.right<=sr.right+0.5&&pr.left>=sr.left-0.5,'the readout stays inside the map column');
  tile.dispatchEvent(new MouseEvent('pointerout',{bubbles:true}));
  qaAssert(stage.getBoundingClientRect().height===before,'showing and hiding the readout never resizes the map');
`);
scenarios[scenarios.length-1].width=880;scenarios[scenarios.length-1].height=620;
add('clean-inspector-scan-skeleton', `
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[]});switchModule('clean');
  const text=()=>document.getElementById('moduleStage').textContent;
  qaAssert(text().includes('选择左侧任一项查看详情')||!document.querySelector('.panel-blank'),'a finished empty scan renders without a scan skeleton');
  qaAssert(!document.querySelector('[data-inspector-scanning]'),'no skeleton after the scan finished');
  onNativeRealScanStarted();
  const blank=document.querySelector('[data-inspector-scanning]');
  qaAssert(blank&&blank.querySelectorAll('.skel').length>=3&&blank.getAttribute('role')==='status','a running scan shows skeleton lines in the inspector');
  qaAssert(!text().includes('选择左侧任一项查看详情'),'a running scan never asks for a selection');
`);
// English: every module, live-switched from Chinese, at a wide and a narrow window. Fails on any CJK left on screen
// (user data and the 中 / EN toggle are exempt) and checks the usual overflow rules.
for (const [width, theme] of [[1380, 'dark'], [880, 'light']]) for (const mod of modules) {
  scenarios.push({ name: `en-${width}-${mod}`, width, height: width === 880 ? 620 : 860, script: `qaReset(); document.documentElement.dataset.theme='${theme}'; switchModule('${mod}'); await qaEn('${mod}');` });
}
add('en-live-roundtrip', `const modules=${JSON.stringify(modules)};for(const mod of modules){switchModule(mod);await qaSettle();const zh=qaStageText();qaAssert(/[㐀-鿿]/.test(zh),mod+' starts in Chinese');await qaEn(mod);qaAssert(!/[㐀-鿿]/.test(qaStageText().replace(/测试编辑工具 \\d+|测试视频\\.mov/g,'')),mod+' header and stage are English');I18N.setLang('zh',false);await qaSettle();qaAssert(document.documentElement.lang==='zh-CN','html lang returns to zh-CN');qaAssert(qaStageText()===zh,mod+' returns to the same Chinese text');}`);
add('en-modal-doctor', `openDoctorModal(true);onNativeDoctorReport({requestId:_doctorRequestId,checks:[{id:'disk',title:'磁盘剩余空间',detail:'隔离测试结果',passed:true},{id:'permission',title:'完全磁盘访问',detail:'隔离测试结果',passed:false}]});await qaEn('doctor',async()=>{});qaAssert(document.getElementById('doctorModal').classList.contains('open'),'doctor stays open across the language switch');`);
add('en-modal-doctor-waiting', `await qaEn('doctor waiting',async()=>{openDoctorModal(true);});`);
add('en-modal-permissions', `I18N.add({'受保护目录读取被拒绝':'Fixture text'});openRuntimePanel();const request=__qa.actions.find(a=>a.action==='requestRuntimeStatus');onNativeRuntimeStatus({requestId:request.requestId,packaged:true,appPath:'/Applications/orbit.app',identifier:'com.mymoles.mole-studio',version:'0.1.0',menuBarRegistered:true,readAccess:{passed:false,detail:'受保护目录读取被拒绝'}});await qaEn('permissions',async()=>{});qaAssert(document.getElementById('runtimeModal').classList.contains('open'),'permissions stay open across the switch');`);
add('en-modal-permissions-dev', `openRuntimePanel();const request=__qa.actions.find(a=>a.action==='requestRuntimeStatus');onNativeRuntimeStatus({requestId:request.requestId,packaged:false,appPath:'/tmp/go-build123/b001/exe/mole-studio',executablePath:'/tmp/go-build123/b001/exe/mole-studio',identifier:'',version:'dev',menuBarRegistered:true,readAccess:{passed:null,detail:''}});await qaEn('permissions (dev run)',async()=>{});`);
add('en-modal-confirm-clean', `switchModule('clean');await qaEn('confirm clean',async()=>{openConfirmSheet();});qaAssert(document.querySelector('.modal-backdrop.open'),'confirm sheet is open');`);
add('en-modal-confirm-apps', `switchModule('software');toggleAppCheck(0);await qaEn('confirm uninstall',async()=>{openConfirmSheet();});`);
add('en-modal-palette', `await qaEn('palette',async()=>{openCommandPalette('');});qaAssert(document.querySelectorAll('#cmdPaletteList .list-row').length>=8,'palette lists every module');`);
add('en-modal-palette-search', `await qaEn('palette search',async()=>{openCommandPalette('Optimize');});qaAssert(document.querySelectorAll('#cmdPaletteList .list-row').length===1,'palette filters by the English module name');document.querySelector('#cmdPaletteList .list-row').click();qaAssert(STATE.module==='optimize','palette reaches Optimize by its English name');`);
add('en-popover', `document.body.classList.add('menubar-mode');renderMenuBarTrayContent();await qaEn('popover',async()=>{});const shell=document.getElementById('menubarTrayShell');const over=[];shell.querySelectorAll('button,[data-tray-key]').forEach(el=>{const r=el.getBoundingClientRect();if(!r.width)return;if(r.left<-1||r.right>innerWidth+1)over.push('viewport:'+(el.dataset.trayKey||el.className));if(el.scrollWidth>el.clientWidth+2)over.push('clipped:'+(el.dataset.trayKey||el.className)+' '+el.scrollWidth+'>'+el.clientWidth);});qaAssert(over.length===0,'popover overflow: '+over.join(', '));qaAssert(shell.scrollHeight<=innerHeight+1,'popover fits its fixed height: '+shell.scrollHeight+'>'+innerHeight);qaAssert(shell.querySelector('[data-tray-key="update"]'),'popover has the update button');`);
scenarios[scenarios.length-1].width=356;scenarios[scenarios.length-1].height=588;
add('en-popover-follows-main', `document.body.classList.add('menubar-mode');window.isMenuBarPopover=true;try{renderMenuBarTrayContent();qaAssert(/[㐀-鿿]/.test(document.getElementById('menubarTrayShell').textContent),'popover starts in Chinese');onNativeLanguage('en');await qaSettle();qaAssert(document.documentElement.lang==='en','popover html lang follows');const leaks=qaLeaks();qaAssert(leaks.length===0,'popover leaks: '+leaks.join(' | '));onNativeLanguage('zh');await qaSettle();qaAssert(/[㐀-鿿]/.test(document.getElementById('menubarTrayShell').textContent),'popover returns to Chinese');}finally{window.isMenuBarPopover=false;}`);
scenarios[scenarios.length-1].width=356;scenarios[scenarios.length-1].height=588;
const emptyAll = `onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],aiMoonBands:[],appsList:[],orphanLeftoversList:[],purgeItems:[],installerFiles:[],largeFiles:[]});STATE.historyLogs=[];STATE.treemap2D=[];STATE.analyzeLargeFiles=[];STATE.analyzeAllocGb=0;STATE.totalAnalyzedGb=0;STATE.analyzeFileCount=0;`;
for (const mod of modules) scenarios.push({ name: `en-empty-${mod}`, width: 1380, height: 860, script: `qaReset();${emptyAll}switchModule('${mod}');await qaEn('empty ${mod}');` });
add('en-no-matches', `switchModule('software');handleSearch('unmatched-keyword');await qaEn('no matches');switchModule('clean');handleSearch('unmatched-keyword');await qaEn('no matches clean');switchModule('analyze');handleSearch('no-such-path');await qaEn('no matches analyze');`);
add('en-trash-failure', `switchModule('clean');await qaEn('partial trash failure',async()=>{openConfirmSheet();executeConfirmedClean();const request=__qa.actions.find(a=>a.action==='trashRealPaths');onNativeTrashResult({requestId:request.requestId,moved:[{original:request.paths[0],trashed:'/trash/one'}],movedCount:1,failed:request.paths.slice(1).map(path=>({path,error:'Permission denied',pane:'fullDiskAccess'}))});});qaAssert(document.getElementById('permHint'),'recovery action is still offered in English');`);
add('en-scan-permission-partial', `I18N.add({'目录读取超时，未计入本次扫描':'Fixture text'});onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],readWarnings:[{path:qaFixture.homePath+'/Desktop',reason:'目录读取超时，未计入本次扫描'}]});switchModule('clean');await qaEn('scan permission partial');`);
// ── 分级汇总 / ≥ 下限 / 受保护解锁 / 按 kind 的测量告警 / 清理后的刷新态（赛道 C）──
// 载荷在场景内就地构造：reason、unlock、partial 都是 Go 侧才有的字段，基础夹具保持旧形态以覆盖旧载荷的兜底。
const tierEnglish = {
  '与当前项目绑定，确认不再需要后再清理。': 'Tied to the current project. Clean it once you are sure it is no longer needed.',
  '正在被运行中的应用使用，保持原状。': 'In use by a running app, so it stays as it is.',
  '退出相关应用后重新扫描即可解锁。': 'Quit the related apps and rescan to unlock.',
  '构建产物可由依赖清单重新生成。': 'Build output can be regenerated from the dependency manifest.',
  '安装包已经装过，确认后再删除。': 'Already installed. Confirm before deleting the installer.',
};
const tierSetup = `
  const R_REVIEW='与当前项目绑定，确认不再需要后再清理。',R_PROT='正在被运行中的应用使用，保持原状。',UNLOCK='退出相关应用后重新扫描即可解锁。';
  I18N.add(${JSON.stringify(tierEnglish)});
  const tp=structuredClone(qaFixture),mark=(row,extra)=>{Object.assign(row,extra);row.subpaths.forEach(sp=>Object.assign(sp,extra,{checked:row.tier==='safe'}));return row;};
  tp.cleanGroups[1].partial=true;tp.cleanGroups[1].subpaths[0].partial=true;
  mark(tp.cleanGroups[2],{reason:R_REVIEW});
  tp.cleanGroups.push(mark({...tp.cleanGroups[0],id:'locked',name:'应用运行缓存',path:'~/Library/Caches/Locked',sizeGb:4,tier:'protected',checked:false,subpaths:[{...tp.cleanGroups[0].subpaths[0],path:'~/Library/Caches/Locked/one',sizeGb:4}]},{reason:R_PROT,unlock:UNLOCK}));
  tp.aiMoonBands[0].partial=true;tp.aiMoonBands[0].subpaths[0].partial=true;
  tp.aiMoonBands[2].unlock=UNLOCK;tp.aiMoonBands[2].reason='正在被运行中的应用使用，保持原状。';
  const pi=tp.purgeItems[0];
  tp.purgeItems=[{...pi,tier:'safe',checked:true,reason:'构建产物可由依赖清单重新生成。'},{...pi,id:'p2',artifact:'target',path:'~/Projects/Canvas/target',sizeGb:3,origSizeGb:3,tier:'review',checked:false,reason:R_REVIEW},{...pi,id:'p3',artifact:'.venv',path:'~/Projects/Canvas/.venv',sizeGb:1.5,origSizeGb:1.5,tier:'protected',checked:false,reason:R_PROT,unlock:UNLOCK},{...pi,id:'p4',artifact:'build',path:'~/Projects/Canvas/build',sizeGb:0.6,origSizeGb:0.6,tier:'safe',checked:true,partial:true,reason:'构建产物可由依赖清单重新生成。'}];
  tp.installerFiles=[{...tp.installerFiles[0],tier:'review',checked:false,reason:'安装包已经装过，确认后再删除。'}];
  onNativeRealScanComplete(tp);
`;
const tierExpect = {
  clean: { safe: 4, review: 2, protected: 4, rows: 'STATE.cleanGroups' },
  ai_moon: { safe: 1, review: 0.2, protected: 5.6, rows: 'STATE.aiMoonBands' },
  purge: { safe: 2.6, review: 4, protected: 1.5, rows: '[].concat(STATE.purgeItems,STATE.installerFiles)' },
};
for (const mod of Object.keys(tierExpect)) for (const theme of ['dark', 'light']) for (const lang of ['zh', 'en']) {
  const e = tierExpect[mod];
  add(`tier-summary-${mod}-${theme}-${lang}`, `
    document.documentElement.dataset.theme='${theme}';${tierSetup}switchModule('${mod}');await qaSettle();
    const bar=document.querySelector('[data-tier-summary="${mod}"]');
    qaAssert(bar,'${mod} shows the recommendation summary');
    const chip=k=>bar.querySelector('[data-tier-'+k+']').textContent;
    qaAssert(chip('safe').includes(fmtDiskSizeGb(${e.safe})),'recommended size is the sum of safe paths: '+chip('safe'));
    qaAssert(chip('review').includes(fmtDiskSizeGb(${e.review})),'review size: '+chip('review'));
    qaAssert(chip('protected').includes(fmtDiskSizeGb(${e.protected})),'protected size: '+chip('protected'));
    qaAssert(chip('safe').includes('≥ ')&&bar.querySelector('[data-tier-partial-note]'),'a partial safe path turns the recommended size into a lower bound');
    qaAssert(!chip('review').includes('≥ ')&&!chip('protected').includes('≥ '),'tiers without partial paths keep exact sizes');
    const rows=${e.rows};
    qaAssert(rows.filter(r=>r.tier==='safe').every(r=>r.checked)&&rows.filter(r=>r.tier!=='safe').every(r=>!r.checked),'safe rows arrive checked and nothing else does');
    qaAssert(document.querySelectorAll('#moduleStage .tier-unlock').length>=1,'a protected row says how to unlock it');
    qaAssert(document.querySelector('#moduleStage .lock-glyph'),'protected rows show a lock instead of a checkbox');
    qaAssert(/≥ /.test(document.getElementById('moduleStage').textContent),'a partial size is shown as a lower bound');
    // 勾上需确认项再点「只选推荐项」：恰好回到推荐集合。
    rows.forEach(r=>{if(r.tier==='review'){r.checked=true;(r.subpaths||[]).forEach(sp=>{sp.checked=true;});}});
    bar.querySelector('[data-tier-select]').click();await qaSettle();
    qaAssert(rows.filter(r=>r.tier==='safe').every(r=>r.checked)&&rows.filter(r=>r.tier!=='safe').every(r=>!r.checked&&(r.subpaths||[]).every(sp=>!sp.checked)),'select recommended leaves exactly the safe set checked');
    ${lang === 'en' ? `await qaEn('tier summary ${mod}');qaAssert(document.querySelector('[data-tier-summary="${mod}"] [data-tier-safe]').textContent.startsWith('Recommended'),'English summary reads Recommended');` : `qaAssert(document.querySelector('[data-tier-summary="${mod}"] [data-tier-safe]').textContent.startsWith('推荐清理'),'summary headline');`}
  `);
}
add('tier-partial-does-not-block-clean', `
  ${tierSetup}switchModule('clean');await qaSettle();
  qaAssert(!document.getElementById('btnPrimary')?.disabled&&!document.querySelector('.top-header .btn-primary:disabled'),'a partial path never disables the primary action');
  openConfirmSheet();await qaSettle();qaAssert(document.querySelector('.modal-backdrop.open'),'confirm opens with a partial path selected');
  executeConfirmedClean();const req=__qa.actions.find(a=>a.action==='trashRealPaths');
  qaAssert(req&&req.paths.some(p=>p.endsWith('/Canvas/one')),'the partial path is sent to the Trash like any other');
  qaAssert(!req.paths.some(p=>p.includes('/Locked/')||p.endsWith('/target/one')),'protected and review paths are never sent');
`);
add('tier-legacy-payload-no-summary', `
  const lp=structuredClone(qaFixture);lp.cleanGroups.forEach(g=>{delete g.tier;g.subpaths.forEach(sp=>delete sp.tier);});
  onNativeRealScanComplete(lp);switchModule('clean');await qaSettle();
  qaAssert(!document.querySelector('[data-tier-summary]'),'a payload without tiers gets no invented summary');
  qaAssert(!document.querySelector('#moduleStage .tier-note'),'and no invented per-row tier note');
`);
add('tier-generic-sentence-without-reason', `
  ${tierSetup}tp.cleanGroups[2].reason='';tp.cleanGroups[2].subpaths.forEach(sp=>{sp.reason='';});onNativeRealScanComplete(tp);switchModule('clean');selectCleanRow(2);await qaSettle();
  qaAssert(document.querySelector('[data-tier-note="review"]')?.textContent.includes('用途需要你确认'),'a review row without reason gets the generic per-tier sentence');
  await qaEn('generic tier sentence',async()=>{});
  qaAssert(document.querySelector('[data-tier-note="review"]')?.textContent.includes('confirm'),'the generic sentence is translated');
`);
// 测量告警按 kind 分组：每种 kind 的标题、徽章、模块内提示、运行时横幅各自独立。
const warnKinds = {
  permission: { warnings: `[{path:qaFixture.homePath+'/Library/Mail',reason:'读取被拒绝（permission denied）',kind:'permission'},{path:qaFixture.homePath+'/Library/Safari',reason:'读取被拒绝（permission denied）',kind:'permission'}]`, head: '2 个路径没有读取权限，请授予完全磁盘访问权限', badge: '部分路径无读取权限', banner: true, pane: true },
  limit: { warnings: `[{path:qaFixture.homePath+'/Projects/Very Large Build',reason:'达到目录探索上限，未完成测量',kind:'limit'},{path:qaFixture.homePath+'/Projects/Huge Cache',reason:'达到目录探索上限，未完成测量',kind:'limit'}]`, head: '权限正常；2 个较大目录只测量了部分体积（不影响清理）', badge: '部分体积为下限', banner: false, pane: false },
  system: { warnings: `[{path:'/Library/Application Support/com.apple.TCC',reason:'macOS 系统保护，无法读取',kind:'system'}]`, head: '1 个位置受 macOS 系统保护，无需处理', badge: '', banner: false, pane: false },
  error: { warnings: `[{path:qaFixture.homePath+'/Projects/Broken',reason:'读取失败（i/o error）',kind:'error'}]`, head: '部分路径未完成测量，请先查看原因', badge: '部分测量受限', banner: true, pane: false },
  legacy: { warnings: `[{path:qaFixture.homePath+'/Library/Containers/Denied App/Data',reason:'读取被拒绝（permission denied）'},{path:qaFixture.homePath+'/Projects/Very Large Build',reason:'达到目录探索上限，未完成测量'}]`, head: '本机扫描：2 个路径未完成测量，按目录查看原因', badge: '部分测量受限', banner: true, pane: true },
};
for (const kind of Object.keys(warnKinds)) for (const lang of ['zh', 'en']) {
  const w = warnKinds[kind];
  add(`warnings-kind-${kind}-${lang}`, `
    I18N.add({'读取被拒绝（permission denied）':'Read denied (permission denied)','达到目录探索上限，未完成测量':'Hit the folder exploration cap; not fully measured','macOS 系统保护，无法读取':'Protected by macOS; cannot be read','读取失败（i/o error）':'Read failed (i/o error)'});
    onNativeRealScanComplete({...structuredClone(qaFixture),readWarnings:${w.warnings}});switchModule('clean');await qaSettle();
    const hint=[...document.querySelectorAll('#moduleStage details.panel-card')].find(d=>d.querySelector('summary'));
    qaAssert(hint,'${kind}: the module shows a measurement hint');
    ${lang === 'zh' ? `
    qaAssert(hint.querySelector('summary').textContent.trim()==='${w.head}','${kind}: hint headline is "'+hint.querySelector('summary').textContent.trim()+'"');
    qaAssert(document.getElementById('scanSourceBadge').textContent==='${w.badge || '本机扫描已返回'}','${kind}: badge is "'+document.getElementById('scanSourceBadge').textContent+'"');
    qaAssert(document.getElementById('runtimeBanner').hidden===${!w.banner},'${kind}: the runtime banner ${w.banner ? 'is' : 'is not'} raised');
    qaAssert(!!hint.querySelector('[onclick*="openPrivacySettings"]')===${w.pane},'${kind}: ${w.pane ? 'links' : 'does not link'} to a privacy pane');
    ${kind === 'limit' ? `qaAssert(!hint.textContent.includes('读取被拒绝')&&hint.textContent.includes('不影响清理'),'limit says cleanup is unaffected and never claims a denial');` : ''}
    ${kind === 'system' ? `qaAssert(hint.textContent.includes('无需处理'),'system locations are explained as needing no action');` : ''}
    ${kind === 'permission' ? `qaAssert(hint.querySelector('[onclick*="fullDiskAccess"]'),'permission links to Full Disk Access');` : ''}
    ` : `await qaEn('warnings ${kind}',async()=>{});`}
    hint.open=true;
    switchModule('clean');
  `);
  Object.assign(scenarios.at(-1), { width: 1060, height: 760 });
}
add('warnings-empty-state-limit', `
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],readWarnings:[{path:qaFixture.homePath+'/Projects/Very Large Build',reason:'x',kind:'limit'}]});
  switchModule('clean');await qaSettle();
  qaAssert(document.getElementById('heroHeadline').textContent.includes('权限正常；1 个较大目录只测量了部分体积'),'an empty list with only limit warnings never says permission is missing: '+document.getElementById('heroHeadline').textContent);
`);
add('warnings-system-only-not-in-headline', `
  onNativeRealScanComplete({...structuredClone(qaFixture),cleanGroups:[],readWarnings:[{path:'/Library/Application Support/com.apple.TCC',reason:'x',kind:'system'}]});
  switchModule('clean');await qaSettle();
  qaAssert(!/测量|权限/.test(document.getElementById('heroHeadline').textContent),'system-protected locations are not counted in the headline: '+document.getElementById('heroHeadline').textContent);
`);
// 清理之后：本地先结算、顶栏安静地显示刷新态、不切模块、不复活已移走的行；兜底重扫只发一次。
for (const theme of ['dark', 'light']) for (const lang of ['zh', 'en']) add(`refresh-after-trash-${theme}-${lang}`, `
  document.documentElement.dataset.theme='${theme}';${tierSetup}switchModule('clean');await qaSettle();
  ${lang === 'en' ? "qaEnglishFixture();I18N.setLang('en',false);await qaSettle();" : ''}
  const modBefore=STATE.module;
  openConfirmSheet();executeConfirmedClean();
  const req=__qa.actions.find(a=>a.action==='trashRealPaths');
  onNativeTrashResult({requestId:req.requestId,moved:req.paths.map(p=>({original:p,trashed:'/trash'+p})),movedCount:req.paths.length,failed:[]});
  await qaSettle();
  qaAssert(STATE.module===modBefore,'refresh never switches module');
  ${lang === 'zh' ? `qaAssert(document.getElementById('scanSourceBadge').textContent==='正在刷新扫描结果','the header says the scan is refreshing');` : ''}
  qaAssert(document.getElementById('moduleStage').getAttribute('aria-busy')==='true','the stage is marked busy while refreshing');
  qaAssert(STATE.cleanGroups.filter(g=>g.tier==='safe').every(g=>g.cleaned||g.sizeGb===0),'trashed rows are settled locally before any rescan returns');
  ${lang === 'en' ? `await qaEn('refreshing',async()=>{});qaAssert(document.getElementById('scanSourceBadge').textContent==='Refreshing scan results','English refreshing badge');` : ''}
  // 迟到的旧扫描结果（里面还有已移走的路径）不能复活它们。
  onNativeRealScanComplete(structuredClone(tp));await qaSettle();
  qaAssert(STATE.cleanGroups.filter(g=>g.tier==='safe').every(g=>g.cleaned||g.sizeGb===0||g.subpaths.every(sp=>sp.cleaned)),'a stale scan cannot resurrect rows trashed after it began');
  qaAssert(refreshing===true,'still refreshing: the stale scan began before the move');
  // Go 的新扫描开始并返回：刷新态结束，结果原样采用。
  const fresh=structuredClone(tp);fresh.cleanGroups=fresh.cleanGroups.filter(g=>g.tier!=='safe');
  onNativeRealScanStarted({});onNativeRealScanComplete(fresh);await qaSettle();
  qaAssert(refreshing===false,'the authoritative scan ends the refreshing state');
  qaAssert(document.getElementById('scanSourceBadge').textContent!==${lang === 'zh' ? "'正在刷新扫描结果'" : "'Refreshing scan results'"},'badge leaves the refreshing state');
  endRefreshing();
`);
add('refresh-fallback-single-rescan', `
  ${tierSetup}switchModule('clean');await qaSettle();
  openConfirmSheet();executeConfirmedClean();
  const req=__qa.actions.find(a=>a.action==='trashRealPaths');
  onNativeTrashResult({requestId:req.requestId,moved:[{original:req.paths[0],trashed:'/trash/a'}],movedCount:1,failed:[]});
  scheduleRefresh([req.paths[1]]);scheduleRefresh([req.paths[2]]);
  __qa.actions=[];
  await new Promise(r=>setTimeout(r,REFRESH_DEBOUNCE_MS+300));
  qaAssert(__qa.actions.filter(a=>a.action==='requestRealSystemScan').length===1,'back-to-back operations cause exactly one fallback rescan, got '+__qa.actions.filter(a=>a.action==='requestRealSystemScan').length);
  endRefreshing();
`);
// 系统加速：诊断清单（需处理 / 需确认 / 无需处理 / 暂不处理 / 不可用）与执行回执。
const optimizePlan = [
  { id: 'cache_refresh', name: '刷新缩略图与图标缓存', detail: '重建 Quick Look 缩略图与图标服务缓存', status: 'needed', reason: '缩略图和图标缓存可以随时重建，刷新后预览会重新生成', tier: 'safe', undo: '无需撤销，缓存会按需重建', checked: true, runnable: true },
  { id: 'saved_state_cleanup', name: '清理旧的应用窗口状态', detail: '把 30 天以上没动过的应用窗口状态移入废纸篓', status: 'needed', reason: '有 3 个超过 30 天的应用窗口状态，移入废纸篓后应用会按需重新生成', tier: 'safe', undo: '在这里点撤销，或在访达的废纸篓里放回原处', checked: true, runnable: true, items: ['~/Library/Saved Application State/com.example.one.savedState', '~/Library/Saved Application State/com.example.two.savedState', '~/Library/Saved Application State/com.example.three.savedState'] },
  { id: 'prevent_network_dsstore', name: '禁止在网络与 U 盘上写 .DS_Store', detail: '设置 Finder 偏好，不再在网络共享和 U 盘上生成 .DS_Store', status: 'needed', reason: 'Finder 现在还会在网络共享和 U 盘上生成 .DS_Store', tier: 'safe', undo: '在这里点撤销，会恢复成设置前的值', checked: true, runnable: true },
  { id: 'login_items_audit', name: '检查失效的登录项', detail: '核对登录项指向的应用是否还在，只报告，不修改', status: 'needed', reason: '核对登录项指向的应用是否还在；首次检查时 macOS 可能询问是否允许 orbit 控制“系统事件”', tier: 'review', undo: '不改动任何东西，无需撤销', checked: false, runnable: true },
  { id: 'sqlite_vacuum', name: '整理邮件、Safari 与信息的数据库', detail: '先做完整性检查，再压缩数据库文件，不删除内容', status: 'skipped', reason: 'Mail, Safari 正在运行，数据库可能被占用，暂不整理', tier: 'protected', unlock: '退出 Mail, Safari 后重新扫描即可整理这些数据库', undo: '无需撤销，数据库内容不变', checked: false, runnable: false },
  { id: 'fix_broken_configs', name: '修复损坏的偏好设置', detail: '把无法读取的第三方偏好设置文件移入废纸篓，应用下次启动会重新生成', status: 'clear', reason: '第三方偏好设置文件都正常', tier: 'safe', undo: '在这里点撤销，或在访达的废纸篓里放回原处', checked: false, runnable: false },
  { id: 'network_optimization', name: '刷新 DNS 缓存', detail: '清空系统 DNS 缓存', status: 'unavailable', reason: '需要管理员权限，orbit 不执行', tier: 'protected', undo: '不执行，没有改动', checked: false, runnable: false },
  { id: 'quarantine_cleanup', name: '清理下载隔离记录', detail: '清除 Gatekeeper 的下载追踪历史', status: 'unavailable', reason: '会直接删除数据库里的记录，无法放进废纸篓还原，orbit 不执行', tier: 'protected', undo: '不执行，没有改动', checked: false, runnable: false },
];
const optimizeReceipts = [
  { id: 'cache_refresh', ok: true, outcome: 'applied', summary: '缩略图与图标缓存已刷新', undo: '无需撤销，缓存会按需重建', canUndo: false },
  { id: 'saved_state_cleanup', ok: true, outcome: 'attention', summary: '已把 2 项移入废纸篓，另有 1 项没有处理', details: ['~/Library/Saved Application State/com.example.one.savedState', '~/Library/Saved Application State/com.example.two.savedState'], failed: [{ path: '~/Library/Saved Application State/com.example.three.savedState', error: '这些项目都没能移入废纸篓' }], undo: '在这里点撤销，或在访达的废纸篓里放回原处', canUndo: true },
  { id: 'prevent_network_dsstore', ok: true, outcome: 'applied', summary: '已设置 2 项 Finder 偏好', details: ['com.apple.desktopservices DSDontWriteNetworkStores'], undo: '在这里点撤销，会恢复成设置前的值', canUndo: true },
];
const optimizeLoad = `STATE.optimize.status==='loading'||requestOptimizePlan();onNativeOptimizePlan({requestId:STATE.optimize.planId,items:${JSON.stringify(optimizePlan)}});`;
const optimizeChecks = `const rows=[...document.querySelectorAll('[data-optimize-task]')];qaAssert(rows.length===8,'every task is listed');qaAssert(rows.filter(r=>r.querySelector('input[type=checkbox]')).length===4,'only runnable tasks have a checkbox');qaAssert(STATE.optimize.items.filter(i=>i.runnable&&STATE.optimize.sel[i.id]).map(i=>i.id).join()==='cache_refresh,saved_state_cleanup,prevent_network_dsstore','safe tasks are checked, the review task is not');qaAssert(document.getElementById('primaryBtnText').textContent.length>0,'primary button has text');`;
const optimizeReceiptStep = `STATE.optimize.receipts=Object.fromEntries(${JSON.stringify(optimizeReceipts)}.map(r=>[r.id,r]));renderModule();qaAssert(document.querySelectorAll('[data-optimize-receipt]').length===3,'each run task shows a receipt');qaAssert(document.querySelectorAll('[data-optimize-receipt] button').length===2,'only undoable receipts offer undo');`;
for (const theme of ['dark', 'light']) {
  const go = `document.documentElement.dataset.theme='${theme}';switchModule('optimize');${optimizeLoad}`;
  add(`optimize-zh-${theme}`, `${go}${optimizeChecks}`);
  add(`optimize-receipts-zh-${theme}`, `${go}${optimizeChecks}${optimizeReceiptStep}`);
  add(`en-optimize-${theme}`, `${go}await qaEn('optimize',async()=>{});${optimizeChecks}`);
  add(`en-optimize-receipts-${theme}`, `${go}await qaEn('optimize receipts',async()=>{${optimizeReceiptStep}});`);
}
const purgeRootsFixture = `const rootsOf=(paths,custom,error)=>({...structuredClone(qaFixture),purgeRoots:{paths,custom,error:error||''},installerFiles:[
  {id:'i1',name:'Canvas Setup.dmg',location:'~/Downloads',path:'~/Downloads/Canvas Setup.dmg',source:'Downloads',sizeGb:1,size:1073741824,mtime:1761739200,age:'3 个月前',checked:true,tier:'safe'},
  {id:'i2',name:'Tool.pkg',location:'~/Library/Caches/Homebrew/downloads',path:'~/Library/Caches/Homebrew/downloads/Tool.pkg',source:'Homebrew',sizeGb:0.4,size:429496729,mtime:1761739200,age:'3 个月前',checked:true,tier:'safe'},
  {id:'i3',name:'Archive.zip',location:'~/Desktop',path:'~/Desktop/Archive.zip',source:'Desktop',sizeGb:0.2,size:214748364,mtime:1761739200,age:'3 个月前',checked:false,tier:'review',reason:'压缩包里含有安装程序，但也可能是你想留的资料，请确认后再清理'}]});`;
for (const [state, args, extra] of [['default', "['~/Projects','~/GitHub'],false", "qaAssert(document.querySelector('.purge-roots summary').textContent.includes('2'),'summary counts the real roots');qaAssert(!document.querySelector('.purge-roots .row-remove'),'default roots cannot be removed');"],
  ['custom', "['~/Work/app-one','~/Work/app-two'],true", "qaAssert(document.querySelectorAll('.purge-roots .row-remove').length===2,'custom roots can be removed');"],
  ['error', "[],false,'无法读取 ~/.config/mole/purge_paths，已停止扫描构建产物：permission denied'", "qaAssert(document.querySelector('.purge-roots').open,'a read failure opens the control');"]])
  for (const theme of ['dark', 'light']) for (const lang of ['zh', 'en'])
    scenarios.push({ name: `purge-roots-${state}-${lang}-${theme}`, width: 1380, height: 860, script: `qaReset();document.documentElement.dataset.theme='${theme}';${purgeRootsFixture}onNativeRealScanComplete(rootsOf(${args}));switchModule('purge');const d=document.querySelector('.purge-roots');qaAssert(d,'scan folders control renders');d.open=true;STATE.purgeRootsOpen=true;${extra}qaAssert(document.getElementById('moduleStage').textContent.includes('Homebrew'),'installer source shows per row');${lang === 'en' ? "await qaEn('purge roots " + state + "',async()=>{document.querySelector('.purge-roots').open=true;});" : ''}` });
add('purge-roots-bridge', `${purgeRootsFixture}onNativeRealScanComplete(rootsOf(['~/Projects'],false));switchModule('purge');document.getElementById('purgeRootInput').value='relative/dir';addPurgeRootInput();qaAssert(!__qa.actions.some(a=>a.action==='addPurgeRoot'),'relative paths never reach Go');document.getElementById('purgeRootInput').value='~/Work/app';addPurgeRootInput();const add=__qa.actions.find(a=>a.action==='addPurgeRoot');qaAssert(add&&add.path==='~/Work/app','add sends the typed path');onNativePurgeRootsChanged({ok:true,action:'addPurgeRoot',path:'~/Work/app',purgeRoots:{paths:['~/Work/app'],custom:true,error:''}});qaAssert(STATE.purgeRoots.custom&&STATE.purgeRoots.paths[0]==='~/Work/app','receipt drives the list');removePurgeRootIdx(0);qaAssert(__qa.actions.some(a=>a.action==='removePurgeRoot'),'remove sends a bridge action');`);
add('purge-installer-identity', `${purgeRootsFixture}onNativeRealScanComplete(rootsOf(['~/Projects'],false));switchModule('purge');STATE.installerFiles[2].checked=true;openConfirmSheet();executeConfirmedClean();const req=__qa.actions.find(a=>a.action==='trashRealPaths');qaAssert(req,'trash request is sent');const key=req.paths.find(p=>p.endsWith('Canvas Setup.dmg'));qaAssert(key&&req.identities[key]&&req.identities[key].size===1073741824&&req.identities[key].mtime===1761739200,'installer identity travels with the request');`);
// Icon contact sheet: every module glyph on its real plates (sidebar gradient, palette/hero wash) at 28 and 56px.
function iconSheet(theme) {
  document.documentElement.dataset.theme = theme;
  const kinds = ['clean', 'ai_moon', 'software', 'optimize', 'analyze', 'status', 'purge', 'history', 'file'];
  const side = (k) => (document.querySelector(`.nav-item[data-mod="${k === 'file' ? 'analyze' : k}"] .nav-svg-icon`) || {}).style.background;
  const cell = (k, px, plate, pad, r, i) => `<span style="display:inline-flex;width:${px}px;height:${px}px;padding:${pad}px;border-radius:${r}px;background:${plate};overflow:hidden">${get_skeuo_svg(k, 'sheet' + px + i + k)}</span>`;
  const rows = [
    ['sidebar 28', (k, i) => cell(k, 28, side(k), 1.5, 7, i)],
    ['sidebar 56', (k, i) => cell(k, 56, side(k), 3, 14, i)],
    ['palette 28', (k, i) => cell(k, 28, MODULE_PLATE[k] || MODULE_PLATE.analyze, 2, 7, i)],
    ['palette 56', (k, i) => cell(k, 56, MODULE_PLATE[k] || MODULE_PLATE.analyze, 4, 14, i)],
    ['sidebar 112', (k, i) => cell(k, 112, side(k), 6, 26, i)],
    ['row 20', (k, i) => `<span style="display:inline-flex;width:20px;height:20px;opacity:.85">${get_skeuo_svg(k, 'sheetrow' + i)}</span>`]
  ];
  const el = document.createElement('div');
  el.id = 'iconSheet';
  el.style.cssText = 'position:fixed;inset:0;z-index:99999;background:var(--bg-window);color:var(--text-secondary);font:11px var(--font-mono,monospace);padding:20px;display:grid;grid-template-columns:90px repeat(' + kinds.length + ',1fr);gap:14px 8px;align-items:center;justify-items:center';
  el.innerHTML = '<span></span>' + kinds.map((k) => `<span>${k}</span>`).join('') + rows.map(([label, fn]) => `<span>${label}</span>` + kinds.map((k, i) => fn(k, i)).join('')).join('');
  document.body.appendChild(el);
}
for (const theme of ['light', 'dark']) scenarios.push({ name: 'icons-sheet-' + theme, width: 1300, height: 500, script: `qaReset();(${iconSheet.toString()})('${theme}');` });
fs.writeFileSync(dir + '/suite.json', JSON.stringify(scenarios));
console.log(`${scenarios.length} scenarios generated, isolated fixture data only.`);

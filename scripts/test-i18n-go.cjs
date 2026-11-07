#!/usr/bin/env node
// Guards the English dictionary for Go-sent strings (Resources/src-ui/i18n/en-backend.js):
//   1. every entry has the same {slots} in key and value, and no CJK in the value;
//   2. every user-visible Chinese string literal in the Go sources is covered by a key;
//   3. a few finished sentences translate through t() the way the page will call it.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const root = path.join(__dirname, '..');
const CJK = /[㐀-鿿]/;
const fail = [];

// ---- load core.js + en-backend.js the way the page does, in English ----
const ctx = vm.createContext({ localStorage: { getItem: () => 'en', setItem() {} } });
for (const f of ['core.js', 'en-backend.js']) {
  vm.runInContext(fs.readFileSync(path.join(root, 'Resources/src-ui/i18n', f), 'utf8'), ctx, { filename: f });
}
const { I18N } = ctx;
const dict = I18N.entries();
const keys = Object.keys(dict);

// ---- 1. slot parity ----
const slotSet = s => [...new Set([...s.matchAll(/\{(\w+)(?:\|[^|}]*\|[^|}]*)?\}/g)].map(m => m[1]))].sort().join(',');
for (const key of keys) {
  if (slotSet(key) !== slotSet(dict[key])) fail.push(`slots differ: ${key}  =>  ${dict[key]}`);
  if (CJK.test(dict[key])) fail.push(`CJK left in value: ${key}  =>  ${dict[key]}`);
  if (/\}\{/.test(key)) fail.push(`adjacent slots can't be matched: ${key}`);
}

// ---- 2. Go literal coverage ----
const norm = s => s.replace(/%%/g, '\u0001').replace(/%[-+# 0-9.*]*[a-zA-Z]/g, '{}').replace(/\{\w+\}/g, '{}').replace(/\u0001/g, '%');
const normKeys = new Set(keys.map(norm));
const normKeyList = [...normKeys];

// Literals that never reach the page: [file, text, why].
const allow = [
  ['push.go', '未知', 'native tray tooltip text; localized with L() in lang.go'],
  ['push.go', '正常', 'native tray tooltip text; localized with L() in lang.go'],
  ['push.go', '偏高', 'native tray tooltip text; localized with L() in lang.go'],
  ['push.go', '很高', 'native tray tooltip text; localized with L() in lang.go'],
];

// Skipped: app.go (native menus/tray, L()-wrapped), lang.go (the L() helper), tests.
const skipFile = f => /(_test\.go|^app\.go|^lang\.go)$/.test(f);

function goFiles(dir, rel = '') {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(e => {
    if (e.isDirectory()) return rel === '' && e.name !== 'internal' ? [] : goFiles(path.join(dir, e.name), path.join(rel, e.name));
    return e.name.endsWith('.go') && !skipFile(path.join(rel, e.name)) ? [path.join(rel, e.name)] : [];
  });
}

// Minimal Go lexer: yields string literals, skipping comments and rune literals.
function* literals(src) {
  let i = 0, line = 1;
  while (i < src.length) {
    const c = src[i];
    if (c === '\n') { line++; i++; }
    else if (c === '/' && src[i + 1] === '/') { while (i < src.length && src[i] !== '\n') i++; }
    else if (c === '/' && src[i + 1] === '*') { const end = src.indexOf('*/', i + 2); const body = src.slice(i, end < 0 ? src.length : end + 2); line += (body.match(/\n/g) || []).length; i += body.length; }
    else if (c === "'") { i++; while (i < src.length && src[i] !== "'") i += src[i] === '\\' ? 2 : 1; i++; }
    else if (c === '"' || c === '`') {
      const start = i, startLine = line; let value = '';
      i++;
      while (i < src.length && src[i] !== c) {
        if (c === '"' && src[i] === '\\') { const n = src[i + 1]; value += n === 'n' ? '\n' : n === 't' ? '\t' : n; i += 2; continue; }
        if (src[i] === '\n') line++;
        value += src[i++];
      }
      i++;
      yield { value, line: startLine, start };
    } else i++;
  }
}

let covered = 0;
for (const file of goFiles(root)) {
  const src = fs.readFileSync(path.join(root, file), 'utf8');
  const lines = src.split('\n');
  for (const lit of literals(src)) {
    if (!CJK.test(lit.value)) continue;
    const text = lines[lit.line - 1];
    const before = src.slice(src.lastIndexOf('\n', lit.start - 1) + 1, lit.start);
    const prev = (lines[lit.line - 2] || '').trimEnd();
    if (/\bL\(\s*$/.test(before)) continue;                                  // L("中文", "English")
    if (/SetToolTip|trayItem|KeepAwake/.test(text)) continue;                // native-only strings
    if (allow.some(([f, s]) => file.endsWith(f) && lit.value === s)) continue;
    const n = norm(lit.value);
    // A fragment of a concatenated sentence only needs to sit inside some key.
    const concatenated = /\+/.test(text) || /\+$/.test(prev) || /^\s*\w+ \+= /.test(lines[lit.line] || '');   // or extended by a following `x += ...`
    if (normKeys.has(n) || (concatenated && normKeyList.some(k => k.includes(n)))) { covered++; continue; }
    fail.push(`uncovered Go literal ${file}:${lit.line}: ${JSON.stringify(lit.value)}`);
  }
}

// ---- 3. finished sentences through t() ----
const cases = [
  ['白名单排除：/Users/x/Library', 'Excluded by whitelist: /Users/x/Library'],
  ['3 个文件 · 1 个路径', '3 files · 1 path'],
  ['1 个文件 · 12 个路径', '1 file · 12 paths'],
  ['1 天未修改', 'Not modified for 1 day'],
  ['12 天未修改', 'Not modified for 12 days'],
  ['Apple M2 (8核)', 'Apple M2 (8 cores)'],
  ['Codex · 过期会话 · Shell 快照', 'Codex · Expired sessions · Shell snapshots'],
  ['Cursor / Cursor Agent · 会话与配置', 'Cursor / Cursor Agent · Sessions and settings'],
  ['路径不存在或无法读取：lstat /x: no such file or directory', "The path doesn't exist or can't be read: lstat /x: no such file or directory"],
  ['健康度 92%，循环 312 次，当前电量 80%', 'Health 92%, 312 cycles, 80% charge'],
  ['健康度 55%，循环 1 次，低于 60%，建议更换电池', 'Health 55%, 1 cycle; under 60%, consider replacing the battery'],
  ['可用 6.0 GB / 100.0 GB（6.0%），低于 8%，请立即清理', '6.0 GB available of 100.0 GB (6.0%); under 8%, clean up now'],
  ['存在未提交或未跟踪内容，保留工作成果。工作区干净也不能证明提交已推送、分支已合并或会话已结束，不在这里移除。', "There are uncommitted or untracked changes; the work is kept. A clean worktree doesn't prove commits were pushed, a branch was merged or a session has ended, so it isn't removed here."],
  ['Codex 正在运行，暂缓清理', 'Codex is running; cleanup postponed'],
  ['…其它 7 项', '…7 other items'],
  ['这是一句没有收录的中文', '这是一句没有收录的中文'],
];
for (const [zh, en] of cases) {
  const got = I18N.t(zh);
  if (got !== en) fail.push(`t(${JSON.stringify(zh)})\n   got  ${JSON.stringify(got)}\n   want ${JSON.stringify(en)}`);
}

if (fail.length) {
  console.error(fail.join('\n'));
  console.error(`\n${fail.length} problem(s)`);
  process.exit(1);
}
console.log(`i18n backend dictionary: ${keys.length} entries, ${covered} Go literals covered`);

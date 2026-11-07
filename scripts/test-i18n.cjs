// Unit checks for the page's i18n: the helper in Resources/src-ui/i18n/core.js and the dictionaries
// next to it. The end-to-end check (no Chinese left on screen in English) is the WebKit scenario in
// scripts/webkit-fixtures.cjs; this one is the cheap gate that runs under `make test`.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const dir = path.join(__dirname, '../Resources/src-ui');
const read = name => fs.readFileSync(path.join(dir, name), 'utf8');
const CJK = /[㐀-鿿]/;
const slots = s => [...s.matchAll(/\{(\w+)(?:\|[^|}]*\|[^|}]*)?\}/g)].map(m => m[1]).filter((v, i, a) => a.indexOf(v) === i).sort().join(',');

function freshI18N(saved, languages = ['en-US']) {
  const store = saved ? { orbit_lang: saved } : {};
  const context = vm.createContext({
    localStorage: { getItem: k => (k in store ? store[k] : null), setItem: (k, v) => { store[k] = v; } },
    navigator: { languages },
  });
  context.window = context;
  for (const file of ['i18n/core.js', 'i18n/en-ui.js', 'i18n/en-backend.js']) vm.runInContext(read(file), context, { filename: file });
  return { I18N: context.I18N, t: context.t, store, context };
}

// --- helper behaviour -------------------------------------------------------------------------
{
  const { I18N, t, store } = freshI18N('zh');
  assert.equal(I18N.lang, 'zh');
  assert.equal(t('深度清理'), '深度清理', 'Chinese is the source: it comes back as written');
  assert.equal(t('没有这个键 {n}', { n: 3 }), '没有这个键 3', 'slots are filled in Chinese too');
  I18N.setLang('en');
  assert.equal(store.orbit_lang, 'en', 'the choice is remembered');
  assert.equal(t('深度清理'), 'Deep Clean');
  assert.equal(t('完全没收录的句子'), '完全没收录的句子', 'a missing key falls back to Chinese');
  I18N.setLang('en', false);
  assert.equal(I18N.lang, 'en');
}
{
  const { t, I18N } = freshI18N(null);
  assert.equal(I18N.lang, 'en', 'no saved choice and an English system gives English');
  I18N.add({ '共 {n} 个文件': '{n} {n|file|files}', '你好，{name}': 'Hello, {name}' });
  assert.equal(t('共 {n} 个文件', { n: 1 }), '1 file');
  assert.equal(t('共 {n} 个文件', { n: 1234 }), '1,234 files', 'integers get thousands separators');
  assert.equal(t('你好，{name}', { name: '世界' }), 'Hello, 世界', 'user data is left alone');
  assert.equal(t('共 5 个文件'), '5 files', 'an already finished sentence matches its template');
}
{
  const { t, I18N } = freshI18N(null, ['zh-Hans-CN', 'en']);
  assert.equal(I18N.lang, 'zh', 'no saved choice and a Chinese system gives Chinese');
  I18N.add({ '共 {n} 个文件': '{n} {n|file|files}' });
  assert.equal(t('共 {n} 个文件', { n: 1234 }), '共 1234 个文件', 'Chinese output is not reformatted');
  assert.equal(freshI18N(null, ['ja-JP']).I18N.lang, 'en', 'any other system language gives English');
}

// --- every literal key is in a dictionary -----------------------------------------------------
const { I18N } = freshI18N('en');
const EN = I18N.entries();
const html = read('index.html');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
const keyRe = /\bt\(\s*(['"`])((?:\\.|(?!\1)[^\\])*)\1/g;
const used = new Set();
let m;
while ((m = keyRe.exec(script))) {
  if (!CJK.test(m[2])) continue;
  assert.ok(!/\$\{/.test(m[2]), `t() key must be static: ${m[0]}`);
  const key = new Function('return ' + m[1] + m[2] + m[1])();
  used.add(key);
  assert.ok(key in EN, `no English for t(${JSON.stringify(key)})`);
}
assert.ok(used.size > 400, `found only ${used.size} t() keys; the pattern no longer matches the page`);

// --- dictionary hygiene ------------------------------------------------------------------------
for (const [zh, en] of Object.entries(EN)) {
  assert.ok(!CJK.test(en), `English value has Chinese: ${zh}`);
  assert.equal(slots(zh), slots(en), `slots differ: ${zh} => ${en}`);
  assert.ok(en.trim() !== '', `blank English: ${zh}`);
  // Only joiner fragments such as "：" or "（约 {size}）" may carry the spaces English needs around them.
  assert.ok(en.trim() === en || /^[\s（）、：；，·]|[\s（）、：；，·]$/.test(zh), `padded English: ${zh}`);
}
assert.ok(!/AI Care/.test(JSON.stringify(EN)), 'the AI module is called AI Cleanup everywhere');

// --- statically marked text and attributes ----------------------------------------------------
{
  const body = html.slice(0, html.indexOf('<script>'));
  const missing = [];
  for (const tag of body.matchAll(/<([a-z0-9]+)\b([^>]*\bdata-i18n(?:-attr="([^"]*)")?[^>]*)>/g)) {
    const attrs = tag[2];
    for (const name of (tag[3] || '').split(/\s+/).filter(Boolean)) {
      const value = (attrs.match(new RegExp('\\s' + name + '="([^"]*)"')) || [])[1];
      if (value && CJK.test(value) && !(value in EN)) missing.push(`${name}="${value}"`);
    }
  }
  for (const text of body.matchAll(/<([a-z0-9]+)\b[^>]*\bdata-i18n(?:\s[^>]*)?>([^<]*)</g)) {
    const own = text[2].trim();
    if (own && CJK.test(own) && !(own in EN)) missing.push(own);
  }
  assert.deepEqual(missing, [], 'static text without an English entry');
}

console.log(`ok i18n: ${used.size} page keys, ${Object.keys(EN).length} dictionary entries`);

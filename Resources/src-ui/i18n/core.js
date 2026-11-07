// Gettext-style i18n. The Chinese source string IS the key: t('已选 {n} 项', {n: 3}).
// Chinese is the default and the source of truth; a missing English entry falls
// back to the Chinese text, so nothing ever renders blank.
//
//   - Dictionaries register with I18N.add({ '中文': 'English', ... }); see en-ui.js (page
//     strings) and en-backend.js (strings the Go side sends as-is).
//   - {name} placeholders interpolate from params; {n|file|files} picks a plural form
//     by params.n (English only needs one/other).
//   - A key may contain {name} slots and then also matches finished sentences: the Go side
//     sends text like "白名单排除：/Users/x/Library" already interpolated, and t() of it
//     finds the key "白名单排除：{path}" and fills the English template with the captures.
//   - Static markup: data-i18n translates an element's own text, data-i18n-attr="title aria-label
//     placeholder" translates those attributes. I18N.apply() re-runs them on a language change.
(function (global) {
  const CJK = /[㐀-鿿]/;
  const KEY = 'orbit_lang';
  const EN = Object.create(null);
  let templates = null;               // lazily compiled from the keys that contain {name}
  const memo = new Map();             // finished sentence -> template match (or null)
  const listeners = [];

  function detect() {
    try { const saved = localStorage.getItem(KEY); if (saved === 'zh' || saved === 'en') return saved; } catch (e) { /* private mode */ }
    const first = (global.navigator && (navigator.languages && navigator.languages[0] || navigator.language)) || 'zh';
    return /^zh/i.test(first) ? 'zh' : 'en';
  }
  let lang = detect();
  if (global.document && document.documentElement) document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';

  function compile() {
    templates = [];
    for (const zh in EN) {
      if (!/\{\w+\}/.test(zh)) continue;
      const names = [];
      const literal = zh.replace(/\{(\w+)\}/g, () => '\u0000');
      const pattern = literal.split('\u0000').map(s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('([\\s\\S]+?)');
      zh.replace(/\{(\w+)\}/g, (_, name) => names.push(name));
      templates.push({ zh, names, re: new RegExp('^' + pattern + '$'), weight: literal.replace(/\u0000/g, '').length });
    }
    templates.sort((a, b) => b.weight - a.weight);   // the most specific sentence wins
  }

  function match(zh) {
    if (memo.has(zh)) return memo.get(zh);
    if (!templates) compile();
    let found = null;
    for (const tpl of templates) {
      const m = tpl.re.exec(zh);
      if (m) { found = { en: EN[tpl.zh], vars: Object.fromEntries(tpl.names.map((n, i) => [n, m[i + 1]])) }; break; }
    }
    memo.set(zh, found);
    return found;
  }

  function fill(text, params) {
    if (!params) return text;
    return text.replace(/\{(\w+)(?:\|([^|}]*)\|([^|}]*))?\}/g, (whole, name, one, many) => {
      if (!(name in params)) return whole;
      const value = params[name];
      if (one !== undefined) return Number(value) === 1 ? one : many;
      if (lang !== 'en') return String(value);
      if (typeof value === 'number' && Number.isInteger(value)) return value.toLocaleString('en-US');
      return typeof value === 'string' && CJK.test(value) ? t(value) : String(value);
    });
  }

  function t(zh, params) {
    if (zh === null || zh === undefined) return '';
    zh = String(zh);
    if (lang === 'en' && CJK.test(zh)) {
      const hit = EN[zh];
      if (hit !== undefined) return fill(hit, params);
      const m = match(zh);
      if (m) return fill(m.en, Object.assign({}, m.vars, params));
    }
    return fill(zh, params);
  }

  // Text or an attribute that script has rewritten since (a live number, a status) is left alone:
  // the code that wrote it already goes through t() and re-renders on a language change.
  function apply(root) {
    (root || document).querySelectorAll('[data-i18n],[data-i18n-attr]').forEach(el => {
      const src = el.__zh || (el.__zh = {});
      if (el.hasAttribute('data-i18n')) {
        if (src.text === undefined) src.text = el.getAttribute('data-i18n') || el.textContent.trim();
        if (src.shown === undefined || el.textContent === src.shown) { el.textContent = t(src.text); src.shown = el.textContent; }
      }
      (el.getAttribute('data-i18n-attr') || '').split(/\s+/).filter(Boolean).forEach(name => {
        if (src['@' + name] === undefined) { src['@' + name] = el.getAttribute(name) || ''; src['#' + name] = undefined; }
        if (src['#' + name] === undefined || el.getAttribute(name) === src['#' + name]) { el.setAttribute(name, t(src['@' + name])); src['#' + name] = el.getAttribute(name); }
      });
    });
  }

  function setLang(next, remember) {
    if (next !== 'zh' && next !== 'en') return;
    lang = next;
    if (remember !== false) { try { localStorage.setItem(KEY, next); } catch (e) { /* ignore */ } }
    if (global.document && document.documentElement) { document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'; apply(document); }
    listeners.forEach(fn => fn(next));
  }

  global.t = t;
  global.I18N = {
    get lang() { return lang; },
    setLang, apply, t,
    onChange(fn) { listeners.push(fn); },
    add(dict) { Object.assign(EN, dict); templates = null; memo.clear(); },
    has(zh) { return zh in EN; },
    entries() { return EN; },
  };
})(typeof window !== 'undefined' ? window : globalThis);

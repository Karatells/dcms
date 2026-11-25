// DCMS frontend core: api, router, hyperscript, ui-kit, icons, events. Vanilla ES modules, no build step.

// Защита от вывода "null"/"undefined"/"false" в DOM при передаче условных элементов в replaceChildren/append/prepend
for (const m of ['replaceChildren', 'append', 'prepend', 'before', 'after']) {
  const orig = Element.prototype[m];
  if (typeof orig === 'function') {
    Element.prototype[m] = function (...args) {
      return orig.apply(this, args.filter(x => x !== null && x !== undefined && x !== false && x !== true));
    };
  }
}

// ───────────────────────────────────────────── helpers
export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

/** Hyperscript: h('div.card#id', {class:'x', onclick:fn, style:{}, dataset:{}, html:'<b>safe</b>'}, ...children) */
export function h(tag, attrs, ...children) {
  let cls = [];
  let id;
  const m = String(tag).match(/^([a-z0-9-]*)((?:[.#][\w-]+)*)$/i);
  let name = tag;
  if (m) {
    name = m[1] || 'div';
    (m[2].match(/[.#][\w-]+/g) || []).forEach(p => (p[0] === '.' ? cls.push(p.slice(1)) : (id = p.slice(1))));
  }
  const svgNS = ['svg', 'path', 'circle', 'rect', 'line', 'polyline', 'polygon', 'g', 'ellipse'].includes(name);
  const el = svgNS ? document.createElementNS('http://www.w3.org/2000/svg', name) : document.createElement(name);
  if (id) el.id = id;
  if (attrs && (typeof attrs !== 'object' || attrs instanceof Node || Array.isArray(attrs) || typeof attrs.nodeType === 'number')) {
    children.unshift(attrs);
    attrs = null;
  }
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null || v === false) continue;
      if (k === 'class' || k === 'className') cls.push(...String(v).split(/\s+/).filter(Boolean));
      else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
      else if (k === 'dataset') Object.assign(el.dataset, v);
      else if (k === 'html') el.innerHTML = v;
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (k === 'value' || k === 'checked' || k === 'selected' || k === 'disabled' || k === 'indeterminate') el[k] = v;
      else if (k === 'href' || k === 'src') {
        const s = String(v).trim();
        const safe = !/^(?:javascript|vbscript|data\s*:\s*text\/html)/i.test(s.replace(/[\x00-\x20]+/g, ''));
        el.setAttribute(k, safe ? s : '#');
      }
      else el.setAttribute(k, v === true ? '' : v);
    }
  }
  if (cls.length) svgNS ? el.setAttribute('class', cls.join(' ')) : (el.className = cls.join(' '));
  append(el, children);
  return el;
}
function append(el, children) {
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false || c === true) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
}
export const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

// ───────────────────────────────────────────── state & events
export const state = {
  user: null, // текущий пользователь (см. /api/me) или null
  config: {},
  counters: {},
  menu: [],
  adminLinks: [], // {group,title,href,access,icon,desc}: модули добавляют свои админ-страницы
  userHooks: [], // функции, вызываемые при смене пользователя
};
export const can = acc => !!state.user && (state.user.accesses || []).includes(acc);
export const isAuth = () => !!state.user;
export const level = () => (state.user ? state.user.level : 0);

const listeners = {};
export const bus = {
  on(ev, fn) { (listeners[ev] ||= new Set()).add(fn); return () => listeners[ev].delete(fn); },
  emit(ev, data) { (listeners[ev] || []).forEach(f => { try { f(data); } catch (e) { console.error(e); } }); },
};

// ───────────────────────────────────────────── api
export class ApiError extends Error {
  constructor(status, message, data) { super(message); this.status = status; this.data = data; }
}
async function request(method, url, body, opts = {}) {
  const init = { method, headers: { 'X-DCMS': '1', 'X-Page': location.pathname.slice(0, 60) }, credentials: 'same-origin' };
  if (body instanceof FormData) init.body = body;
  else if (body !== undefined && method !== 'GET') { init.headers['Content-Type'] = 'application/json'; init.body = JSON.stringify(body); }
  let res;
  try { res = await fetch(url, init); } catch (e) { const err = new ApiError(0, 'Нет соединения с сервером'); if (!opts.silent) ui.toast(err.message, 'error'); throw err; }
  const ct = res.headers.get('content-type') || '';
  const data = ct.includes('json') ? await res.json().catch(() => ({})) : await res.text();
  if (!res.ok) {
    const err = new ApiError(res.status, (data && data.error) || res.statusText, data);
    if (res.status === 401 && state.user) { state.user = null; bus.emit('user', null); }
    if (!opts.silent) ui.toast(err.message, 'error');
    throw err;
  }
  return data;
}
const qs = p => { if (!p) return ''; const u = new URLSearchParams(); for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== null && v !== '') u.set(k, v); const s = u.toString(); return s ? '?' + s : ''; };
export const api = {
  get: (url, params, opts) => request('GET', url + qs(params), undefined, opts),
  post: (url, body = {}, opts) => request('POST', url, body, opts),
  put: (url, body = {}, opts) => request('PUT', url, body, opts),
  del: (url, body, opts) => request('DELETE', url, body, opts),
  /** upload(url, {field: File|string, ...}) → multipart */
  upload: (url, fields, opts) => { const fd = new FormData(); for (const [k, v] of Object.entries(fields || {})) if (v != null) fd.append(k, v); return request('POST', url, fd, opts); },
};

// ───────────────────────────────────────────── icons (24x24 stroke, lucide-like)
const I = {
  home: 'M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  users: 'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21a7 7 0 0 1 14 0M16 3.1a4 4 0 0 1 0 7.8M22 21a7 7 0 0 0-4.5-6.5',
  'user-plus': 'M10 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM3 21a7 7 0 0 1 14 0M19 8v6M16 11h6',
  'user-check': 'M10 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM3 21a7 7 0 0 1 14 0M16 11l2 2 4-4',
  'user-x': 'M10 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM3 21a7 7 0 0 1 14 0M17 8l5 5M22 8l-5 5',
  bell: 'M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 0 0 3.4 0',
  mail: 'M4 4h16a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1zM22 6l-10 7L2 6',
  send: 'M22 2 11 13M22 2l-7 20-4-9-9-4z',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3',
  menu: 'M4 6h16M4 12h16M4 18h16',
  x: 'M18 6 6 18M6 6l12 12',
  check: 'M20 6 9 17l-5-5',
  'check-all': 'M18 6 7 17l-5-5M22 10l-7.5 7.5L13 16',
  plus: 'M12 5v14M5 12h14',
  minus: 'M5 12h14',
  edit: 'M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z',
  trash: 'M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1L5 6M10 11v6M14 11v6',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z',
  'log-out': 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
  'log-in': 'M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4M10 17l5-5-5-5M15 12H3',
  heart: 'M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1-1.1a5.5 5.5 0 0 0-7.8 7.8l1 1.1L12 21l7.8-7.5 1-1.1a5.5 5.5 0 0 0 0-7.8z',
  'thumbs-up': 'M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.3a2 2 0 0 0 2-1.7l1.4-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3',
  'thumbs-down': 'M10 15v4a3 3 0 0 0 3 3l4-9V2H5.7a2 2 0 0 0-2 1.7l-1.4 9a2 2 0 0 0 2 2.3zM17 2h2.7A2.3 2.3 0 0 1 22 4v7a2.3 2.3 0 0 1-2.3 2H17',
  star: 'M12 2l3.1 6.3 6.9 1-5 4.9 1.2 6.9L12 17.8 5.8 21.1 7 14.2 2 9.3l6.9-1z',
  message: 'M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z',
  'message-circle': 'M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8z',
  image: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM8.5 10a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3zM21 15l-5-5L5 21',
  camera: 'M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2zM12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  folder: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z',
  'folder-plus': 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2zM12 11v6M9 14h6',
  file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6',
  'file-text': 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M16 13H8M16 17H8M10 9H8',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
  upload: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12',
  link: 'M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7',
  external: 'M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14 21 3',
  clock: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2',
  calendar: 'M19 4H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2zM16 2v4M8 2v4M3 10h18',
  'map-pin': 'M21 10c0 7-9 13-9 13S3 17 3 10a9 9 0 0 1 18 0zM12 13a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  lock: 'M19 11H5a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7a2 2 0 0 0-2-2zM7 11V7a5 5 0 0 1 10 0v4',
  unlock: 'M19 11H5a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7a2 2 0 0 0-2-2zM7 11V7a5 5 0 0 1 9.9-1',
  shield: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z',
  gavel: 'M14 13l-8.5 8.5a2.1 2.1 0 0 1-3-3L11 10M16 16l6-6M8 8l6-6M9 7l8 8M21 11l-8-8',
  eye: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  'eye-off': 'M17.9 17.9A10.1 10.1 0 0 1 12 20c-7 0-11-8-11-8a18.5 18.5 0 0 1 5.1-5.9M9.9 4.2A9.1 9.1 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.2 3.2M14.1 14.1a3 3 0 1 1-4.2-4.2M1 1l22 22',
  flag: 'M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1zM4 22v-7',
  bookmark: 'M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z',
  gift: 'M20 12v10H4V12M2 7h20v5H2zM12 22V7M12 7H7.5a2.5 2.5 0 0 1 0-5C11 2 12 7 12 7zM12 7h4.5a2.5 2.5 0 0 0 0-5C13 2 12 7 12 7z',
  coins: 'M8 14a6 6 0 1 0 0-12 6 6 0 0 0 0 12zM18.1 10.4A6 6 0 1 1 10.4 18M7 6h1v4',
  trophy: 'M6 9H4.5a2.5 2.5 0 0 1 0-5H6M18 9h1.5a2.5 2.5 0 0 0 0-5H18M4 22h16M10 14.7V17c0 .6-.5 1-1.2 1.2C7.8 18.5 7 20.2 7 22M14 14.7V17c0 .6.5 1 1.2 1.2 1 .3 1.8 2 1.8 3.8M18 2H6v7a6 6 0 0 0 12 0z',
  music: 'M9 18V5l12-2v13M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 19a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  play: 'M5 3l14 9-14 9z',
  pause: 'M6 4h4v16H6zM14 4h4v16h-4z',
  video: 'M23 7l-7 5 7 5zM14 5H3a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2z',
  rss: 'M4 11a9 9 0 0 1 9 9M4 4a16 16 0 0 1 16 16M5 20a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
  pencil: 'M17 3a2.8 2.8 0 0 1 4 4L7.5 20.5 2 22l1.5-5.5z',
  book: 'M4 19.5A2.5 2.5 0 0 1 6.5 17H20V2H6.5A2.5 2.5 0 0 0 4 4.5zM20 22H6.5a2.5 2.5 0 0 1 0-5H20z',
  forum: 'M17 9V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10l3-3h2M21 21l-3-3h-7a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2z',
  info: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01',
  alert: 'M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01',
  help: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01',
  'chevron-left': 'M15 18l-6-6 6-6',
  'chevron-right': 'M9 18l6-6-6-6',
  'chevron-up': 'M18 15l-6-6-6 6',
  'chevron-down': 'M6 9l6 6 6-6',
  'arrow-left': 'M19 12H5M12 19l-7-7 7-7',
  'arrow-right': 'M5 12h14M12 5l7 7-7 7',
  'arrow-up': 'M12 19V5M5 12l7-7 7 7',
  more: 'M12 13a1 1 0 1 0 0-2 1 1 0 0 0 0 2zM19 13a1 1 0 1 0 0-2 1 1 0 0 0 0 2zM5 13a1 1 0 1 0 0-2 1 1 0 0 0 0 2z',
  refresh: 'M23 4v6h-6M1 20v-6h6M3.5 9a9 9 0 0 1 14.9-3.4L23 10M1 14l4.6 4.4A9 9 0 0 0 20.5 15',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z',
  pin: 'M12 17v5M9 10.8a2 2 0 0 1-1.1 1.8l-1.8.9A2 2 0 0 0 5 15.2V17h14v-1.8a2 2 0 0 0-1.1-1.8l-1.8-.9a2 2 0 0 1-1.1-1.8V5h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1z',
  smile: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM8 14s1.5 2 4 2 4-2 4-2M9 9h.01M15 9h.01',
  tag: 'M20.6 13.4l-7.2 7.2a2 2 0 0 1-2.8 0L2 12V2h10l8.6 8.6a2 2 0 0 1 0 2.8zM7 7h.01',
  share: 'M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM8.6 13.5l6.8 4M15.4 6.5l-6.8 4',
  trending: 'M23 6l-9.5 9.5-5-5L1 18M17 6h6v6',
  chart: 'M18 20V10M12 20V4M6 20v-6',
  database: 'M12 8c4.97 0 9-1.34 9-3s-4.03-3-9-3-9 1.34-9 3 4.03 3 9 3zM21 12c0 1.66-4 3-9 3s-9-1.34-9-3M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5',
  wrench: 'M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94z',
  key: 'M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8zm0 0L15.5 7.5m0 0 3 3L22 7l-3-3',
  globe: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM2 12h20M12 2a15 15 0 0 1 4 10 15 15 0 0 1-4 10 15 15 0 0 1-4-10 15 15 0 0 1 4-10z',
  hash: 'M4 9h16M4 15h16M10 3 8 21M16 3l-2 18',
  quote: 'M3 21c3 0 7-1 7-8V5c0-1.3-1-2-2-2H4c-1 0-2 .7-2 2v6c0 1 1 2 2 2h4M15 21c3 0 7-1 7-8V5c0-1.3-1-2-2-2h-4c-1 0-2 .7-2 2v6c0 1 1 2 2 2h4',
  filter: 'M22 3H2l8 9.5V19l4 2v-8.5z',
  grid: 'M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z',
  list: 'M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01',
  copy: 'M20 9h-9a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2v-9a2 2 0 0 0-2-2zM5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1',
  volume: 'M11 5 6 9H2v6h4l5 4zM15.5 8.5a5 5 0 0 1 0 7M19 5a9 9 0 0 1 0 14',
  'volume-x': 'M11 5 6 9H2v6h4l5 4zM23 9l-6 6M17 9l6 6',
  paperclip: 'M21.4 11.1l-9.2 9.2a6 6 0 0 1-8.5-8.5l9.2-9.2a4 4 0 0 1 5.7 5.7l-9.2 9.2a2 2 0 0 1-2.8-2.8l8.5-8.5',
  power: 'M18.4 6.6a9 9 0 1 1-12.8 0M12 2v10',
  ban: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM4.9 4.9l14.2 14.2',
  pause2: 'M6 4h4v16H6zM14 4h4v16h-4z',
  crown: 'M2 20h20M4 17 2 6l6 5 4-7 4 7 6-5-2 11z',
  bold: 'M6 4h8a4 4 0 0 1 0 8H6zM6 12h9a4 4 0 0 1 0 8H6z',
  italic: 'M19 4h-9M14 20H5M15 4 9 20',
  underline: 'M6 3v7a6 6 0 0 0 12 0V3M4 21h16',
  'bar-chart': 'M12 20V10M18 20V4M6 20v-4',
  pie: 'M21.2 15.9A10 10 0 1 1 8 2.8M22 12A10 10 0 0 0 12 2v10z',
  zap: 'M13 2 3 14h9l-1 8 10-12h-9z',
  inbox: 'M22 12h-6l-2 3h-4l-2-3H2M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.1z',
  smartphone: 'M17 2H7a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2zM12 18h.01',
  monitor: 'M20 3H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2zM8 21h8M12 17v4',
  'rotate-ccw': 'M1 4v6h6M3.5 15a9 9 0 1 0 2.1-9.4L1 10',
  'corner-up-left': 'M9 14 4 9l5-5M20 20v-7a4 4 0 0 0-4-4H4',
  maximize: 'M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3',
  dice: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM8 8h.01M16 8h.01M8 16h.01M16 16h.01M12 12h.01',
  mic: 'M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3zM19 10v2a7 7 0 0 1-14 0v-2M12 19v4M8 23h8',
  music: 'M9 18V5l12-2v13M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 19a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
  award: 'M12 15a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM8.21 13.89 7 23l5-3 5 3-1.21-9.12',
  'skip-back': 'M19 20 9 12l10-8v16zM5 19V5',
  'skip-forward': 'M5 4l10 8-10 8V4zM19 5v14',
  'shopping-bag': 'M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4zM3 6h18M16 10a4 4 0 0 1-8 0',
};
export function icon(name, size = 18, cls = '') {
  const svg = h('svg', { class: 'icon ' + cls, width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2, 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' });
  const d = I[name] || I.info;
  d.split(/(?=M)/).forEach(seg => svg.append(h('path', { d: seg })));
  return svg;
}
export const iconNames = Object.keys(I);

// ───────────────────────────────────────────── format
export const fmt = {
  num: n => new Intl.NumberFormat('ru-RU').format(n ?? 0),
  size: n => (n >= 1073741824 ? (n / 1073741824).toFixed(2) + ' ГБ' : n >= 1048576 ? (n / 1048576).toFixed(2) + ' МБ' : n >= 1024 ? (n / 1024).toFixed(1) + ' КБ' : (n || 0) + ' Б'),
  date: ts => (ts ? new Date(ts * 1000).toLocaleDateString('ru-RU', { day: '2-digit', month: 'long', year: 'numeric' }) : ''),
  datetime: ts => (ts ? new Date(ts * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : ''),
  /** как vremja() старого движка: «5 мин назад», «вчера в 14:03», «12 мая 2024» */
  ago(ts) {
    if (!ts) return '';
    const d = new Date(ts * 1000), now = new Date(), s = Math.floor((now - d) / 1000);
    if (s < 60) return 'только что';
    if (s < 3600) return Math.floor(s / 60) + ' мин. назад';
    const t = d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
    const day = x => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
    const diff = (day(now) - day(d)) / 86400000;
    if (diff === 0) return 'сегодня в ' + t;
    if (diff === 1) return 'вчера в ' + t;
    return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: d.getFullYear() === now.getFullYear() ? undefined : 'numeric' }) + ' в ' + t;
  },
  plural: (n, one, few, many) => { n = Math.abs(n) % 100; const m = n % 10; return n > 10 && n < 20 ? many : m === 1 ? one : m >= 2 && m <= 4 ? few : many; },
  age(y, m, d) { if (!y) return ''; const now = new Date(); let a = now.getFullYear() - y; if (m && (now.getMonth() + 1 < m || (now.getMonth() + 1 === m && now.getDate() < (d || 1)))) a--; return a; },
};

// ───────────────────────────────────────────── router
const routes = [];
let currentCleanup = null;
let renderSeq = 0;
let lastRenderedPath = '';
let navBarEl = null;

function navProgress(on) {
  if (!navBarEl) {
    navBarEl = h('div.nav-progress');
    document.body.append(navBarEl);
  }
  navBarEl.classList.toggle('active', !!on);
}

function swapView(view, nodes, isSamePage) {
  view.classList.remove('is-loading', 'view-enter', 'view-fade');
  if (nodes instanceof Node) view.replaceChildren(nodes);
  else if (Array.isArray(nodes)) view.replaceChildren(...nodes.filter(Boolean));
  else return;
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!reduced && !document.body.classList.contains('shorts-fs')) {
    // Для мессенджера и перерисовки той же страницы — только мягкий fade без сдвига (чтобы не дёргать fixed-элементы)
    const cls = (isSamePage || document.body.classList.contains('msg-page')) ? 'view-fade' : 'view-enter';
    void view.offsetWidth;
    view.classList.add(cls);
    view.addEventListener('animationend', () => view.classList.remove('view-enter', 'view-fade'), { once: true });
  }
}

/** Проверка, что интерфейс открыт внутри нативного/десктопного клиента (Electron или Android). */
export const isNativeClient = () => !!(window.dcmsNative || /DcmsAndroid\//i.test(navigator.userAgent));
export const nativePlatform = () => {
  if (!window.dcmsNative) return /DcmsAndroid\//i.test(navigator.userAgent) ? 'android' : '';
  return typeof window.dcmsNative.platform === 'function' ? window.dcmsNative.platform() : (window.dcmsNative.platform || 'desktop');
};
export const canAndroidSign2FA = () => !!(window.dcmsNative && typeof window.dcmsNative.canSign2FA === 'function' && window.dcmsNative.canSign2FA());
const NATIVE_GUEST_RE = /^\/(login|reg(\/activate\/[^/]+\/[^/]+)?|pass(\/[^/]+\/[^/]+)?|ban)$/;

function compile(pattern) {
  const keys = [];
  const re = new RegExp('^' + pattern.replace(/\/+$/, '').replace(/:([a-zA-Z_]+)(\*)?/g, (_, k, star) => { keys.push(k); return star ? '(.+)' : '([^/]+)'; }) + '/?$');
  return { re, keys };
}
export const router = {
  /** router.add('/forum/:id', async ctx => Node|void). ctx: {params, query, path, page, title(t), onLeave(fn), el} */
  add(pattern, handler, opts = {}) { routes.push({ pattern, handler, ...compile(pattern), opts }); },
  go(path, replace = false) { if (path === location.pathname + location.search) return router.render(); history[replace ? 'replaceState' : 'pushState'](null, '', path); return router.render(); },
  back() { history.back(); },
  async render() {
    const view = $('#view');
    if (!view) return;
    const seq = ++renderSeq;
    const url = new URL(location.href);
    const path = url.pathname.replace(/\/+$/, '') || '/';
    const guestClient = isNativeClient() && !state.user;
    document.body.classList.toggle('native-auth-only', guestClient);
    if (guestClient && !NATIVE_GUEST_RE.test(path)) {
      history.replaceState(null, '', '/login');
      return router.render();
    }
    const isSamePage = path === lastRenderedPath;

    for (const r of routes) {
      const m = path.match(r.re);
      if (!m) continue;
      const params = {};
      r.keys.forEach((k, i) => (params[k] = decodeURIComponent(m[i + 1])));
      const query = Object.fromEntries(url.searchParams);
      let nextCleanup = null;
      const ctx = {
        params, query, path, page: Math.max(1, parseInt(query.page) || 1), el: view,
        title: t => { document.title = (t ? t + ' — ' : '') + (state.config.site_name || 'DCMS Social'); bus.emit('title', t); },
        onLeave: fn => { nextCleanup = fn; },
      };

      // Быстрые проверки прав без мигания
      if (r.opts.auth && !state.user) {
        if (currentCleanup) { try { currentCleanup(); } catch {} currentCleanup = null; }
        swapView(view, ui.needAuth(), false);
        lastRenderedPath = path;
        bus.emit('route', path);
        return;
      }
      if ((r.opts.level && level() < r.opts.level) || (r.opts.access && !can(r.opts.access))) {
        if (currentCleanup) { try { currentCleanup(); } catch {} currentCleanup = null; }
        swapView(view, ui.forbidden(), false);
        lastRenderedPath = path;
        bus.emit('route', path);
        return;
      }

      navProgress(true);
      if (!isSamePage && view.childNodes.length) view.classList.add('is-loading');
      // Показываем спиннер только если загрузка реально затянулась (> 350 мс)
      const spinTimer = setTimeout(() => {
        if (seq === renderSeq && !isSamePage) view.replaceChildren(ui.loading());
      }, 350);

      try {
        const out = await r.handler(ctx);
        clearTimeout(spinTimer);
        if (seq !== renderSeq) {
          if (nextCleanup) { try { nextCleanup(); } catch {} }
          return;
        }
        if (currentCleanup) { try { currentCleanup(); } catch {} }
        currentCleanup = nextCleanup;
        if (out !== undefined) swapView(view, out, isSamePage);
        else view.classList.remove('is-loading');
        if (!isSamePage) window.scrollTo(0, 0);
        lastRenderedPath = path;
      } catch (e) {
        clearTimeout(spinTimer);
        if (seq !== renderSeq) return;
        console.error(e);
        if (currentCleanup) { try { currentCleanup(); } catch {} currentCleanup = null; }
        const errNode = e instanceof ApiError
          ? (e.status === 404 ? ui.notFound(e.message) : e.status === 403 ? ui.forbidden(e.message) : e.status === 401 ? ui.needAuth() : ui.errorBox(e.message))
          : ui.errorBox('Ошибка отображения: ' + e.message);
        swapView(view, errNode, false);
        lastRenderedPath = path;
      } finally {
        if (seq === renderSeq) navProgress(false);
      }
      bus.emit('route', path);
      return;
    }
    if (currentCleanup) { try { currentCleanup(); } catch {} currentCleanup = null; }
    swapView(view, ui.notFound(), false);
    lastRenderedPath = path;
  },
  start() {
    addEventListener('popstate', () => router.render());
    document.addEventListener('click', e => {
      const a = e.target.closest('a');
      if (!a || e.defaultPrevented || e.button || e.metaKey || e.ctrlKey || e.shiftKey || a.target === '_blank' || a.hasAttribute('download')) return;
      const href = a.getAttribute('href');
      if (!href || !href.startsWith('/') || href.startsWith('/api/') || a.dataset.native != null) return;
      e.preventDefault();
      router.go(href);
    });
    return router.render();
  },
};
/** Ссылка SPA. */
export const link = (href, ...c) => h('a', { href }, ...c);

// ───────────────────────────────────────────── ui kit
export const ui = {
  icon,
  loading: () => h('div.loading', h('div.spinner')),
  empty: (text = 'Здесь пока пусто', ic = 'inbox') => h('div.empty', icon(ic, 36), h('div', text)),
  errorBox: msg => h('div.card.pad.err', icon('alert'), ' ', msg),
  notFound: msg => h('div.empty', icon('search', 40), h('h2', 'Не найдено'), h('p.muted', msg || 'Такой страницы не существует.'), link('/', 'На главную')),
  forbidden: msg => h('div.empty', icon('lock', 40), h('h2', 'Нет доступа'), h('p.muted', msg || 'У вас недостаточно прав для просмотра.'), link('/', 'На главную')),
  needAuth: () => h('div.empty', icon('log-in', 40), h('h2', 'Нужна авторизация'), h('p.muted', 'Войдите или зарегистрируйтесь, чтобы продолжить.'),
    h('div.row.center', link('/login?return=' + encodeURIComponent(location.pathname + location.search), h('span.btn.primary', 'Войти')), link('/reg', h('span.btn', 'Регистрация')))),

  /** Заголовок страницы с кнопками справа. */
  pageHead(title, ...actions) { return h('div.page-head', h('h1', title), h('div.row', ...actions)); },
  breadcrumbs(items) {
    return h('nav.crumbs', items.map((it, i) => [i ? icon('chevron-right', 14) : null, it.href ? link(it.href, it.title) : h('span', it.title)]));
  },
  card: (...c) => h('div.card', ...c),
  section: (title, ...c) => h('section.card', title ? h('div.card-title', title) : null, ...c),
  badge: (t, kind = '') => h('span.badge' + (kind ? '.' + kind : ''), t),
  btn(label, opts = {}) {
    const { href, onclick, kind = '', ic, small, title, type = 'button', disabled } = opts;
    const content = [ic ? icon(ic, small ? 14 : 16) : null, label ? h('span', label) : null];
    const cls = 'btn' + (kind ? '.' + kind.split(' ').join('.') : '') + (small ? '.sm' : '') + (!label ? '.icon-only' : '');
    return href ? h('a.' + cls.slice(0).replace(/^/, ''), { href, title }, ...content) : h('button.' + cls, { type, onclick, title, disabled }, ...content);
  },
  /** Аватар пользователя (ref из API). */
  avatar(ref, size = 40) {
    const st = { width: size + 'px', height: size + 'px', fontSize: Math.round(size * 0.42) + 'px' };
    const fr = ref && ref.avatar_frame ? '.ava-' + ref.avatar_frame : '';
    const wrap = h('span.avatar' + (ref && ref.online ? '.online' : '') + fr, { style: st });
    if (ref && ref.avatar) wrap.append(h('img', { src: ref.avatar.replace(/s=\d+/, 's=' + (size > 60 ? 200 : 100)), alt: '', loading: 'lazy' }));
    else wrap.append(h('span.ph.pol' + (ref ? ref.pol : 1), (ref && ref.nick ? ref.nick[0] : '?').toUpperCase()));
    return wrap;
  },
  /** Ссылка на пользователя: ник + значки (онлайн/бан/группа/медаль). */
  userLink(ref, o = {}) {
    if (!ref) return h('span', '—');
    const el = h(ref.id ? 'a.user-link' : 'span.user-link', ref.id ? { href: '/id' + ref.id } : null);
    if (o.avatar) el.append(ui.avatar(ref, o.avatar === true ? 24 : o.avatar));
    el.append(h('span.nick' + (ref.vip && ref.vip_nick_style ? '.vip-nick-' + ref.vip_nick_style : ''), ref.nick));
    if (o.badges !== false && ref.id) {
      if (ref.vip) el.append(h('span.ub.vip-ic', { title: 'VIP-подписка' }, ref.vip_badge || '👑'));
      if (ref.verified) el.append(h('span.ub.ver', { title: 'Подтверждённый аккаунт', style: ref.accent ? { background: ref.accent, boxShadow: '0 1px 3px ' + ref.accent + '66' } : null }, icon('check', 10)));
      if (ref.banned) el.append(h('span.ub.ban', { title: 'Забанен' }, icon('gavel', 13)));
      else if (ref.level >= 2) el.append(h('span.ub.adm', { title: ref.group }, icon('shield', 13)));
      else if (ref.level >= 1) el.append(h('span.ub.mod', { title: ref.group }, icon('shield', 13)));
      if (ref.medal) el.append(h('span.ub.medal.m' + ref.medal, { title: 'Рейтинг' }, icon('star', 13)));
      if (ref.online) el.append(h('span.ub.on', { title: ref.mobile ? 'Онлайн (телефон)' : 'Онлайн' }, ref.mobile ? icon('smartphone', 12) : '●'));
    }
    return el;
  },
  time: ts => h('time.muted', { title: fmt.datetime(ts), datetime: ts ? new Date(ts * 1000).toISOString() : '' }, fmt.ago(ts)),

  toast(msg, kind = 'info', ms = 3500) {
    let box = $('#toasts');
    if (!box) { box = h('div#toasts'); document.body.append(box); }
    const t = h('div.toast.' + kind, icon(kind === 'error' ? 'alert' : kind === 'success' ? 'check' : 'info', 16), h('span', msg));
    box.append(t);
    setTimeout(() => { t.classList.add('out'); setTimeout(() => t.remove(), 300); }, ms);
  },
  ok: m => ui.toast(m, 'success'),

  modal({ title, body, actions = [], wide = false, onClose } = {}) {
    const close = v => { back.remove(); document.removeEventListener('keydown', esc); onClose && onClose(v); };
    const esc = e => e.key === 'Escape' && close();
    const box = h('div.modal' + (wide ? '.wide' : ''), { role: 'dialog' },
      h('div.modal-head', h('h3', title || ''), h('button.btn.icon-only.ghost', { onclick: () => close(), 'aria-label': 'Закрыть' }, icon('x'))),
      h('div.modal-body', body), actions.length ? h('div.modal-foot', actions.map(a => h('button.btn' + (a.kind ? '.' + a.kind : ''), { onclick: () => { if (a.onclick) { const r = a.onclick(); if (r === false) return; } close(a.value); } }, a.label))) : null);
    const back = h('div.modal-back', { onclick: e => e.target === back && close() }, box);
    document.body.append(back);
    document.addEventListener('keydown', esc);
    const f = $('input,textarea,select', box); f && f.focus();
    return { close, el: box };
  },
  confirm(message, { okLabel = 'Да', cancelLabel = 'Отмена', danger = false, title = 'Подтверждение' } = {}) {
    return new Promise(res => ui.modal({ title, body: h('p', message), onClose: v => res(!!v), actions: [{ label: cancelLabel, value: false }, { label: okLabel, value: true, kind: danger ? 'danger' : 'primary' }] }));
  },
  prompt(message, value = '', { title = 'Введите значение', multiline = false } = {}) {
    return new Promise(res => {
      const inp = multiline ? h('textarea.input', { rows: 4 }, value) : h('input.input', { value });
      ui.modal({ title, body: [h('p', message), inp], onClose: v => res(v ? inp.value : null), actions: [{ label: 'Отмена' }, { label: 'ОК', kind: 'primary', value: true }] });
    });
  },

  /** Пагинация по ответу API {page,pages}. Ссылки сохраняют текущие query-параметры, меняя ?page=. */
  pager(data, { onPage } = {}) {
    if (!data || data.pages <= 1) return null;
    const cur = data.page, last = data.pages;
    const href = p => { const u = new URL(location.href); if (p <= 1) u.searchParams.delete('page'); else u.searchParams.set('page', p); return u.pathname + u.search; };
    const mk = (p, label, cls = '') => p === cur ? h('span.pg.cur', label ?? p) : h('a.pg' + cls, { href: href(p), onclick: onPage ? e => { e.preventDefault(); onPage(p); } : null }, label ?? p);
    const nums = new Set([1, 2, cur - 1, cur, cur + 1, last - 1, last].filter(p => p >= 1 && p <= last));
    const out = [];
    if (cur > 1) out.push(mk(cur - 1, icon('chevron-left', 16), '.arrow'));
    let prev = 0;
    [...nums].sort((a, b) => a - b).forEach(p => { if (p - prev > 1) out.push(h('span.pg.gap', '…')); out.push(mk(p)); prev = p; });
    if (cur < last) out.push(mk(cur + 1, icon('chevron-right', 16), '.arrow'));
    return h('nav.pager', out);
  },

  tabs(items, active, onChange) {
    const el = h('div.tabs');
    const draw = a => { el.replaceChildren(...items.map(it => h(it.href ? 'a.tab' : 'button.tab', { class: it.key === a ? 'active' : '', href: it.href, onclick: it.href ? null : () => { draw(it.key); onChange && onChange(it.key); } }, it.icon ? icon(it.icon, 16) : null, it.label, it.badge ? h('span.badge', it.badge) : null))); };
    draw(active);
    return el;
  },

  /**
   * Конструктор форм. fields: [{name,label,type,value,options,placeholder,hint,required,rows,min,max,accept,bb}]
   * type: text|password|email|number|textarea|bb|select|checkbox|radio|file|hidden|date|color|info
   * options: [{value,label}] | ['a','b'].  opts: {submit:'Сохранить', onSubmit: async values => {}, cancel, inline}
   */
  form(fields, opts = {}) {
    const inputs = {};
    const form = h('form.form' + (opts.inline ? '.inline' : ''), { novalidate: true });
    for (const f of fields) {
      let ctl;
      const t = f.type || 'text';
      const common = { name: f.name, placeholder: f.placeholder, required: f.required, maxlength: f.maxlength, disabled: f.disabled };
      if (t === 'textarea') ctl = h('textarea.input', { ...common, rows: f.rows || 4 }, f.value ?? '');
      else if (t === 'bb') { const ed = ui.bbEditor({ value: f.value ?? '', rows: f.rows || 5, placeholder: f.placeholder, name: f.name, maxlength: f.maxlength }); inputs[f.name] = ed; form.append(h('div.field', f.label ? h('label', f.label) : null, ed.el, f.hint ? h('div.hint', f.hint) : null)); continue; }
      else if (t === 'captcha') { const cap = ui.captcha({ name: f.name }); inputs[f.name] = cap; form.append(h('div.field', f.label ? h('label', f.label, f.required ? h('span.req', '*') : null) : null, cap.el)); continue; }
      else if (t === 'select') ctl = h('select.input', common, (f.options || []).map(o => { o = typeof o === 'object' ? o : { value: o, label: o }; return h('option', { value: o.value, selected: String(o.value) === String(f.value ?? '') }, o.label); }));
      else if (t === 'checkbox') { ctl = h('input', { type: 'checkbox', name: f.name, checked: !!f.value, disabled: f.disabled }); inputs[f.name] = ctl; form.append(h('label.check', ctl, h('span', f.label), f.hint ? h('small.muted', ' — ' + f.hint) : null)); continue; }
      else if (t === 'radio') { const group = (f.options || []).map(o => { o = typeof o === 'object' ? o : { value: o, label: o }; return h('label.check', h('input', { type: 'radio', name: f.name, value: o.value, checked: String(o.value) === String(f.value ?? '') }), h('span', o.label)); }); inputs[f.name] = { get value() { const c = form.querySelector(`input[name="${f.name}"]:checked`); return c ? c.value : ''; } }; form.append(h('div.field', f.label ? h('label', f.label) : null, h('div.stack-s', group), f.hint ? h('div.hint', f.hint) : null)); continue; }
      else if (t === 'hidden') { ctl = h('input', { type: 'hidden', name: f.name, value: f.value ?? '' }); inputs[f.name] = ctl; form.append(ctl); continue; }
      else if (t === 'info') { form.append(h('div.field', f.label ? h('label', f.label) : null, h('div', f.value))); continue; }
      else if (t === 'file') ctl = h('input.input', { ...common, type: 'file', accept: f.accept, multiple: f.multiple });
      else ctl = h('input.input', { ...common, type: t, value: f.value ?? '', min: f.min, max: f.max, step: f.step, autocomplete: f.autocomplete });
      inputs[f.name] = ctl;
      form.append(h('div.field', f.label ? h('label', { for: f.name }, f.label, f.required ? h('span.req', '*') : null) : null, ctl, f.hint ? h('div.hint', f.hint) : null));
    }
    const values = () => {
      const v = {};
      for (const f of fields) {
        const c = inputs[f.name]; if (!c) continue;
        if (f.type === 'checkbox') v[f.name] = c.checked;
        else if (f.type === 'file') v[f.name] = f.multiple ? [...c.files] : c.files[0] || null;
        else if (f.type === 'number') v[f.name] = c.value === '' ? null : Number(c.value);
        else v[f.name] = c.value;
      }
      return v;
    };
    const err = h('div.form-err');
    const btn = h('button.btn.primary', { type: 'submit' }, opts.submit || 'Сохранить');
    form.append(err, h('div.row', btn, opts.cancel ? h('button.btn', { type: 'button', onclick: opts.cancel }, 'Отмена') : null, opts.extra));
    form.addEventListener('submit', async e => {
      e.preventDefault();
      err.textContent = '';
      btn.disabled = true;
      try { await opts.onSubmit(values(), form); } catch (ex) { err.textContent = ex.message || 'Ошибка'; if (!(ex instanceof ApiError)) console.error(ex); } finally { btn.disabled = false; }
    });
    form.values = values;
    form.inputs = inputs;
    return form;
  },

  /** BB-редактор с интерактивным тулбаром, цветами и встроенной панелью смайлов. Возвращает {el, value, set(v), insert(text)}. */
  bbEditor({ value = '', rows = 4, placeholder = '', name, maxlength } = {}) {
    const ta = h('textarea.input.bb', { rows, placeholder, name, maxlength }, value);
    const wrap = (a, b = '') => { const s = ta.selectionStart, e = ta.selectionEnd, v = ta.value; ta.value = v.slice(0, s) + a + v.slice(s, e) + (b || '') + v.slice(e); ta.focus(); ta.selectionStart = s + a.length; ta.selectionEnd = e + a.length; };
    const bb = state.config.bb || {};
    const smilesBox = h('div.bb-smiles-inline', { style: { display: 'none' } });
    let smilesLoaded = false;

    const tools = [
      [h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'), 'b'],
      [h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'), 'i'],
      [h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'), 'u'],
      [h('del', 'S'), 'Зачёркнутый', () => wrap('[s]', '[/s]'), 's'],
      [icon('eye-off', 14), 'Спойлер (скрытый текст)', () => wrap('[spoiler]', '[/spoiler]'), 'spoiler'],
      [h('span.bb-rainbow', 'A'), 'Радужный градиент', () => wrap('[rainbow]', '[/rainbow]'), 'rainbow'],
      [h('code', { style: { fontSize: '.75rem' } }, '<>'), 'Моноширинный', () => wrap('[mono]', '[/mono]'), 'mono'],
      [icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'), 'quote'],
      [icon('chevron-down', 14), 'Сворачиваемый блок [cut]', async () => {
        const t = await ui.prompt('Заголовок скрытого блока', 'Подробнее', { title: 'Скрытый блок' });
        if (t !== null) wrap(t.trim() ? `[cut=${t.trim()}]` : '[cut]', '[/cut]');
      }, 'cut'],
      [icon('link', 14), 'Вставить ссылку', async () => {
        const u = await ui.prompt('Адрес ссылки (https://...)', 'https://', { title: 'Вставить ссылку' });
        if (u && u.trim()) {
          const sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
          wrap(`[url=${u.trim()}]`, sel ? '[/url]' : `${u.trim()}[/url]`);
        }
      }, 'url'],
      [icon('image', 14), 'Вставить картинку', async () => {
        const u = await ui.prompt('Ссылка на картинку (https://...)', 'https://', { title: 'Вставить картинку' });
        if (u && u.trim()) wrap(`[img]${u.trim()}[/img]`);
      }, 'img'],
      [icon('hash', 14), 'Блок кода', () => wrap('[code]', '[/code]'), 'code'],
    ];

    const colors = [['red', '#e11d48', 'Красный'], ['yellow', '#eab308', 'Жёлтый'], ['green', '#16a34a', 'Зелёный'], ['blue', '#2563eb', 'Синий']]
      .filter(([k]) => bb[k] !== false)
      .map(([k, col, title]) => h('button.msg-color-dot', { type: 'button', title, style: { background: col }, onclick: () => wrap(`[${k}]`, `[/${k}]`) }));

    const hexTextIn = h('input', { type: 'color', value: '#ff4081', onchange: () => wrap(`[color=${hexTextIn.value}]`, '[/color]') });
    const hexBgIn = h('input', { type: 'color', value: '#fef08a', onchange: () => wrap(`[bg=${hexBgIn.value}]`, '[/bg]') });
    const hexTextBtn = bb.color !== false ? h('label.bb-color-pick', { title: 'Любой HEX-цвет текста [color=#...]' }, h('span', { style: { color: '#ff4081' } }, 'A'), hexTextIn) : null;
    const hexBgBtn = bb.bg !== false ? h('label.bb-color-pick', { title: 'Любой HEX-цвет маркера/фона [bg=#...]' }, h('span', { style: { background: '#fef08a', color: '#111', padding: '0 3px', borderRadius: '3px' } }, 'bg'), hexBgIn) : null;

    const smileBtn = h('button.btn.ghost.icon-only.sm', {
      type: 'button', title: 'Смайлы',
      onclick: async () => {
        const open = smilesBox.style.display === 'none';
        smilesBox.style.display = open ? 'flex' : 'none';
        if (open && !smilesLoaded) {
          smilesBox.replaceChildren(ui.loading());
          try {
            const list = await api.get('/api/smiles/all', null, { silent: true });
            const arr = list.items || list || [];
            smilesLoaded = true;
            smilesBox.replaceChildren(...(arr.length ? arr.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))) : [h('span.small.muted', 'Смайлов нет')]));
          } catch { smilesBox.replaceChildren(h('span.small.muted', 'Смайлы недоступны')); }
        }
      },
    }, icon('smile', 15));

    const bar = h('div.bb-bar',
      ...tools.filter(t => !t[3] || bb[t[3]] !== false).map(([lbl, title, fn]) => h('button.btn.ghost.icon-only.sm', { type: 'button', title, onclick: fn }, lbl)),
      h('span.bb-colors', ...colors, hexTextBtn, hexBgBtn),
      smileBtn);
    return { el: h('div.bb-editor', bar, smilesBox, ta), ta, get value() { return ta.value; }, set value(v) { ta.value = v; }, insert: t => wrap(t) };
  },

  dropdown(trigger, items) {
    const menu = h('div.dropdown-menu', items.filter(Boolean).map(it => it.sep ? h('hr') : h(it.href ? 'a.dd-item' : 'button.dd-item', { href: it.href, class: it.danger ? 'danger' : '', onclick: e => { menu.classList.remove('open'); it.onclick && it.onclick(e); } }, it.icon ? icon(it.icon, 15) : null, it.label)));
    const wrap = h('div.dropdown', trigger, menu);
    trigger.addEventListener('click', e => { e.stopPropagation(); const o = menu.classList.toggle('open'); if (o) { const off = () => { menu.classList.remove('open'); document.removeEventListener('click', off); }; setTimeout(() => document.addEventListener('click', off), 0); } });
    return wrap;
  },

  /** Простая таблица: cols [{title, render(row)|key, class}], rows []. */
  table(cols, rows, { empty = 'Нет данных' } = {}) {
    if (!rows || !rows.length) return ui.empty(empty);
    return h('div.table-wrap', h('table.table', h('thead', h('tr', cols.map(c => h('th', { class: c.class }, c.title)))), h('tbody', rows.map(r => h('tr', cols.map(c => h('td', { class: c.class }, c.render ? c.render(r) : r[c.key])))))));
  },

  /** Разметка лайков/голосов: кнопка с счётчиком. */
  reaction(ic, count, active, onclick, title) {
    return h('button.react' + (active ? '.active' : ''), { type: 'button', onclick, title }, icon(ic, 15), h('span', fmt.num(count)));
  },

  /** Текст из API: msg_html — безопасный HTML от сервера. */
  html: (htmlStr, cls = 'text') => h('div.' + cls, { html: htmlStr || '' }),

  /** Лента сообщений-«комментариев»: общий вид элемента. */
  post({ user, time, body, actions = [], extra, id }) {
    return h('article.post', { id },
      h('div.post-side', ui.avatar(user, 44)),
      h('div.post-main',
        h('div.post-head', ui.userLink(user), h('span.post-time', time ? ui.time(time) : null), h('span.grow'), ...actions),
        body, extra));
  },

  /** Выбор файла/картинки с превью. */
  imagePreview(input, img) { input.addEventListener('change', () => { const f = input.files[0]; if (f) img.src = URL.createObjectURL(f); }); },

  /** Интерактивная «человечная» капча (пазл / поворот / выбор фигуры). Возвращает {el, value, reset()}. */
  captcha({ name = 'captcha', onSolved } = {}) {
    const TYPES = ['slider', 'rotate', 'click'];
    let token = '', open = false, loading = false, curType = '', challenge = null, openedAt = 0;
    const hidden = h('input', { type: 'hidden', name, value: '' });
    const chk = h('span.cap-check');
    const lbl = h('span.cap-label', 'Я не робот');
    const bar = h('button.cap-bar', { type: 'button', onclick: () => { if (!token && !loading) toggle(); } },
      chk, lbl, h('span.grow'), h('span.cap-brand', icon('shield', 14), 'Защита'));
    const panel = h('div.cap-panel', { style: { display: 'none' } });
    const wrap = h('div.cap-box', hidden, bar, panel);

    const setSolved = tok => {
      token = tok;
      hidden.value = tok;
      open = false;
      panel.style.display = 'none';
      panel.replaceChildren();
      wrap.classList.add('solved');
      wrap.classList.remove('open');
      chk.replaceChildren(icon('check', 16));
      lbl.textContent = 'Проверка пройдена';
      onSolved && onSolved(tok);
    };

    const reset = () => {
      token = '';
      hidden.value = '';
      open = false;
      loading = false;
      challenge = null;
      wrap.classList.remove('solved', 'open');
      chk.replaceChildren();
      lbl.textContent = 'Я не робот';
      panel.style.display = 'none';
      panel.replaceChildren();
    };

    const shake = (msgEl, text) => {
      if (msgEl) { msgEl.textContent = text; msgEl.className = 'cap-msg err'; }
      panel.classList.remove('shake');
      void panel.offsetWidth;
      panel.classList.add('shake');
    };

    const load = async wantType => {
      loading = true;
      wrap.classList.add('open');
      panel.style.display = '';
      panel.replaceChildren(h('div.cap-loading', 'Загрузка задания…'));
      try {
        const q = wantType ? { type: wantType } : null;
        challenge = await api.get('/api/captcha/challenge', q, { silent: true });
        curType = challenge.type;
        openedAt = Date.now();
        renderChallenge();
      } catch {
        panel.replaceChildren(h('div.cap-msg.err', 'Не удалось загрузить задание'), h('button.btn.sm', { type: 'button', onclick: () => load(curType) }, 'Повторить'));
      } finally { loading = false; }
    };

    const toggle = () => {
      if (open) { open = false; wrap.classList.remove('open'); panel.style.display = 'none'; return; }
      open = true;
      load(curType);
    };

    const nextType = () => {
      const idx = TYPES.indexOf(curType);
      load(TYPES[(idx + 1) % TYPES.length]);
    };

    const renderChallenge = () => {
      const ch = challenge;
      const msg = h('div.cap-msg');
      const head = h('div.cap-head',
        h('div.cap-prompt', ch.prompt),
        h('div.cap-tools',
          h('button.cap-ico', { type: 'button', title: 'Другой тип задания', onclick: () => nextType() }, icon('grid', 15)),
          h('button.cap-ico', { type: 'button', title: 'Обновить задание', onclick: () => load(curType) }, icon('rotate-ccw', 15)),
        ));
      const body = h('div.cap-body');

      if (ch.type === 'slider') {
        const maxX = ch.w - ch.pw;
        let curX = 0, moves = 0, dragging = false, startClientX = 0, startX = 0, busy = false;
        const piece = h('img.cap-piece', {
          src: ch.piece, alt: '', draggable: false,
          style: { top: (ch.y / ch.h * 100) + '%', width: (ch.pw / ch.w * 100) + '%', left: '0%' },
        });
        const stage = h('div.cap-stage', { style: { aspectRatio: `${ch.w} / ${ch.h}` } },
          h('img.cap-bg', { src: ch.bg, alt: '', draggable: false }), piece);
        const fill = h('div.cap-fill');
        const hint = h('span.cap-track-hint', 'Потяните ползунок вправо →');
        const knob = h('div.cap-knob', { tabindex: 0, role: 'slider', 'aria-label': ch.prompt, 'aria-valuemin': 0, 'aria-valuemax': maxX, 'aria-valuenow': 0 }, icon('chevron-right', 18));
        const track = h('div.cap-track', fill, hint, knob);

        const setX = x => {
          curX = Math.max(0, Math.min(maxX, x));
          const pct = curX / ch.w * 100;
          piece.style.left = pct + '%';
          const tw = Math.max(1, track.clientWidth - knob.offsetWidth);
          const ratio = curX / maxX;
          knob.style.transform = `translateX(${ratio * tw}px)`;
          fill.style.width = (ratio * tw + knob.offsetWidth / 2) + 'px';
          knob.setAttribute('aria-valuenow', String(Math.round(curX)));
          hint.style.opacity = curX > 8 ? '0' : '1';
        };

        const submitSlider = async () => {
          if (busy) return;
          busy = true;
          try {
            const r = await api.post('/api/captcha/verify', { id: ch.id, answer: curX, duration_ms: Date.now() - openedAt, moves }, { silent: true });
            if (r.ok) { setSolved(r.token); return; }
            shake(msg, r.error || 'Попробуйте ещё раз');
            setX(0); moves = 0;
            if (r.expired) setTimeout(() => load('slider'), 650);
          } catch { shake(msg, 'Ошибка проверки'); }
          finally { busy = false; }
        };

        knob.addEventListener('pointerdown', e => {
          if (busy) return;
          dragging = true;
          startClientX = e.clientX;
          startX = curX;
          knob.setPointerCapture(e.pointerId);
          knob.classList.add('active');
        });
        knob.addEventListener('pointermove', e => {
          if (!dragging) return;
          const tw = Math.max(1, track.clientWidth - knob.offsetWidth);
          const dx = (e.clientX - startClientX) / tw * maxX;
          moves++;
          setX(startX + dx);
        });
        const finishDrag = () => {
          if (!dragging) return;
          dragging = false;
          knob.classList.remove('active');
          if (curX > 4) submitSlider();
        };
        knob.addEventListener('pointerup', finishDrag);
        knob.addEventListener('pointercancel', finishDrag);
        knob.addEventListener('keydown', e => {
          if (e.key === 'ArrowRight') { e.preventDefault(); moves++; setX(curX + (e.shiftKey ? 12 : 4)); }
          else if (e.key === 'ArrowLeft') { e.preventDefault(); moves++; setX(curX - (e.shiftKey ? 12 : 4)); }
          else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); moves = Math.max(moves, 3); submitSlider(); }
        });
        body.append(stage, track);
      } else if (ch.type === 'rotate') {
        let angle = 0, busy = false;
        const img = h('img.cap-rot-img', { src: ch.img, alt: '', draggable: false });
        const stage = h('div.cap-rot-wrap', img);
        const range = h('input.cap-rot-range', { type: 'range', min: 0, max: 360, step: 5, value: 0 });
        const applyAngle = a => {
          angle = ((a % 360) + 360) % 360;
          range.value = String(angle);
          img.style.transform = `rotate(${angle}deg)`;
        };
        range.oninput = () => applyAngle(+range.value);
        const submitRot = async () => {
          if (busy) return;
          busy = true;
          try {
            const r = await api.post('/api/captcha/verify', { id: ch.id, answer: angle, duration_ms: Date.now() - openedAt, moves: 3 }, { silent: true });
            if (r.ok) { setSolved(r.token); return; }
            shake(msg, r.error || 'Поверните точнее');
            if (r.expired) setTimeout(() => load('rotate'), 650);
          } catch { shake(msg, 'Ошибка проверки'); }
          finally { busy = false; }
        };
        const ctrls = h('div.cap-rot-ctrls',
          h('button.btn.sm', { type: 'button', onclick: () => applyAngle(angle - 15), title: 'Влево на 15°' }, '⟲'),
          range,
          h('button.btn.sm', { type: 'button', onclick: () => applyAngle(angle + 15), title: 'Вправо на 15°' }, '⟳'),
        );
        body.append(stage, ctrls, h('button.btn.primary.sm', { type: 'button', onclick: submitRot, style: { width: '100%' } }, icon('check', 15), ' Готово'));
      } else if (ch.type === 'click') {
        let busy = false;
        const grid = h('div.cap-grid', (ch.items || []).map(it => {
          const tile = h('button.cap-tile', {
            type: 'button',
            onclick: async () => {
              if (busy) return;
              busy = true;
              tile.classList.add('picked');
              try {
                const r = await api.post('/api/captcha/verify', { id: ch.id, key: it.key, duration_ms: Date.now() - openedAt, moves: 1 }, { silent: true });
                if (r.ok) { setSolved(r.token); return; }
                tile.classList.remove('picked');
                tile.classList.add('wrong');
                shake(msg, r.error || 'Не та фигура, попробуйте новую');
                setTimeout(() => load('click'), 650);
              } catch { shake(msg, 'Ошибка проверки'); busy = false; }
            },
          }, h('img', { src: it.img, alt: '', draggable: false }));
          return tile;
        }));
        body.append(grid);
      }

      panel.replaceChildren(head, body, msg);
    };

    return { el: wrap, get value() { return token; }, reset };
  },
};

// ───────────────────────────────────────────── realtime (SSE)
const topics = new Map(); // topic -> Set(handler)
let es = null, esTimer = null;
function reconnect() {
  clearTimeout(esTimer);
  esTimer = setTimeout(() => {
    if (es) es.close();
    const t = [...topics.keys()].join(',');
    es = new EventSource('/api/events' + (t ? '?topics=' + encodeURIComponent(t) : ''));
    ['notify', 'counters', 'chat', 'message', 'event', 'group_dm', 'group_chat', 'shorts_ready', 'shorts_progress', 'typing', 'mail_read', 'mail_react', 'mail_edit', 'adm_chat', 'adm_chat_del', 'gchat_msg', 'gchat_edit', 'gchat_react', 'gchat_typing', 'gchat_del', 'duel_update', 'mfa_request'].forEach(ev => es.addEventListener(ev, e => { let d; try { d = JSON.parse(e.data); } catch { return; } bus.emit('sse:' + ev, d); topics.forEach((hs, tp) => hs.forEach(fn => fn(ev, d, tp))); }));
    es.onerror = () => { es.close(); es = null; setTimeout(reconnect, 5000); };
  }, 50);
}
export const events = {
  /** Подписка на тему ('chat:3'); fn(eventName, data). Возвращает функцию отписки. Личные события (user:<id>) приходят автоматически — слушайте bus.on('sse:notify'|'sse:counters'|'sse:message'). */
  join(topic, fn) { if (!topics.has(topic)) topics.set(topic, new Set()); topics.get(topic).add(fn); reconnect(); return () => { const s = topics.get(topic); s && s.delete(fn); if (s && !s.size) topics.delete(topic); reconnect(); }; },
  start: reconnect,
  restart: reconnect,
};

// ───────────────────────────────────────────── misc
export async function refreshMe() {
  const r = await api.get('/api/me', null, { silent: true });
  state.user = r.user;
  state.config = r.config || {};
  state.counters = (r.user && r.user.counters) || {};
  bus.emit('user', state.user);
  return state.user;
}
export async function refreshCounters() {
  if (!state.user) return;
  try { state.counters = await api.get('/api/counters', null, { silent: true }); bus.emit('counters', state.counters); } catch {}
}
/** Подставляет ссылки возврата ?return= */
export const returnTo = () => { const r = new URLSearchParams(location.search).get('return'); return r && r.startsWith('/') && !r.startsWith('//') ? r : '/'; };
export const debounce = (fn, ms = 300) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };

/** Универсальный постраничный список: load(page) → {items,total,page,pages}; render(item)→Node. */
export async function pagedList(ctx, load, render, { empty = 'Пока ничего нет', cls = 'list' } = {}) {
  const data = await load(ctx.page);
  const body = data.items && data.items.length ? h('div.' + cls, data.items.map(render)) : ui.empty(empty);
  return h('div', body, ui.pager(data));
}

import { h, $, api, bus, can, events, icon, link, refreshCounters, refreshMe, router, state, ui } from './core.js';
import pages from './pages/manifest.js';

// mdi-имена иконок меню старого движка → наши иконки
const MDI = { 'rss-box': 'rss', chat: 'message-circle', pencil: 'pencil', 'folder-open-outline': 'folder', forum: 'forum', 'camera-image': 'camera', 'book-open': 'book', 'account-heart': 'heart', information: 'info', 'account-multiple': 'users', video: 'video' };
const TILE_COLORS = ['#6366f1', '#0ea5e9', '#f43f5e', '#f59e0b', '#10b981', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#3b82f6'];
export const menuIcon = n => (MDI[n] || (n && n.replace(/-outline$/, '')) || 'grid');

// ── тема
function applyTheme() {
  const t = (state.user && state.user.theme) || localStorage.getItem('theme') || state.config.theme_default || 'auto';
  document.documentElement.dataset.theme = t;
  const acc = (state.user && state.user.accent) || localStorage.getItem('accent') || state.config.accent_default;
  if (acc) document.documentElement.style.setProperty('--accent', acc);
  const dark = t === 'dark' || (t === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
  const m = document.querySelector('meta[name=theme-color]'); if (m) m.content = dark ? '#0b0d1a' : (acc || '#6366f1');
}
matchMedia('(prefers-color-scheme: dark)').addEventListener?.('change', () => { applyTheme(); });
function isDarkActive() {
  const t = document.documentElement.dataset.theme;
  return t === 'dark' || (t === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
}
export function setTheme(t, ev) {
  const applyNow = () => {
    localStorage.setItem('theme', t);
    if (state.user) state.user.theme = t;
    applyTheme();
    drawTop();
  };
  const save = () => {
    if (state.user) api.post('/api/settings/theme', { theme: t, accent: (state.user.accent || '') }, { silent: true }).catch(() => {});
  };
  const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!document.startViewTransition || reduce) {
    document.documentElement.classList.add('theme-fade');
    applyNow();
    setTimeout(() => document.documentElement.classList.remove('theme-fade'), 380);
    save();
    return;
  }
  let x = window.innerWidth - 36, y = 28;
  if (ev && ev.currentTarget && ev.currentTarget.getBoundingClientRect) {
    const r = ev.currentTarget.getBoundingClientRect();
    x = Math.round(r.left + r.width / 2);
    y = Math.round(r.top + r.height / 2);
  } else if (ev && typeof ev.clientX === 'number') {
    x = ev.clientX;
    y = ev.clientY;
  }
  const endR = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y));
  const vt = document.startViewTransition(() => applyNow());
  vt.ready.then(() => {
    document.documentElement.animate(
      { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${endR}px at ${x}px ${y}px)`] },
      { duration: 480, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', pseudoElement: '::view-transition-new(root)' }
    );
  }).catch(() => {});
  save();
}
bus.on('user', applyTheme);

// ── каркас
const app = $('#app');
const sideEl = h('aside.side');
const scrimEl = h('div.nav-scrim', { onclick: closeNav });
const view = h('main#view');
const topCounters = {};
let _admUnread = 0;
bus.on('sse:adm_chat', async d => {
  if (!state.user || (d && d.user && d.user.id === state.user.id)) return;
  if (location.pathname === '/admin/chat') return;
  const mod = await import('./pages/chatnews.js').catch(() => null);
  if (mod && mod.isAdminChatPopupOpen && mod.isAdminChatPopupOpen()) return;
  _admUnread++;
  const dot = $('#admChatDot');
  if (dot) { dot.textContent = String(_admUnread); dot.style.display = ''; }
});

function closeNav() { document.body.classList.remove('nav-open'); }
function toggleNav() { document.body.classList.toggle('nav-open'); }

document.addEventListener('keydown', e => { if (e.key === 'Escape') closeNav(); });
document.addEventListener('click', e => {
  const sp = e.target.closest('.bb-spoiler');
  if (sp) sp.classList.toggle('open');
});
{
  let sx = 0, sy = 0, swiping = false;
  document.addEventListener('touchstart', e => {
    if (!document.body.classList.contains('nav-open') || e.touches.length !== 1) return;
    sx = e.touches[0].clientX; sy = e.touches[0].clientY; swiping = true;
  }, { passive: true });
  document.addEventListener('touchend', e => {
    if (!swiping || !e.changedTouches.length) return;
    swiping = false;
    const dx = e.changedTouches[0].clientX - sx, dy = e.changedTouches[0].clientY - sy;
    if (dx < -45 && Math.abs(dx) > Math.abs(dy)) closeNav();
  }, { passive: true });
}

function counterBadge(key, cls = 'dot') { const v = state.counters[key]; return v ? h('span.' + cls, v) : null; }
function topButton(href, ic, key, title) {
  const el = h('a.icon-btn', { href, title }, icon(ic, 21));
  const upd = () => { el.querySelector('.dot')?.remove(); const b = counterBadge(key); b && el.append(b); };
  upd(); topCounters[href] = upd; return el;
}

function drawTop() {
  const top = $('.top-actions');
  top.replaceChildren();
  const dark = isDarkActive();
  const searchBtn = h('a.icon-btn', { href: '/search', title: 'Поиск по сайту', 'aria-label': 'Поиск' }, icon('search', 20));
  const themeBtn = h('button.icon-btn', { title: dark ? 'Светлая тема' : 'Тёмная тема', onclick: e => setTheme(isDarkActive() ? 'light' : 'dark', e) }, icon(dark ? 'sun' : 'moon', 20));
  if (!state.user) { top.append(searchBtn, themeBtn, link('/login', h('span.btn.sm', 'Войти')), link('/reg', h('span.btn.sm.primary', 'Регистрация'))); return; }
  const u = state.user;
  const menu = ui.dropdown(h('button.icon-btn', { title: u.nick }, ui.avatar({ ...u, online: true }, 32)), [
    { label: u.nick, href: '/id' + u.id, icon: 'user' }, { sep: true },
    { label: 'Моя лента', href: '/user/tape', icon: 'rss' }, { label: 'Друзья', href: '/user/frends', icon: 'users' },
    { label: 'Закладки', href: '/user/bookmark', icon: 'bookmark' }, { label: 'Баланс: ' + u.money, href: '/user/money', icon: 'coins' },
    { label: 'Настройки', href: '/user/settings', icon: 'settings' },
    can('adm_panel_show') ? { label: 'Администрирование', href: '/admin', icon: 'shield' } : null, { sep: true },
    { label: 'Выйти', icon: 'log-out', danger: true, onclick: logout },
  ]);
  const createBtn = h('a.icon-btn', { href: '/create', title: 'Создать публикацию', 'aria-label': 'Создать' }, icon('plus', 20));
  const isStaff = u.level >= 1 || can('adm_panel_show');
  let staffBtn = null;
  if (isStaff) {
    const dot = h('span.dot#admChatDot', { style: { display: _admUnread > 0 ? '' : 'none' } }, _admUnread > 0 ? String(_admUnread) : '');
    staffBtn = h('button.icon-btn', {
      type: 'button', title: 'Админ-чат (всплывающее окно)', 'aria-label': 'Админ-чат',
      onclick: async () => {
        _admUnread = 0; dot.style.display = 'none';
        const { toggleAdminChatPopup } = await import('./pages/chatnews.js');
        await toggleAdminChatPopup();
      },
    }, icon('shield', 20), dot);
  }
  top.append(searchBtn, createBtn, themeBtn, staffBtn, topButton('/user/discussions', 'message-circle', 'discussions', 'Обсуждения'), topButton('/mail', 'mail', 'mail', 'Почта'), topButton('/user/notification', 'bell', 'notification', 'Уведомления'), menu);
}
async function logout() { if (state.config.exit !== '0' && !(await ui.confirm('Выйти из аккаунта?'))) return; await api.post('/api/logout'); await refreshMe(); events.restart(); router.go('/'); }

async function drawSide() {
  let items = state.menu;
  const nav = h('nav.nav', h('div.nav-title', 'Разделы'), link('/', icon('home', 19), 'Главная'));
  items.forEach(m => {
    if (m.type === 'razd') nav.append(h('div.nav-title', m.name));
    else nav.append(h('a', { href: m.url }, icon(menuIcon(m.icon), 19), m.name, m.count ? h('span.cnt', m.count) : null));
  });
  const parts = [nav];
  if (state.user) {
    const u = state.user, c = state.counters;
    const mk = (href, ic, label, key) => h('a', { href }, icon(ic, 19), label, key && c[key] ? h('span.cnt', c[key]) : null);
    parts.push(h('nav.nav', h('div.nav-title', 'Мой кабинет'),
      mk('/create', 'plus', 'Создать публикацию'),
      mk('/id' + u.id, 'user', 'Моя страница'), mk('/user/tape', 'rss', 'Лента', 'tape'), mk('/user/discussions', 'message-circle', 'Обсуждения', 'discussions'),
      mk('/user/notification', 'bell', 'Уведомления', 'notification'), mk('/mail', 'mail', 'Почта', 'mail'), mk('/user/frends', 'users', 'Друзья', 'friends_new'),
      mk('/user/myguest', 'eye', 'Гости', 'myguest'), mk('/user/bookmark', 'bookmark', 'Закладки'), mk('/user/gift', 'gift', 'Подарки'),
      mk('/vip', 'crown', 'VIP-подписка'), mk('/user/money', 'coins', 'Монеты'), mk('/user/settings', 'settings', 'Настройки')));
    if (can('adm_panel_show')) parts.push(h('nav.nav', mk('/admin', 'shield', 'Администрирование', 'admin')));
  }
  parts.push(h('nav.nav', h('div.nav-title', 'Инструменты'), link('/developers', icon('key', 19), 'Разработчикам (API)')));
  const head = h('div.side-head',
    h('a.logo', { href: '/' }, h('span.logo-mark', icon('message-circle', 18)), h('span', state.config.site_name || 'DCMS Social')),
    h('button.icon-btn', { onclick: closeNav, 'aria-label': 'Закрыть меню', title: 'Закрыть' }, icon('x', 20)));
  sideEl.replaceChildren(head, h('div.side-body', ...parts));
  markActive();
}
function markActive() {
  const p = location.pathname;
  sideEl.querySelectorAll('a').forEach(a => { const hr = a.getAttribute('href'); a.classList.toggle('active', hr === p || (hr !== '/' && p.startsWith(hr.replace(/\/$/, '') + '/'))); });
  $$tab();
  closeNav();
}
function $$tab() { document.querySelectorAll('.tabbar a').forEach(a => { const hr = a.getAttribute('href'); a.classList.toggle('active', hr === location.pathname || (hr !== '/' && location.pathname.startsWith(hr))); }); }

function drawTabbar() {
  const bar = $('.tabbar'); if (!bar) return;
  const mk = (href, ic, label, key) => h('a', { href }, icon(ic, 22), h('span', label), key && state.counters[key] ? h('span.dot', state.counters[key]) : null);
  const addBtn = h('a.tabbar-add', { href: state.user ? '/create' : '/login', title: 'Создать публикацию', 'aria-label': 'Создать' },
    h('span.tabbar-add-pill', icon('plus', 22)), h('span', 'Создать'));
  bar.replaceChildren(
    mk('/', 'home', 'Главная'),
    mk('/user/tape', 'rss', 'Лента', 'tape'),
    addBtn,
    mk('/mail', 'mail', 'Почта', 'mail'),
    state.user ? mk('/id' + state.user.id, 'user', 'Профиль') : mk('/login', 'log-in', 'Войти'));
  $$tab();
}

function siteInitial() {
  const s = (state.config.site_name || 'DCMS Social').trim();
  return (Array.from(s)[0] || 'D').toUpperCase();
}

function build() {
  app.replaceChildren(
    h('header.topbar', h('button.icon-btn.burger', { onclick: toggleNav, 'aria-label': 'Меню' }, icon('menu', 22)),
      h('a.logo', { href: '/', title: state.config.site_name || 'DCMS Social' },
        h('span.logo-mark', h('span.logo-ic', icon('message-circle', 18)), h('span.logo-ch#sitechar', siteInitial())),
        h('span#sitename', state.config.site_name || 'DCMS Social')),
      h('div.top-actions')),
    scrimEl,
    h('div.shell', sideEl, view), h('div.footer', h('span', (state.config.site_name || 'DCMS Social') + ' © ' + new Date().getFullYear())), h('nav.tabbar'));
}

async function refreshAll() {
  const m = await api.get('/api/menu', null, { silent: true }).catch(() => []);
  state.menu = m;
  applyTheme(); drawTop(); drawTabbar(); await drawSide();
  const sn = $('#sitename'); sn && (sn.textContent = state.config.site_name || 'DCMS Social');
  const sc = $('#sitechar'); sc && (sc.textContent = siteInitial());
}
bus.on('user', () => { refreshAll(); events.restart(); });
bus.on('counters', () => { drawTop(); drawSide(); drawTabbar(); });
bus.on('route', () => { markActive(); syncMsgViewport(); });
let _prevMsgH = 0;
function syncMsgViewport() {
  if (!document.body.classList.contains('msg-page')) {
    document.documentElement.style.removeProperty('--msg-h');
    _prevMsgH = 0;
    return;
  }
  const vv = window.visualViewport;
  const hPx = vv ? Math.round(vv.height) : window.innerHeight;
  const topEl = document.querySelector('header.topbar');
  const topH = topEl ? Math.round(topEl.getBoundingClientRect().height) : 56;
  const nextH = Math.max(200, hPx - topH);
  const sc = document.querySelector('.msg-scroll');
  const nearBottom = sc ? (sc.scrollHeight - sc.scrollTop - sc.clientHeight < 90) : false;
  document.documentElement.style.setProperty('--msg-h', nextH + 'px');
  if (window.scrollY !== 0) window.scrollTo(0, 0);
  if (sc && (nearBottom || (_prevMsgH > 0 && nextH < _prevMsgH))) {
    requestAnimationFrame(() => { sc.scrollTop = sc.scrollHeight; });
  }
  _prevMsgH = nextH;
}
if (window.visualViewport) {
  window.visualViewport.addEventListener('resize', syncMsgViewport);
  window.visualViewport.addEventListener('scroll', syncMsgViewport);
}
window.addEventListener('resize', syncMsgViewport);
bus.on('sse:counters', () => refreshCounters());
bus.on('sse:notify', d => { refreshCounters(); ui.toast('🔔 ' + (d.from ? d.from.nick + ': ' : '') + (notifyTitle[d.type] || 'новое уведомление'), 'info'); });
const notifyTitle = { stena_komm: 'ответил на стене', stena: 'написал на вашей стене', status_komm: 'прокомментировал статус', them_komm: 'ответил в теме', new_gift: 'прислал подарок', ok_frend: 'принял заявку в друзья', no_frend: 'хочет дружить', news_komm: 'ответил к новости', foto_komm: 'прокомментировал фото', obmen_komm: 'прокомментировал файл', notes_komm: 'прокомментировал дневник', guest: 'ответил в гостевой', files_komm: 'прокомментировал файл', shorts_komm: 'прокомментировал клип', shorts_like: 'оценил ваш клип', group_invite: 'приглашает вас в сообщество', group_req: 'подал заявку в ваше сообщество', group_req_ok: 'одобрил вашу заявку в сообщество', group_dm: 'ответили от имени сообщества' };
setInterval(() => { if (state.user && document.visibilityState === 'visible') refreshCounters(); }, 60000);

// ── старт
(async function main() {
  await refreshMe().catch(() => {});
  build();
  // подключаем страницы модулей параллельно, но регистрируем маршруты в порядке манифеста
  const bust = Date.now();
  const mods = await Promise.all(pages.map(n => import(`./pages/${n}.js?v=${bust}`).then(() => null).catch(e => { console.warn('[pages/' + n + '] не загружен:', e.message); return n; })));
  await refreshAll();
  events.start();
  router.add('/:rest*', async () => ui.notFound(), {}); // последний — 404 (сработает, только если ничего не подошло)
  await router.start();
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  }
})();

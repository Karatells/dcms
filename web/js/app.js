import { h, $, api, bus, can, canAndroidSign2FA, events, icon, isNativeClient, link, nativePlatform, refreshCounters, refreshMe, router, state, ui } from './core.js';
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
  const createBtn = h('a.icon-btn.top-hide-mob', { href: '/create', title: 'Создать публикацию', 'aria-label': 'Создать' }, icon('plus', 20));
  const mailBtn = topButton('/mail', 'mail', 'mail', 'Почта');
  mailBtn.classList.add('top-hide-mob');
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
  top.append(searchBtn, createBtn, themeBtn, staffBtn, topButton('/user/discussions', 'message-circle', 'discussions', 'Обсуждения'), mailBtn, topButton('/user/notification', 'bell', 'notification', 'Уведомления'), menu);
}
async function logout() { if (state.config.exit !== '0' && !(await ui.confirm('Выйти из аккаунта?'))) return; await api.post('/api/logout'); await refreshMe(); events.restart(); router.go(isNativeClient() ? '/login' : '/', true); }

async function drawSide() {
  const u = state.user, c = state.counters || {};
  const mk = (href, ic, label, key) => h('a', { href }, icon(ic, 18), label, key && c[key] ? h('span.cnt', c[key]) : null);
  const parts = [];

  // 1. Главное — самые частые разделы (всегда сверху)
  const mainNav = h('nav.nav',
    u ? h('a.nav-create', { href: '/create' }, icon('plus', 18), 'Создать публикацию') : null,
    mk('/', 'home', 'Главная'),
    mk('/feed', 'trending', 'Умная лента'));
  if (u) {
    mainNav.append(
      mk('/id' + u.id, 'user', 'Моя страница'),
      mk('/mail', 'mail', 'Сообщения', 'mail'),
      mk('/user/frends', 'users', 'Друзья', 'friends_new'),
      mk('/user/notification', 'bell', 'Уведомления', 'notification'));
  }
  parts.push(mainNav);

  // 2. Разделы и медиа (сообщества, клипы, музыка, форум, фото, файлы и др. из /api/menu)
  const mediaNav = h('nav.nav', h('div.nav-title', 'Разделы и медиа'));
  const seenUrls = new Set(['/', '/feed', '/music', '/games', '/shop', '/quests', '/vip', '/mail', '/chats']);
  const menuItems = (state.menu || []).filter(m => m.type !== 'razd');
  // Приоритетные разделы из /api/menu (Клипы, Сообщества) ставим первыми, затем Музыку, затем остальные
  const normUrl = url => (url || '').replace(/\/+$/, '') || '/';
  const priority = ['/shorts', '/groups'];
  const firstMenu = [];
  const restMenu = [];
  menuItems.forEach(m => {
    const nu = normUrl(m.url);
    if (seenUrls.has(nu)) return;
    seenUrls.add(nu);
    if (priority.includes(nu)) firstMenu.push(m);
    else restMenu.push(m);
  });
  firstMenu.forEach(m => mediaNav.append(h('a', { href: m.url }, icon(menuIcon(m.icon), 18), m.name, m.count ? h('span.cnt', m.count) : null)));
  mediaNav.append(mk('/music', 'music', 'Музыка'));
  restMenu.slice(0, 3).forEach(m => mediaNav.append(h('a', { href: m.url }, icon(menuIcon(m.icon), 18), m.name, m.count ? h('span.cnt', m.count) : null)));
  parts.push(mediaNav);

  // 3. Активность, игры и бонусы
  parts.push(h('nav.nav', h('div.nav-title', 'Игры и бонусы'),
    u ? mk('/quests', 'award', 'Задания и бонусы') : null,
    mk('/games', 'dice', 'Мини-игры'),
    mk('/shop', 'shopping-bag', 'Магазин рамок'),
    u ? mk('/vip', 'crown', 'VIP-подписка') : null));

  // 4. Администрирование (видимо сразу для модераторов/админов)
  if (u && can('adm_panel_show')) {
    parts.push(h('nav.nav', mk('/admin', 'shield', 'Администрирование', 'admin')));
  }

  // 5. Ещё — сворачиваемый блок для дополнительных разделов и инструментов
  const moreNav = h('div.nav-more-list');
  if (u) {
    moreNav.append(
      mk('/user/tape', 'rss', 'События', 'tape'),
      mk('/user/discussions', 'message-circle', 'Обсуждения', 'discussions'),
      mk('/user/myguest', 'eye', 'Гости', 'myguest'),
      mk('/user/bookmark', 'bookmark', 'Закладки'),
      mk('/user/gift', 'gift', 'Подарки'),
      mk('/user/money', 'coins', 'Монеты'),
      mk('/user/settings', 'settings', 'Настройки'));
  }
  restMenu.slice(3).forEach(m => moreNav.append(h('a', { href: m.url }, icon(menuIcon(m.icon), 18), m.name, m.count ? h('span.cnt', m.count) : null)));
  if (!isNativeClient()) moreNav.append(mk('/apps', 'smartphone', 'Приложения'));
  moreNav.append(mk('/developers', 'key', 'Разработчикам (API)'));

  const moreOpen = location.pathname.startsWith('/user/') || location.pathname.startsWith('/developers') || location.pathname.startsWith('/apps');
  const moreDetails = h('details.nav.nav-more', moreOpen ? { open: true } : null,
    h('summary.nav-more-sum', icon('grid', 17), 'Ещё разделы', icon('chevron-down', 16)),
    moreNav);
  parts.push(moreDetails);

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
function syncNativeBadge() {
  if (!window.dcmsNative || typeof window.dcmsNative.setBadge !== 'function') return;
  const c = state.counters || {};
  window.dcmsNative.setBadge((c.mail || 0) + (c.notification || 0));
}
bus.on('user', () => { refreshAll(); syncNativeBadge(); events.restart(); });
bus.on('counters', () => { drawTop(); drawSide(); drawTabbar(); syncNativeBadge(); });
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
function fireSystemPush(title, body, url = '/') {
  if (!('Notification' in window) || Notification.permission !== 'granted' || !document.hidden) return;
  if (navigator.serviceWorker && navigator.serviceWorker.ready) {
    navigator.serviceWorker.ready.then(reg => reg.showNotification(title, { body, data: { url } })).catch(() => {});
  } else {
    try { new Notification(title, { body }); } catch {}
  }
}
bus.on('sse:counters', () => refreshCounters());
bus.on('sse:message', m => {
  if (m && m.from) fireSystemPush(m.from.nick, m.text || 'Новое сообщение', '/mail/' + m.from.id);
});
bus.on('sse:notify', d => {
  refreshCounters();
  const txt = (d.from ? d.from.nick + ': ' : '') + (notifyTitle[d.type] || 'новое уведомление');
  ui.toast('🔔 ' + txt, 'info');
  fireSystemPush(state.config.site_name || 'DCMS Social', txt, '/user/notification');
});

// ── Подтверждение 2FA-входа (работает исключительно в Android-приложении с AndroidKeyStore)
let _mfaOpenId = '';
function showMfaApproveModal(req) {
  if (!req || !req.challenge_id || !state.user || !canAndroidSign2FA()) return;
  if (_mfaOpenId === req.challenge_id) return;
  _mfaOpenId = req.challenge_id;
  let busy = false;
  const errEl = h('div.form-err');
  const sendResp = async (action, code) => {
    if (busy) return;
    busy = true;
    errEl.textContent = '';
    try {
      const devId = window.dcmsNative.getDeviceId();
      const payload = `dcms-2fa-v1:${req.challenge_id}:${action}:${code || ''}:${req.nonce}`;
      const sig = window.dcmsNative.sign2FA(payload);
      await api.post('/api/2fa/app/respond', {
        challenge_id: req.challenge_id,
        device_id: devId,
        action,
        match_code: code || '',
        signature: sig,
      }, { silent: true });
      modal.close();
      ui.ok(action === 'approve' ? 'Вход подтверждён' : 'Вход отклонён');
    } catch (e) {
      errEl.textContent = e.message || 'Ошибка подтверждения';
      if (String(e.message || '').includes('устарел') || String(e.message || '').includes('отклонён')) {
        setTimeout(() => modal.close(), 1200);
      }
    } finally { busy = false; }
  };
  const choices = h('div.mfa-choices', (req.choices || []).map(num =>
    h('button.mfa-choice-btn', { type: 'button', onclick: () => sendResp('approve', String(num)) }, String(num))));
  const body = h('div.stack',
    h('div.card.pad.stack-s', { style: { background: 'var(--surf-2)' } },
      h('div.row', icon('shield', 18), h('b', req.device_label || 'Новое устройство')),
      h('div.small.muted', 'IP-адрес: ', req.ip || '—')),
    h('div.small', 'Выберите число, которое показано на экране входа:'),
    choices,
    errEl,
    h('button.btn.danger', { type: 'button', onclick: () => sendResp('deny', ''), style: { width: '100%' } }, icon('ban', 15), ' Это не я — отклонить'));
  const modal = ui.modal({ title: 'Подтверждение входа', body, onClose: () => { if (_mfaOpenId === req.challenge_id) _mfaOpenId = ''; } });
}
async function checkPendingMfa() {
  if (!state.user || !canAndroidSign2FA()) return;
  try {
    const r = await api.get('/api/2fa/app/pending', null, { silent: true });
    if (r && r.items && r.items.length) showMfaApproveModal(r.items[0]);
  } catch {}
}
bus.on('sse:mfa_request', showMfaApproveModal);
bus.on('user', () => { if (state.user) checkPendingMfa(); });
document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') checkPendingMfa(); });
const notifyTitle = { stena_komm: 'ответил на стене', stena: 'написал на вашей стене', status_komm: 'прокомментировал статус', them_komm: 'ответил в теме', new_gift: 'прислал подарок', ok_frend: 'принял заявку в друзья', no_frend: 'хочет дружить', news_komm: 'ответил к новости', foto_komm: 'прокомментировал фото', obmen_komm: 'прокомментировал файл', notes_komm: 'прокомментировал дневник', guest: 'ответил в гостевой', files_komm: 'прокомментировал файл', shorts_komm: 'прокомментировал клип', shorts_like: 'оценил ваш клип', group_invite: 'приглашает вас в сообщество', group_req: 'подал заявку в ваше сообщество', group_req_ok: 'одобрил вашу заявку в сообщество', group_dm: 'ответили от имени сообщества', mention: 'упомянул вас на стене', chat_mention: 'упомянул вас в беседе', duel: 'вызвал вас на дуэль' };
setInterval(() => { if (state.user && document.visibilityState === 'visible') refreshCounters(); }, 60000);

// ── старт
(async function main() {
  if (isNativeClient()) {
    document.body.classList.add('native-client');
    const plat = nativePlatform();
    if (plat) document.body.classList.add('native-' + plat);
  }
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

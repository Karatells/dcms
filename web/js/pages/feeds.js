import { h, api, ui, router, state, icon, fmt, bus, refreshCounters } from '../core.js';

const verb = (u, m, f) => (u && u.pol === 1 ? m : f);
const cnt = k => state.counters[k] || 0;

const feedTabs = active => ui.tabs([
  { key: 'tape', label: 'Лента', icon: 'rss', href: '/user/tape' }, { key: 'disc', label: 'Обсуждения', icon: 'message-circle', href: '/user/discussions' },
  { key: 'notif', label: 'Уведомления', icon: 'bell', href: '/user/notification' },
].map(t => ({ ...t, label: h('span', t.label, ' ', (t.key === 'tape' ? cnt('tape') : t.key === 'disc' ? cnt('discussions') : cnt('notification')) ? h('span.badge.danger', t.key === 'tape' ? cnt('tape') : t.key === 'disc' ? cnt('discussions') : cnt('notification')) : null) })), active);

const descBlock = d => d ? h('a.feed-desc', { href: d.link }, d.image ? h('img', { src: d.image, alt: '', loading: 'lazy' }) : null,
  h('div', h('div.title', d.title), d.text ? h('div.small.muted', d.text) : null)) : h('div.muted.small', 'Объект удалён');

// ───────── лента
const TAPE_TEXT = {
  status: (a, mine) => `установил${verb(a, '', 'а')} новый статус`,
  status_like: (a, mine) => (mine ? 'считает классным ваш статус' : 'считает классным статус'),
  avatar: (a) => `сменил${verb(a, '', 'а')} аватар`,
  foto: (a) => `добавил${verb(a, '', 'а')} фотографию`,
  album: (a) => `создал${verb(a, '', 'а')} фотоальбом`,
  obmen: (a) => `добавил${verb(a, '', 'а')} файл в обменник`,
  files: (a) => `добавил${verb(a, '', 'а')} файл`,
  them: (a) => `создал${verb(a, '', 'а')} тему на форуме`,
  notes: (a) => `написал${verb(a, '', 'а')} в блоге`,
  frends: (a) => `подружил${verb(a, 'ся', 'ась')} с`,
};
router.add('/user/tape', async ctx => {
  ctx.title('Лента');
  const reload = () => router.render();
  const data = await api.get('/api/tape', { page: ctx.page });
  refreshCounters();
  const rows = data.items.map(t => {
    const who = t.who || t.author;
    return h('article.feed-item' + (t.read ? '' : '.unread'), ui.avatar(who, 42), h('div.main',
      h('div.feed-head', ui.userLink(who), ' ', h('span.muted', (TAPE_TEXT[t.type] || (() => 'событие'))(t.author, t.mine)), ' ',
        t.type === 'status_like' && !t.mine ? ui.userLink(t.author) : null, h('span.grow'), ui.time(t.time)),
      descBlock(t.desc)));
  });
  return [ui.pageHead('Лента', ui.btn('', { ic: 'settings', href: '/user/tape/settings', title: 'Настройки ленты' }),
    ui.dropdown(ui.btn('', { ic: 'more' }), [
      { label: 'Прочитать все', icon: 'check', onclick: async () => { await api.post('/api/tape/read'); reload(); } },
      { label: 'Очистить ленту', icon: 'trash', danger: true, onclick: async () => { if (await ui.confirm('Очистить всю ленту?', { danger: true })) { await api.del('/api/tape'); reload(); } } }])),
    feedTabs('tape'), ui.card(rows.length ? h('div.list', rows) : ui.empty('Нет новых событий', 'rss')), ui.pager(data)];
}, { auth: true });

// ───────── обсуждения
const DISC_TEXT = { them: 'Форум · тема', obmen: 'Обменник · файл', files: 'Обменник · файл', foto: 'Фото', notes: 'Блог', status: 'Статус' };
router.add('/user/discussions', async ctx => {
  ctx.title('Обсуждения');
  const reload = () => router.render();
  const order = ctx.query.order || '';
  const data = await api.get('/api/discussions', { page: ctx.page, order });
  const tab = (k, l, dot) => h('a.chip' + (order === k ? '.active' : ''), { href: k ? '/user/discussions?order=' + k : '/user/discussions' }, l, dot ? h('span.dot-o') : null);
  const rows = data.items.map(d => h('article.feed-item' + (d.count ? '.unread' : ''), ui.avatar(d.author, 42), h('div.main',
    h('div.feed-head', h('span.badge', (d.mine ? 'Ваше · ' : '') + (DISC_TEXT[d.type] || d.type)), ' ', ui.userLink(d.author), h('span.grow'), d.count ? h('span.badge.danger', '+' + d.count) : null, ui.time(d.time)),
    descBlock(d.desc)),
    ui.btn('', { ic: 'x', small: true, title: 'Убрать из обсуждений', onclick: async () => { await api.del('/api/discussions/' + d.id); reload(); } })));
  return [ui.pageHead('Обсуждения', ui.btn('', { ic: 'settings', href: '/user/discussions/settings', title: 'Настройки' }),
    ui.dropdown(ui.btn('', { ic: 'more' }), [
      { label: 'Отметить прочитанными', icon: 'check', onclick: async () => { await api.post('/api/discussions/read'); reload(); } },
      { label: 'Очистить список', icon: 'trash', danger: true, onclick: async () => { if (await ui.confirm('Очистить список обсуждений?', { danger: true })) { await api.del('/api/discussions'); reload(); } } }])),
    feedTabs('disc'), h('div.chips', tab('', 'Все'), tab('my', 'Мои', data.unread_my), tab('frends', 'Друзей', data.unread_frends)),
    ui.card(rows.length ? h('div.list', rows) : ui.empty('Обсуждений пока нет', 'message-circle')), ui.pager(data)];
}, { auth: true });

// ───────── уведомления
const NOTIF_TEXT = {
  money_in: a => `перевел${verb(a, '', 'а')} вам монеты`, ok_gift: a => `принял${verb(a, '', 'а')} ваш подарок`, no_gift: a => `отклонил${verb(a, '', 'а')} ваш подарок`, new_gift: a => `сделал${verb(a, '', 'а')} вам подарок`,
  files_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к файлу`, obmen_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к файлу`,
  news_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к новости`, status_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к статусу`,
  foto_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к фотографии`, notes_komm: a => `ответил${verb(a, '', 'а')} вам в комментариях к блогу`,
  them_komm: a => `ответил${verb(a, '', 'а')} вам в теме форума`, stena_komm: a => `ответил${verb(a, '', 'а')} вам на стене`, stena_komm2: a => `написал${verb(a, '', 'а')} у вас в записи на стене`,
  stena: a => `написал${verb(a, '', 'а')} на вашей стене`, guest: a => `ответил${verb(a, '', 'а')} вам в гостевой`, adm_komm: a => `ответил${verb(a, '', 'а')} вам в админ-чате`,
  del_frend: a => `к сожалению удалил${verb(a, '', 'а')} вас из списка друзей`, no_frend: a => `к сожалению отказал${verb(a, '', 'а')} вам в дружбе`,
  ok_frend: a => `стал${verb(a, '', 'а')} вашим другом`, otm_frend: a => `отменил${verb(a, '', 'а')} свою заявку в друзья`,
  user_rating: a => `оценил${verb(a, '', 'а')} вашу анкету`,
  group_invite: a => `приглашает вас вступить в сообщество`,
  group_req: a => `подал${verb(a, '', 'а')} заявку на вступление в ваше сообщество`,
  group_req_ok: a => `одобрил${verb(a, '', 'а')} вашу заявку на вступление в сообщество`,
  group_dm: () => `ответили вам от имени сообщества`,
};
router.add('/user/notification', async ctx => {
  ctx.title('Уведомления');
  const reload = () => router.render();
  const data = await api.get('/api/notification', { page: ctx.page });
  refreshCounters();
  const rows = data.items.map(n => {
    const inv = n.type === 'group_invite' && n.desc?.extra?.pending ? n.desc.extra : null;
    const invActs = inv ? h('div.row', { style: { marginTop: '8px' } },
      ui.btn('Принять приглашение', {
        ic: 'check', kind: 'primary', small: true,
        onclick: async () => {
          const r = await api.post(`/api/group-invite/${inv.invite_id}/accept`);
          await api.del('/api/notification/' + n.id).catch(() => {});
          ui.ok('Вы вступили в сообщество');
          router.go('/groups/' + r.group_id);
        },
      }),
      ui.btn('Отклонить', {
        ic: 'x', small: true,
        onclick: async () => {
          await api.post(`/api/group-invite/${inv.invite_id}/decline`);
          await api.del('/api/notification/' + n.id).catch(() => {});
          ui.ok('Приглашение отклонено');
          reload();
        },
      })) : null;
    return h('article.feed-item' + (n.read ? '' : '.unread'), ui.avatar(n.author, 42), h('div.main',
      h('div.feed-head', ui.userLink(n.author), ' ', h('span.muted', (NOTIF_TEXT[n.type] || (() => 'уведомление'))(n.author)), h('span.grow'), ui.time(n.time)),
      n.link ? h('a.feed-desc', { href: n.link }, h('div', h('div.title', n.desc?.title || 'Перейти'), n.desc?.text ? h('div.small.muted', n.desc.text) : null)) : null,
      invActs),
      ui.btn('', { ic: 'x', small: true, title: 'Удалить', onclick: async () => { await api.del('/api/notification/' + n.id); reload(); } }));
  });
  return [ui.pageHead('Уведомления', ui.btn('', { ic: 'settings', href: '/user/notification/settings', title: 'Настройки' }),
    data.total ? ui.btn('Очистить', { ic: 'trash', onclick: async () => { if (await ui.confirm('Удалить все уведомления?', { danger: true })) { await api.del('/api/notification'); reload(); } } }) : null),
    feedTabs('notif'), ui.card(rows.length ? h('div.list', rows) : ui.empty('Уведомлений нет', 'bell')), ui.pager(data)];
}, { auth: true });

// ───────── настройки ленты/обсуждений/уведомлений
const TAPE_FLAGS = [['lenta_frends', 'Новые друзья'], ['lenta_notes', 'Новые записи в блогах'], ['lenta_forum', 'Новые темы на форуме'], ['lenta_foto', 'Новые фотографии'], ['lenta_avatar', 'Смена аватара'], ['lenta_files', 'Новые файлы'], ['lenta_obmen', 'Новые файлы'], ['lenta_status', 'Новые статусы'], ['lenta_status_like', 'Оценки статусов']];
const DISC_FLAGS = [['disc_notes', 'Блоги'], ['disc_forum', 'Форум'], ['disc_foto', 'Фотографии'], ['disc_files', 'Файлы'], ['disc_obmen', 'Файлы'], ['disc_status', 'Статусы'], ['disc_frends', 'Друзья']];
function flagsForm(values, defs, save, note) {
  const fields = defs.filter(([k]) => k in values).map(([k, l]) => ({ name: k, label: l, type: 'radio', value: String(values[k]), options: [{ value: '1', label: 'Да' }, { value: '0', label: 'Нет' }] }));
  return ui.form(fields, { submit: 'Сохранить', onSubmit: async v => { const o = {}; for (const [k, x] of Object.entries(v)) o[k] = +x; await save(o); ui.ok('Настройки сохранены'); } });
}
const settingsPage = (path, title, flags, apiBase, hint) => router.add(path, async ctx => {
  ctx.title(title);
  const v = await api.get(apiBase);
  return [ui.pageHead(title), ui.card(h('div.pad.stack', h('p.muted', hint), flagsForm(v, flags, o => api.put(apiBase, o))))];
}, { auth: true });
settingsPage('/user/tape/settings', 'Настройки ленты', TAPE_FLAGS, '/api/tape/settings', 'Какие события друзей показывать в вашей ленте.');
settingsPage('/user/discussions/settings', 'Настройки обсуждений', DISC_FLAGS, '/api/discussions/settings', 'Какие обсуждения друзей попадают в ваш список.');
for (const [kind, flags, title] of [['tape', TAPE_FLAGS, 'Лента: настройки друга'], ['discussions', DISC_FLAGS, 'Обсуждения: настройки друга']]) {
  router.add(`/user/${kind}/friend/:id`, async ctx => {
    ctx.title(title);
    const d = await api.get(`/api/${kind}/settings/friend/${ctx.params.id}`);
    return [ui.pageHead(title), ui.card(h('div.pad.stack', h('div.row', ui.userLink(d.user, { avatar: 32 })), flagsForm(d.settings, flags, o => api.put(`/api/${kind}/settings/friend/${ctx.params.id}`, o))))];
  }, { auth: true });
}
router.add('/user/notification/settings', async ctx => {
  ctx.title('Настройки уведомлений');
  const s = await api.get('/api/notification/settings');
  const form = ui.form([{ name: 'komm', type: 'checkbox', label: 'Уведомлять об ответах на мои комментарии', value: !!s.komm }],
    { onSubmit: async v => { await api.put('/api/notification/settings', { komm: v.komm ? 1 : 0 }); ui.ok('Настройки сохранены'); } });
  return [ui.pageHead('Настройки уведомлений'), ui.card(h('div.pad', form))];
}, { auth: true });

// ───────── почта
const KONT_TABS = [['common', 'Контакты', 'users'], ['favorite', 'Избранные', 'star'], ['ignor', 'Игнор', 'ban'], ['deleted', 'Удалённые', 'trash']];
const mailTabs = active => ui.tabs(
  KONT_TABS.map(([k, l, i]) => ({
    key: k,
    label: k === 'common' && cnt('mail') ? h('span', l, ' ', h('span.badge.danger', cnt('mail'))) : l,
    icon: i,
    href: '/mail?type=' + k,
  })),
  active
);

router.add('/mail', async ctx => {
  ctx.title('Почта');
  const type = ctx.query.type || 'common';
  const reload = () => router.render();
  const [d, gd, gc] = await Promise.all([
    api.get('/api/konts', { page: ctx.page, type }),
    ctx.page === 1 ? api.get('/api/my-group-dialogs', { type }, { silent: true }).catch(() => ({ items: [] })) : Promise.resolve({ items: [] }),
    ctx.page === 1 ? api.get('/api/chats', { type }, { silent: true }).catch(() => ({ items: [] })) : Promise.resolve({ items: [] }),
  ]);
  const selUsers = new Set();
  const selGroups = new Set();
  const selChats = new Set();
  const rowRefs = [];

  const totalSel = () => selUsers.size + selGroups.size + selChats.size;
  const selBar = h('div.card.pad.mail-sel-bar', { style: { display: 'none' } });
  const selCount = h('b');

  const doMove = async targetType => {
    if (!totalSel()) return;
    const reqs = [];
    if (selUsers.size) reqs.push(api.post('/api/konts/move', { ids: [...selUsers], type: targetType }));
    if (selGroups.size) reqs.push(api.post('/api/my-group-dialogs/move', { ids: [...selGroups], type: targetType }));
    if (selChats.size) reqs.push(api.post('/api/chats/move', { ids: [...selChats], type: targetType }));
    await Promise.all(reqs);
    reload();
  };

  const clearSel = () => {
    selUsers.clear();
    selGroups.clear();
    selChats.clear();
    syncSelUI();
  };

  const syncSelUI = () => {
    const cnt = totalSel();
    selBar.style.display = cnt > 0 ? '' : 'none';
    selCount.textContent = `Выбрано: ${cnt}`;
    for (const r of rowRefs) {
      const on = r.kind === 'group' ? selGroups.has(r.id) : r.kind === 'chat' ? selChats.has(r.id) : selUsers.has(r.id);
      r.el.classList.toggle('mail-row-sel', on);
      r.el.classList.toggle('mail-sel-mode', cnt > 0);
    }
  };

  const toggleItem = (kind, id) => {
    const set = kind === 'group' ? selGroups : kind === 'chat' ? selChats : selUsers;
    if (set.has(id)) set.delete(id);
    else set.add(id);
    syncSelUI();
  };

  selBar.append(
    h('div.row',
      ui.btn('', { ic: 'x', small: true, title: 'Отменить выделение', onclick: clearSel }),
      selCount,
      h('span.grow'),
      ...KONT_TABS.filter(([k]) => k !== type).map(([k]) => ui.btn(
        k === 'deleted' ? 'Удалить' : k === 'common' ? 'В контакты' : k === 'favorite' ? 'В избранные' : 'В игнор',
        { small: true, kind: k === 'deleted' ? 'danger' : '', onclick: () => doMove(k) }))));

  const bindSelectable = (el, kind, id, href) => {
    let lpTimer = null;
    let lpFired = false;
    let sx = 0, sy = 0;

    el.addEventListener('touchstart', e => {
      if (e.touches.length !== 1) return;
      lpFired = false;
      sx = e.touches[0].clientX;
      sy = e.touches[0].clientY;
      lpTimer = setTimeout(() => {
        lpFired = true;
        if (navigator.vibrate) try { navigator.vibrate(25); } catch {}
        toggleItem(kind, id);
      }, 420);
    }, { passive: true });

    el.addEventListener('touchmove', e => {
      if (!lpTimer) return;
      const t = e.touches[0];
      if (Math.hypot(t.clientX - sx, t.clientY - sy) > 10) {
        clearTimeout(lpTimer);
        lpTimer = null;
      }
    }, { passive: true });

    const cancelLP = () => { if (lpTimer) { clearTimeout(lpTimer); lpTimer = null; } };
    el.addEventListener('touchend', cancelLP);
    el.addEventListener('touchcancel', cancelLP);

    el.addEventListener('contextmenu', e => {
      e.preventDefault();
      toggleItem(kind, id);
    });

    el.addEventListener('click', e => {
      if (lpFired) {
        e.preventDefault();
        lpFired = false;
        return;
      }
      if (totalSel() > 0) {
        e.preventDefault();
        toggleItem(kind, id);
      } else {
        e.preventDefault();
        router.go(href);
      }
    });
    rowRefs.push({ el, kind, id });
  };

  const initialOf = s => (Array.from((s || 'С').trim())[0] || 'С').toUpperCase();
  const merged = [
    ...d.items.map(k => ({ kind: 'user', time: (k.last && k.last.time) || k.time || 0, data: k })),
    ...(gd.items || []).map(g => ({ kind: 'group', time: g.time || 0, data: g })),
    ...(gc.items || []).map(c => ({ kind: 'chat', time: c.last_time || 0, data: c })),
  ].sort((a, b) => b.time - a.time);

  const rows = merged.map(entry => {
    const checkMark = h('span.mail-sel-chk', icon('check', 13));
    if (entry.kind === 'chat') {
      const c = entry.data;
      const href = '/chats/' + c.id;
      const el = h('a.list-item.mail-chat-row', { href },
        checkMark,
        h('div.grp-ava-sm', { style: { width: '44px', height: '44px', borderRadius: '12px', fontSize: '1.05rem', flexShrink: '0', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#06b6d4)', color: '#fff', fontWeight: '800' } },
          initialOf(c.title)),
        h('div.main',
          h('div.title', c.title,
            h('span.badge', { style: { marginLeft: '6px' } }, `Беседа · ${c.members}`)),
          c.last_text ? h('div.small.muted.clip', c.last_nick ? `${c.last_nick}: ${c.last_text}` : c.last_text) : h('div.small.muted', 'Сообщений пока нет')),
        c.last_time ? ui.time(c.last_time) : null);
      bindSelectable(el, 'chat', c.id, href);
      return el;
    }
    if (entry.kind === 'group') {
      const item = entry.data;
      const g = item.group;
      const href = '/mail/group/' + g.id;
      const el = h('a.list-item.mail-chat-row' + (item.new ? '.unread' : ''), { href },
        checkMark,
        h('div.grp-ava-sm', { style: { width: '44px', height: '44px', borderRadius: '12px', fontSize: '1.05rem', flexShrink: '0', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#ec4899)', color: '#fff', fontWeight: '800', overflow: 'hidden' } },
          g.avatar ? h('img', { src: g.avatar, alt: g.name, style: { width: '100%', height: '100%', objectFit: 'cover' } }) : initialOf(g.name)),
        h('div.main',
          h('div.title', g.name,
            h('span.badge.primary', { style: { marginLeft: '6px' } }, 'Сообщество'),
            item.new ? h('span.badge.danger', { style: { marginLeft: '6px' } }, '+' + item.new) : null),
          item.last ? h('div.small.muted.clip', (item.last.mine ? 'Вы: ' : '') + item.last.text) : h('div.small.muted', 'Сообщений нет')),
        item.last ? ui.time(item.last.time) : null);
      bindSelectable(el, 'group', g.id, href);
      return el;
    }
    const k = entry.data;
    const href = '/mail/' + k.user.id;
    const el = h('a.list-item.mail-chat-row' + (k.new ? '.unread' : ''), { href },
      checkMark,
      ui.avatar(k.user, 44),
      h('div.main',
        h('div.title', k.name || k.user.nick, k.new ? h('span.badge.danger', { style: { marginLeft: '8px' } }, '+' + k.new) : null),
        k.last ? h('div.small.muted.clip', (k.last.mine ? 'Вы: ' : '') + k.last.text) : h('div.small.muted', 'Сообщений нет')),
      k.last ? ui.time(k.last.time) : null,
      ...(type === 'deleted' ? [h('span.small.muted', 'удалится ' + fmt.date(k.expires))] : []));
    bindSelectable(el, 'user', k.user.id, href);
    return el;
  });

  const to = h('input.input', { placeholder: 'Логин получателя' });
  const msg = h('textarea.input', { rows: 3, placeholder: 'Сообщение (2–1024 символа)', maxlength: 1024 });
  const compose = h('details.card.pad', h('summary', icon('edit', 16), ' Написать по логину'), h('div.stack', { style: { marginTop: '12px' } }, to, msg,
    ui.btn('Отправить', { kind: 'primary', ic: 'send', onclick: async () => { const r = await api.post('/api/mail/send', { to: to.value, msg: msg.value }); ui.ok('Сообщение отправлено'); router.go('/mail/' + r.id); } })));
  return [ui.pageHead('Почта', ui.btn('Создать беседу', { ic: 'users', small: true, kind: 'primary', onclick: async () => { const m = await import('./chats.js'); m.openCreateChatModal(); } })), mailTabs(type), selBar, compose,
    ui.card(rows.length ? h('div.list', rows) : ui.empty(type === 'common' ? 'Контактов пока нет' : 'Список пуст', 'mail')),
    rows.length ? h('div.muted.small', 'Удерживайте диалог (или нажмите правой кнопкой), чтобы выбрать, переместить или удалить.') : null,
    type === 'deleted' ? h('div.muted.small', 'Удалённые контакты очищаются через 30 дней вместе с перепиской.') : null, ui.pager(d)];
}, { auth: true });

router.add('/mail/new', async () => { router.go('/mail'); }, { auth: true });

let _smilesCache = null;
export async function getSmiles() {
  if (_smilesCache) return _smilesCache;
  try {
    const list = await api.get('/api/smiles/all', null, { silent: true });
    _smilesCache = list.items || list || [];
  } catch { _smilesCache = []; }
  return _smilesCache;
}

export function dayLabel(ts) {
  const d = new Date(ts * 1000), now = new Date();
  const same = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  if (same(d, now)) return 'Сегодня';
  const y = new Date(now); y.setDate(y.getDate() - 1);
  if (same(d, y)) return 'Вчера';
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: d.getFullYear() !== now.getFullYear() ? 'numeric' : undefined });
}

// ───────── выбор личного вложения (Фото / Файлы / Клипы)
export function openAttachPicker(onPick) {
  const myId = state.user.id;
  let tab = 'foto';
  const bodyWrap = h('div.stack');
  const content = h('div');

  const tabsEl = h('div.tabs');
  const drawTabs = () => {
    tabsEl.replaceChildren(...[['foto', 'Фото', 'camera'], ['files', 'Файлы', 'folder'], ['shorts', 'Клипы', 'video']].map(([k, l, ic]) =>
      h('button.tab' + (tab === k ? '.active' : ''), { type: 'button', onclick: () => { tab = k; drawTabs(); loadTab(); } }, icon(ic, 15), l)));
  };

  let modal;
  const pick = att => { modal && modal.close(); onPick(att); };

  async function loadFoto(albumId = 0) {
    content.replaceChildren(ui.loading());
    if (!albumId) {
      const d = await api.get(`/api/foto/user/${myId}`);
      const albums = d.items || [];
      if (!albums.length) { content.replaceChildren(ui.empty('У вас пока нет фотоальбомов', 'camera')); return; }
      content.replaceChildren(h('div.list', ...albums.map(al => h('button.list-item', {
        type: 'button', style: { width: '100%', background: 'none', border: 0, textAlign: 'left', cursor: 'pointer' },
        onclick: () => loadFoto(al.id),
      }, icon('camera', 20), h('div.main', h('div.title', al.name)), h('span.badge', al.count || 0)))));
    } else {
      const d = await api.get(`/api/foto/album/${albumId}`);
      const photos = d.items || [];
      const back = ui.btn('← К альбомам', { small: true, onclick: () => loadFoto(0) });
      if (!photos.length) { content.replaceChildren(back, ui.empty('В альбоме нет фотографий', 'camera')); return; }
      content.replaceChildren(back, h('div.att-grid', { style: { marginTop: '8px' } }, ...photos.map(p =>
        h('button.att-thumb', {
          type: 'button', title: p.name || 'Фото',
          onclick: () => pick({ type: 'foto', id: p.id, title: p.name || 'Фотография', image: p.thumb || `/api/foto/img/${p.id}.${p.ras || 'jpg'}?s=200` }),
        }, h('img', { src: p.thumb || `/api/foto/img/${p.id}.${p.ras || 'jpg'}?s=200`, alt: p.name || '' }), h('span', p.name || 'Фото')))));
    }
  }

  async function loadFiles(dirId = 0) {
    content.replaceChildren(ui.loading());
    const d = await api.get(`/api/pfiles/${myId}/${dirId}`);
    const head = d.path && d.path.length > 1
      ? ui.btn('← Назад', { small: true, onclick: () => loadFiles(d.path[d.path.length - 2].id) })
      : null;
    const rows = (d.items || []).map(x => x.kind === 'dir'
      ? h('button.list-item', {
        type: 'button', style: { width: '100%', background: 'none', border: 0, textAlign: 'left', cursor: 'pointer' },
        onclick: () => loadFiles(x.id),
      }, icon('folder', 20), h('div.main', h('div.title', x.name)), h('span.badge', x.files))
      : h('button.list-item', {
        type: 'button', style: { width: '100%', background: 'none', border: 0, textAlign: 'left', cursor: 'pointer' },
        onclick: () => pick({ type: 'files', id: x.id, title: `${x.name}.${x.ras}`, image: x.screen ? x.screen_url : '' }),
      }, icon('file', 20), h('div.main', h('div.title', `${x.name}.${x.ras}`), h('div.small.muted', x.size_str))));
    content.replaceChildren(head, rows.length ? h('div.list', { style: { maxHeight: '48vh', overflowY: 'auto' } }, ...rows) : ui.empty('Папка пуста', 'folder'));
  }

  async function loadShorts() {
    content.replaceChildren(ui.loading());
    const d = await api.get('/api/shorts', { user: myId, page: 1 });
    const list = d.items || [];
    if (!list.length) { content.replaceChildren(ui.empty('У вас пока нет клипов', 'video')); return; }
    content.replaceChildren(h('div.att-grid', ...list.map(s =>
      h('button.att-thumb.clip', {
        type: 'button', title: s.title,
        onclick: () => pick({ type: 'shorts', id: s.id, title: s.title, image: s.poster_url }),
      }, h('img', { src: s.poster_url, alt: s.title }), h('span', s.title)))));
  }

  const loadTab = () => {
    if (tab === 'foto') loadFoto(0);
    else if (tab === 'files') loadFiles(0);
    else loadShorts();
  };

  drawTabs();
  bodyWrap.append(tabsEl, content);
  modal = ui.modal({ title: 'Прикрепить из личного', body: bodyWrap, wide: true, actions: [{ label: 'Закрыть' }] });
  loadTab();
}

// ───────── рендер вложения внутри пузыря
export function renderBubbleAttach(att) {
  if (!att) return null;
  if (att.type === 'foto' && att.image) {
    return h('a.bubble-att', { href: att.link || '#' }, h('img.bubble-att-img', { src: att.image, alt: att.title || 'Фото', loading: 'lazy' }));
  }
  if (att.type === 'shorts') {
    return h('a.bubble-att.bubble-att-clip', { href: att.link || ('/shorts/' + att.id) },
      att.image ? h('img', { src: att.image, alt: att.title || 'Клип', loading: 'lazy' }) : null,
      h('span.play-badge', icon('play', 32)),
      h('span.clip-cap', att.title || 'Клип'));
  }
  return h('a.bubble-att.bubble-att-file', { href: att.link || ('/obmen/file/' + att.id) },
    icon('file', 22),
    h('div.main', h('div.title', att.title || 'Файл'), att.text ? h('div.small.muted.clip', att.text) : null));
}

// ───────── плеер голосового сообщения (1x / 1.5x / 2x)
export function renderVoicePlayer(v) {
  if (!v || !v.url) return null;
  const audio = new Audio(v.url);
  audio.preload = 'metadata';
  let rateIdx = 0;
  const rates = [1, 1.5, 2];
  const fmtT = s => { s = Math.max(0, Math.round(s || 0)); return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`; };
  const fill = h('div.voice-fill');
  const bar = h('div.voice-bar', fill);
  const timeEl = h('span', fmtT(v.dur));
  const playBtn = h('button.voice-play', { type: 'button', title: 'Воспроизвести' }, icon('play', 16));
  const spdBtn = h('button.voice-spd', {
    type: 'button', title: 'Скорость воспроизведения',
    onclick: e => {
      e.stopPropagation();
      rateIdx = (rateIdx + 1) % rates.length;
      audio.playbackRate = rates[rateIdx];
      spdBtn.textContent = rates[rateIdx] + 'x';
    },
  }, '1x');

  playBtn.onclick = e => {
    e.stopPropagation();
    if (audio.paused) {
      audio.play().catch(() => {});
      playBtn.replaceChildren(icon('pause2', 15));
    } else {
      audio.pause();
      playBtn.replaceChildren(icon('play', 16));
    }
  };
  audio.ontimeupdate = () => {
    const dur = (audio.duration && isFinite(audio.duration)) ? audio.duration : (v.dur || 1);
    fill.style.width = Math.min(100, (audio.currentTime / dur) * 100) + '%';
    timeEl.textContent = `${fmtT(audio.currentTime)} / ${fmtT(dur)}`;
  };
  audio.onended = () => {
    playBtn.replaceChildren(icon('play', 16));
    fill.style.width = '0%';
    timeEl.textContent = fmtT(v.dur);
  };
  bar.onclick = e => {
    e.stopPropagation();
    const dur = (audio.duration && isFinite(audio.duration)) ? audio.duration : (v.dur || 1);
    const rect = bar.getBoundingClientRect();
    audio.currentTime = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)) * dur;
  };
  return h('div.voice-msg', { onclick: e => e.stopPropagation() },
    playBtn,
    h('div.voice-track', bar, h('div.voice-meta', timeEl, spdBtn)));
}

// ───────── пересылка сообщения в другой диалог
export async function openForwardModal(m, fromUser) {
  const box = h('div.stack', ui.loading());
  const modal = ui.modal({ title: 'Переслать сообщение', body: box, actions: [{ label: 'Закрыть' }] });
  try {
    const d = await api.get('/api/konts', { type: 'common', page: 1 });
    const list = d.items || [];
    if (!list.length) { box.replaceChildren(h('div.small.muted', 'Нет контактов для пересылки')); return; }
    box.replaceChildren(h('div.list', ...list.map(k => {
      const btn = ui.btn('Переслать', {
        ic: 'send', small: true, kind: 'primary',
        onclick: async () => {
          btn.disabled = true;
          try {
            await api.post('/api/mail/' + k.user.id, {
              msg: m.raw || '',
              fwd_from: m.mine ? state.user.id : fromUser.id,
              att_type: m.attach ? m.attach.type : '',
              att_id: m.attach ? m.attach.id : 0,
              sticker: m.sticker || '',
            });
            btn.textContent = 'Отправлено ✓';
            ui.ok('Переслано пользователю ' + k.user.nick);
          } catch { btn.disabled = false; }
        },
      });
      return h('div.list-item', ui.avatar(k.user, 34), h('div.main', h('div.title', k.name || k.user.nick)), btn);
    })));
  } catch { box.replaceChildren(h('div.small.muted', 'Не удалось загрузить контакты')); }
}

// ───────── контекстное меню сообщения (с быстрой панелью реакций)
const MSG_EMOJIS = ['👍', '❤️', '😂', '🔥', '😱', '👎'];
export function openMsgMenu(m, { onReact, onReply, onEdit, onForward, onPin, onDelete, onSpam }) {
  const close = () => back.remove();
  const item = (ic, label, fn, danger = false) => h('button.msg-ctx-item' + (danger ? '.danger' : ''), {
    type: 'button',
    onclick: () => { close(); fn(); },
  }, icon(ic, 18), label);
  const reactBar = onReact ? h('div.msg-react-bar', ...MSG_EMOJIS.map(em => h('button.msg-react-btn', {
    type: 'button',
    onclick: () => { close(); onReact(em); },
  }, em))) : null;
  const menu = h('div.msg-ctx',
    reactBar,
    item('corner-up-left', 'Ответить', onReply),
    onEdit && m.mine && m.raw && !m.voice && !m.sticker ? item('edit', 'Редактировать', onEdit) : null,
    onForward && !m.voice ? item('share', 'Переслать', onForward) : null,
    item('pin', m.pinned ? 'Открепить' : 'Закрепить', onPin),
    m.raw ? item('copy', 'Копировать текст', () => navigator.clipboard?.writeText(m.raw).then(() => ui.ok('Скопировано'))) : null,
    !m.mine ? item('flag', 'Пожаловаться', onSpam) : null,
    item('trash', 'Удалить у себя', onDelete, true));
  const back = h('div.msg-ctx-back', { onclick: e => { if (e.target === back) close(); } }, menu);
  document.body.append(back);
}

router.add('/mail/:id', async ctx => {
  const uid = +ctx.params.id;
  const d = await api.get('/api/mail/' + uid, { page: 1 });
  refreshCounters();
  ctx.title('Диалог: ' + d.user.nick);
  document.body.classList.add('msg-page');

  let curPage = 1;
  const totalPages = d.pages || 1;
  let items = [...(d.items || [])].reverse();
  const seenIds = new Set(items.map(x => x.id));
  let pinnedMsg = d.pinned || null;
  let replyTo = null; // { id, nick, text }
  let editMsg = null; // { id, raw }
  let attach = null;  // { type, id, title, image }
  let inputTa = null;

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
  const searchHost = h('div');
  const chipHost = h('div');
  const reload = () => router.render();

  const jumpToMsg = id => {
    const el = scrollEl.querySelector(`.bubble[data-mid="${id}"]`);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('flash');
    setTimeout(() => el.classList.remove('flash'), 1200);
  };

  const renderPinned = () => {
    if (!pinnedMsg) { pinnedHost.replaceChildren(); return; }
    pinnedHost.replaceChildren(h('div.msg-pinned', { onclick: () => jumpToMsg(pinnedMsg.id) },
      icon('pin', 16),
      h('div.msg-pinned-body',
        h('div.msg-pinned-title', `Закреплено · ${pinnedMsg.nick}`),
        h('div.msg-pinned-text', pinnedMsg.text || 'Сообщение')),
      h('button.icon-btn', {
        type: 'button', title: 'Открепить',
        onclick: async e => { e.stopPropagation(); await api.post(`/api/mail/msg/${pinnedMsg.id}/pin`); pinnedMsg = null; items.forEach(x => { x.pinned = false; }); renderPinned(); },
      }, icon('x', 16))));
  };

  let syncTa = () => {};
  const renderChips = () => {
    const nodes = [];
    if (editMsg) {
      nodes.push(h('div.msg-bar-chip',
        icon('edit', 16),
        h('div.chip-main', h('div.chip-title', 'Редактирование сообщения'), h('div.chip-text', editMsg.raw)),
        h('button.icon-btn', { type: 'button', onclick: () => { editMsg = null; if (inputTa) inputTa.value = ''; renderChips(); syncTa(); } }, icon('x', 16))));
    }
    if (replyTo) {
      nodes.push(h('div.msg-bar-chip',
        icon('corner-up-left', 16),
        h('div.chip-main', h('div.chip-title', `Ответ · ${replyTo.nick}`), h('div.chip-text', replyTo.text)),
        h('button.icon-btn', { type: 'button', onclick: () => { replyTo = null; renderChips(); } }, icon('x', 16))));
    }
    if (attach) {
      const lbl = { foto: 'Фотография', files: 'Личный файл', shorts: 'Клип' }[attach.type] || 'Вложение';
      nodes.push(h('div.msg-bar-chip',
        attach.image ? h('img', { src: attach.image, alt: '' }) : icon('paperclip', 16),
        h('div.chip-main', h('div.chip-title', lbl), h('div.chip-text', attach.title)),
        h('button.icon-btn', { type: 'button', onclick: () => { attach = null; renderChips(); syncTa(); } }, icon('x', 16))));
    }
    chipHost.replaceChildren(...nodes);
  };

  const renderMessages = (keepBottom = true) => {
    const prevH = scrollEl.scrollHeight;
    const prevTop = scrollEl.scrollTop;
    const nodes = [];
    if (curPage < totalPages) {
      nodes.push(ui.btn('Загрузить предыдущие сообщения', {
        small: true, cls: 'msg-older',
        onclick: async () => {
          const next = await api.get('/api/mail/' + uid, { page: curPage + 1 });
          curPage++;
          const older = (next.items || []).reverse();
          older.forEach(x => seenIds.add(x.id));
          items = [...older, ...items];
          renderMessages(false);
        },
      }));
    }
    if (!items.length) {
      nodes.push(ui.empty('Сообщений пока нет — напишите первым!', 'mail'));
    } else {
      let lastDay = '';
      items.forEach(m => {
        const dl = dayLabel(m.time);
        if (dl !== lastDay) { lastDay = dl; nodes.push(h('div.msg-day', dl)); }
        const doReact = async em => {
          const r = await api.post(`/api/mail/msg/${m.id}/react`, { emoji: em }, { silent: true });
          m.reactions = r.reactions || [];
          renderMessages(false);
        };
        const openMenu = e => {
          if (e.target.closest('a, .bubble-reply, .bubble-react-pill, .voice-msg')) return;
          e.preventDefault();
          openMsgMenu(m, {
            onReact: doReact,
            onReply: () => { editMsg = null; replyTo = { id: m.id, nick: m.mine ? state.user.nick : d.user.nick, text: (m.raw || '').slice(0, 80) || (m.voice ? '🎤 Голосовое' : m.sticker ? m.sticker + ' Стикер' : (m.attach ? m.attach.title : 'Сообщение')) }; renderChips(); inputTa && inputTa.focus(); },
            onEdit: () => { replyTo = null; attach = null; editMsg = { id: m.id, raw: m.raw }; renderChips(); if (inputTa) { inputTa.value = m.raw; syncTa(); inputTa.focus(); } },
            onForward: () => openForwardModal(m, d.user),
            onPin: async () => {
              const r = await api.post(`/api/mail/msg/${m.id}/pin`);
              items.forEach(x => { x.pinned = false; });
              m.pinned = !!r.pinned;
              pinnedMsg = m.pinned ? { id: m.id, nick: m.mine ? state.user.nick : d.user.nick, text: (m.raw || '').slice(0, 80) || (m.attach ? m.attach.title : 'Сообщение') } : null;
              renderPinned();
              renderMessages(false);
            },
            onSpam: async () => {
              const t = await ui.prompt('Причина жалобы', '', { multiline: true, title: 'Жалоба' });
              if (t) { await api.post(`/api/mail/msg/${m.id}/spam`, { msg: t, types: 0 }); ui.ok('Жалоба отправлена'); }
            },
            onDelete: async () => {
              await api.del('/api/mail/msg/' + m.id);
              items = items.filter(x => x.id !== m.id);
              if (pinnedMsg && pinnedMsg.id === m.id) { pinnedMsg = null; renderPinned(); }
              renderMessages(false);
            },
          });
        };
        const reactPills = m.reactions && m.reactions.length
          ? h('div.bubble-reacts', ...m.reactions.map(rx => h('button.bubble-react-pill' + (rx.mine ? '.mine' : ''), {
            type: 'button',
            onclick: e => { e.stopPropagation(); doReact(rx.emoji); },
          }, rx.emoji, rx.count > 1 ? h('span', String(rx.count)) : null)))
          : null;
        const bubble = h('div.bubble' + (m.mine ? '.mine' : '') + (m.sticker ? '.is-sticker' : ''), {
          dataset: { mid: String(m.id) },
          onclick: openMenu,
          oncontextmenu: openMenu,
        },
          m.fwd ? h('div.bubble-fwd', icon('share', 12), 'Переслано от ', m.fwd.nick) : null,
          m.reply ? h('div.bubble-reply', { onclick: e => { e.stopPropagation(); jumpToMsg(m.reply.id); } }, h('b', m.reply.nick), h('span', m.reply.text)) : null,
          m.sticker ? h('div.bubble-sticker', m.sticker) : null,
          m.voice ? renderVoicePlayer(m.voice) : null,
          renderBubbleAttach(m.attach),
          m.msg ? ui.html(m.msg, 'text') : null,
          h('div.bubble-meta',
            m.pinned ? icon('pin', 11) : null,
            m.edited_at ? h('span', 'изм. ') : null,
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })),
            m.mine ? icon(m.read ? 'check-all' : 'check', 14) : null));
        // Свайп вправо для быстрого ответа
        let swX = 0, swY = 0, swMoved = 0;
        bubble.addEventListener('touchstart', e => {
          if (e.touches.length !== 1) return;
          swX = e.touches[0].clientX; swY = e.touches[0].clientY; swMoved = 0;
        }, { passive: true });
        bubble.addEventListener('touchmove', e => {
          if (!swX) return;
          const dx = e.touches[0].clientX - swX, dy = Math.abs(e.touches[0].clientY - swY);
          if (dx > 12 && dx < 72 && dy < 24) {
            swMoved = dx;
            bubble.style.transform = `translateX(${dx}px)`;
          }
        }, { passive: true });
        bubble.addEventListener('touchend', () => {
          bubble.style.transform = '';
          if (swMoved > 46) {
            if (navigator.vibrate) try { navigator.vibrate(15); } catch {}
            editMsg = null;
            replyTo = { id: m.id, nick: m.mine ? state.user.nick : d.user.nick, text: (m.raw || '').slice(0, 80) || (m.voice ? '🎤 Голосовое' : m.sticker ? m.sticker + ' Стикер' : (m.attach ? m.attach.title : 'Сообщение')) };
            renderChips();
            inputTa && inputTa.focus();
          }
          swX = 0; swMoved = 0;
        });
        const isNew = !seenIds.has(m.id);
        if (isNew) seenIds.add(m.id);
        nodes.push(h('div.bubble-row' + (m.mine ? '.mine' : '') + (isNew ? '.new-msg' : ''),
          reactPills ? h('div.msg-bubble-col', bubble, reactPills) : bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) {
      requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
    } else {
      requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight - prevH + prevTop; });
    }
  };

  // Шапка диалога (с живым статусом «печатает…»)
  const statusEl = h('div.msg-status' + (d.user.online ? '.on' : ''), d.user.online ? 'в сети' : 'не в сети');
  const setStatusNormal = () => {
    statusEl.className = 'msg-status' + (d.user.online ? ' on' : '');
    statusEl.textContent = d.user.online ? 'в сети' : 'не в сети';
  };
  let searchOpen = false;
  const toggleSearch = () => {
    searchOpen = !searchOpen;
    if (!searchOpen) { searchHost.replaceChildren(); return; }
    const sIn = h('input.input', { placeholder: 'Поиск по переписке…' });
    const sRes = h('div.msg-search-res');
    let st = null;
    sIn.oninput = () => {
      clearTimeout(st);
      const q = sIn.value.trim();
      if (!q) { sRes.replaceChildren(); return; }
      st = setTimeout(async () => {
        const r = await api.get(`/api/mail/${uid}/search`, { q }, { silent: true });
        const hits = r.items || [];
        if (!hits.length) { sRes.replaceChildren(h('div.pad.small.muted', 'Ничего не найдено')); return; }
        sRes.replaceChildren(h('div.list', ...hits.map(hm => h('button.list-item', {
          type: 'button', style: { width: '100%', background: 'none', border: 0, textAlign: 'left', cursor: 'pointer' },
          onclick: () => { jumpToMsg(hm.id); toggleSearch(); },
        }, h('div.main', h('div.small.muted', hm.mine ? 'Вы' : d.user.nick, ' · ', fmt.datetime(hm.time)), h('div.clip', hm.raw || 'Сообщение'))))));
      }, 250);
    };
    searchHost.replaceChildren(
      h('div.msg-search-bar', icon('search', 16), sIn, h('button.icon-btn', { type: 'button', onclick: toggleSearch }, icon('x', 18))),
      sRes);
    sIn.focus();
  };

  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/mail', title: 'Назад к почте', 'aria-label': 'Назад' }, icon('arrow-left', 20)),
    ui.avatar(d.user, 38),
    h('div.msg-head-info', ui.userLink(d.user, { badges: true }), statusEl),
    h('button.icon-btn', { type: 'button', title: 'Поиск по переписке', onclick: toggleSearch }, icon('search', 19)),
    !d.contact ? ui.btn('', { ic: 'user-plus', small: true, title: 'В контакты', onclick: async () => { await api.post('/api/konts/' + d.user.id); ui.ok('Добавлено в контакты'); reload(); } }) : null,
    ui.dropdown(h('button.icon-btn', { title: 'Меню' }, icon('more', 20)), [
      { label: 'Профиль', icon: 'user', href: '/id' + d.user.id },
      { label: 'В избранные', icon: 'star', onclick: async () => { await api.post('/api/konts/move', { ids: [d.user.id], type: 'favorite' }); ui.ok('Готово'); } },
      { label: 'В игнор', icon: 'ban', onclick: async () => { await api.post('/api/konts/move', { ids: [d.user.id], type: 'ignor' }); ui.ok('Контакт в игнор-листе'); } },
      { sep: true },
      { label: 'Очистить переписку', icon: 'trash', danger: true, onclick: async () => { if (await ui.confirm('Удалить переписку у себя?', { danger: true })) { await api.del('/api/mail/' + d.user.id); reload(); } } },
    ]));

  // Нижняя компактная панель ввода с прикреплением, BB-тулбаром и смайлами
  const foot = h('div.msg-foot');
  if (d.can_write) {
    const ta = h('textarea.msg-input', { rows: 1, placeholder: 'Сообщение…', maxlength: 1024 });
    inputTa = ta;
    const charEl = h('span.msg-char');
    const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить', 'aria-label': 'Отправить', disabled: true }, icon('send', 18));
    const micBtn = h('button.msg-tool', { type: 'button', title: 'Записать голосовое сообщение', 'aria-label': 'Голосовое' }, icon('mic', 19));
    const recHost = h('div');
    const drawerHost = h('div');
    let activeDrawer = '';

    syncTa = () => {
      ta.style.height = 'auto';
      const sh = ta.scrollHeight;
      ta.style.height = Math.min(120, Math.max(36, sh)) + 'px';
      ta.style.overflowY = sh > 120 ? 'auto' : 'hidden';
      const n = ta.value.trim().length;
      sendBtn.disabled = n < 1 && !attach;
      if (ta.value.length > 850) {
        charEl.textContent = `${ta.value.length}/1024`;
        charEl.classList.toggle('warn', ta.value.length > 980);
      } else {
        charEl.textContent = '';
      }
    };

    const wrap = (a, b = '') => {
      const s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
      ta.value = v.slice(0, s) + a + v.slice(s, e) + b + v.slice(e);
      ta.focus();
      ta.selectionStart = s + a.length;
      ta.selectionEnd = e + a.length;
      syncTa();
    };

    const plusBtn = h('button.msg-tool', { type: 'button', title: 'Вложения, форматирование и смайлы', 'aria-label': 'Меню ввода' }, icon('plus', 20));
    let subTab = 'bb';

    const drawToolsDrawer = async () => {
      if (activeDrawer !== 'tools') { drawerHost.replaceChildren(); return; }
      const body = h('div');
      const tabsRow = h('div.msg-tools-tabs',
        h('button.msg-tools-tab', { type: 'button', onclick: () => { activeDrawer = ''; plusBtn.classList.remove('active'); drawerHost.replaceChildren(); openAttachPicker(a => { attach = a; renderChips(); syncTa(); }); } }, icon('paperclip', 15), 'Прикрепить'),
        h('button.msg-tools-tab' + (subTab === 'stickers' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'stickers'; drawToolsDrawer(); } }, icon('star', 15), 'Стикеры'),
        h('button.msg-tools-tab' + (subTab === 'bb' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'bb'; drawToolsDrawer(); } }, h('b', 'Aa'), 'Формат'),
        h('button.msg-tools-tab' + (subTab === 'smiles' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'smiles'; drawToolsDrawer(); } }, icon('smile', 15), 'Смайлы'));
      drawerHost.replaceChildren(h('div.msg-drawer', tabsRow, body));

      if (subTab === 'stickers') {
        body.replaceChildren(h('div.pad.small.muted', 'Загрузка стикеров…'));
        const stData = await api.get('/api/stickers', null, { silent: true });
        const packs = stData.packs || [];
        let curPack = packs[0]?.id || '';
        const pTabs = h('div.sticker-tabs');
        const pGrid = h('div.sticker-grid');
        const drawPack = () => {
          pTabs.replaceChildren(...packs.map(pk => h('button.btn.sm' + (pk.id === curPack ? '.primary' : ''), {
            type: 'button', onclick: () => { curPack = pk.id; drawPack(); },
          }, pk.vip ? '👑 ' + pk.title : pk.title)));
          const pk = packs.find(x => x.id === curPack) || packs[0];
          const locked = pk && pk.vip && !stData.is_vip;
          pGrid.replaceChildren(...((pk && pk.stickers) || []).map(st => h('button.sticker-btn' + (locked ? '.locked' : ''), {
            type: 'button',
            title: locked ? 'Доступно только для VIP' : 'Отправить стикер',
            onclick: async () => {
              if (locked) { ui.toast('Этот набор стикеров доступен только для VIP-подписчиков', 'warn'); return; }
              await api.post('/api/mail/' + d.user.id, { sticker: st, reply_id: replyTo ? replyTo.id : 0 });
              replyTo = null; renderChips();
              activeDrawer = ''; plusBtn.classList.remove('active'); drawerHost.replaceChildren();
              const fresh = await api.get('/api/mail/' + uid, { page: 1 }, { silent: true });
              curPage = 1; items = [...(fresh.items || [])].reverse(); renderMessages(true);
            },
          }, st)));
        };
        drawPack();
        body.replaceChildren(pTabs, pGrid);
        return;
      }

      if (subTab === 'bb') {
        const bb = state.config.bb || {};
        const ribbon = h('div.msg-bb-ribbon');
        const addBtn = (lbl, title, fn, key) => {
          if (key && bb[key] === false) return;
          ribbon.append(h('button.msg-bb-btn', { type: 'button', title, onclick: fn }, lbl));
        };
        addBtn(h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'), 'b');
        addBtn(h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'), 'i');
        addBtn(h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'), 'u');
        addBtn(h('del', 'S'), 'Зачёркнутый', () => wrap('[s]', '[/s]'), 's');
        addBtn(icon('eye-off', 14), 'Спойлер', () => wrap('[spoiler]', '[/spoiler]'), 'spoiler');
        addBtn(h('span.bb-rainbow', 'A'), 'Градиент', () => wrap('[rainbow]', '[/rainbow]'), 'rainbow');
        addBtn(h('code', { style: { fontSize: '.74rem' } }, '<>'), 'Моно', () => wrap('[mono]', '[/mono]'), 'mono');
        addBtn(icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'), 'quote');
        addBtn(icon('hash', 14), 'Код', () => wrap('[code]', '[/code]'), 'code');
        addBtn(icon('link', 14), 'Вставить ссылку', async () => {
          const u = await ui.prompt('Адрес ссылки (https://...)', 'https://', { title: 'Вставить ссылку' });
          if (u && u.trim()) {
            const sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
            wrap(`[url=${u.trim()}]`, sel ? '[/url]' : `${u.trim()}[/url]`);
          }
        }, 'url');
        addBtn(icon('image', 14), 'Вставить картинку', async () => {
          const u = await ui.prompt('Ссылка на картинку (https://...)', 'https://', { title: 'Вставить картинку' });
          if (u && u.trim()) wrap(`[img]${u.trim()}[/img]`);
        }, 'img');
        ribbon.append(h('span.msg-bb-sep'));
        [['red', '#e11d48', 'Красный'], ['yellow', '#eab308', 'Жёлтый'], ['green', '#16a34a', 'Зелёный'], ['blue', '#2563eb', 'Синий']].forEach(([tag, col, title]) => {
          if (bb[tag] === false) return;
          ribbon.append(h('button.msg-color-dot', { type: 'button', title, style: { background: col }, onclick: () => wrap(`[${tag}]`, `[/${tag}]`) }));
        });
        if (bb.color !== false) {
          const hexT = h('input', { type: 'color', value: '#ff4081', onchange: () => wrap(`[color=${hexT.value}]`, '[/color]') });
          ribbon.append(h('label.bb-color-pick', { title: 'Любой HEX-цвет текста [color=#...]' }, h('span', { style: { color: '#ff4081' } }, 'A'), hexT));
        }
        if (bb.bg !== false) {
          const hexB = h('input', { type: 'color', value: '#fef08a', onchange: () => wrap(`[bg=${hexB.value}]`, '[/bg]') });
          ribbon.append(h('label.bb-color-pick', { title: 'Любой HEX-цвет фона [bg=#...]' }, h('span', { style: { background: '#fef08a', color: '#111', padding: '0 3px', borderRadius: '3px' } }, 'bg'), hexB));
        }
        body.replaceChildren(ribbon);
      } else {
        const box = h('div.msg-smiles-drawer', ui.loading());
        body.replaceChildren(box);
        const list = await getSmiles();
        if (!list.length) box.replaceChildren(h('span.small.muted', 'Смайлов нет'));
        else box.replaceChildren(...list.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))));
      }
    };

    plusBtn.onclick = () => {
      activeDrawer = activeDrawer === 'tools' ? '' : 'tools';
      plusBtn.classList.toggle('active', activeDrawer === 'tools');
      drawToolsDrawer();
    };

    const doSend = async () => {
      const msg = ta.value.trim();
      if ((msg.length < 1 && !attach) || sendBtn.disabled) return;
      sendBtn.disabled = true;
      try {
        if (editMsg) {
          const upd = await api.put('/api/mail/msg/' + editMsg.id, { msg });
          const idx = items.findIndex(x => x.id === editMsg.id);
          if (idx >= 0) items[idx] = upd;
          editMsg = null;
          ta.value = '';
          renderChips();
          syncTa();
          renderMessages(false);
          return;
        }
        await api.post('/api/mail/' + d.user.id, {
          msg,
          reply_id: replyTo ? replyTo.id : 0,
          att_type: attach ? attach.type : '',
          att_id: attach ? attach.id : 0,
        });
        ta.value = '';
        replyTo = null;
        attach = null;
        renderChips();
        syncTa();
        const fresh = await api.get('/api/mail/' + uid, { page: 1 }, { silent: true });
        curPage = 1;
        items = [...(fresh.items || [])].reverse();
        pinnedMsg = fresh.pinned || null;
        renderPinned();
        renderMessages(true);
      } finally { syncTa(); }
    };

    // Запись голосового сообщения
    let recorder = null, recChunks = [], recTimer = null, recSec = 0, recStream = null;
    const stopRec = sendVoice => {
      clearInterval(recTimer);
      recHost.replaceChildren();
      if (recorder && recorder.state !== 'inactive') {
        recorder.onstop = async () => {
          recStream && recStream.getTracks().forEach(t => t.stop());
          if (!sendVoice || !recChunks.length) return;
          const mime = recorder.mimeType || 'audio/webm';
          const ext = mime.includes('ogg') ? 'ogg' : mime.includes('mp4') ? 'm4a' : 'webm';
          const blob = new Blob(recChunks, { type: mime });
          const fd = new FormData();
          fd.append('file', blob, 'voice.' + ext);
          fd.append('dur', String(Math.max(1, recSec)));
          try {
            const res = await fetch(`/api/mail/${d.user.id}/voice`, { method: 'POST', body: fd, headers: { 'X-DCMS': '1' } });
            if (!res.ok) throw new Error('Ошибка отправки');
            const fresh = await api.get('/api/mail/' + uid, { page: 1 }, { silent: true });
            curPage = 1; items = [...(fresh.items || [])].reverse(); renderMessages(true);
          } catch { ui.toast('Не удалось отправить голосовое', 'err'); }
        };
        recorder.stop();
      } else if (recStream) {
        recStream.getTracks().forEach(t => t.stop());
      }
      recorder = null;
    };
    micBtn.onclick = async () => {
      if (recorder) { stopRec(true); return; }
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof MediaRecorder === 'undefined') {
        ui.toast('Запись голоса не поддерживается вашим браузером', 'warn');
        return;
      }
      try {
        recStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        recChunks = []; recSec = 0;
        recorder = new MediaRecorder(recStream);
        recorder.ondataavailable = e => { if (e.data && e.data.size) recChunks.push(e.data); };
        recorder.start();
        const tEl = h('span', '0:00');
        recTimer = setInterval(() => { recSec++; tEl.textContent = `${Math.floor(recSec / 60)}:${String(recSec % 60).padStart(2, '0')}`; }, 1000);
        recHost.replaceChildren(h('div.msg-rec-bar',
          h('span.msg-rec-dot'), h('span', 'Запись… '), tEl, h('span.grow'),
          ui.btn('Отмена', { small: true, onclick: () => stopRec(false) }),
          ui.btn('Отправить', { ic: 'send', small: true, kind: 'primary', onclick: () => stopRec(true) })));
      } catch { ui.toast('Нет доступа к микрофону', 'err'); }
    };

    let lastTypingSent = 0;
    sendBtn.onclick = doSend;
    ta.oninput = () => {
      syncTa();
      const now = Date.now();
      if (ta.value.trim() && now - lastTypingSent > 2500) {
        lastTypingSent = now;
        api.post(`/api/mail/${d.user.id}/typing`, {}, { silent: true }).catch(() => {});
      }
    };
    ta.onkeydown = e => {
      if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) {
        e.preventDefault();
        doSend();
      }
    };

    foot.append(chipHost, recHost, drawerHost, h('div.msg-compose', plusBtn, h('div.msg-input-wrap', ta, charEl), micBtn, sendBtn));
  } else {
    foot.append(h('div.pad.small.muted.center', 'Вы не можете писать этому пользователю.'));
  }

  let typingTimer = null;
  const onTyping = bus.on('sse:typing', ev => {
    if (ev && ev.from === d.user.id) {
      statusEl.className = 'msg-status typing';
      statusEl.textContent = 'печатает…';
      clearTimeout(typingTimer);
      typingTimer = setTimeout(setStatusNormal, 3500);
    }
  });
  const onRead = bus.on('sse:mail_read', ev => {
    if (ev && ev.by === d.user.id) {
      let changed = false;
      items.forEach(x => { if (x.mine && !x.read) { x.read = true; changed = true; } });
      if (changed) renderMessages(false);
    }
  });
  const onReact = bus.on('sse:mail_react', ev => {
    if (!ev) return;
    const m = items.find(x => x.id === ev.id);
    if (m) { m.reactions = ev.reactions || []; renderMessages(false); }
  });
  const onEdit = bus.on('sse:mail_edit', ev => {
    if (!ev) return;
    const m = items.find(x => x.id === ev.id);
    if (m) { m.msg = ev.msg; m.raw = ev.raw; m.edited_at = ev.edited_at; renderMessages(false); }
  });
  const onMsg = bus.on('sse:message', async m => {
    if (m.from && m.from.id === d.user.id) {
      clearTimeout(typingTimer);
      setStatusNormal();
      const fresh = await api.get('/api/mail/' + uid, { page: 1 }, { silent: true });
      curPage = 1;
      items = [...(fresh.items || [])].reverse();
      pinnedMsg = fresh.pinned || null;
      renderPinned();
      renderMessages(true);
      refreshCounters();
    }
  });
  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    clearTimeout(typingTimer);
    onMsg && onMsg();
    onTyping && onTyping();
    onRead && onRead();
    onReact && onReact();
    onEdit && onEdit();
  });

  renderPinned();
  renderMessages(true);
  return h('div.msg-shell', head, searchHost, pinnedHost, scrollEl, foot);
}, { auth: true });

// ───────── УНИВЕРСАЛЬНЫЙ ПОЛНОФУНКЦИОНАЛЬНЫЙ МЕССЕНДЖЕР ДЛЯ ПЕРЕПИСОК С СООБЩЕСТВАМИ
function renderRichGroupDialog({ ctx, head, initialItems, initialPinned, initialKeyboard, botCommands, canWrite, emptyText, disabledText, placeholder, isMine, senderLabel, showStaff, canDeleteMsg, onSend, onPin, onDelete, onInlineClick, onKeyboardClick, onMarkRead, onSubscribe }) {
  document.body.classList.add('msg-page');
  let items = [...(initialItems || [])];
  const seenIds = new Set(items.map(x => x.id));
  let pinnedMsg = initialPinned || null;
  let replyKB = (initialKeyboard && initialKeyboard.length) ? initialKeyboard : null;
  let kbVisible = !!replyKB;
  const cmds = Array.isArray(botCommands) ? botCommands : [];
  let replyTo = null;
  let attach = null;

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
  const chipHost = h('div');
  const kbHost = h('div');

  const jumpToMsg = mid => {
    const el = scrollEl.querySelector(`.bubble[data-mid="${mid}"]`);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('flash');
    setTimeout(() => el.classList.remove('flash'), 1200);
  };

  const renderPinned = () => {
    if (!pinnedMsg) { pinnedHost.replaceChildren(); return; }
    pinnedHost.replaceChildren(h('div.msg-pinned', { onclick: () => jumpToMsg(pinnedMsg.id) },
      icon('pin', 16),
      h('div.msg-pinned-body',
        h('div.msg-pinned-title', `Закреплено · ${pinnedMsg.nick}`),
        h('div.msg-pinned-text', pinnedMsg.text || 'Сообщение')),
      h('button.icon-btn', {
        type: 'button', title: 'Открепить',
        onclick: async e => { e.stopPropagation(); await onPin(pinnedMsg); pinnedMsg = null; items.forEach(x => { x.pinned = false; }); renderPinned(); renderMessages(false); },
      }, icon('x', 16))));
  };

  let syncTa = () => {};
  const renderChips = () => {
    const nodes = [];
    if (replyTo) {
      nodes.push(h('div.msg-bar-chip',
        icon('corner-up-left', 16),
        h('div.chip-main', h('div.chip-title', `Ответ · ${replyTo.nick}`), h('div.chip-text', replyTo.text)),
        h('button.icon-btn', { type: 'button', onclick: () => { replyTo = null; renderChips(); } }, icon('x', 16))));
    }
    if (attach) {
      const lbl = { foto: 'Фотография', files: 'Личный файл', shorts: 'Клип' }[attach.type] || 'Вложение';
      nodes.push(h('div.msg-bar-chip',
        attach.image ? h('img', { src: attach.image, alt: '' }) : icon('paperclip', 16),
        h('div.chip-main', h('div.chip-title', lbl), h('div.chip-text', attach.title)),
        h('button.icon-btn', { type: 'button', onclick: () => { attach = null; renderChips(); syncTa(); } }, icon('x', 16))));
    }
    chipHost.replaceChildren(...nodes);
  };

  const renderMessages = (keepBottom = true) => {
    const prevH = scrollEl.scrollHeight;
    const prevTop = scrollEl.scrollTop;
    const nodes = [];
    if (!items.length) {
      nodes.push(ui.empty(emptyText, 'mail'));
    } else {
      let lastDay = '';
      items.forEach(m => {
        const mine = isMine(m);
        const dl = dayLabel(m.time);
        if (dl !== lastDay) { lastDay = dl; nodes.push(h('div.msg-day', dl)); }
        const openMenu = e => {
          if (e.target.closest('a, .bubble-reply')) return;
          e.preventDefault();
          const close = () => back.remove();
          const itemBtn = (ic, label, fn, danger = false) => h('button.msg-ctx-item' + (danger ? '.danger' : ''), {
            type: 'button', onclick: () => { close(); fn(); },
          }, icon(ic, 18), label);
          const menu = h('div.msg-ctx',
            itemBtn('corner-up-left', 'Ответить', () => {
              replyTo = { id: m.id, nick: senderLabel(m), text: (m.raw || '').slice(0, 80) || (m.attach ? m.attach.title : 'Сообщение') };
              renderChips();
            }),
            itemBtn('pin', m.pinned ? 'Открепить' : 'Закрепить', async () => {
              const r = await onPin(m);
              items.forEach(x => { x.pinned = false; });
              m.pinned = !!r.pinned;
              pinnedMsg = m.pinned ? { id: m.id, nick: senderLabel(m), text: (m.raw || '').slice(0, 80) || (m.attach ? m.attach.title : 'Сообщение') } : null;
              renderPinned();
              renderMessages(false);
            }),
            m.raw ? itemBtn('copy', 'Копировать текст', () => navigator.clipboard?.writeText(m.raw).then(() => ui.ok('Скопировано'))) : null,
            canDeleteMsg(m) ? itemBtn('trash', 'Удалить', async () => {
              await onDelete(m);
              items = items.filter(x => x.id !== m.id);
              if (pinnedMsg && pinnedMsg.id === m.id) { pinnedMsg = null; renderPinned(); }
              renderMessages(false);
            }, true) : null);
          const back = h('div.msg-ctx-back', { onclick: ev => { if (ev.target === back) close(); } }, menu);
          document.body.append(back);
        };
        const bubble = h('div.bubble' + (mine ? '.mine' : ''), {
          dataset: { mid: String(m.id) },
          onclick: openMenu,
          oncontextmenu: openMenu,
        },
          showStaff && mine && m.staff ? h('div', { style: { fontSize: '.74rem', opacity: '.75', marginBottom: '2px' } }, `От имени сообщества · ${m.staff.nick}`) : null,
          m.reply ? h('div.bubble-reply', { onclick: e => { e.stopPropagation(); jumpToMsg(m.reply.id); } }, h('b', m.reply.nick), h('span', m.reply.text)) : null,
          renderBubbleAttach(m.attach),
          m.msg ? ui.html(m.msg, 'text') : null,
          h('div.bubble-meta',
            m.pinned ? icon('pin', 11) : null,
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })),
            mine ? icon(m.read ? 'check-all' : 'check', 14) : null));
        const isNew = !seenIds.has(m.id);
        if (isNew) seenIds.add(m.id);
        const inlineKB = (!mine && m.inline_keyboard && m.inline_keyboard.length)
          ? h('div.msg-inline-kb', ...m.inline_keyboard.map(row =>
            h('div.msg-inline-row', ...row.map(btn => {
              const cls = 'msg-inline-btn' + (btn.color && btn.color !== 'default' ? '.' + btn.color : '');
              if (btn.url) {
                return h('a.' + cls, { href: btn.url, target: '_blank', rel: 'noopener nofollow', 'data-native': '' }, btn.text, icon('link', 12));
              }
              return h('button.' + cls, {
                type: 'button',
                onclick: async e => {
                  e.stopPropagation();
                  if (!onInlineClick) return;
                  e.currentTarget.disabled = true;
                  try { await onInlineClick(m, btn); }
                  finally { e.currentTarget.disabled = false; }
                },
              }, btn.text);
            }))))
          : null;
        nodes.push(h('div.bubble-row' + (mine ? '.mine' : '') + (isNew ? '.new-msg' : ''),
          inlineKB ? h('div.msg-bubble-col', bubble, inlineKB) : bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
    else requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight - prevH + prevTop; });
  };

  let kbToggleBtn = null;
  const renderReplyKB = () => {
    if (kbToggleBtn) {
      kbToggleBtn.style.display = replyKB && replyKB.length ? '' : 'none';
      kbToggleBtn.classList.toggle('active', kbVisible);
    }
    if (!replyKB || !replyKB.length || !kbVisible) {
      kbHost.replaceChildren();
      return;
    }
    kbHost.replaceChildren(h('div.msg-reply-kb', ...replyKB.map(row =>
      h('div.msg-reply-row', ...row.map(btn => {
        const cls = 'msg-reply-btn' + (btn.color && btn.color !== 'default' ? '.' + btn.color : '');
        return h('button.' + cls, {
          type: 'button',
          onclick: async e => {
            if (!onKeyboardClick) return;
            e.currentTarget.disabled = true;
            try {
              const r = await onKeyboardClick(btn);
              if (r && r.keyboard_removed) { replyKB = null; kbVisible = false; renderReplyKB(); }
            } finally { e.currentTarget.disabled = false; }
          },
        }, btn.text);
      })))));
  };

  const foot = h('div.msg-foot');
  if (canWrite) {
    const ta = h('textarea.msg-input', { rows: 1, placeholder: placeholder || 'Сообщение…', maxlength: 2000 });
    const charEl = h('span.msg-char');
    const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить', 'aria-label': 'Отправить', disabled: true }, icon('send', 18));
    const drawerHost = h('div');
    let activeDrawer = '';

    syncTa = () => {
      ta.style.height = 'auto';
      const sh = ta.scrollHeight;
      ta.style.height = Math.min(120, Math.max(36, sh)) + 'px';
      ta.style.overflowY = sh > 120 ? 'auto' : 'hidden';
      const n = ta.value.trim().length;
      sendBtn.disabled = n < 1 && !attach;
      if (ta.value.length > 1700) {
        charEl.textContent = `${ta.value.length}/2000`;
        charEl.classList.toggle('warn', ta.value.length > 1920);
      } else {
        charEl.textContent = '';
      }
    };

    const wrap = (a, b = '') => {
      const s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
      ta.value = v.slice(0, s) + a + v.slice(s, e) + b + v.slice(e);
      ta.focus();
      ta.selectionStart = s + a.length;
      ta.selectionEnd = e + a.length;
      syncTa();
    };

    const cmdBtn = cmds.length ? h('button.msg-tool', {
      type: 'button', title: 'Команды бота', 'aria-label': 'Команды бота',
    }, h('b', { style: { fontSize: '1.05rem', fontFamily: 'ui-monospace,monospace' } }, '/')) : null;
    kbToggleBtn = h('button.msg-tool', {
      type: 'button', title: 'Клавиатура бота', 'aria-label': 'Клавиатура бота',
      style: { display: replyKB && replyKB.length ? '' : 'none' },
      onclick: () => { kbVisible = !kbVisible; renderReplyKB(); },
    }, icon('grid', 19));
    const plusBtn = h('button.msg-tool', { type: 'button', title: 'Вложения, форматирование и смайлы', 'aria-label': 'Меню ввода' }, icon('plus', 20));
    let subTab = 'bb';

    const toggleDrawer = async which => {
      activeDrawer = activeDrawer === which ? '' : which;
      plusBtn.classList.toggle('active', activeDrawer === 'tools');
      cmdBtn && cmdBtn.classList.toggle('active', activeDrawer === 'cmds');
      if (!activeDrawer) { drawerHost.replaceChildren(); return; }

      if (activeDrawer === 'cmds') {
        drawerHost.replaceChildren(h('div.msg-drawer',
          h('div.msg-cmd-drawer', ...cmds.map(c => h('button.msg-cmd-item', {
            type: 'button',
            onclick: async () => {
              toggleDrawer('');
              const res = await onSend({ msg: '/' + c.command, reply_id: 0, att_type: '', att_id: 0 });
              if (res && !items.some(x => x.id === res.id)) {
                items.push(res);
                renderMessages(true);
              }
            },
          }, h('span.msg-cmd-name', '/' + c.command), h('span.msg-cmd-desc', c.description || ''))))));
      } else if (activeDrawer === 'tools') {
        const body = h('div');
        const tabsRow = h('div.msg-tools-tabs',
          h('button.msg-tools-tab', { type: 'button', onclick: () => { toggleDrawer(''); openAttachPicker(a => { attach = a; renderChips(); syncTa(); }); } }, icon('paperclip', 15), 'Прикрепить'),
          h('button.msg-tools-tab' + (subTab === 'bb' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'bb'; activeDrawer = ''; toggleDrawer('tools'); } }, h('b', 'Aa'), 'Формат'),
          h('button.msg-tools-tab' + (subTab === 'smiles' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'smiles'; activeDrawer = ''; toggleDrawer('tools'); } }, icon('smile', 15), 'Смайлы'));
        drawerHost.replaceChildren(h('div.msg-drawer', tabsRow, body));

        if (subTab === 'bb') {
          const bb = state.config.bb || {};
          const ribbon = h('div.msg-bb-ribbon');
          const addBtn = (lbl, title, fn, key) => {
            if (key && bb[key] === false) return;
            ribbon.append(h('button.msg-bb-btn', { type: 'button', title, onclick: fn }, lbl));
          };
          addBtn(h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'), 'b');
          addBtn(h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'), 'i');
          addBtn(h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'), 'u');
          addBtn(h('del', 'S'), 'Зачёркнутый', () => wrap('[s]', '[/s]'), 's');
          addBtn(icon('eye-off', 14), 'Спойлер', () => wrap('[spoiler]', '[/spoiler]'), 'spoiler');
          addBtn(h('span.bb-rainbow', 'A'), 'Градиент', () => wrap('[rainbow]', '[/rainbow]'), 'rainbow');
          addBtn(h('code', { style: { fontSize: '.74rem' } }, '<>'), 'Моно', () => wrap('[mono]', '[/mono]'), 'mono');
          addBtn(icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'), 'quote');
          addBtn(icon('hash', 14), 'Код', () => wrap('[code]', '[/code]'), 'code');
          addBtn(icon('link', 14), 'Вставить ссылку', async () => {
            const u = await ui.prompt('Адрес ссылки (https://...)', 'https://', { title: 'Вставить ссылку' });
            if (u && u.trim()) {
              const sel = ta.value.slice(ta.selectionStart, ta.selectionEnd);
              wrap(`[url=${u.trim()}]`, sel ? '[/url]' : `${u.trim()}[/url]`);
            }
          }, 'url');
          addBtn(icon('image', 14), 'Вставить картинку', async () => {
            const u = await ui.prompt('Ссылка на картинку (https://...)', 'https://', { title: 'Вставить картинку' });
            if (u && u.trim()) wrap(`[img]${u.trim()}[/img]`);
          }, 'img');
          ribbon.append(h('span.msg-bb-sep'));
          [['red', '#e11d48', 'Красный'], ['yellow', '#eab308', 'Жёлтый'], ['green', '#16a34a', 'Зелёный'], ['blue', '#2563eb', 'Синий']].forEach(([tag, col, title]) => {
            if (bb[tag] === false) return;
            ribbon.append(h('button.msg-color-dot', { type: 'button', title, style: { background: col }, onclick: () => wrap(`[${tag}]`, `[/${tag}]`) }));
          });
          if (bb.color !== false) {
            const hexT = h('input', { type: 'color', value: '#ff4081', onchange: () => wrap(`[color=${hexT.value}]`, '[/color]') });
            ribbon.append(h('label.bb-color-pick', { title: 'Любой HEX-цвет текста [color=#...]' }, h('span', { style: { color: '#ff4081' } }, 'A'), hexT));
          }
          if (bb.bg !== false) {
            const hexB = h('input', { type: 'color', value: '#fef08a', onchange: () => wrap(`[bg=${hexB.value}]`, '[/bg]') });
            ribbon.append(h('label.bb-color-pick', { title: 'Любой HEX-цвет фона [bg=#...]' }, h('span', { style: { background: '#fef08a', color: '#111', padding: '0 3px', borderRadius: '3px' } }, 'bg'), hexB));
          }
          body.replaceChildren(ribbon);
        } else {
          const box = h('div.msg-smiles-drawer', ui.loading());
          body.replaceChildren(box);
          const list = await getSmiles();
          if (!list.length) box.replaceChildren(h('span.small.muted', 'Смайлов нет'));
          else box.replaceChildren(...list.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))));
        }
      }
    };

    cmdBtn && (cmdBtn.onclick = () => toggleDrawer('cmds'));
    plusBtn.onclick = () => toggleDrawer('tools');

    const doSend = async () => {
      const msg = ta.value.trim();
      if ((msg.length < 1 && !attach) || sendBtn.disabled) return;
      sendBtn.disabled = true;
      try {
        const res = await onSend({
          msg,
          reply_id: replyTo ? replyTo.id : 0,
          att_type: attach ? attach.type : '',
          att_id: attach ? attach.id : 0,
        });
        ta.value = '';
        replyTo = null;
        attach = null;
        renderChips();
        syncTa();
        if (res && !items.some(x => x.id === res.id)) {
          items.push(res);
          renderMessages(true);
        }
      } finally { syncTa(); }
    };

    sendBtn.onclick = doSend;
    ta.oninput = syncTa;
    ta.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) { e.preventDefault(); doSend(); } };
    foot.append(chipHost, drawerHost, h('div.msg-compose', plusBtn, cmdBtn, h('div.msg-input-wrap', ta, charEl), kbToggleBtn, sendBtn), kbHost);
  } else {
    foot.append(h('div.pad.small.muted.center', disabledText || 'Отправка сообщений отключена.'));
  }

  const unsub = onSubscribe ? onSubscribe(async incoming => {
    if (incoming) {
      if (incoming.keyboard_update) {
        replyKB = (incoming.keyboard && incoming.keyboard.length) ? incoming.keyboard : null;
        kbVisible = !!replyKB;
        renderReplyKB();
      }
      if (!items.some(x => x.id === incoming.id)) {
        items.push(incoming);
        renderMessages(true);
        if (!isMine(incoming) && onMarkRead) {
          await onMarkRead().catch(() => {});
        }
        refreshCounters();
      }
    }
  }) : null;
  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    unsub && unsub();
  });

  renderPinned();
  renderReplyKB();
  renderMessages(true);
  return h('div.msg-shell', head, pinnedHost, scrollEl, foot);
}

state.renderRichGroupDialog = renderRichGroupDialog;

// ───────── ДИАЛОГ С СООБЩЕСТВОМ В ПОЧТЕ (/mail/group/:id)
router.add('/mail/group/:id', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/dm`)]);
  refreshCounters();
  ctx.title(`Диалог: ${g.name}`);
  const initialOf = s => (Array.from((s || 'С').trim())[0] || 'С').toUpperCase();

  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/mail', title: 'Назад к почте', 'aria-label': 'Назад' }, icon('arrow-left', 20)),
    h('div.grp-ava-sm', { style: { width: '38px', height: '38px', borderRadius: '10px', fontSize: '1rem', flexShrink: '0', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#ec4899)', color: '#fff', fontWeight: '800', overflow: 'hidden' } },
      g.avatar ? h('img', { src: g.avatar, alt: g.name, style: { width: '100%', height: '100%', objectFit: 'cover' } }) : initialOf(g.name)),
    h('div.msg-head-info',
      h('a', { href: '/groups/' + id, style: { fontWeight: '700', color: 'inherit', textDecoration: 'none' } }, g.name),
      h('div.msg-status', 'Сообщество · личные сообщения')),
    ui.btn('', { ic: 'users', small: true, title: 'Перейти в сообщество', href: '/groups/' + id }));

  return renderRichGroupDialog({
    ctx, head,
    initialItems: d.items,
    initialPinned: d.pinned,
    initialKeyboard: d.keyboard,
    botCommands: d.bot_commands,
    canWrite: d.can_write,
    emptyText: 'Напишите первое сообщение сообществу — администрация сможет ответить вам прямо в этом диалоге.',
    disabledText: 'Отправка новых сообщений в это сообщество отключена.',
    placeholder: 'Сообщение сообществу…',
    isMine: m => !m.from_group,
    senderLabel: m => (m.from_group ? g.name : state.user.nick),
    showStaff: false,
    canDeleteMsg: m => !m.from_group,
    onSend: payload => api.post(`/api/groups/${id}/dm`, payload),
    onPin: m => api.post(`/api/group-dm/${m.id}/pin`),
    onDelete: m => api.del(`/api/group-dm/${m.id}`),
    onInlineClick: (m, btn) => api.post(`/api/groups/${id}/dm/callback`, { message_id: m.id, callback_data: btn.callback_data || btn.text, text: btn.text }),
    onKeyboardClick: btn => api.post(`/api/groups/${id}/dm/keyboard-click`, { callback_data: btn.callback_data || btn.text, text: btn.text }),
    onMarkRead: () => api.post(`/api/groups/${id}/dm/read`, {}, { silent: true }),
    onSubscribe: cb => bus.on('sse:group_dm', data => {
      if (data && data.group_id === id && data.user_id === state.user?.id) cb(data);
    }),
  });
}, { auth: true });

// ───────── ГЛОБАЛЬНЫЙ ПОИСК ПО САЙТУ (/search)
router.add('/search', async ctx => {
  const q = (ctx.query.q || '').trim();
  ctx.title(q ? `Поиск: ${q}` : 'Поиск по сайту');
  const inp = h('input.input', { value: q, placeholder: 'Искать людей, сообщества, клипы…', autofocus: !q });
  const go = () => router.go('/search' + (inp.value.trim() ? '?q=' + encodeURIComponent(inp.value.trim()) : ''));
  inp.onkeydown = e => e.key === 'Enter' && go();

  const out = [
    ui.pageHead('Поиск по сайту'),
    h('div.card.pad', h('div.row.nowrap', inp, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: go }))),
  ];
  if (!q) {
    out.push(ui.card(ui.empty('Введите имя, ник, название сообщества или клипа', 'search')));
    return out;
  }
  const d = await api.get('/api/search', { q });
  const total = (d.users?.length || 0) + (d.groups?.length || 0) + (d.shorts?.length || 0);
  if (!total) {
    out.push(ui.card(ui.empty(`По запросу «${q}» ничего не найдено`, 'search')));
    return out;
  }
  if (d.users && d.users.length) {
    out.push(ui.section(`Пользователи (${d.users.length})`, h('div.list', ...d.users.map(u => h('a.list-item', { href: '/id' + u.id },
      ui.avatar(u, 40), h('div.main', h('div.title', u.nick, u.online ? h('span.badge.ok', { style: { marginLeft: '6px' } }, 'онлайн') : null), h('div.small.muted', u.group || 'Пользователь')))))));
  }
  if (d.groups && d.groups.length) {
    const ini = s => (Array.from((s || 'С').trim())[0] || 'С').toUpperCase();
    out.push(ui.section(`Сообщества (${d.groups.length})`, h('div.list', ...d.groups.map(g => h('a.list-item', { href: '/groups/' + g.id },
      h('div.grp-ava-sm', { style: { width: '40px', height: '40px', borderRadius: '10px', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#ec4899)', color: '#fff', fontWeight: '800', overflow: 'hidden', flexShrink: '0' } },
        g.avatar ? h('img', { src: g.avatar, alt: g.name, style: { width: '100%', height: '100%', objectFit: 'cover' } }) : ini(g.name)),
      h('div.main', h('div.title', g.name, h('span.badge', { style: { marginLeft: '6px' } }, g.category)), h('div.small.muted', `Участников: ${fmt.num(g.members)}`)))))));
  }
  if (d.shorts && d.shorts.length) {
    out.push(ui.section(`Клипы (${d.shorts.length})`, h('div.list', ...d.shorts.map(s => h('a.list-item', { href: '/shorts/' + s.id },
      h('img', { src: s.poster_url, alt: s.title, style: { width: '42px', height: '58px', objectFit: 'cover', borderRadius: '6px', flexShrink: '0' } }),
      h('div.main', h('div.title', s.title), h('div.small.muted', s.user.nick, ' · ', fmt.num(s.views), ' просм. · ♥ ', fmt.num(s.likes))))))));
  }
  return out;
});

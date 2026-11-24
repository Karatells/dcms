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
  const [d, gd] = await Promise.all([
    api.get('/api/konts', { page: ctx.page, type }),
    type === 'common' && ctx.page === 1 ? api.get('/api/my-group-dialogs', null, { silent: true }).catch(() => ({ items: [] })) : Promise.resolve({ items: [] }),
  ]);
  const sel = new Set();

  const initialOf = s => (Array.from((s || 'С').trim())[0] || 'С').toUpperCase();
  const merged = [
    ...d.items.map(k => ({ kind: 'user', time: (k.last && k.last.time) || k.time || 0, data: k })),
    ...(gd.items || []).map(g => ({ kind: 'group', time: g.time || 0, data: g })),
  ].sort((a, b) => b.time - a.time);

  const rows = merged.map(entry => {
    if (entry.kind === 'group') {
      const item = entry.data;
      const g = item.group;
      return h('a.list-item' + (item.new ? '.unread' : ''), { href: '/mail/group/' + g.id },
        h('div.grp-ava-sm', { style: { width: '44px', height: '44px', borderRadius: '12px', fontSize: '1.05rem', flexShrink: '0', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#ec4899)', color: '#fff', fontWeight: '800', overflow: 'hidden' } },
          g.avatar ? h('img', { src: g.avatar, alt: g.name, style: { width: '100%', height: '100%', objectFit: 'cover' } }) : initialOf(g.name)),
        h('div.main',
          h('div.title', g.name,
            h('span.badge.primary', { style: { marginLeft: '6px' } }, 'Сообщество'),
            item.new ? h('span.badge.danger', { style: { marginLeft: '6px' } }, '+' + item.new) : null),
          item.last ? h('div.small.muted.clip', (item.last.mine ? 'Вы: ' : '') + item.last.text) : h('div.small.muted', 'Сообщений нет')),
        item.last ? ui.time(item.last.time) : null);
    }
    const k = entry.data;
    return h('div.list-item' + (k.new ? '.unread' : ''),
      h('input', { type: 'checkbox', onchange: e => e.target.checked ? sel.add(k.user.id) : sel.delete(k.user.id) }),
      ui.avatar(k.user, 44), h('a.main', { href: '/mail/' + k.user.id, style: { color: 'inherit', textDecoration: 'none' } },
        h('div.title', k.name || k.user.nick, k.new ? h('span.badge.danger', { style: { marginLeft: '8px' } }, '+' + k.new) : null),
        k.last ? h('div.small.muted.clip', (k.last.mine ? 'Вы: ' : '') + k.last.text) : h('div.small.muted', 'Сообщений нет')),
      k.last ? ui.time(k.last.time) : null, ...(type === 'deleted' ? [h('span.small.muted', 'удалится ' + fmt.date(k.expires))] : []));
  });
  const move = (t, l, kind) => ui.btn(l, { small: true, kind, onclick: async () => { if (!sel.size) return ui.toast('Отметьте контакты', 'warn'); await api.post('/api/konts/move', { ids: [...sel], type: t }); reload(); } });
  const to = h('input.input', { placeholder: 'Логин получателя' });
  const msg = h('textarea.input', { rows: 3, placeholder: 'Сообщение (2–1024 символа)', maxlength: 1024 });
  const compose = h('details.card.pad', h('summary', icon('edit', 16), ' Написать по логину'), h('div.stack', { style: { marginTop: '12px' } }, to, msg,
    ui.btn('Отправить', { kind: 'primary', ic: 'send', onclick: async () => { const r = await api.post('/api/mail/send', { to: to.value, msg: msg.value }); ui.ok('Сообщение отправлено'); router.go('/mail/' + r.id); } })));
  return [ui.pageHead('Почта'), mailTabs(type), compose,
    ui.card(rows.length ? h('div.list', rows) : ui.empty(type === 'common' ? 'Контактов пока нет' : 'Список пуст', 'mail')),
    d.items.length ? h('div.row', ...KONT_TABS.filter(([k]) => k !== type).map(([k, l]) => move(k, k === 'deleted' ? 'Удалить' : k === 'common' ? 'В контакты' : k === 'favorite' ? 'В избранные' : 'В игнор', k === 'deleted' ? 'danger' : ''))) : null,
    type === 'deleted' ? h('div.muted.small', 'Удалённые контакты очищаются через 30 дней вместе с перепиской.') : null, ui.pager(d)];
}, { auth: true });

router.add('/mail/new', async () => { router.go('/mail'); }, { auth: true });

let _smilesCache = null;
async function getSmiles() {
  if (_smilesCache) return _smilesCache;
  try {
    const list = await api.get('/api/smiles/all', null, { silent: true });
    _smilesCache = list.items || list || [];
  } catch { _smilesCache = []; }
  return _smilesCache;
}

function dayLabel(ts) {
  const d = new Date(ts * 1000), now = new Date();
  const same = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  if (same(d, now)) return 'Сегодня';
  const y = new Date(now); y.setDate(y.getDate() - 1);
  if (same(d, y)) return 'Вчера';
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: d.getFullYear() !== now.getFullYear() ? 'numeric' : undefined });
}

// ───────── выбор личного вложения (Фото / Файлы / Клипы)
function openAttachPicker(onPick) {
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
function renderBubbleAttach(att) {
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

// ───────── контекстное меню сообщения
function openMsgMenu(m, { onReply, onPin, onDelete, onSpam }) {
  const close = () => back.remove();
  const item = (ic, label, fn, danger = false) => h('button.msg-ctx-item' + (danger ? '.danger' : ''), {
    type: 'button',
    onclick: () => { close(); fn(); },
  }, icon(ic, 18), label);
  const menu = h('div.msg-ctx',
    item('corner-up-left', 'Ответить', onReply),
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
  let pinnedMsg = d.pinned || null;
  let replyTo = null; // { id, nick, text }
  let attach = null;  // { type, id, title, image }

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
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
          items = [...(next.items || []).reverse(), ...items];
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
        const openMenu = e => {
          if (e.target.closest('a, .bubble-reply')) return;
          e.preventDefault();
          openMsgMenu(m, {
            onReply: () => { replyTo = { id: m.id, nick: m.mine ? state.user.nick : d.user.nick, text: (m.raw || '').slice(0, 80) || (m.attach ? m.attach.title : 'Сообщение') }; renderChips(); },
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
        const bubble = h('div.bubble' + (m.mine ? '.mine' : ''), {
          dataset: { mid: String(m.id) },
          onclick: openMenu,
          oncontextmenu: openMenu,
        },
          m.reply ? h('div.bubble-reply', { onclick: e => { e.stopPropagation(); jumpToMsg(m.reply.id); } }, h('b', m.reply.nick), h('span', m.reply.text)) : null,
          renderBubbleAttach(m.attach),
          m.msg ? ui.html(m.msg, 'text') : null,
          h('div.bubble-meta',
            m.pinned ? icon('pin', 11) : null,
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })),
            m.mine ? icon(m.read ? 'check-all' : 'check', 14) : null));
        nodes.push(h('div.bubble-row' + (m.mine ? '.mine' : ''), bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) {
      requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
    } else {
      requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight - prevH + prevTop; });
    }
  };

  // Шапка диалога
  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/mail', title: 'Назад к почте', 'aria-label': 'Назад' }, icon('arrow-left', 20)),
    ui.avatar(d.user, 38),
    h('div.msg-head-info',
      ui.userLink(d.user, { badges: true }),
      h('div.msg-status' + (d.user.online ? '.on' : ''), d.user.online ? 'в сети' : 'не в сети')),
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
    const charEl = h('span.msg-char');
    const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить', 'aria-label': 'Отправить', disabled: true }, icon('send', 18));
    const drawerHost = h('div');
    let activeDrawer = '';

    syncTa = () => {
      ta.style.height = 'auto';
      ta.style.height = Math.min(120, Math.max(38, ta.scrollHeight)) + 'px';
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

    const attBtn = h('button.msg-tool', {
      type: 'button', title: 'Прикрепить фото, файл или клип', 'aria-label': 'Прикрепить',
      onclick: () => openAttachPicker(a => { attach = a; renderChips(); syncTa(); }),
    }, icon('paperclip', 20));
    const bbBtn = h('button.msg-tool', { type: 'button', title: 'Форматирование (BBCode)', 'aria-label': 'Форматирование' }, h('b', { style: { fontSize: '.9rem' } }, 'Aa'));
    const smBtn = h('button.msg-tool', { type: 'button', title: 'Смайлы', 'aria-label': 'Смайлы' }, icon('smile', 20));

    const toggleDrawer = async which => {
      activeDrawer = activeDrawer === which ? '' : which;
      bbBtn.classList.toggle('active', activeDrawer === 'bb');
      smBtn.classList.toggle('active', activeDrawer === 'smiles');
      if (!activeDrawer) { drawerHost.replaceChildren(); return; }

      if (activeDrawer === 'bb') {
        const bb = state.config.bb || {};
        const ribbon = h('div.msg-bb-ribbon');
        const addBtn = (lbl, title, fn, key) => {
          if (key && bb[key] === false) return;
          ribbon.append(h('button.msg-bb-btn', { type: 'button', title, onclick: fn }, lbl));
        };
        addBtn(h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'), 'b');
        addBtn(h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'), 'i');
        addBtn(h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'), 'u');
        addBtn(icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'));
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
        drawerHost.replaceChildren(h('div.msg-drawer', ribbon));
      } else if (activeDrawer === 'smiles') {
        const box = h('div.msg-smiles-drawer', ui.loading());
        drawerHost.replaceChildren(h('div.msg-drawer', box));
        const list = await getSmiles();
        if (!list.length) {
          box.replaceChildren(h('span.small.muted', 'Смайлов нет'));
        } else {
          box.replaceChildren(...list.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))));
        }
      }
    };

    bbBtn.onclick = () => toggleDrawer('bb');
    smBtn.onclick = () => toggleDrawer('smiles');

    const doSend = async () => {
      const msg = ta.value.trim();
      if ((msg.length < 1 && !attach) || sendBtn.disabled) return;
      sendBtn.disabled = true;
      try {
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

    sendBtn.onclick = doSend;
    ta.oninput = syncTa;
    ta.onkeydown = e => {
      if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) {
        e.preventDefault();
        doSend();
      }
    };

    foot.append(chipHost, drawerHost, h('div.msg-compose', attBtn, smBtn, bbBtn, h('div.msg-input-wrap', ta, charEl), sendBtn));
  } else {
    foot.append(h('div.pad.small.muted.center', 'Вы не можете писать этому пользователю.'));
  }

  const onMsg = bus.on('sse:message', async m => {
    if (m.from && m.from.id === d.user.id) {
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
    onMsg && onMsg();
  });

  renderPinned();
  renderMessages(true);
  return h('div.msg-shell', head, pinnedHost, scrollEl, foot);
}, { auth: true });

// ───────── УНИВЕРСАЛЬНЫЙ ПОЛНОФУНКЦИОНАЛЬНЫЙ МЕССЕНДЖЕР ДЛЯ ПЕРЕПИСОК С СООБЩЕСТВАМИ
function renderRichGroupDialog({ ctx, head, initialItems, initialPinned, canWrite, emptyText, disabledText, placeholder, isMine, senderLabel, showStaff, canDeleteMsg, onSend, onPin, onDelete, onSubscribe }) {
  document.body.classList.add('msg-page');
  let items = [...(initialItems || [])];
  let pinnedMsg = initialPinned || null;
  let replyTo = null;
  let attach = null;

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
  const chipHost = h('div');

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
        nodes.push(h('div.bubble-row' + (mine ? '.mine' : ''), bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
    else requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight - prevH + prevTop; });
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
      ta.style.height = Math.min(120, Math.max(38, ta.scrollHeight)) + 'px';
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

    const attBtn = h('button.msg-tool', {
      type: 'button', title: 'Прикрепить фото, файл или клип', 'aria-label': 'Прикрепить',
      onclick: () => openAttachPicker(a => { attach = a; renderChips(); syncTa(); }),
    }, icon('paperclip', 20));
    const bbBtn = h('button.msg-tool', { type: 'button', title: 'Форматирование (BBCode)', 'aria-label': 'Форматирование' }, h('b', { style: { fontSize: '.9rem' } }, 'Aa'));
    const smBtn = h('button.msg-tool', { type: 'button', title: 'Смайлы', 'aria-label': 'Смайлы' }, icon('smile', 20));

    const toggleDrawer = async which => {
      activeDrawer = activeDrawer === which ? '' : which;
      bbBtn.classList.toggle('active', activeDrawer === 'bb');
      smBtn.classList.toggle('active', activeDrawer === 'smiles');
      if (!activeDrawer) { drawerHost.replaceChildren(); return; }

      if (activeDrawer === 'bb') {
        const bb = state.config.bb || {};
        const ribbon = h('div.msg-bb-ribbon');
        const addBtn = (lbl, title, fn, key) => {
          if (key && bb[key] === false) return;
          ribbon.append(h('button.msg-bb-btn', { type: 'button', title, onclick: fn }, lbl));
        };
        addBtn(h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'), 'b');
        addBtn(h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'), 'i');
        addBtn(h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'), 'u');
        addBtn(icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'));
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
        drawerHost.replaceChildren(h('div.msg-drawer', ribbon));
      } else if (activeDrawer === 'smiles') {
        const box = h('div.msg-smiles-drawer', ui.loading());
        drawerHost.replaceChildren(h('div.msg-drawer', box));
        const list = await getSmiles();
        if (!list.length) box.replaceChildren(h('span.small.muted', 'Смайлов нет'));
        else box.replaceChildren(...list.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))));
      }
    };

    bbBtn.onclick = () => toggleDrawer('bb');
    smBtn.onclick = () => toggleDrawer('smiles');

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
    foot.append(chipHost, drawerHost, h('div.msg-compose', attBtn, smBtn, bbBtn, h('div.msg-input-wrap', ta, charEl), sendBtn));
  } else {
    foot.append(h('div.pad.small.muted.center', disabledText || 'Отправка сообщений отключена.'));
  }

  const unsub = onSubscribe ? onSubscribe(incoming => {
    if (incoming && !items.some(x => x.id === incoming.id)) {
      items.push(incoming);
      renderMessages(true);
      refreshCounters();
    }
  }) : null;
  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    unsub && unsub();
  });

  renderPinned();
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
    onSubscribe: cb => bus.on('sse:group_dm', data => {
      if (data && data.group_id === id && data.user_id === state.user?.id) cb(data);
    }),
  });
}, { auth: true });

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
};
router.add('/user/notification', async ctx => {
  ctx.title('Уведомления');
  const reload = () => router.render();
  const data = await api.get('/api/notification', { page: ctx.page });
  refreshCounters();
  const rows = data.items.map(n => h('article.feed-item' + (n.read ? '' : '.unread'), ui.avatar(n.author, 42), h('div.main',
    h('div.feed-head', ui.userLink(n.author), ' ', h('span.muted', (NOTIF_TEXT[n.type] || (() => 'уведомление'))(n.author)), h('span.grow'), ui.time(n.time)),
    n.link ? h('a.feed-desc', { href: n.link }, h('div', h('div.title', n.desc?.title || 'Перейти'), n.desc?.text ? h('div.small.muted', n.desc.text) : null)) : null),
    ui.btn('', { ic: 'x', small: true, title: 'Удалить', onclick: async () => { await api.del('/api/notification/' + n.id); reload(); } })));
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
const mailTabs = active => ui.tabs([{ key: 'new', label: h('span', 'Новые ', cnt('mail') ? h('span.badge.danger', cnt('mail')) : null), icon: 'mail', href: '/mail/new' },
  ...KONT_TABS.map(([k, l, i]) => ({ key: k, label: l, icon: i, href: '/mail?type=' + k }))], active);

router.add('/mail', async ctx => {
  ctx.title('Почта');
  const type = ctx.query.type || 'common';
  const reload = () => router.render();
  const d = await api.get('/api/konts', { page: ctx.page, type });
  const sel = new Set();
  const rows = d.items.map(k => h('div.list-item' + (k.new ? '.unread' : ''),
    h('input', { type: 'checkbox', onchange: e => e.target.checked ? sel.add(k.user.id) : sel.delete(k.user.id) }),
    ui.avatar(k.user, 44), h('a.main', { href: '/mail/' + k.user.id, style: { color: 'inherit', textDecoration: 'none' } },
      h('div.title', k.name || k.user.nick, k.new ? h('span.badge.danger', { style: { marginLeft: '8px' } }, '+' + k.new) : null),
      k.last ? h('div.small.muted.clip', (k.last.mine ? 'Вы: ' : '') + k.last.text) : h('div.small.muted', 'Сообщений нет')),
    k.last ? ui.time(k.last.time) : null, ...(type === 'deleted' ? [h('span.small.muted', 'удалится ' + fmt.date(k.expires))] : [])));
  const move = (t, l, kind) => ui.btn(l, { small: true, kind, onclick: async () => { if (!sel.size) return ui.toast('Отметьте контакты', 'warn'); await api.post('/api/konts/move', { ids: [...sel], type: t }); reload(); } });
  const to = h('input.input', { placeholder: 'Логин получателя' });
  const msg = h('textarea.input', { rows: 3, placeholder: 'Сообщение (2–1024 символа)', maxlength: 1024 });
  const compose = h('details.card.pad', h('summary', icon('edit', 16), ' Написать по логину'), h('div.stack', { style: { marginTop: '12px' } }, to, msg,
    ui.btn('Отправить', { kind: 'primary', ic: 'send', onclick: async () => { const r = await api.post('/api/mail/send', { to: to.value, msg: msg.value }); ui.ok('Сообщение отправлено'); router.go('/mail/' + r.id); } })));
  return [ui.pageHead('Почта'), mailTabs(type), compose,
    ui.card(rows.length ? h('div.list', rows) : ui.empty(type === 'common' ? 'Контактов пока нет' : 'Список пуст', 'mail')),
    rows.length ? h('div.row', ...KONT_TABS.filter(([k]) => k !== type).map(([k, l]) => move(k, k === 'deleted' ? 'Удалить' : k === 'common' ? 'В контакты' : k === 'favorite' ? 'В избранные' : 'В игнор', k === 'deleted' ? 'danger' : ''))) : null,
    type === 'deleted' ? h('div.muted.small', 'Удалённые контакты очищаются через 30 дней вместе с перепиской.') : null, ui.pager(d)];
}, { auth: true });

router.add('/mail/new', async ctx => {
  ctx.title('Новые сообщения');
  const d = await api.get('/api/mail/new');
  return [ui.pageHead('Новые сообщения'), mailTabs('new'), ui.card(d.items.length ? h('div.list', d.items.map(x => h('a.list-item', { href: '/mail/' + x.user.id },
    ui.avatar(x.user, 44), h('div.main', h('div.title', x.user.nick, h('span.badge.danger', { style: { marginLeft: '8px' } }, '+' + x.count)), h('div.small.muted.clip', x.text)), ui.time(x.time)))) : ui.empty('Нет новых сообщений', 'mail'))];
}, { auth: true });

router.add('/mail/:id', async ctx => {
  const reload = () => router.render();
  const d = await api.get('/api/mail/' + ctx.params.id, { page: ctx.page });
  refreshCounters();
  ctx.title('Диалог: ' + d.user.nick);
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Сообщение…', maxlength: 1024 });
  const send = async () => { await api.post('/api/mail/' + d.user.id, { msg: ed.value }); ed.value = ''; reload(); };
  const bubbles = d.items.map(m => h('div.bubble-row' + (m.mine ? '.mine' : ''), h('div.bubble' + (m.mine ? '.mine' : ''),
    ui.html(m.msg, 'text'), h('div.bubble-meta', ui.time(m.time), m.mine ? icon(m.read ? 'check-all' : 'check', 14) : null,
      h('span.bubble-act',
        !m.mine ? h('button.link', { title: 'Пожаловаться', onclick: async () => { const t = await ui.prompt('Причина жалобы', '', { multiline: true, title: 'Жалоба' }); if (t) { await api.post(`/api/mail/msg/${m.id}/spam`, { msg: t, types: 0 }); ui.ok('Жалоба отправлена'); } } }, icon('flag', 13)) : null,
        h('button.link', { title: 'Удалить у себя', onclick: async () => { await api.del('/api/mail/msg/' + m.id); reload(); } }, icon('trash', 13)))))));
  const onMsg = bus.on('sse:message', m => { if (m.from && m.from.id === d.user.id) reload(); });
  ctx.onLeave(() => onMsg && onMsg());
  return [ui.breadcrumbs([{ href: '/mail', title: 'Почта' }, { title: d.user.nick }]),
    h('div.card.pad.row', ui.avatar(d.user, 44), h('div.grow', ui.userLink(d.user, { badges: true }), h('div.small.muted', d.user.online ? 'онлайн' : '')),
      !d.contact ? ui.btn('В контакты', { ic: 'plus', small: true, onclick: async () => { await api.post('/api/konts/' + d.user.id); reload(); } }) : null,
      ui.dropdown(ui.btn('', { ic: 'more' }), [
        { label: 'Профиль', icon: 'user', href: '/id' + d.user.id },
        { label: 'В избранные', icon: 'star', onclick: async () => { await api.post('/api/konts/move', { ids: [d.user.id], type: 'favorite' }); ui.ok('Готово'); } },
        { label: 'В игнор', icon: 'ban', onclick: async () => { await api.post('/api/konts/move', { ids: [d.user.id], type: 'ignor' }); ui.ok('Контакт в игнор-листе'); } },
        { label: 'Очистить переписку', icon: 'trash', danger: true, onclick: async () => { if (await ui.confirm('Удалить переписку у себя?', { danger: true })) { await api.del('/api/mail/' + d.user.id); reload(); } } }])),
    d.can_write ? h('div.card.pad.stack', ed.el, h('div.row', ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: send }))) : h('div.card.pad.muted', 'Вы не можете писать этому пользователю.'),
    h('div.chat-box', bubbles.length ? bubbles : ui.empty('Сообщений пока нет', 'mail')), ui.pager(d)];
}, { auth: true });

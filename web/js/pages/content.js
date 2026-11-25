import { h, api, ui, router, state, icon, link, fmt, bus } from '../core.js';

const me = () => state.user;
const crumbs = (...c) => ui.breadcrumbs(c);
state.adminLinks.push(
  { group: 'Контент', title: 'Реклама', href: '/admin/rekl', access: 'adm_rekl', icon: 'megaphone', desc: 'Рекламные ссылки и баннеры на сайте' },
  { group: 'Контент', title: 'Смайлы', href: '/admin/smiles', access: 'adm_panel_show', icon: 'smile', desc: 'Категории смайлов, загрузка, коды' });

// ───────── блоги: общие элементы
const shareEl = s => s && h('div.share-quote', h('div.small.muted', icon('share', 14), s.type === 'forum' ? ' Репост темы форума ' : ' Репост записи ', s.link ? link(s.link, h('b', s.name)) : h('b', s.name)), h('div.text', { html: s.text }));
const noteItem = n => h('div.list-item.note-item', h('div.main',
  h('div.row.nowrap', h('a.title', { href: '/notes/' + n.id }, n.name), h('span.grow'), h('span.small.muted', fmt.ago(n.time))),
  n.preview ? h('div.small.muted', n.preview) : null, shareEl(n.share),
  h('div.row.small', ui.userLink(n.user), h('span.badge', icon('message', 12), ' ', n.komm), h('a.badge', { href: `/notes/${n.id}/bookmarks` }, icon('star', 12), ' ', n.bookmarks), h('span.badge', icon('share', 12), ' ', n.shares),
    n.private ? h('span.badge', icon('lock', 12), n.private === 1 ? ' друзья' : ' только я') : null)));

function noteTabs(active) {
  const t = [['/notes', 'Блоги', 'list'], ['/notes/dirs', 'Категории', 'dirs']];
  if (me()) t.push([`/notes/user/${me().id}`, 'Мои', 'mine']);
  return h('div.tabs', t.map(([href, l, k]) => h('a.tab' + (active === k ? '.active' : ''), { href }, l)));
}
const chipsRow = (items, cur, mk) => h('div.chips', items.map(([v, l]) => h('a.chip' + (cur === v ? '.active' : ''), { href: mk(v) }, l)));

const listPage = async ctx => {
  ctx.title('Блоги');
  const sort = ctx.query.sort === 'c' ? 'c' : 't', nw = ctx.query.new || '';
  const d = await api.get('/api/notes', { page: ctx.page, sort, new: nw });
  const q = h('input.input', { placeholder: 'Поиск по названию', maxlength: 64 });
  const go = () => q.value.trim() && router.go('/notes/search?q=' + encodeURIComponent(q.value.trim()));
  q.onkeydown = e => e.key === 'Enter' && go();
  const out = [ui.pageHead('Блоги', me() ? ui.btn('Создать запись', { ic: 'edit', kind: 'primary', small: true, href: '/notes/add' }) : null), noteTabs('list'),
    h('div.card.pad.row.nowrap', q, ui.btn('Найти', { ic: 'search', onclick: go })),
    chipsRow([['t', 'Новые'], ['c', 'Популярные']], sort, v => `/notes?sort=${v}`)];
  if (sort === 'c') out.push(chipsRow([['', 'Все время'], ['t', 'За сутки'], ['m', 'За месяц']], nw, v => `/notes?sort=c&new=${v}`));
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(noteItem)) : ui.empty('Нет записей', 'book')), ui.pager(d));
  return out;
};
router.add('/notes', listPage);
router.add('/plugins/notes', listPage);

router.add('/notes/search', async ctx => {
  const q = ctx.query.q || '';
  ctx.title('Поиск блогов');
  const d = q ? await api.get('/api/notes-search', { q, page: ctx.page }) : null;
  const input = h('input.input', { value: q, placeholder: 'Название записи', maxlength: 64 });
  const go = () => router.go('/notes/search?q=' + encodeURIComponent(input.value.trim()));
  input.onkeydown = e => e.key === 'Enter' && go();
  return [crumbs({ href: '/notes', title: 'Блоги' }, { title: 'Поиск' }), ui.pageHead('Поиск блогов'), h('div.card.pad.row.nowrap', input, ui.btn('Искать', { ic: 'search', kind: 'primary', onclick: go })),
    d ? ui.card(d.items.length ? h('div.list', d.items.map(noteItem)) : ui.empty('Нет записей', 'search')) : null, d ? ui.pager(d) : null];
});

router.add('/notes/user/:uid', async ctx => {
  const uid = ctx.params.uid, sort = ctx.query.sort === 'c' ? 'c' : 't';
  const d = await api.get('/api/notes-user/' + uid, { page: ctx.page, sort });
  ctx.title('Блоги ' + d.owner.nick);
  return [ui.pageHead('Блоги ' + d.owner.nick, d.mine ? ui.btn('Создать запись', { ic: 'edit', kind: 'primary', small: true, href: '/notes/add' }) : null), noteTabs(d.mine ? 'mine' : ''),
    chipsRow([['t', 'Новые'], ['c', 'Популярные']], sort, v => `/notes/user/${uid}?sort=${v}`),
    ui.card(d.items.length ? h('div.list', d.items.map(noteItem)) : ui.empty('Нет записей', 'book')), ui.pager(d)];
});

// категории
router.add('/notes/dirs', async ctx => {
  ctx.title('Категории блогов');
  const d = await api.get('/api/notes-dirs');
  const reload = () => router.render();
  const out = [ui.pageHead('Категории блогов'), noteTabs('dirs')];
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', h('a.row.nowrap.grow', { href: '/notes/dirs/' + x.id }, h('div.forum-ico', icon('folder', 22)), h('div.main', h('div.title', x.name), x.msg_html ? h('div.small.muted', { html: x.msg_html }) : null)),
    h('span.badge', x.count, x.new ? h('span.new', ' +' + x.new) : null),
    d.can_delete ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить категорию «${x.name}» вместе со всеми записями?`, { danger: true })) { await api.del('/api/notes-dirs/' + x.id); reload(); } } }) : null))) : ui.empty('Нет категорий', 'folder')));
  if (d.can_create) {
    const name = h('input.input', { maxlength: 32, placeholder: 'Название' }), msg = ui.bbEditor({ rows: 3, placeholder: 'Описание' });
    out.push(h('details.card.pad', h('summary', icon('plus', 16), ' Добавить категорию'), h('div.stack', name, msg.el,
      ui.btn('Создать', { ic: 'check', kind: 'primary', onclick: async () => { await api.post('/api/notes-dirs', { name: name.value, msg: msg.value }); ui.ok('Категория создана'); reload(); } }))));
  }
  return out;
});
router.add('/notes/dirs/:id', async ctx => {
  const id = ctx.params.id, sort = ctx.query.sort === 'c' ? 'c' : 't';
  const d = await api.get('/api/notes-dirs/' + id, { page: ctx.page, sort });
  ctx.title(d.dir.name);
  return [crumbs({ href: '/notes', title: 'Блоги' }, { href: '/notes/dirs', title: 'Категории' }, { title: d.dir.name }),
    ui.pageHead(d.dir.name, me() ? ui.btn('Создать запись', { ic: 'edit', kind: 'primary', small: true, href: '/notes/add?id_dir=' + id }) : null),
    chipsRow([['t', 'Новые'], ['c', 'Популярные']], sort, v => `/notes/dirs/${id}?sort=${v}`),
    ui.card(d.items.length ? h('div.list', d.items.map(noteItem)) : ui.empty('Нет записей', 'book')), ui.pager(d)];
});

// форма записи
async function noteForm(ctx, note) {
  const dirs = (await api.get('/api/notes-dirs')).items;
  const st = { private: note?.private ?? 0, private_komm: note?.private_komm ?? 0 };
  const name = h('input.input', { value: note?.name || '', maxlength: 32, placeholder: 'О чём поведаете?', required: true });
  const ed = ui.bbEditor({ value: note?.msg || '', rows: 12, placeholder: 'Ваша история начинается здесь…' });
  const dir = h('select.input', h('option', { value: 0 }, 'Без категории'), dirs.map(d => h('option', { value: d.id, selected: d.id === (note ? note.id_dir : +ctx.query.id_dir || 0) }, d.name)));
  const radios = (label, key, opts) => h('div.field', h('label', label), h('div.chips', opts.map(([v, l]) => h('label.chip', h('input', { type: 'radio', name: key, value: v, checked: st[key] === v, onchange: () => (st[key] = v) }), ' ', l))));
  const u = me() || {};
  const needCap = !note && ((u.rating ?? 0) < 2 || (u.group_access ?? 0) < 6);
  const cap = needCap ? ui.captcha() : null;
  const save = async () => {
    const body = { title: name.value, name: name.value, msg: ed.value, id_dir: +dir.value, ...st, captcha: cap ? cap.value : '' };
    try {
      const r = note ? (await api.put('/api/notes/' + note.id, body), { id: note.id }) : await api.post('/api/notes', body);
      ui.ok(note ? 'Изменения сохранены' : 'Блог успешно создан'); router.go('/notes/' + r.id);
    } catch (e) { if (cap && String(e.message || '').includes('робот')) cap.reset(); throw e; }
  };
  return h('div.card.pad.stack', h('div.field', h('label', 'Заголовок'), name), h('div.field', h('label', 'Текст'), ed.el), h('div.field', h('label', 'Категория'), dir),
    radios('Могут смотреть', 'private', [[0, 'Все'], [1, 'Друзья'], [2, 'Только я']]), radios('Могут комментировать', 'private_komm', [[0, 'Все'], [1, 'Друзья'], [2, 'Только я']]),
    cap ? h('div.field', h('label', 'Проверка'), cap.el) : null, ui.btn(note ? 'Сохранить' : 'Опубликовать', { ic: 'check', kind: 'primary', onclick: save }));
}
router.add('/notes/add', async ctx => { ctx.title('Новый блог'); return [crumbs({ href: '/notes', title: 'Блоги' }, { title: 'Новая запись' }), ui.pageHead('Новая запись'), await noteForm(ctx, null)]; }, { auth: true });
router.add('/notes/:id/edit', async ctx => {
  const n = await api.get('/api/notes/' + ctx.params.id);
  ctx.title('Редактирование');
  if (!n.can_edit) return ui.forbidden();
  return [crumbs({ href: '/notes', title: 'Блоги' }, { href: '/notes/' + n.id, title: n.name }, { title: 'Редактирование' }), ui.pageHead('Редактирование'), await noteForm(ctx, n)];
}, { auth: true });

router.add('/notes/:id/share', async ctx => {
  const n = await api.get('/api/notes/' + ctx.params.id);
  ctx.title('Поделиться');
  const ed = ui.bbEditor({ rows: 4, placeholder: 'Ваш комментарий к репосту (необязательно)' });
  return [crumbs({ href: '/notes', title: 'Блоги' }, { href: '/notes/' + n.id, title: n.name }, { title: 'Поделиться' }), ui.pageHead('Поделиться записью'),
    h('div.card.pad.stack', h('div', ui.userLink(n.user), ' : ', link('/notes/' + n.id, h('b', n.name))), n.can_share ? [ed.el, ui.btn('Поделиться', { ic: 'share', kind: 'primary', onclick: async () => { const r = await api.post(`/api/notes/${n.id}/share`, { text: ed.value }); ui.ok('Запись добавлена в ваш блог'); router.go('/notes/' + r.id); } })] : h('div.muted', n.mine ? 'Нельзя репостить свои записи' : 'Вы уже поделились данной записью'))];
}, { auth: true });
router.add('/notes/forum-share/:id', async ctx => {
  const d = await api.get('/api/notes-forum-share/' + ctx.params.id);
  ctx.title('Поделиться темой');
  const ed = ui.bbEditor({ rows: 4, placeholder: 'Ваш комментарий к репосту (необязательно)' });
  return [ui.pageHead('Поделиться темой форума'), h('div.card.pad.stack', h('div', ui.userLink(d.user), ' : ', link(d.link, h('b', d.title))),
    d.done ? h('div.muted', 'Вы уже поделились данной темой') : [ed.el, ui.btn('Поделиться', { ic: 'share', kind: 'primary', onclick: async () => { const r = await api.post('/api/notes-forum-share/' + ctx.params.id, { text: ed.value }); ui.ok('Тема добавлена в ваш блог'); router.go('/notes/' + r.id); } })])];
}, { auth: true });

router.add('/notes/:id/bookmarks', async ctx => {
  const d = await api.get(`/api/notes/${ctx.params.id}/bookmarks`, { page: ctx.page });
  ctx.title('Добавили в закладки');
  return [crumbs({ href: '/notes/' + ctx.params.id, title: 'Запись' }, { title: 'Закладки' }), ui.pageHead('Добавили в закладки'),
    ui.card(d.items.length ? h('div.list', d.items.map(b => h('div.list-item', ui.avatar(b.user, 36), h('div.main', ui.userLink(b.user)), h('span.small.muted', fmt.ago(b.time))))) : ui.empty('Результатов нет', 'star')), ui.pager(d)];
});

// запись
router.add('/notes/:id', async ctx => {
  const id = ctx.params.id, reload = () => router.render();
  const n = await api.get('/api/notes/' + id, { page: ctx.query.page || 1, tp: ctx.query.tp, sort: ctx.query.sort });
  ctx.title(n.name);
  const path = [{ href: '/notes', title: 'Блоги' }];
  if (n.dir) path.push({ href: '/notes/dirs/' + n.dir.id, title: n.dir.name });
  path.push({ title: n.name });
  const out = [ui.breadcrumbs(path), ui.pageHead(n.name)];
  out.push(h('div.card.pad.stack', h('div.text', { html: n.msg_html }), shareEl(n.share),
    n.text_pages > 1 ? h('div.chips', Array.from({ length: n.text_pages }, (_, k) => h('a.chip' + (n.text_page === k + 1 ? '.active' : ''), { href: `/notes/${id}?tp=${k + 1}` }, 'Стр. ' + (k + 1)))) : null,
    h('div.row.small.muted', ui.userLink(n.user), h('span', icon('clock', 13), ' ', fmt.datetime(n.time)), h('span', icon('eye', 13), ' Рейтинг: ', n.count),
      me() ? (n.can_share ? ui.btn('Поделиться (' + n.shares + ')', { ic: 'share', small: true, href: `/notes/${id}/share` }) : h('span', icon('share', 13), ' Поделились: ', n.shares)) : h('span', icon('share', 13), ' Поделились: ', n.shares))));
  const acts = [];
  if (n.can_edit) acts.push(ui.btn('Редактировать', { ic: 'edit', small: true, href: `/notes/${id}/edit` }));
  if (n.can_delete) acts.push(ui.btn('Удалить', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить блог «${n.name}»?`, { danger: true })) { await api.del('/api/notes/' + id); ui.ok('Блог успешно удалён'); router.go('/notes'); } } }));
  const vote = like => async () => { await api.post(`/api/notes/${id}/like`, { like }); ui.ok('Ваш голос засчитан'); reload(); };
  const voteOk = me() && n.can_like;
  out.push(h('div.row', ...acts, h('span.grow'),
    ui.btn(String(n.likes), { ic: 'thumbs-up', small: true, kind: n.my_like === 1 ? 'primary' : undefined, onclick: voteOk ? vote(true) : undefined }),
    ui.btn(String(n.dislikes), { ic: 'thumbs-down', small: true, kind: n.my_like === 0 ? 'primary' : undefined, onclick: voteOk ? vote(false) : undefined }),
    me() ? ui.btn(n.bookmarked ? 'Из закладок' : 'В закладки', { ic: 'star', small: true, kind: n.bookmarked ? 'primary' : undefined, onclick: async () => { await api.post('/api/bookmark/toggle', { type: 'notes', id: +id }); reload(); } }) : null,
    h('a.badge', { href: `/notes/${id}/bookmarks` }, '(' + n.bookmarks + ')')));
  const k = n.komms;
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Комментарий', maxlength: 1024 });
  let replyTo = 0;
  const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
  out.push(h('h3', 'Комментарии (' + k.total + ')'));
  if (me() && k.total) out.push(chipsRow([['1', 'Внизу'], ['0', 'Вверху']], String(n.sort), v => `/notes/${id}?sort=${v}`));
  out.push(ui.card(k.items.length ? h('div.posts', k.items.map(p => ui.post({ user: p.user, time: p.time, body: h('div.post-body', { html: p.msg }), id: 'k' + p.id, actions: [
    p.can_reply ? ui.btn('', { ic: 'corner-up-left', small: true, onclick: () => answer(p) }) : null,
    p.can_spam ? ui.btn('', { ic: 'flag', small: true, title: 'Пожаловаться', onclick: async () => { const t = await ui.prompt('Опишите нарушение', '', { title: 'Жалоба', multiline: true }); if (t) { await api.post(`/api/notes-komm/${p.id}/spam`, { msg: t, types: 0 }); ui.ok('Заявка на рассмотрение отправлена'); } } }) : null,
    p.can_delete ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/notes-komm/' + p.id); ui.ok('Комментарий удалён'); reload(); } } }) : null].filter(Boolean) }))) : ui.empty('Нет сообщений', 'message')), ui.pager(k));
  if (me()) out.push(n.can_komm ? h('div.card.pad.stack', ed.el, h('div.row', h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/notes/${id}/komm`, { msg: ed.value, reply_to: replyTo }); ui.ok('Сообщение успешно отправлено'); reload(); } }))) : h('div.card.pad.muted', 'Комментирование ограничено автором'));
  else out.push(h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы комментировать.'));
  return out;
});

// ───────── правила сайта (rules.php) и информация (plugins/rules)
router.add('/rules', async ctx => {
  ctx.title('Правила сайта');
  const d = await api.get('/api/rules/site');
  return [ui.pageHead('Правила сайта'), ui.card(h('div.list', d.items.map((t, k) => h('div.list-item', h('div.main', h('span.muted', (k + 1) + ') '), t)))))];
});

const infoPage = async ctx => {
  ctx.title('Информация');
  const reload = () => router.render();
  const d = await api.get('/api/rules');
  const mv = (id, dir) => async () => { await api.post(`/api/rules/${id}/move`, { dir }); reload(); };
  const ctl = r => d.admin ? h('span.row', ui.btn('', { ic: 'arrow-up', small: true, onclick: mv(r.id, 'up') }), ui.btn('', { ic: 'arrow-down', small: true, onclick: mv(r.id, 'down') }),
    ui.btn('', { ic: 'edit', small: true, onclick: () => editRule(r, reload) }),
    ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить пункт?', { danger: true })) { await api.del('/api/rules/' + r.id); ui.ok('Пункт успешно удалён'); reload(); } } })) : null;
  const row = r => h('div.list-item', h('div.main',
    r.type === 'razd' ? h('a.title', { href: '/plugins/rules/' + r.id }, (d.admin ? r.pos + ') ' : ''), r.title)
      : r.type === 'link' ? h('a.title', { href: r.url }, (d.admin ? r.pos + ') ' : ''), r.name_url) : h('div.text', { html: (d.admin ? r.pos + ') ' : '') + r.msg_html })), ctl(r));
  const out = [ui.pageHead('Информация'), ui.card(d.items.length ? h('div.list', d.items.map(row)) : ui.empty('Раздел информации не заполнен', 'info'))];
  if (d.admin) out.push(h('div.card.pad.row', ui.btn('Добавить текст', { ic: 'plus', onclick: () => addRule('msg', reload) }), ui.btn('Добавить пункт', { ic: 'folder', onclick: () => addRule('razd', reload) }), ui.btn('Добавить ссылку', { ic: 'link', onclick: () => addRule('link', reload) })));
  out.push(h('div.card.pad.row', link('/plugins/rules/bb-code', icon('code', 16), ' Справка по BB-кодам'), link('/rules', icon('shield', 16), ' Правила сайта')));
  return out;
};
router.add('/plugins/rules', infoPage);
router.add('/info', infoPage);

function ruleFields(r) {
  const f = {};
  if (r.type === 'razd') f.title = h('input.input', { value: r.title || '', maxlength: 60, placeholder: 'Название пункта' });
  if (r.type === 'msg') f.msg = ui.bbEditor({ value: r.msg || '', rows: 5 });
  if (r.type === 'link') { f.name_url = h('input.input', { value: r.name_url || '', maxlength: 52, placeholder: 'Название ссылки' }); f.url = h('input.input', { value: r.url || '/', placeholder: 'Адрес' }); }
  return f;
}
const ruleBody = f => Object.fromEntries(Object.entries(f).map(([k, v]) => [k, v.value]));
const ruleBox = f => h('div.stack', Object.values(f).map(v => v.el || v));
function addRule(type, reload) {
  const f = ruleFields({ type });
  ui.modal({ title: { msg: 'Добавить текст', razd: 'Добавить пункт', link: 'Добавить ссылку' }[type], body: ruleBox(f), actions: [{ label: 'Отмена' }, { label: 'Добавить', kind: 'primary', onclick: () => { api.post('/api/rules', { type, ...ruleBody(f) }).then(() => { ui.ok('Добавлено'); reload(); }); return false; } }] });
}
function editRule(r, reload) {
  const f = ruleFields(r);
  ui.modal({ title: 'Редактирование', body: ruleBox(f), actions: [{ label: 'Отмена' }, { label: 'Сохранить', kind: 'primary', onclick: () => { api.put('/api/rules/' + r.id, ruleBody(f)).then(() => { ui.ok('Изменения сохранены'); reload(); }); return false; } }] });
}

router.add('/plugins/rules/bb-code', async ctx => {
  ctx.title('BB-коды');
  const rows = [['Стили текста', [['[br]', 'Перенос строки'], ['[b]ваш текст[/b]', '<b>Жирный</b>'], ['[i]ваш текст[/i]', '<i>Курсив</i>'], ['[c]ваш текст[/c]', '<center>По центру</center>'], ['[small]Ваш текст[/small]', '<small>Маленький</small>']]],
    ['Цвета текста', [['[red]ваш текст[/red]', '<span style="color:red">Красный</span>'], ['[green]ваш текст[/green]', '<span style="color:green">Зелёный</span>'], ['[blue]ваш текст[/blue]', '<span style="color:blue">Синий</span>'], ['[yellow]ваш текст[/yellow]', '<span style="color:#d4a800">Жёлтый</span>']]],
    ['Ссылки', [['[url=http://адрес]название[/url]', 'Ссылка'], ['[u=id пользователя]ник[/u]', 'Ссылка на пользователя']]]];
  return [crumbs({ href: '/plugins/rules', title: 'Информация' }, { title: 'BB-коды' }), ui.pageHead('Справка по тегам'),
    rows.map(([t, l]) => ui.section(t, h('div.list', l.map(([code, res]) => h('div.list-item', h('code', code), h('span.grow'), h('span', { html: res }))))))];
});

router.add('/plugins/rules/:id', async ctx => {
  const id = ctx.params.id, reload = () => router.render();
  const d = await api.get('/api/rules/' + id);
  ctx.title(d.title);
  const mv = (p, dir) => async () => { await api.post(`/api/rules-post/${p.id}/move`, { dir }); reload(); };
  const out = [crumbs({ href: '/plugins/rules', title: 'Информация' }, { title: d.title }), ui.pageHead(d.title)];
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(p => h('div.list-item', h('div.main', h('div.text', { html: (d.admin ? p.pos + ') ' : '') + p.msg_html })),
    d.admin ? h('span.row', ui.btn('', { ic: 'arrow-up', small: true, onclick: mv(p, 'up') }), ui.btn('', { ic: 'arrow-down', small: true, onclick: mv(p, 'down') }),
      ui.btn('', { ic: 'edit', small: true, onclick: () => { const e = ui.bbEditor({ value: p.msg, rows: 6 }); ui.modal({ title: 'Редактирование поста', body: e.el, actions: [{ label: 'Отмена' }, { label: 'Сохранить', kind: 'primary', onclick: () => { api.put('/api/rules-post/' + p.id, { msg: e.value }).then(() => { ui.ok('Изменения сохранены'); reload(); }); return false; } }] }); } }),
      ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить пост?', { danger: true })) { await api.del('/api/rules-post/' + p.id); reload(); } } })) : null))) : ui.empty('Нет записей', 'info')));
  if (d.admin) { const e = ui.bbEditor({ rows: 4, placeholder: 'Новый пост' }); out.push(h('div.card.pad.stack', e.el, ui.btn('Опубликовать', { ic: 'check', kind: 'primary', onclick: async () => { await api.post(`/api/rules/${id}/posts`, { msg: e.value }); ui.ok('Ваш пост успешно опубликован'); reload(); } }))); }
  return out;
});

// ───────── смайлы
const smilesIndex = async ctx => {
  ctx.title('Смайлы');
  const reload = () => router.render();
  const d = await api.get('/api/smiles');
  const out = [ui.pageHead('Смайлы'), ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', h('a.row.nowrap.grow', { href: '/plugins/smiles/dir/' + x.id }, h('div.forum-ico', icon('smile', 22)), h('div.main', h('div.title', x.name), x.opis ? h('div.small.muted', x.opis) : null)), h('span.badge', x.count),
    d.admin ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить категорию «${x.name}» со всеми смайлами?`, { danger: true })) { await api.del('/api/smiles/dir/' + x.id); ui.ok('Категория успешно удалена'); reload(); } } }) : null))) : ui.empty('Нет категорий', 'smile'))];
  if (d.admin) {
    const name = h('input.input', { maxlength: 32, placeholder: 'Название категории' });
    out.push(h('details.card.pad', h('summary', icon('plus', 16), ' Добавить категорию'), h('div.row.nowrap', name, ui.btn('Добавить', { kind: 'primary', onclick: async () => { await api.post('/api/smiles/dir', { name: name.value }); ui.ok('Категория создана'); reload(); } }))));
  }
  return out;
};
router.add('/plugins/smiles', smilesIndex);
router.add('/admin/smiles', smilesIndex, { auth: true, access: 'adm_panel_show' });
router.add('/plugins/smiles/dir/:id', async ctx => {
  const id = ctx.params.id, reload = () => router.render();
  const d = await api.get('/api/smiles/dir/' + id, { page: ctx.page });
  ctx.title(d.dir.name + ' | Смайлы');
  const copy = t => { navigator.clipboard && navigator.clipboard.writeText(t).then(() => ui.toast('Скопировано')); };
  const out = [crumbs({ href: '/plugins/smiles', title: 'Смайлы' }, { title: d.dir.name }), ui.pageHead(d.dir.name)];
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(s => h('div.list-item', h('img.smile', { src: s.url, alt: s.smile, style: { height: '28px' } }), h('div.main', h('code', s.smile)), ui.btn('', { ic: 'copy', small: true, onclick: () => copy(s.smile.split('|')[0]) }),
    d.admin ? h('span.row', ui.btn('', { ic: 'edit', small: true, onclick: async () => { const v = await ui.prompt('Код смайла (несколько через |)', s.smile, { title: 'Редактирование смайла' }); if (v) { await api.put('/api/smiles/' + s.id, { smile: v }); ui.ok('Изменения приняты'); reload(); } } }),
      ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить смайл?', { danger: true })) { await api.del('/api/smiles/' + s.id); ui.ok('Смайл успешно удалён'); reload(); } } })) : null))) : ui.empty('Список смайлов пуст', 'smile')), ui.pager(d));
  if (d.admin) {
    const rows = [];
    const box = h('div.stack');
    const add = () => { const f = h('input', { type: 'file', accept: 'image/*' }), c = h('input.input', { maxlength: 32, placeholder: 'Код, например :-) или :-D' }); rows.push({ f, c }); box.append(h('div.row.nowrap', f, c)); };
    add();
    out.push(h('details.card.pad', h('summary', icon('upload', 16), ' Загрузить смайлы'), h('div.stack', box, h('div.row', ui.btn('Ещё файл', { ic: 'plus', small: true, onclick: add }),
      ui.btn('Загрузить', { ic: 'upload', kind: 'primary', onclick: async () => {
        const fd = { forms: rows.length };
        rows.forEach((r, k) => { fd['file_' + k] = r.f.files[0] || ''; fd['smile_' + k] = r.c.value; });
        const r = await api.upload(`/api/smiles/dir/${id}/upload?forms=${rows.length}`, fd);
        r.failed ? ui.toast(`Не выгружено: ${r.failed}`, 'warn') : ui.ok('Выгрузка прошла успешно'); reload();
      } })))));
  }
  return out;
});

// ───────── реклама: переход и вывод
router.add('/go/:id', async ctx => {
  ctx.title('Перенаправление');
  const d = await api.get('/api/go/' + ctx.params.id);
  setTimeout(() => (location.href = d.url), 1500);
  return [ui.pageHead('Перенаправление'), h('div.card.pad.stack', h('div', 'Вы переходите на ', h('b', d.name)), h('a.btn.primary', { href: d.url }, 'Перейти сейчас'))];
});

const reklEl = list => h('div.rekl', list.map(r => h('a.badge.rekl-item', { href: r.link, target: '_blank', rel: 'noopener sponsored' }, r.img ? h('img', { src: r.img, alt: r.name }) : r.name)));
let reklBox = null, reklData = null, reklAt = 0;
async function showRekl(path) {
  try {
    if (!reklData || Date.now() - reklAt > 60000) { reklData = await api.get('/api/rekl/show', null, { silent: true }); reklAt = Date.now(); }
  } catch { return; }
  const foot = document.querySelector('.footer');
  if (!foot) return;
  if (!reklBox) { reklBox = h('div.rekl-wrap'); foot.prepend(reklBox); }
  const list = [...(reklData[path === '/' ? '3' : '4'] || [])];
  reklBox.replaceChildren(...(list.length ? [reklEl(list)] : []));
}
bus.on('route', p => showRekl(p));
document.head.append(h('style', `.share-quote{margin:8px 0 0 12px;padding:6px 0 6px 12px;border-left:3px solid var(--border,#d0d5dd)}.rekl{display:flex;flex-wrap:wrap;gap:6px;justify-content:center;margin-bottom:8px}.rekl-item img{max-height:40px;vertical-align:middle}img.smile{vertical-align:middle;max-height:24px}.note-item .title{font-weight:600}`));

// ───────── админка рекламы
const selNames = { 3: 'Низ сайта (главная)', 4: 'Низ сайта (остальные)' };
router.add('/admin/rekl', async ctx => {
  ctx.title('Реклама');
  const d = await api.get('/api/admin/rekl');
  return [crumbs({ href: '/admin', title: 'Панель управления' }, { title: 'Реклама' }), ui.pageHead('Реклама'), ui.card(h('div.list', d.places.map(p => h('a.list-item', { href: '/admin/rekl/' + p.sel }, h('div.main', h('div.title', p.name)), h('span.badge', p.count)))))];
}, { auth: true, access: 'adm_rekl' });
router.add('/admin/rekl/:sel', async ctx => {
  const sel = +ctx.params.sel, reload = () => router.render();
  const d = await api.get('/api/admin/rekl', { sel, page: ctx.page });
  ctx.title('Реклама — ' + d.name);
  const form = (r, submit) => {
    const name = h('input.input', { value: r.name || '', maxlength: 32, placeholder: 'Название' }), lk = h('input.input', { value: r.real_link || '', maxlength: 64, placeholder: 'Ссылка' }), img = h('input.input', { value: r.img || '', maxlength: 64, placeholder: 'Картинка (адрес, необязательно)' });
    const ch = h('input.input', { type: 'number', min: 0, value: r.id ? 0 : 1, style: { width: '90px' } }), mn = h('select.input', [['1', 'Дней'], ['7', 'Недель'], ['31', 'Месяцев']].map(([v, l]) => h('option', { value: v, selected: v === (r.id ? '1' : '7') }, l)));
    const dop = h('input', { type: 'checkbox', checked: r.id ? r.dop_str : true });
    return h('div.stack', name, lk, img, h('div.row', h('span.small.muted', r.id ? (r.active ? 'Продлить на:' : 'Продлить до:') : 'Срок действия:'), ch, mn), h('label.check', dop, ' Доп. страница (счётчик переходов)'),
      ui.btn(submit, { ic: 'check', kind: 'primary', onclick: () => submit === 'Добавить' ? api.post('/api/admin/rekl', { sel, name: name.value, link: lk.value, img: img.value, ch: +ch.value, mn: +mn.value, dop_str: dop.checked }).then(() => { ui.ok('Рекламная ссылка добавлена'); reload(); })
        : api.put('/api/admin/rekl/' + r.id, { name: name.value, link: lk.value, img: img.value, ch: +ch.value, mn: +mn.value, dop_str: dop.checked }).then(() => { ui.ok('Рекламная ссылка изменена'); reload(); }) }));
  };
  const item = r => h('div.list-item', h('div.main', h('div.title', r.name), h('div.small.muted', r.active ? 'до ' + fmt.datetime(r.time_last) : 'срок показа истёк'), h('div.small.muted', 'Ссылка: ', r.real_link, r.img ? ' · картинка: ' + r.img : '', r.dop_str ? ' · переходов: ' + r.count : ''),
    h('details', h('summary', 'Редактировать'), form(r, 'Сохранить'))), ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить рекламную ссылку?', { danger: true })) { await api.del('/api/admin/rekl/' + r.id); ui.ok('Рекламная ссылка удалена'); reload(); } } }));
  return [crumbs({ href: '/admin', title: 'Панель управления' }, { href: '/admin/rekl', title: 'Реклама' }, { title: selNames[sel] || d.name }), ui.pageHead(selNames[sel] || d.name),
    ui.card(d.items.length ? h('div.list', d.items.map(item)) : ui.empty('Нет результатов', 'megaphone')), ui.pager(d), h('div.card.pad', h('h3', 'Новая реклама'), form({}, 'Добавить'))];
}, { auth: true, access: 'adm_rekl' });

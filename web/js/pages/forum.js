import { h, api, ui, router, state, icon, link, fmt, bus } from '../core.js';

const me = () => state.user;
const crumbs = (...c) => ui.breadcrumbs([{ href: '/forum', title: 'Форум' }, ...c]);
const can = a => me() && (state.user.accesses || []).includes(a);
const themeHref = t => `/forum/${t.id_forum}/${t.id_razdel}/${t.id}`;

state.adminLinks.push({ group: 'Форум', title: 'Настройки форума', href: '/admin/forum', access: 'adm_set_forum', icon: 'forum', desc: 'Счётчик, расширения файлов, номера сообщений' });

const toolbar = (active) => h('div.chips', [['/forum', 'Разделы', 'forum'], ['/forum/new_t', 'Новые темы', 'plus'], ['/forum/new_p', 'Новые сообщения', 'message'], ['/forum/files', 'Файлы', 'paperclip'], ['/forum/on', 'Онлайн', 'users'], ...(me() ? [['/forum/my', 'Мои темы', 'user']] : [])]
  .map(([href, l, ic]) => h('a.chip' + (href === active ? '.active' : ''), { href }, icon(ic, 14), l)));

const searchForm = (inVal = '') => {
  const q = h('input.input', { placeholder: 'Поиск по форуму…', value: '' });
  const go = () => q.value.trim().length >= 3 ? router.go('/forum/search?q=' + encodeURIComponent(q.value.trim()) + (inVal ? '&in=' + inVal : '')) : ui.toast('Введите минимум 3 символа', 'warn');
  q.onkeydown = e => e.key === 'Enter' && go();
  return h('div.row.nowrap', q, ui.btn('', { ic: 'search', onclick: go, kind: 'primary' }));
};

const themeItem = t => h('a.list-item.theme-item', { href: t.link || themeHref(t) },
  h('div.theme-ico', icon(t.close ? 'lock' : t.up ? 'pin' : 'message', 20)),
  h('div.main', h('div.title', t.up ? h('span.badge.ok', 'закреплена') : null, t.close ? h('span.badge.danger', 'закрыта') : null, t.poll ? h('span.badge', 'опрос') : null, ' ', t.name),
    h('div.small.muted', t.forum ? `${t.forum.name} › ${t.razdel.name} · ` : '', 'автор ', t.author.nick, t.last ? ` · последний ${t.last.user.nick || 'Система'}, ${fmt.ago(t.last.time)}` : '')),
  h('span.badge', icon('message', 12), ' ', t.posts));

// ───────── главная
router.add('/forum', async ctx => {
  ctx.title('Форум');
  const reload = () => router.render();
  const d = await api.get('/api/forum');
  const rows = d.items.map(f => h('a.list-item', { href: '/forum/' + f.id },
    h('div.forum-ico', icon('forum', 22)), h('div.main', h('div.title', f.name, f.adm ? h('span.badge', { style: { marginLeft: '8px' } }, 'для администрации') : null), f.opis ? h('div.small.muted', f.opis) : null),
    h('div.small.muted', `${f.themes} тем · ${f.posts} сообщ.`)));
  const create = can('forum_for_create') ? h('details.card.pad', h('summary', icon('plus', 16), ' Создать подфорум'), forumForm({}, async v => { await api.post('/api/forum', v); ui.ok('Подфорум создан'); reload(); }, 'Создать')) : null;
  return [ui.pageHead('Форум'), toolbar('/forum'), searchForm(), ui.card(rows.length ? h('div.list', rows) : ui.empty('Подфорумов пока нет', 'forum')), create,
    d.online.length ? h('div.card.pad.small', icon('users', 14), ' На форуме: ', d.online.map(u => [ui.userLink(u), ' ']), d.online_total > d.online.length ? link('/forum/on', ' и ещё ' + (d.online_total - d.online.length)) : null) : null];
});

function forumForm(f, onSubmit, submit) {
  return ui.form([
    { name: 'name', label: 'Название', value: f.name, maxlength: 32 }, { name: 'opis', label: 'Описание', type: 'textarea', value: f.opis, rows: 3, maxlength: 512 },
    { name: 'pos', label: 'Позиция', type: 'number', value: f.pos ?? 0 }, { name: 'icon', label: 'Иконка (имя файла)', value: f.icon || 'default' },
    ...(me() && state.user.level >= 3 ? [{ name: 'adm', type: 'checkbox', label: 'Только для администрации', value: f.adm }] : []),
  ], { submit, onSubmit: v => onSubmit({ ...v, pos: +v.pos || 0, adm: !!v.adm }) });
}

// ───────── служебные списки (регистрируются до /forum/:f)
const listPage = (path, title, url, active, extra = {}) => router.add(path, async ctx => {
  ctx.title(title);
  const d = await api.get(url, { page: ctx.page, ...(extra.params || {}) });
  return [ui.breadcrumbs([{ href: '/forum', title: 'Форум' }, { title }]), ui.pageHead(title), toolbar(active), ui.card(d.items.length ? h('div.list', d.items.map(themeItem)) : ui.empty('Ничего нет', 'forum')), ui.pager(d)];
}, extra.opts);
listPage('/forum/new_t', 'Новые темы', '/api/forum-new', '/forum/new_t', { params: { kind: 'themes' } });
listPage('/forum/new_p', 'Новые сообщения', '/api/forum-new', '/forum/new_p');
listPage('/forum/my', 'Мои темы', '/api/forum-my', '/forum/my', { opts: { auth: true } });

router.add('/forum/on', async ctx => {
  ctx.title('Кто на форуме');
  const d = await api.get('/api/forum-online', { page: ctx.page });
  return [crumbs({ title: 'Онлайн' }), ui.pageHead('Сейчас на форуме'), toolbar('/forum/on'), ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', ui.avatar(x.user, 40), h('div.main', ui.userLink(x.user)), h('span.small.muted', fmt.ago(x.date_last))))) : ui.empty('Никого нет', 'users')), ui.pager(d)];
});

router.add('/forum/files', async ctx => {
  ctx.title('Файлы форума');
  const d = await api.get('/api/forum-files', { page: ctx.page });
  return [crumbs({ title: 'Файлы' }), ui.pageHead('Файлы форума'), toolbar('/forum/files'),
    ui.card(d.items.length ? h('div.list', d.items.map(f => h('div.list-item', f.image ? h('img.thumb', { src: `${f.url}?s=64`, alt: '' }) : h('div.forum-ico', icon('file', 22)),
      h('div.main', h('a.title', { href: f.url }, f.name + '.' + f.ext), h('div.small.muted', f.size, ' · скачан ', f.count, ' · ', link(f.theme.link, f.theme.name))), h('span.badge', `${f.rating}/${f.votes}`)))) : ui.empty('Файлов пока нет', 'paperclip')), ui.pager(d)];
});

router.add('/forum/search', async ctx => {
  ctx.title('Поиск по форуму');
  const q = ctx.query.q || '', inn = ctx.query.in || '';
  let d = null;
  if (q) d = await api.post('/api/forum-search?page=' + ctx.page, { text: q, in: inn });
  const input = h('input.input', { value: q, placeholder: 'Что ищем?' });
  const go = () => router.go('/forum/search?q=' + encodeURIComponent(input.value) + (inn ? '&in=' + inn : ''));
  input.onkeydown = e => e.key === 'Enter' && go();
  return [crumbs({ title: 'Поиск' }), ui.pageHead('Поиск по форуму'), h('div.card.pad.row.nowrap', input, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: go })),
    d ? ui.card(d.items.length ? h('div.list', d.items.map(t => h('a.list-item', { href: t.link + (t.post ? '?post=' + t.post : '') }, h('div.main', h('div.title', t.name), t.snippet ? h('div.small.muted.clip', t.snippet) : null, h('div.small.muted', t.forum.name + ' › ' + t.razdel.name)), h('span.badge', t.posts)))) : ui.empty('Ничего не найдено', 'search')) : null,
    d ? ui.pager(d) : null];
});

// ───────── подфорум
router.add('/forum/:f', async ctx => {
  const reload = () => router.render();
  const f = await api.get('/api/forum/' + ctx.params.f);
  ctx.title(f.name);
  const rows = f.items.map(r => h('a.list-item', { href: `/forum/${f.id}/${r.id}` }, h('div.forum-ico', icon('folder', 22)), h('div.main', h('div.title', r.name), r.opis ? h('div.small.muted', r.opis) : null), h('div.small.muted', `${r.themes} тем · ${r.posts} сообщ.`)));
  const blocks = [crumbs({ title: f.name }), ui.pageHead(f.name), searchForm('f' + f.id), ui.card(rows.length ? h('div.list', rows) : ui.empty('Разделов пока нет', 'folder'))];
  if (can('forum_razd_create')) blocks.push(h('details.card.pad', h('summary', icon('plus', 16), ' Создать раздел'), ui.form([{ name: 'name', label: 'Название', maxlength: 32 }, { name: 'opis', label: 'Описание', type: 'textarea', rows: 2, maxlength: 256 }], { submit: 'Создать', onSubmit: async v => { await api.post(`/api/forum/${f.id}/razdel`, v); ui.ok('Раздел создан'); reload(); } })));
  if (can('forum_for_edit')) blocks.push(h('details.card.pad', h('summary', icon('edit', 16), ' Настройки подфорума'), forumForm(f, async v => { await api.put('/api/forum/' + f.id, v); ui.ok('Сохранено'); reload(); }, 'Сохранить')));
  if (can('forum_for_delete')) blocks.push(ui.btn('Удалить подфорум', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить подфорум со всеми разделами и темами?', { danger: true })) { await api.del('/api/forum/' + f.id); router.go('/forum'); } } }));
  return blocks;
});

// ───────── раздел
router.add('/forum/:f/:r', async ctx => {
  const reload = () => router.render();
  const d = await api.get(`/api/forum/${ctx.params.f}/${ctx.params.r}`, { page: ctx.page });
  ctx.title(d.razdel.name);
  const base = `/forum/${d.forum.id}/${d.razdel.id}`;
  const blocks = [crumbs({ href: '/forum/' + d.forum.id, title: d.forum.name }, { title: d.razdel.name }), ui.pageHead(d.razdel.name, d.can_create ? ui.btn('Новая тема', { ic: 'plus', kind: 'primary', href: base + '/new' }) : null),
    d.razdel.opis ? h('div.muted', d.razdel.opis) : null, searchForm('r' + d.razdel.id), d.throttled ? h('div.card.pad.muted.small', 'Новую тему можно создавать раз в 10 минут.') : null,
    ui.card(d.items.length ? h('div.list', d.items.map(themeItem)) : ui.empty('Тем пока нет', 'message')), ui.pager(d)];
  if (d.can_edit) {
    blocks.push(h('details.card.pad', h('summary', icon('edit', 16), ' Управление разделом'), h('div.stack',
      ui.form([{ name: 'name', label: 'Название', value: d.razdel.name, maxlength: 32 }, { name: 'opis', label: 'Описание', type: 'textarea', value: d.razdel.opis, rows: 2, maxlength: 256 }], { submit: 'Сохранить', onSubmit: async v => { await api.put(base, v); ui.ok('Сохранено'); reload(); } }),
      h('div.row', ui.btn('Переместить в другой подфорум', { ic: 'folder', onclick: () => moveRazdel(d, base) }),
        ui.btn('Удалить раздел', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить раздел со всеми темами?', { danger: true })) { await api.del(base); router.go('/forum/' + d.forum.id); } } })))));
  }
  return blocks;
});
async function moveRazdel(d, base) {
  const tree = await api.get('/api/forum-tree');
  const sel = h('select.input', tree.map(f => h('option', { value: f.id, selected: f.id === d.forum.id }, f.name)));
  ui.modal({ title: 'Переместить раздел', body: sel, actions: [{ label: 'Отмена' }, { label: 'Переместить', kind: 'primary', onclick: () => { api.post(base + '/move', { forum: +sel.value }).then(r => { ui.ok('Раздел перемещён'); router.go(`/forum/${r.forum}/${d.razdel.id}`); }); } }] });
}

// ───────── новая тема
function pollEditor(init) {
  const text = h('input.input', { placeholder: 'Тема опроса (до 42 символов)', maxlength: 42, value: init?.plain || '' });
  const opts = Array.from({ length: 6 }, (_, i) => h('input.input', { placeholder: `Вариант ${i + 1}` + (i < 2 ? ' *' : ''), maxlength: 23, value: init?.options?.[i]?.plain || '' }));
  const close = h('input', { type: 'checkbox', checked: !!init?.closed });
  return { el: h('div.stack', text, ...opts, init ? h('label.check', close, ' Закрыть опрос') : null), value: () => text.value.trim() ? { text: text.value, options: opts.map(o => o.value), close: close.checked } : null };
}
router.add('/forum/:f/:r/new', async ctx => {
  ctx.title('Новая тема');
  const { f, r } = ctx.params;
  const name = h('input.input', { maxlength: 32, placeholder: 'Название темы (3–32 символа)' });
  const ed = ui.bbEditor({ rows: 10, placeholder: 'Текст темы (от 10 символов)', maxlength: 30000 });
  const poll = pollEditor();
  const withPoll = h('details', h('summary', icon('chart', 15), ' Добавить опрос'), poll.el);
  const send = async () => { const x = await api.post(`/api/forum/${f}/${r}/new`, { name: name.value, msg: ed.value, poll: withPoll.open ? poll.value() : null }); ui.ok('Тема создана'); router.go(x.link); };
  return [crumbs({ href: `/forum/${f}/${r}`, title: 'Раздел' }, { title: 'Новая тема' }), ui.pageHead('Новая тема'), h('div.card.pad.stack', h('div.field', h('label', 'Название'), name), h('div.field', h('label', 'Сообщение'), ed.el), withPoll, ui.btn('Создать тему', { ic: 'send', kind: 'primary', onclick: send }))];
}, { auth: true });

// ───────── тема
router.add('/forum/:f/:r/:t', async ctx => {
  const { f, r, t } = ctx.params;
  const reload = () => router.render();
  const params = { page: ctx.query.page || ctx.page, post: ctx.query.post, order: ctx.query.order };
  const d = await api.get(`/api/forum/${f}/${r}/${t}`, params);
  ctx.title(d.name);
  const base = `/forum/${f}/${r}/${t}`, apiBase = `/api/forum/${f}/${r}/${t}`;
  const ed = ui.bbEditor({ rows: 4, placeholder: 'Ваше сообщение (до 1024 символов)', maxlength: 1024 });
  let cit = 0, replyTo = 0;
  const files = h('input', { type: 'file', multiple: true });
  const citBox = h('div.cit-box.hidden');
  const setCit = p => { cit = p.id; replyTo = p.user.id; citBox.classList.remove('hidden'); citBox.replaceChildren(icon('quote', 14), ' Цитата: ', h('b', p.user.nick || 'Система'), ' ', ui.btn('', { ic: 'x', small: true, onclick: () => { cit = 0; replyTo = 0; citBox.classList.add('hidden'); } })); ed.ta.focus(); };
  const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
  const sendWithFiles = async () => {
    if (!files.files.length) { await api.post(`${apiBase}/post`, { msg: ed.value, cit, reply_to: replyTo }); ui.ok('Сообщение добавлено'); return router.go(`${base}?page=last`, true); }
    const fdm = new FormData(); fdm.append('msg', ed.value); fdm.append('cit', cit); fdm.append('reply_to', replyTo); for (const x of files.files) fdm.append('files', x);
    await fetch(`${apiBase}/post`, { method: 'POST', headers: { 'X-DCMS': '1' }, body: fdm, credentials: 'same-origin' }).then(async r => { if (!r.ok) { const e = await r.json().catch(() => ({})); ui.toast(e.error || 'Ошибка', 'error'); throw new Error(); } });
    ui.ok('Сообщение добавлено'); router.go(`${base}?page=last`, true);
  };
  const posts = d.items.map((p, idx) => forumPost(p, d, { reload, setCit, answer, base }));

  const poll = d.poll ? pollBlock(d, apiBase, reload) : null;
  const head = h('div.card.pad.stack',
    h('div.row', ui.userLink(d.author, { avatar: 36 }), h('span.grow'), h('span.small.muted', fmt.datetime(d.time_create))),
    d.close ? h('div.err-note', icon('lock', 14), ' Тема закрыта для обсуждения', d.closed_by ? ' — ' + d.closed_by.nick : '') : null,
    h('div.theme-text', { html: d.text }),
    d.edited_by ? h('div.small.muted', icon('pencil', 12), ' Изменил(а) ', d.edited_by.nick, ' ', fmt.ago(d.time_edit)) : null);

  const actions = h('div.row', me() ? ui.btn(d.bookmarked ? 'Из закладок' : 'В закладки', { ic: 'bookmark', small: true, onclick: async () => { await api.post(`/api/forum-theme/${d.id}/bookmark`, { on: !d.bookmarked }); reload(); } }) : null,
    me() ? ui.btn('Поделиться', { ic: 'share', small: true, href: `/notes/forum-share/${d.id}` }) : null,
    ui.btn('Скачать текстом', { ic: 'download', small: true, onclick: () => downloadTxt(d) }),
    d.can_edit ? ui.btn('Настройки', { ic: 'settings', small: true, href: base + '/edit' }) : null,
    d.can_edit && !d.poll ? ui.btn('Добавить опрос', { ic: 'chart', small: true, href: base + '/poll' }) : null,
    d.can_delete ? ui.btn('Удалить', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить тему?', { danger: true })) { await api.del(apiBase); router.go(`/forum/${f}/${r}`); } } }) : null);

  const composer = d.can_post ? h('div.card.pad.stack', citBox, ed.el, h('div.row', h('label.btn.sm', icon('paperclip', 14), ' Файлы ', files), h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: sendWithFiles })), d.my_posts ? h('div.small.muted', `Вы опубликовали ${d.my_posts} ${fmt.plural(d.my_posts, 'сообщение', 'сообщения', 'сообщений')}`) : null)
    : me() ? h('div.card.pad.muted', 'Тема закрыта для обсуждения.') : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы писать на форуме.');
  return [crumbs({ href: '/forum/' + d.forum.id, title: d.forum.name }, { href: `/forum/${f}/${r}`, title: d.razdel.name }, { title: d.name }), ui.pageHead(d.name, ...(d.up ? [h('span.badge.ok', 'закреплена')] : [])),
    head, poll, actions, ui.card(posts.length ? h('div.posts', posts) : ui.empty('Сообщений пока нет', 'message')), ui.pager(d), composer];
});

const downloadTxt = d => {
  const text = `${d.name}\n\n${d.raw_text}\n\n` + d.items.map(p => `${p.user.nick || 'Система'} (${fmt.datetime(p.time)}):\n${p.raw || p.msg.replace(/<[^>]+>/g, '')}`).join('\n\n');
  const a = h('a', { href: URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' })), download: `forum_${d.id}.txt` }); a.click();
};

function forumPost(p, d, { reload, setCit, answer }) {
  const act = [];
  if (me() && !p.system && d.can_post) {
    act.push(ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => answer(p) }), ui.btn('', { ic: 'quote', small: true, title: 'Цитировать', onclick: () => setCit(p) }));
  }
  if (me() && !p.system && p.user.id !== me().id) act.push(ui.btn('', { ic: 'flag', small: true, title: 'Пожаловаться', onclick: async () => { const t = await ui.prompt('Опишите нарушение', '', { title: 'Жалоба', multiline: true }); if (t) { await api.post(`/api/forum-post/${p.id}/spam`, { msg: t, types: 0 }); ui.ok('Жалоба отправлена'); } } }));
  if (p.can_edit) act.push(ui.btn('', { ic: 'edit', small: true, title: 'Изменить', onclick: async () => { const t = await ui.prompt('Текст сообщения', p.raw, { title: 'Редактирование', multiline: true }); if (t != null && t !== p.raw) { await api.put('/api/forum-post/' + p.id, { msg: t }); reload(); } } }));
  if (p.can_delete) act.push(ui.btn('', { ic: 'trash', small: true, title: 'Удалить', onclick: async () => { if (await ui.confirm('Удалить сообщение?', { danger: true })) { await api.del('/api/forum-post/' + p.id); reload(); } } }));
  const body = h('div', p.cit ? h('blockquote.cit', h('b', p.cit.user.nick || 'Система'), ' ', h('span.small.muted', fmt.datetime(p.cit.time)), h('div', { html: p.cit.msg })) : null, h('div.post-body' + (p.system ? '.sys' : ''), { html: p.msg }));
  const extra = p.files.length ? h('div.attach', p.files.map(f => h('div.attach-item', f.image ? h('a', { href: `${f.url}?inline=1`, target: '_blank' }, h('img', { src: `${f.url}?s=160`, alt: f.name, loading: 'lazy' })) : icon('file', 20),
    h('div', h('a', { href: f.url }, f.name + '.' + f.ext), h('div.small.muted', f.size_str, ' · скачан ', f.count)),
    h('span.row.small', me() && f.can_vote ? ui.btn('', { ic: 'minus', small: true, onclick: async () => { await api.post(`/api/forum-file/${f.id}/rating`, { rating: 'down' }); reload(); } }) : null, h('b', `${f.rating}/${f.votes}`),
      me() && f.can_vote ? ui.btn('', { ic: 'plus', small: true, onclick: async () => { await api.post(`/api/forum-file/${f.id}/rating`, { rating: 'up' }); reload(); } }) : null,
      d.is_staff ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить вложение?', { danger: true })) { await api.del('/api/forum-file/' + f.id); reload(); } } }) : null)))) : null;
  return ui.post({ user: p.user.id ? p.user : { id: 0, nick: 'Система', pol: 1, online: false }, time: p.time, body, actions: act, extra, id: 'p' + p.id });
}

function pollBlock(d, apiBase, reload) {
  const p = d.poll;
  return ui.section('Опрос: ' + p.plain, h('div.pad.stack', ...p.options.map(o => h('div.poll-opt',
    h('div.row', p.can_vote ? ui.btn(o.plain, { small: true, onclick: async () => { await api.post(`/api/forum-theme/${d.id}/vote/${o.num}`); reload(); } }) : h('span' + (p.my === o.num ? '.b' : ''), o.plain, p.my === o.num ? ' ✓' : ''), h('span.grow'),
      h('span.small.muted', `${o.count} (${o.percent}%)`), o.count ? link(`/forum/${d.forum.id}/${d.razdel.id}/${d.id}/voters/${o.num}`, 'кто') : null),
    h('div.poll-bar', h('div.poll-fill', { style: { width: o.percent + '%' } })))),
    h('div.small.muted', `Всего голосов: ${p.total}`, p.closed ? ' · опрос закрыт' : '')));
}

router.add('/forum/:f/:r/:t/voters/:num', async ctx => {
  const { f, r, t, num } = ctx.params;
  const d = await api.get(`/api/forum-theme/${t}/vote/${num}/users`, { page: ctx.page });
  return [crumbs({ href: `/forum/${f}/${r}/${t}`, title: 'Тема' }, { title: 'Голоса' }), ui.pageHead('Проголосовали за вариант'), ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', ui.avatar(x.user, 36), h('div.main', ui.userLink(x.user)), ui.time(x.time)))) : ui.empty('Пока никто', 'users')), ui.pager(d)];
});

// ───────── настройки темы / опрос
router.add('/forum/:f/:r/:t/edit', async ctx => {
  const { f, r, t } = ctx.params;
  const d = await api.get(`/api/forum/${f}/${r}/${t}`, { page: 1 });
  const apiBase = `/api/forum/${f}/${r}/${t}`;
  const tree = await api.get('/api/forum-tree');
  const fields = [{ name: 'name', label: 'Название', value: d.name, maxlength: 32 }, { name: 'text', label: 'Текст темы', type: 'textarea', rows: 8, value: d.raw_text, maxlength: 30000 },
    ...(d.is_staff ? [{ name: 'up', type: 'checkbox', label: 'Закрепить тему', value: d.up }] : []), { name: 'close', type: 'checkbox', label: 'Закрыть тему', value: d.close },
    ...(me().id !== d.author.id ? [{ name: 'autor', type: 'checkbox', label: 'Сделать себя автором' }] : [])];
  const sel = h('select.input', tree.map(fo => h('optgroup', { label: fo.name }, fo.razdels.map(ra => h('option', { value: ra.id, selected: ra.id === +r }, ra.name)))));
  return [crumbs({ href: `/forum/${f}/${r}/${t}`, title: d.name }, { title: 'Настройки' }), ui.pageHead('Настройки темы'),
    ui.card(h('div.pad', ui.form(fields, { submit: 'Сохранить', onSubmit: async v => { await api.put(apiBase, { name: v.name, text: v.text, up: d.is_staff ? !!v.up : null, close: !!v.close, autor: !!v.autor }); ui.ok('Сохранено'); router.go(`/forum/${f}/${r}/${t}`); } }))),
    ui.section('Перенос темы', h('div.pad.row', sel, ui.btn('Переместить', { ic: 'folder', onclick: async () => { const x = await api.post(apiBase + '/move', { razdel: +sel.value }); ui.ok('Тема перемещена'); router.go(x.link); } })))];
}, { auth: true });

router.add('/forum/:f/:r/:t/poll', async ctx => {
  const { f, r, t } = ctx.params;
  const d = await api.get(`/api/forum/${f}/${r}/${t}`, { page: 1 });
  const ed = pollEditor(d.poll);
  return [crumbs({ href: `/forum/${f}/${r}/${t}`, title: d.name }, { title: 'Опрос' }), ui.pageHead(d.poll ? 'Редактирование опроса' : 'Новый опрос'),
    h('div.card.pad.stack', ed.el, h('div.row', ui.btn('Сохранить', { ic: 'check', kind: 'primary', onclick: async () => { await api.put(`/api/forum-theme/${t}/poll`, ed.value() || { text: '', options: [] }); ui.ok('Опрос сохранён'); router.go(`/forum/${f}/${r}/${t}`); } }),
      d.poll ? ui.btn('Удалить опрос', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить опрос и все голоса?', { danger: true })) { await api.del(`/api/forum-theme/${t}/poll`); router.go(`/forum/${f}/${r}/${t}`); } } }) : null))];
}, { auth: true });

// ───────── админка форума
router.add('/admin/forum', async ctx => {
  ctx.title('Настройки форума');
  const s = await api.get('/api/admin/forum/settings');
  const yn = [{ value: '1', label: 'Да' }, { value: '0', label: 'Нет' }];
  const form = ui.form([
    { name: 'show_num_post', label: 'Показывать номера сообщений', type: 'radio', value: String(s.show_num_post), options: yn },
    { name: 'echo_rassh_forum', label: 'Показывать расширения файлов', type: 'radio', value: String(s.echo_rassh_forum), options: yn },
    { name: 'forum_counter', label: 'Счётчик в меню отключён', type: 'radio', value: String(s.forum_counter), options: yn },
  ], { submit: 'Сохранить', onSubmit: async v => { await api.put('/api/admin/forum/settings', { show_num_post: +v.show_num_post, echo_rassh_forum: +v.echo_rassh_forum, forum_counter: +v.forum_counter }); ui.ok('Настройки сохранены'); } });
  return [ui.pageHead('Настройки форума'), ui.card(h('div.pad', form)),
    ui.section('Обслуживание', h('div.pad.stack', h('p.muted', 'Синхронизация удаляет осиротевшие разделы, темы и сообщения.'),
      ui.btn('Синхронизировать таблицы форума', { ic: 'refresh', onclick: async () => { const r = await api.post('/api/admin/forum/sync'); ui.ok(`Удалено: разделов ${r.razdels}, тем ${r.themes}, сообщений ${r.posts}`); } })))];
}, { auth: true, access: 'adm_set_forum' });

import { h, api, ui, router, state, icon, link, fmt, events, can } from '../core.js';

const me = () => state.user;
const msgHtml = html => h('div.post-body', { html });
state.adminLinks.push(
  { group: 'Чат и гостевая', title: 'Настройки чата', href: '/admin/chat-settings', access: 'adm_set_chat', icon: 'message', desc: 'Боты Умник/Шутник, вопросы, шутки, гостевая' },
  { group: 'Чат и гостевая', title: 'Админ-чат', href: '/admin/chat', access: 'adm_panel_show', icon: 'message-circle', desc: 'Закрытый чат персонала' },
  { group: 'Новости', title: 'Добавить новость', href: '/news/add', access: 'adm_news', icon: 'rss', desc: 'Публикация новостей' });

const mini = (title, ic, ...c) => h('div.card.pad.row.small', icon(ic, 14), title, ...c);

// ───────── главная
router.add('/', async ctx => {
  ctx.title(state.config.site_name || 'Главная');
  const [d, menu, { menuIcon }] = await Promise.all([api.get('/api/home'), api.get('/api/menu'), import('../app.js')]);
  const out = [];
  if (d.news) {
    const n = d.news;
    out.push(h('div.card.pad.news-banner', h('div.row', icon('rss', 18), h('b', n.title), h('span.grow'), me() ? ui.btn('', { ic: 'x', small: true, title: 'Скрыть', onclick: async e => { await api.post('/api/news-read'); router.render(); } }) : null),
      h('div', { html: n.msg }), h('div.small.muted', link('/news/' + n.id, 'Подробнее · комментариев: ' + n.komm))));
  }
  if (me() && d.admin) out.push(h('a.card.pad.admin-btn', { href: '/admin' }, icon('shield', 22), h('div', h('b', 'Панель управления'), h('div.small.muted', 'Доступно только персоналу'))));
  out.push(h('div.grid-links', menu.map(m => h('a.ptile', { href: m.url }, icon(menuIcon(m.icon), 24), h('span', m.name), m.count ? h('span.badge', m.count) : null))));
  const notes = d.blocks && d.blocks.notes;
  if (notes) out.push(ui.section('Блоги', h('div.list', notes.items.length ? notes.items.map(n => h('a.list-item', { href: '/notes/' + n.id }, h('div.main', h('div.title', n.title), h('div.small.muted', n.author.nick, ' · ', n.preview)), h('span.badge', icon('message', 12), ' ', n.comm))) : ui.empty('Записей пока нет', 'book'))));
  out.push(h('div.card.pad.small.muted.row', icon('users', 14), `Онлайн: ${d.users_online}`, d.guests_online ? ` · гостей: ${d.guests_online}` : '', h('span.grow'), state.config.site_name || ''));
  return out;
});

// ───────── общие блоки
const whoPage = (path, url, title, back) => router.add(path, async ctx => {
  ctx.title(title);
  const d = await api.get(url, { page: ctx.page });
  return [ui.breadcrumbs([{ href: back, title: back === '/chat' ? 'Чат' : 'Гостевая' }, { title }]), ui.pageHead(title),
    ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', ui.avatar(x.user, 40), h('div.main', ui.userLink(x.user)), h('span.small.muted', fmt.ago(x.date_last))))) : ui.empty('Никого нет', 'users')), ui.pager(d)];
});
whoPage('/chat/who', '/api/chat-who', 'Кто в чате', '/chat');
whoPage('/guest/who', '/api/guest-who', 'Кто в гостевой', '/guest');

function postCard(p, { reply, del, extraActions = [] }) {
  const act = [...extraActions];
  if (me() && p.can_reply && reply) act.push(ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => reply(p) }));
  if (p.can_delete && del) act.push(ui.btn('', { ic: 'trash', small: true, title: 'Удалить', onclick: () => del(p) }));
  return ui.post({ user: p.user.id ? p.user : { id: 0, nick: p.user.nick, pol: 1, online: false }, time: p.time, body: msgHtml(p.msg), actions: act, id: 'p' + p.id });
}

// ───────── чат
router.add('/chat', async ctx => {
  ctx.title('Чат');
  const d = await api.get('/api/chat');
  const reload = () => router.render();
  const rows = d.items.map(r => h('a.list-item', { href: '/chat/' + r.id }, h('div.forum-ico', icon(r.umnik ? 'zap' : r.shutnik ? 'smile' : 'message', 22)),
    h('div.main', h('div.title', r.name), r.opis ? h('div.small.muted', r.opis) : null, r.bots ? h('div.small', r.umnik ? h('span.badge.ok', 'Умник') : null, ' ', r.shutnik ? h('span.badge', 'Шутник') : null) : null),
    h('span.badge', icon('users', 12), ' ', r.online)));
  const out = [ui.pageHead('Чат', d.can_clear ? ui.btn('Очистить всё', { ic: 'trash', kind: 'danger', small: true, onclick: async () => { if (await ui.confirm('Удалить все сообщения всех комнат?', { danger: true })) { await api.post('/api/chat-clear'); ui.ok('Очищено'); } } }) : null),
    ui.card(h('div.list', rows)), mini(`Всего в чате: ${d.online} · `, 'users', link('/chat/who', 'кто в чате'))];
  if (d.can_manage) {
    const form = (r = {}) => ui.form([{ name: 'name', label: 'Название', value: r.name, maxlength: 32 }, { name: 'opis', label: 'Описание', type: 'textarea', rows: 2, value: r.opis },
      { name: 'pos', label: 'Позиция', type: 'number', value: r.pos ?? 0 },
      { name: 'bots', label: 'Боты', type: 'select', value: String(r.bots ?? 0), options: [{ value: '0', label: 'Нет' }, { value: '1', label: 'Умник' }, { value: '2', label: 'Шутник' }, { value: '3', label: 'Умник и Шутник' }] }],
      { submit: r.id ? 'Сохранить' : 'Создать', onSubmit: async v => { const b = { ...v, pos: +v.pos || 0, bots: +v.bots }; r.id ? await api.put('/api/chat-room/' + r.id, b) : await api.post('/api/chat-room', b); ui.ok('Готово'); reload(); } });
    out.push(h('details.card.pad', h('summary', icon('plus', 16), ' Добавить комнату'), form()));
    out.push(ui.section('Управление комнатами', h('div.pad.stack', d.items.map(r => h('details', h('summary', r.name), form(r), ui.btn('Удалить комнату', { ic: 'trash', kind: 'danger', small: true, onclick: async () => { if (await ui.confirm('Удалить комнату и её сообщения?', { danger: true })) { await api.del('/api/chat-room/' + r.id); reload(); } } }))))));
  }
  return out;
});

router.add('/chat/:id', async ctx => {
  const id = ctx.params.id;
  const first = await api.get('/api/chat/' + id, { page: ctx.page });
  ctx.title(first.room.name);
  const list = h('div.chat-list'), pagerBox = h('div');
  let privat = 0;
  const ed = h('textarea.input', { rows: 2, placeholder: 'Сообщение…', maxlength: 512 });
  const toBox = h('div.small.muted.hidden');
  const setTo = p => { privat = p.user.id; toBox.classList.remove('hidden'); toBox.replaceChildren('Приватно для ', h('b', p.user.nick), ' ', ui.btn('', { ic: 'x', small: true, onclick: () => { privat = 0; toBox.classList.add('hidden'); } })); ed.value = `${p.user.nick}, ` + ed.value; ed.focus(); };
  const paint = d => {
    list.replaceChildren(...(d.items.length ? d.items.map(p => postCard(p, { reply: p.bot ? null : setTo, del: async x => { await api.del('/api/chat-post/' + x.id); load(); },
      extraActions: p.private ? [h('span.badge', icon('lock', 12), p.to_me ? ' вам' : ' приватно')] : [] })) : [ui.empty('Сообщений пока нет', 'message')]));
    pagerBox.replaceChildren(...[ui.pager(d)].filter(Boolean));
  };
  const load = async () => { try { paint(await api.get('/api/chat/' + id, { page: ctx.page }, { silent: true })); } catch { } };
  paint(first);
  const send = async () => { if (!ed.value.trim()) return; await api.post('/api/chat/' + id, { msg: ed.value, privat }); ed.value = ''; privat = 0; toBox.classList.add('hidden'); load(); };
  ed.onkeydown = e => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) send(); };
  const off = events.join('chat:' + id, () => ctx.page === 1 && load());
  const t = setInterval(load, Math.max(5, first.refresh || 10) * 1000);
  ctx.onLeave(() => { off(); clearInterval(t); });
  return [ui.breadcrumbs([{ href: '/chat', title: 'Чат' }, { title: first.room.name }]), ui.pageHead(first.room.name, ui.btn('', { ic: 'refresh', small: true, title: 'Обновить', onclick: load })),
    first.can_post ? h('div.card.pad.stack', toBox, ed, h('div.row', h('span.small.muted', 'Ctrl+Enter — отправить'), h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: send }))) : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы писать в чат.'),
    ui.card(list), pagerBox, mini('Сейчас в чате: ', 'users', link('/chat/who', 'список'))];
});

// ───────── гостевая / админ-чат
function wall(kind) {
  const adm = kind === 'adm';
  const base = adm ? '/api/adm-chat' : '/api/guest';
  return async ctx => {
    ctx.title(adm ? 'Админ-чат' : 'Гостевая');
    const d = await api.get(base, { page: ctx.page });
    const reload = () => router.render();
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Ваше сообщение', maxlength: 1024 });
    let replyTo = 0;
    const cap = h('input.input', { placeholder: 'Число с картинки', maxlength: 8, autocomplete: 'off' });
    const img = h('img', { src: '/api/captcha?' + Date.now(), alt: 'код', style: { height: '44px', cursor: 'pointer', borderRadius: '8px' } }); img.onclick = () => (img.src = '/api/captcha?' + Date.now());
    const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
    const send = async () => { await api.post(base, { msg: ed.value, reply_to: replyTo, captcha: cap.value }); ui.ok('Сообщение добавлено'); ed.value = ''; cap.value = ''; replyTo = 0; ctx.page === 1 ? reload() : router.go(location.pathname); };
    const form = d.can_write ? h('div.card.pad.stack', ed.el, d.captcha ? h('div.row', img, cap) : null, d.wait ? h('div.small.muted', `Гостям можно писать раз в 5 минут. Подождите ${d.wait} сек.`) : null, h('div.row', h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: send })))
      : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы писать в гостевую.');
    const out = [ui.pageHead(adm ? 'Админ-чат' : 'Гостевая книга', h('div.row', !adm ? link('/guest/top', 'Топ') : null)), form,
      ui.card(d.items.length ? h('div.posts', d.items.map(p => postCard(p, { reply: answer, del: async x => { if (await ui.confirm('Удалить сообщение?', { danger: true })) { await api.del(base + '/' + x.id); reload(); } } }))) : ui.empty('Сообщений нет', 'message')), ui.pager(d),
      h('div.card.pad.small.muted.row', icon('users', 14), 'Сейчас читают: ', d.reading, !adm ? link('/guest/who', ' список') : null)];
    if (!adm && d.can_clear) {
      const n = h('input.input', { type: 'number', value: 1, min: 1, style: { width: '80px' } }), u = h('select.input', h('option', { value: 'sut' }, 'суток'), h('option', { value: 'mes' }, 'месяцев'));
      out.push(h('details.card.pad', h('summary', icon('trash', 16), ' Очистка гостевой'), h('div.row', 'Удалить сообщения старше', n, u, ui.btn('Очистить', { kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить старые сообщения?', { danger: true })) { const r = await api.post('/api/guest-clear', { write: +n.value, unit: u.value }); ui.ok('Удалено: ' + r.deleted); reload(); } } }))));
    }
    return out;
  };
}
router.add('/guest/top', async ctx => {
  ctx.title('Топ гостевой');
  const d = await api.get('/api/guest-top', { page: ctx.page });
  return [ui.breadcrumbs([{ href: '/guest', title: 'Гостевая' }, { title: 'Топ' }]), ui.pageHead('Самые активные'), ui.card(d.items.length ? h('div.list', d.items.map((x, k) => h('div.list-item', h('b', (ctx.page - 1) * d.per + k + 1 + '.'), ui.avatar(x.user, 36), h('div.main', ui.userLink(x.user)), h('span.badge', x.total)))) : ui.empty('Пусто', 'users')), ui.pager(d)];
});
router.add('/guest', wall('guest'));
router.add('/admin/chat', wall('adm'), { auth: true, access: 'adm_panel_show' });

// ───────── новости
function newsForm(n, onSubmit, submit) {
  const ed = ui.bbEditor({ rows: 10, value: n.raw || '', placeholder: 'Текст новости' });
  const title = h('input.input', { value: n.title || '', maxlength: 50, placeholder: 'Заголовок' });
  const lnk = h('input.input', { value: n.link || '', placeholder: 'Ссылка (необязательно)' });
  const days = h('input.input', { type: 'number', min: 0, value: 0, style: { width: '90px' } });
  const unit = h('select.input', h('option', { value: 1 }, 'дней'), h('option', { value: 7 }, 'недель'), h('option', { value: 31 }, 'месяцев'));
  const mail = h('input', { type: 'checkbox' });
  return h('div.card.pad.stack', h('div.field', h('label', 'Заголовок'), title), h('div.field', h('label', 'Текст'), ed.el), h('div.field', h('label', 'Ссылка'), lnk),
    h('div.row', 'Показывать на главной:', days, unit), !n.id ? h('label.check', mail, ' Разослать на e-mail подписчикам') : null,
    ui.btn(submit, { ic: 'check', kind: 'primary', onclick: () => onSubmit({ title: title.value, msg: ed.value, link: lnk.value, days: +days.value || 0, unit: +unit.value, mail: mail.checked }) }));
}
router.add('/news', async ctx => {
  ctx.title('Новости');
  const d = await api.get('/api/news', { page: ctx.page });
  return [ui.pageHead('Новости', d.can_add ? ui.btn('Добавить', { ic: 'plus', kind: 'primary', href: '/news/add' }) : null),
    ...(d.items.length ? d.items.map(n => h('div.card.pad.stack', h('div.row', h('a.title', { href: '/news/' + n.id }, h('b', n.title)), n.main ? h('span.badge.ok', 'на главной') : null, h('span.grow'), h('span.small.muted', fmt.datetime(n.time))),
      h('div', { html: n.msg }), h('div.small.muted.row', ui.userLink(n.author), h('span.grow'), link('/news/' + n.id, 'Комментарии: ' + n.komm)))) : [ui.card(ui.empty('Новостей пока нет', 'rss'))]), ui.pager(d)];
});
router.add('/news/add', async ctx => {
  ctx.title('Новая новость');
  return [ui.breadcrumbs([{ href: '/news', title: 'Новости' }, { title: 'Добавление' }]), ui.pageHead('Добавить новость'), newsForm({}, async v => { const r = await api.post('/api/news', v); ui.ok('Новость добавлена'); router.go('/news/' + r.id); }, 'Опубликовать')];
}, { auth: true, access: 'adm_news' });
router.add('/news/:id/edit', async ctx => {
  const n = await api.get('/api/news/' + ctx.params.id);
  ctx.title('Изменение новости');
  return [ui.breadcrumbs([{ href: '/news', title: 'Новости' }, { href: '/news/' + n.id, title: n.title }, { title: 'Изменение' }]), ui.pageHead('Изменить новость'),
    newsForm(n, async v => { await api.put('/api/news/' + n.id, v); ui.ok('Сохранено'); router.go('/news/' + n.id); }, 'Сохранить')];
}, { auth: true, access: 'adm_news' });
router.add('/news/:id', async ctx => {
  const n = await api.get('/api/news/' + ctx.params.id, { page: ctx.page });
  ctx.title(n.title);
  const reload = () => router.render();
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Комментарий', maxlength: 1024 });
  let replyTo = 0;
  const vote = async like => { await api.post(`/api/news/${n.id}/like`, { like }); reload(); };
  const voted = n.my_like >= 0;
  const out = [ui.breadcrumbs([{ href: '/news', title: 'Новости' }, { title: n.title }]),
    h('div.card.pad.stack', h('div.row', h('h2', n.title), h('span.grow'), n.can_edit ? h('div.row', ui.btn('', { ic: 'edit', small: true, href: `/news/${n.id}/edit` }), ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить новость?', { danger: true })) { await api.del('/api/news/' + n.id); router.go('/news'); } } })) : null),
      h('div', { html: n.msg }), n.link ? h('div', link(n.link, n.link)) : null,
      h('div.row.small.muted', ui.userLink(n.author), h('span', fmt.datetime(n.time)), h('span.grow'),
        ui.btn(String(n.likes), { ic: 'thumbs-up', small: true, kind: n.my_like === 1 ? 'primary' : undefined, onclick: me() && !voted ? () => vote(true) : undefined }),
        ui.btn(String(n.dislikes), { ic: 'thumbs-down', small: true, kind: n.my_like === 0 ? 'primary' : undefined, onclick: me() && !voted ? () => vote(false) : undefined })),
      h('div.row', n.newer ? link('/news/' + n.newer, '← Новее') : null, h('span.grow'), h('span.small.muted', `${n.pos} из ${n.count}`), h('span.grow'), n.older ? link('/news/' + n.older, 'Старее →') : null))];
  const k = n.komms;
  const sortBar = me() ? h('div.chips', ['Вверху', 'Внизу'].map((l, v) => h('a.chip' + (n.sort === v ? '.active' : ''), { href: `/news/${n.id}?sort=${v}` }, l))) : null;
  const answer = p => { replyTo = p.user.id; ed.ta.value = `[b]${p.user.nick}[/b], `; ed.ta.focus(); };
  const send = async () => { await api.post(`/api/news/${n.id}/komm`, { msg: ed.value, reply_to: replyTo }); ui.ok('Комментарий добавлен'); reload(); };
  out.push(me() ? h('div.card.pad.stack', ed.el, h('div.row', h('span.grow'), ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: send }))) : h('div.card.pad.muted', link('/login', 'Войдите'), ', чтобы комментировать.'));
  out.push(h('h3', 'Комментарии (' + k.total + ')'), sortBar, ui.card(k.items.length ? h('div.posts', k.items.map(p => postCard(p, { reply: answer, del: async x => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/news-komm/' + x.id); reload(); } } }))) : ui.empty('Комментариев нет', 'message')), ui.pager(k));
  return out;
});

// ───────── настройки чата (админка)
router.add('/admin/chat-settings', async ctx => {
  ctx.title('Настройки чата');
  const s = await api.get('/api/admin/chat/settings');
  const num = (name, label, hint) => ({ name, label, type: 'number', value: s[name], hint });
  const form = ui.form([num('time_chat', 'Автообновление чата (сек)'), num('umnik_new', 'Умник: пауза между вопросами (сек)'), num('umnik_help', 'Умник: интервал подсказок (сек)'), num('umnik_time', 'Умник: время на вопрос (сек)'), num('shutnik_new', 'Шутник: интервал анекдотов (сек)'),
    { name: 'chat_umnik', label: 'Имя Умника', value: s.chat_umnik }, { name: 'chat_shutnik', label: 'Имя Шутника', value: s.chat_shutnik },
    { name: 'write_guest', label: 'Гости могут писать в гостевую', type: 'select', value: String(s.write_guest), options: [{ value: '0', label: 'Нет' }, { value: '1', label: 'Да' }] }],
    { submit: 'Сохранить', onSubmit: async v => { await api.put('/api/admin/chat/settings', { ...Object.fromEntries(['time_chat', 'umnik_new', 'umnik_help', 'umnik_time', 'shutnik_new'].map(k => [k, +v[k]])), chat_umnik: v.chat_umnik, chat_shutnik: v.chat_shutnik, write_guest: +v.write_guest }); ui.ok('Настройки сохранены'); } });
  const imp = (kind, label, hint) => {
    const f = h('input', { type: 'file', accept: '.txt' }), sep = h('input.input', { value: '|', style: { width: '70px' } }), rep = h('input', { type: 'checkbox' });
    return h('div.stack', h('b', label + ` (сейчас: ${kind === 'vopros' ? s.vopros : s.shutki})`), h('div.small.muted', hint), f, kind === 'vopros' ? h('div.row', 'Разделитель:', sep) : null, h('label.check', rep, ' Заменить существующие'),
      h('div.row', ui.btn('Загрузить', { ic: 'upload', onclick: async () => { if (!f.files[0]) return ui.toast('Выберите файл', 'warn'); const r = await api.upload('/api/admin/chat/import', { file: f.files[0], kind, sep: sep.value, replace: rep.checked ? '1' : '0' }); ui.ok('Добавлено: ' + r.added); router.render(); } }),
        ui.btn('Очистить', { kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить все записи?', { danger: true })) { await api.post('/api/admin/chat/clear-bots?kind=' + kind); router.render(); } } })));
  };
  return [ui.pageHead('Настройки чата и гостевой'), ui.card(h('div.pad', form)), ui.section('Вопросы Умника', h('div.pad', imp('vopros', 'Файл: «вопрос|ответ» построчно', 'Кодировка UTF-8'))), ui.section('Анекдоты Шутника', h('div.pad', imp('shutki', 'Файл: по анекдоту в строке', 'От 10 символов в строке')))];
}, { auth: true, access: 'adm_set_chat' });

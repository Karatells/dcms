import { h, api, ui, router, state, icon, link, fmt, bus, pagedList, can } from '../core.js';

const me = () => state.user;
const MONTHS = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
const plural = fmt.plural;

// ───────── общие кирпичики
const infoRow = (label, value) => (value || value === 0) ? h('div.krow', h('div.k', label), h('div.v', value)) : null;
const medal = n => n > 0 ? h('span.medal', { title: 'Рейтинг' }, icon('star', 14), n) : null;

function friendButton(u, st, reload) {
  const act = (url, method = 'post', msg) => async () => { await api[method](url); msg && ui.ok(msg); reload(); };
  switch (st) {
    case 'none': return ui.btn('В друзья', { ic: 'plus', kind: 'primary', onclick: act(`/api/frends/${u.id}/add`, 'post', 'Заявка отправлена') });
    case 'sent': return ui.btn('Отменить заявку', { ic: 'x', onclick: act(`/api/frends/${u.id}/cancel`) });
    case 'incoming': return h('span.row', ui.btn('Принять', { ic: 'check', kind: 'primary', onclick: act(`/api/frends/${u.id}/accept`, 'post', 'Теперь вы друзья') }), ui.btn('Отклонить', { onclick: act(`/api/frends/${u.id}/decline`) }));
    case 'friend': return ui.btn('Удалить из друзей', { ic: 'users', onclick: async () => { if (await ui.confirm('Удалить пользователя из друзей?', { danger: true })) act(`/api/frends/${u.id}`, 'del')(); } });
  }
  return null;
}

// ───────── страница пользователя
router.add('/id:id', async ctx => {
  const id = ctx.params.id;
  const render = async () => {
    const d = await api.get('/api/user/' + id);
    const u = d.user;
    ctx.title(u.nick);
    const self = d.self;
    const head = h('div.card.profile-head',
      h('div.profile-cover'),
      h('div.profile-main',
        h('div.profile-ava', ui.avatar(u, 112)),
        h('div.profile-name',
          h('h1', u.nick, ' ', medal(u.medal)),
          h('div.muted.small', u.group ? ui.badge(u.group, u.level >= 2 ? 'adm' : u.level >= 1 ? 'mod' : '') : null, ' ',
            u.online ? h('span.online-dot', 'сейчас на сайте') : h('span', 'был' + (u.pol ? '' : 'а') + ' ' + fmt.ago(d.date_last))),
          d.status ? h('div.profile-status', { html: d.status.msg }) : null),
        h('div.profile-actions', self
          ? [ui.btn('Редактировать', { ic: 'edit', href: '/user/info/edit' }), ui.btn('Настройки', { ic: 'settings', href: '/user/settings' })]
          : [me() ? ui.btn('Написать', { ic: 'mail', kind: 'primary', href: '/mail/' + u.id }) : null,
             me() ? friendButton(u, d.friend, render) : null,
             me() ? ui.btn('', { ic: d.favorite ? 'heart' : 'bookmark', title: d.favorite ? 'Убрать из закладок' : 'В закладки', onclick: async () => { await api.post(`/api/user/${u.id}/favorite`, { on: !d.favorite }); render(); } }) : null,
             me() ? ui.btn('', { ic: 'gift', title: 'Подарить подарок', href: '/user/gift/' + u.id }) : null,
             can('user_prof_edit') ? ui.btn('', { ic: 'settings', title: 'Редактировать (админ)', href: '/admin/user/' + u.id }) : null,
             can('user_ban_set') || can('user_ban_set_h') ? ui.btn('', { ic: 'gavel', title: 'Забанить', href: '/admin/ban/' + u.id }) : null])));
    const out = [head];
    if (d.banned) out.push(h('div.card.pad.err', icon('gavel'), ' Этот пользователь заблокирован', d.banned.forever ? ' навсегда' : ' до ' + fmt.datetime(d.banned.until), d.banned.reason ? h('div.small', 'Причина: ' + d.banned.reason) : null));
    if (!d.access.ok) {
      out.push(h('div.empty', icon('lock', 36), h('div', d.access.state === 'friends' ? 'Страница доступна только друзьям пользователя' : 'Пользователь закрыл свою страницу'),
        me() && d.friend === 'none' ? friendButton(u, d.friend, render) : null));
      return swap(out);
    }
    // статус (форма для себя)
    if (self) out.push(statusForm(d.status, render));
    else if (d.status && me()) out.push(statusVote(d, render));

    // статистика
    const stats = [
      ['star', 'Рейтинг', fmt.num(d.rating)], ['coins', 'Баллы', fmt.num(d.balls)], ['calendar', 'Регистрация', fmt.date(d.date_reg)],
      ...(d.money !== undefined ? [['coins', 'Монеты', fmt.num(d.money)]] : []),
    ];
    out.push(h('div.stat-row', stats.map(([ic, k, v]) => h('div.stat', icon(ic, 18), h('div.v', v), h('div.k', k)))));

    // анкета
    const i = d.info;
    const bday = i.day && i.month ? `${i.day} ${MONTHS[i.month - 1]}${i.year ? ' ' + i.year : ''}` : i.year ? String(i.year) : '';
    const rows = [
      infoRow('Имя', i.name), infoRow('Пол', u.pol ? 'Мужской' : 'Женский'), infoRow('Возраст', i.age ? `${i.age} ${plural(i.age, 'год', 'года', 'лет')}` : ''),
      infoRow('Дата рождения', bday), infoRow('Город', i.city), infoRow('Семейное положение', i.marital), infoRow('Дети', i.baby),
      infoRow('Курение', i.smok), infoRow('Алкоголь', i.alko), infoRow('Занятие', i.job),
      infoRow('Цели знакомства', i.meet && i.meet.length ? i.meet.join(', ') : ''), infoRow('О себе', i.about),
      infoRow('Skype', i.skype), infoRow('Telegram', i.telegram), infoRow('VK', i.vk), infoRow('Телефон', i.phone), infoRow('E-mail', i.email),
      d.url ? infoRow('Сейчас на странице', h('code', d.url)) : null,
    ].filter(Boolean);
    out.push(ui.section('Анкета', h('div.krows.pad', rows.length ? rows : h('span.muted', 'Анкета не заполнена'))));
    if (d.staff) out.push(ui.section('Для администрации', h('div.krows.pad', infoRow('IP', d.staff.ip), infoRow('Браузер', d.staff.ua), infoRow('E-mail', d.staff.email), infoRow('Монеты', d.staff.money), infoRow('Группа', u.group))));

    // разделы
    if (d.items && d.items.length) out.push(h('div.grid-links', d.items.map(it => h('a.ptile', { href: it.link }, icon(it.icon || 'folder', 22), h('div.t', it.title), h('div.c', fmt.num(it.count))))));

    // оценка анкеты
    out.push(ratingBox(d, render));

    // стена
    out.push(await wallBlock(d, render));
    return swap(out);
  };
  const root = h('div.stack');
  const swap = nodes => { root.replaceChildren(...nodes); return root; };
  await render();
  return root;
});

function statusForm(st, reload) {
  const input = h('input.input', { value: st ? st.raw : '', placeholder: 'Что у вас нового? Статус на странице', maxlength: 512 });
  const send = async () => { await api.post('/api/status', { msg: input.value }); ui.ok(input.value.trim() ? 'Статус обновлён' : 'Статус убран'); reload(); };
  input.onkeydown = e => e.key === 'Enter' && send();
  return h('div.card.pad.row.nowrap', icon('message', 18), input, ui.btn('Сохранить', { kind: 'primary', onclick: send }),
    st ? ui.btn('', { ic: 'message', title: 'Комментарии', href: '/user/status/' + st.id }) : null);
}
function statusVote(d, reload) {
  const s = d.status;
  const vote = like => async () => { try { await api.post(`/api/status/${s.id}/like`, { like }); reload(); } catch (e) { } };
  return h('div.card.pad.row', h('span.grow.muted.small', 'Статус пользователя'),
    ui.reaction('thumbs-up', s.likes, s.my_like === 2, s.my_like ? null : vote(1), 'Нравится'),
    ui.reaction('thumbs-down', 0, s.my_like === 1, s.my_like ? null : vote(0), 'Не нравится'),
    link('/user/status/' + s.id, h('span.btn.sm', icon('message', 14), ' ' + fmt.num(s.komm))));
}

function ratingBox(d, reload) {
  const v = d.votes;
  const body = h('div.pad.stack',
    h('div.row', h('span.badge.ok', '+ ' + v.plus), h('span.badge', '± ' + v.neutral), h('span.badge.danger', '− ' + v.minus), h('span.grow'), link('/user/info/who_rating/' + d.user.id, 'Кто оценил')));
  if (me() && !d.self) {
    const mk = (val, label) => ui.btn(label, { small: true, kind: d.my_vote === val ? 'primary' : '', onclick: async () => { await api.post(`/api/user/${d.user.id}/rating`, { rating: val }); reload(); } });
    body.append(d.can_vote ? h('div.row', h('span.muted.small', 'Ваша оценка анкеты:'), mk(-2, '−2'), mk(-1, '−1'), mk(0, '0'), mk(1, '+1'), mk(2, '+2'))
      : h('div.muted.small', 'Оценивать анкеты можно от 50 баллов.'));
  }
  return ui.section('Оценки анкеты', body);
}

async function wallBlock(d, reload) {
  const u = d.user;
  const wrap = h('div');
  const load = async page => {
    const data = await api.get(`/api/user/${u.id}/stena`, { page, per: 10 });
    const list = data.items.map(p => stenaPost(p, d, () => load(page)));
    const composer = me() ? stenaComposer(u, () => load(1)) : null;
    wrap.replaceChildren(h('div.card-title', 'Стена ', h('span.muted', fmt.num(data.total))), composer,
      list.length ? h('div.posts', list) : ui.empty('Сообщений пока нет', 'message'), ui.pager(data, { onPage: load }));
  };
  await load(1);
  return h('section.card', wrap);
}
export function stenaComposer(u, done, preset = '', replyTo = 0) {
  const ed = ui.bbEditor({ value: preset, rows: 3, placeholder: 'Написать на стене…', maxlength: 1024 });
  const send = async () => { await api.post(`/api/user/${u.id}/stena`, { msg: ed.value, reply_to: +ed.ta.dataset.reply || replyTo }); ed.value = ''; delete ed.ta.dataset.reply; ui.ok('Сообщение добавлено'); done(); };
  return h('div.pad.stack', ed.el, h('div.row', ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: send })));
}
function stenaPost(p, d, reload) {
  const u = d.user, viewer = me();
  const act = [
    ui.reaction('heart', p.likes, p.liked, viewer && !p.liked ? async () => { await api.post(`/api/stena/${p.id}/like`); reload(); } : null, 'Нравится'),
    viewer && p.user.id !== viewer.id ? ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => { const box = document.querySelector('.card textarea'); if (box) { box.value = `[b]${p.user.nick}[/b], `; box.focus(); box.dataset.reply = p.user.id; } } }) : null,
    viewer && p.user.id !== viewer.id ? ui.btn('', { ic: 'flag', small: true, title: 'Пожаловаться', onclick: () => complain(`/api/stena/${p.id}/spam`) }) : null,
    viewer && (viewer.id === u.id || (state.user.accesses || []).includes('guest_delete')) ? ui.btn('', { ic: 'trash', small: true, title: 'Удалить', onclick: async () => { if (await ui.confirm('Удалить сообщение?', { danger: true })) { await api.del('/api/stena/' + p.id); reload(); } } }) : null,
  ];
  return ui.post({ user: p.user, time: p.time, body: ui.html(p.msg), actions: act, id: 'p' + p.id });
}
export async function complain(url) {
  const msg = await ui.prompt('Опишите нарушение', '', { title: 'Жалоба', multiline: true });
  if (!msg) return;
  await api.post(url, { msg, types: 0 });
  ui.ok('Жалоба отправлена модераторам');
}

// ───────── стена отдельной страницей
router.add('/user/stena/:id', async ctx => {
  const u = (await api.get('/api/user/' + ctx.params.id)).user;
  ctx.title('Стена ' + u.nick);
  const reload = () => router.render();
  const data = await api.get(`/api/user/${u.id}/stena`, { page: ctx.page });
  const d = { user: u };
  return [ui.breadcrumbs([{ href: '/id' + u.id, title: u.nick }, { title: 'Стена' }]), ui.pageHead('Стена: ' + u.nick),
    ui.card(me() ? stenaComposer(u, reload) : null, data.items.length ? h('div.posts', data.items.map(p => stenaPost(p, d, reload))) : ui.empty('Сообщений пока нет'), ui.pager(data))];
});

// ───────── статус: комментарии/лайки
router.add('/user/status/:id', async ctx => {
  const d = await api.get('/api/status/' + ctx.params.id, { page: ctx.page });
  ctx.title('Статус ' + d.user.nick);
  const reload = () => router.render();
  const ed = ui.bbEditor({ rows: 3, placeholder: 'Ваш комментарий…', maxlength: 1024 });
  const komm = d.komm;
  const posts = komm.items.map(k => ui.post({ user: k.user, time: k.time, body: ui.html(k.msg), id: 'k' + k.id, actions: [
    me() && k.user.id !== me().id ? ui.btn('', { ic: 'flag', small: true, title: 'Жалоба', onclick: () => complain(`/api/status/komm/${k.id}/spam`) }) : null,
    me() && (d.can_delete || k.user.id === me().id) ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/status/komm/' + k.id); reload(); } } }) : null] }));
  return [ui.breadcrumbs([{ href: '/id' + d.user.id, title: d.user.nick }, { title: 'Статус' }]),
    ui.card(h('div.pad.stack', h('div.row', ui.userLink(d.user, { avatar: 36 }), h('span.grow'), ui.time(d.time)), h('div.profile-status.big', { html: d.msg }),
      h('div.row', ui.reaction('thumbs-up', d.plus, d.my_like === 2, me() && !d.my_like && me().id !== d.user.id ? async () => { await api.post(`/api/status/${d.id}/like`, { like: 1 }); reload(); } : null, 'Нравится'),
        ui.reaction('thumbs-down', d.minus, d.my_like === 1, me() && !d.my_like && me().id !== d.user.id ? async () => { await api.post(`/api/status/${d.id}/like`, { like: 0 }); reload(); } : null, 'Не нравится'),
        link(`/user/status/${d.id}/likes`, 'Кто оценил'), h('span.grow'),
        d.can_delete ? ui.btn('Удалить статус', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить статус?', { danger: true })) { await api.del('/api/status/' + d.id); router.go('/id' + d.user.id); } } }) : null))),
    ui.section('Комментарии ' + komm.total,
      me() ? h('div.pad.stack', ed.el, ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/status/${d.id}/komm`, { msg: ed.value }); ed.value = ''; reload(); } })) : null,
      posts.length ? h('div.posts', posts) : ui.empty('Комментариев пока нет', 'message'), ui.pager({ page: komm.page, pages: komm.pages }))];
});
router.add('/user/status/:id/likes', async ctx => {
  ctx.title('Оценки статуса');
  return [ui.pageHead('Кто оценил статус'), ui.card(await pagedList(ctx, p => api.get(`/api/status/${ctx.params.id}/likes`, { page: p }), l =>
    h('div.list-item', ui.avatar(l.user, 36), h('div.main', ui.userLink(l.user), l.status ? h('div.small', { html: l.status.msg }) : null),
      icon(l.like ? 'thumbs-up' : 'thumbs-down', 16), ui.time(l.time)), { empty: 'Пока никто не оценивал' }))];
});

// ───────── кто оценил анкету
router.add('/user/info/who_rating/:id', async ctx => {
  ctx.title('Оценки анкеты');
  return [ui.pageHead('Кто оценил анкету'), ui.card(await pagedList(ctx, p => api.get(`/api/user/${ctx.params.id}/voters`, { page: p }), v =>
    h('div.list-item', ui.avatar(v.user, 36), h('div.main', ui.userLink(v.user)), h('span.badge' + (v.rating > 0 ? '.ok' : v.rating < 0 ? '.danger' : ''), (v.rating > 0 ? '+' : '') + v.rating), ui.time(v.time)), { empty: 'Оценок пока нет' }))];
});

// ───────── редактирование анкеты
router.add('/user/info/edit', async ctx => {
  ctx.title('Моя анкета');
  const d = await api.get('/api/user/edit');
  const o = d.options;
  const opt = list => list.map((l, i) => ({ value: i, label: l.split('/')[0] }));
  const days = [{ value: 0, label: '—' }, ...Array.from({ length: 31 }, (_, i) => ({ value: i + 1, label: i + 1 }))];
  const months = [{ value: 0, label: '—' }, ...MONTHS.map((m, i) => ({ value: i + 1, label: m }))];
  const y = new Date().getFullYear();
  const years = [{ value: 0, label: '—' }, ...Array.from({ length: y - 7 - 1930 + 1 }, (_, i) => ({ value: y - 7 - i, label: y - 7 - i }))];
  const meet = new Set(d.meet);
  const meetBox = h('div.checks', o.meet.map((l, i) => h('label.check', h('input', { type: 'checkbox', checked: meet.has(i + 1), onchange: e => e.target.checked ? meet.add(i + 1) : meet.delete(i + 1) }), ' ', l)));
  const form = ui.form([
    { name: 'name', label: 'Имя', value: d.name, maxlength: 32 },
    { name: 'pol', label: 'Пол', type: 'radio', value: String(d.pol), options: [{ value: '1', label: 'Мужской' }, { value: '0', label: 'Женский' }] },
    { name: 'day', label: 'День рождения', type: 'select', value: d.day, options: days },
    { name: 'month', label: 'Месяц', type: 'select', value: d.month, options: months },
    { name: 'year', label: 'Год', type: 'select', value: d.year, options: years },
    { name: 'city', label: 'Город', value: d.city, maxlength: 32 },
    { name: 'marital', label: 'Семейное положение', type: 'select', value: d.marital, options: opt(o.marital) },
    { name: 'baby', label: 'Дети', type: 'select', value: d.baby, options: opt(o.baby) },
    { name: 'smok', label: 'Курение', type: 'select', value: d.smok, options: opt(o.smok) },
    { name: 'alko', label: 'Алкоголь', type: 'select', value: d.alko, options: opt(o.alko) },
    { name: 'job', label: 'Занятие', value: d.job, maxlength: 215 },
    { name: 'about', label: 'О себе', type: 'textarea', value: d.about, rows: 5, maxlength: 512 },
    { name: 'skype', label: 'Skype', value: d.skype }, { name: 'telegram', label: 'Telegram', value: d.telegram },
    { name: 'vk', label: 'VK (ссылка или id)', value: d.vk }, { name: 'phone', label: 'Телефон', value: d.phone },
    { name: 'show_mail', type: 'checkbox', label: 'Показывать e-mail в анкете', value: d.show_mail },
  ], {
    submit: 'Сохранить',
    onSubmit: async v => {
      const n = k => +v[k] || 0;
      await api.put('/api/user/edit', { ...v, pol: n('pol'), day: n('day'), month: n('month'), year: n('year'), marital: n('marital'), baby: n('baby'), smok: n('smok'), alko: n('alko'), meet: [...meet] });
      ui.ok('Анкета сохранена');
    },
  });
  const sub = form.querySelector('[type=submit]')?.closest('.field, .actions') || form.lastElementChild;
  sub.before(h('div.field', h('label', 'Цели знакомства'), meetBox));
  return [ui.breadcrumbs([{ href: '/id' + me().id, title: 'Моя страница' }, { title: 'Анкета' }]), ui.pageHead('Моя анкета'), ui.card(h('div.pad', form))];
}, { auth: true });

// ───────── пользователи
const USER_FILTERS = [['all', 'Все', 'users'], ['online', 'Онлайн', 'globe'], ['today', 'Сегодня', 'clock'], ['male', 'Парни', 'user'], ['female', 'Девушки', 'user'], ['birthday', 'Именинники', 'gift'], ['staff', 'Администрация', 'shield'], ['banned', 'Забаненные', 'gavel']];
const userItem = x => h('div.list-item', ui.avatar(x.user, 44), h('div.main',
  h('div.title', ui.userLink(x.user), ' ', medal(x.user.medal)),
  h('div.small.muted', [x.city, x.age ? `${x.age} ${plural(x.age, 'год', 'года', 'лет')}` : '', `рейтинг ${fmt.num(x.rating)}`].filter(Boolean).join(' · ')),
  x.url ? h('div.small.muted', x.url) : null), h('div.small.muted', x.user.online ? 'онлайн' : fmt.ago(x.date_last)));

async function usersPage(ctx, defFilter) {
  const filter = ctx.query.filter || defFilter, sort = ctx.query.sort || '', order = ctx.query.order || 'desc', q = ctx.query.q || '';
  ctx.title('Пользователи');
  const [stats, data] = await Promise.all([api.get('/api/users/stats'), api.get('/api/users', { page: ctx.page, filter, sort, order, q })]);
  const go = (patch) => { const p = new URLSearchParams({ ...ctx.query, ...patch }); p.delete('page'); router.go('/user/users?' + p); };
  const search = h('input.input', { placeholder: 'Поиск по нику или ID', value: q });
  search.onkeydown = e => e.key === 'Enter' && go({ q: search.value });
  const cnt = k => stats[k === 'all' ? 'all' : k];
  return [ui.pageHead('Пользователи'),
    h('div.card.pad.stack', h('div.row.nowrap', search, ui.btn('', { ic: 'search', kind: 'primary', onclick: () => go({ q: search.value }) })),
      h('div.chips', USER_FILTERS.map(([k, l, ic]) => h('a.chip' + (k === filter ? '.active' : ''), { href: '#', onclick: e => { e.preventDefault(); go({ filter: k }); } }, icon(ic, 14), l, ' ', h('span.muted', fmt.num(cnt(k)))))),
      h('div.row.small', h('span.muted', 'Сортировка:'), [['id', 'Дата'], ['rating', 'Рейтинг'], ['balls', 'Баллы'], ['nick', 'Ник'], ['last', 'Активность']].map(([k, l]) =>
        h('a.chip' + (k === (sort || 'id') ? '.active' : ''), { href: '#', onclick: e => { e.preventDefault(); go({ sort: k, order: k === sort && order === 'desc' ? 'asc' : 'desc' }); } }, l, k === sort ? (order === 'desc' ? ' ↓' : ' ↑') : '')))),
    ui.card(data.items.length ? h('div.list', data.items.map(userItem)) : ui.empty('Никого не найдено', 'users')), ui.pager(data)];
}
router.add('/user/users', ctx => usersPage(ctx, 'all'));
router.add('/user/users/online', ctx => usersPage(ctx, 'online'));

// ───────── друзья
router.add('/user/frends', ctx => router.go('/user/frends/' + (me() ? me().id : 0), true), { auth: true });
router.add('/user/frends/new', async ctx => {
  ctx.title('Заявки в друзья');
  const reload = () => router.render();
  const r = await api.get('/api/frends/requests');
  const row = (x, btns) => h('div.list-item', ui.avatar(x.user, 40), h('div.main', ui.userLink(x.user), h('div.small.muted', fmt.ago(x.time))), h('div.row', ...btns));
  return [ui.breadcrumbs([{ href: '/user/frends', title: 'Друзья' }, { title: 'Заявки' }]), ui.pageHead('Заявки в друзья'),
    ui.section('Входящие ' + r.incoming.length, r.incoming.length ? h('div.list', r.incoming.map(x => row(x, [
      ui.btn('Принять', { kind: 'primary', small: true, onclick: async () => { await api.post(`/api/frends/${x.user.id}/accept`); reload(); } }),
      ui.btn('Отклонить', { small: true, onclick: async () => { await api.post(`/api/frends/${x.user.id}/decline`); reload(); } })]))) : ui.empty('Нет входящих заявок', 'users')),
    ui.section('Отправленные ' + r.outgoing.length, r.outgoing.length ? h('div.list', r.outgoing.map(x => row(x, [
      ui.btn('Отменить', { small: true, onclick: async () => { await api.post(`/api/frends/${x.user.id}/cancel`); reload(); } })]))) : ui.empty('Нет исходящих заявок', 'send'))];
}, { auth: true });
router.add('/user/frends/:id', async ctx => {
  const online = ctx.query.online === '1';
  const d = await api.get(`/api/user/${ctx.params.id}/frends`, { page: ctx.page, online: online ? 1 : 0 });
  const own = me() && me().id === d.owner.id;
  ctx.title('Друзья ' + d.owner.nick);
  const sel = new Set();
  const reload = () => router.render();
  const rows = d.items.map(f => h('div.list-item',
    own ? h('input', { type: 'checkbox', onchange: e => e.target.checked ? sel.add(f.user.id) : sel.delete(f.user.id) }) : null,
    ui.avatar(f.user, 44), h('div.main', ui.userLink(f.user), h('div.small.muted', 'в друзьях с ' + fmt.date(f.time))),
    me() ? ui.btn('', { ic: 'mail', small: true, title: 'Написать', href: '/mail/' + f.user.id }) : null));
  return [ui.breadcrumbs([{ href: '/id' + d.owner.id, title: d.owner.nick }, { title: 'Друзья' }]),
    ui.pageHead('Друзья', own && d.requests ? ui.btn('Заявки ' + d.requests, { ic: 'users', kind: 'primary', href: '/user/frends/new' }) : null),
    h('div.chips', h('a.chip' + (!online ? '.active' : ''), { href: `/user/frends/${d.owner.id}` }, 'Все ', h('span.muted', d.all)), h('a.chip' + (online ? '.active' : ''), { href: `/user/frends/${d.owner.id}?online=1` }, 'Онлайн ', h('span.muted', d.online))),
    ui.card(rows.length ? h('div.list', rows) : ui.empty('Список друзей пуст', 'users')),
    own && rows.length ? ui.btn('Удалить отмеченных', { ic: 'trash', onclick: async () => { if (sel.size && await ui.confirm('Удалить отмеченных из друзей?', { danger: true })) { await api.post('/api/frends/delete', { ids: [...sel] }); reload(); } } }) : null,
    ui.pager(d)];
});

// ───────── мои гости
router.add('/user/myguest', async ctx => {
  ctx.title('Мои гости');
  const reload = () => router.render();
  const data = await api.get('/api/myguest', { page: ctx.page });
  bus.emit('counters');
  return [ui.pageHead('Мои гости', data.total ? ui.btn('Очистить', { ic: 'trash', onclick: async () => { if (await ui.confirm('Очистить список гостей?', { danger: true })) { await api.del('/api/myguest'); reload(); } } }) : null),
    ui.card(data.items.length ? h('div.list', data.items.map(g => h('div.list-item' + (g.new ? '.unread' : ''), ui.avatar(g.user, 40), h('div.main', ui.userLink(g.user), g.new ? ui.badge('новый', 'ok') : null), ui.time(g.time)))) : ui.empty('У вас пока не было гостей', 'eye')), ui.pager(data)];
}, { auth: true });

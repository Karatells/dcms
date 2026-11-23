import { h, api, ui, router, state, icon, fmt, events } from '../core.js';

if (!document.querySelector('link[href="/css/groups.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/groups.css' }));
}

const me = () => state.user;
const CATS = ['Общение', 'Игры', 'Музыка', 'Технологии', 'Спорт', 'Творчество', 'Кино', 'Образование', 'Другое'];
const initialOf = s => (Array.from((s || 'Г').trim())[0] || 'Г').toUpperCase();

function roleBadges(m) {
  const b = [];
  if (m.role === 'creator') b.push(h('span.badge.danger', 'Создатель'));
  else if (m.role === 'admin') b.push(h('span.badge.primary', 'Администратор'));
  else {
    if (m.mod_blog) b.push(h('span.badge.ok', 'Мод. блога'));
    if (m.mod_forum) b.push(h('span.badge.ok', 'Мод. форума'));
    if (m.mod_chat) b.push(h('span.badge.ok', 'Мод. чата'));
    if (m.mod_files) b.push(h('span.badge.ok', 'Мод. файлов'));
  }
  return b.length ? h('div.grp-role-badges', ...b) : null;
}

const grpCrumbs = (g, ...rest) => ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { href: '/groups/' + g.id, title: g.name }, ...rest]);

// ───────── создание сообщества
function openCreateGroupModal() {
  if (!me()) { router.go('/login'); return; }
  const name = h('input.input', { placeholder: 'Название сообщества', maxlength: 64 });
  const cat = h('select.input', CATS.map(c => h('option', { value: c }, c)));
  const opis = h('textarea.input', { rows: 3, placeholder: 'О чём это сообщество?', maxlength: 1000 });
  const cBlog = h('input', { type: 'checkbox', checked: true });
  const cForum = h('input', { type: 'checkbox', checked: true });
  const cChat = h('input', { type: 'checkbox', checked: true });
  const cFiles = h('input', { type: 'checkbox', checked: true });
  const cPriv = h('input', { type: 'checkbox' });
  const err = h('div.form-err');

  const modal = ui.modal({
    title: 'Создать сообщество',
    body: h('div.stack',
      h('div.field', h('label', 'Название'), name),
      h('div.field', h('label', 'Тематика'), cat),
      h('div.field', h('label', 'Описание'), opis),
      h('label.check', cPriv, ' Приватное сообщество (вступление по одобрению или уникальной ссылке)'),
      h('div.field', h('label', 'Включённые разделы'),
        h('div.checks',
          h('label.check', cBlog, ' Блог'),
          h('label.check', cForum, ' Форум'),
          h('label.check', cChat, ' Чат'),
          h('label.check', cFiles, ' Файлы'))),
      err),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Создать', kind: 'primary',
        onclick: () => {
          err.textContent = '';
          api.post('/api/groups', {
            name: name.value, category: cat.value, opis: opis.value, private: cPriv.checked,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked,
          }).then(r => { modal.close(); ui.ok('Сообщество создано'); router.go('/groups/' + r.id); })
            .catch(e => { err.textContent = e.message || 'Ошибка'; });
          return false;
        },
      },
    ],
  });
}

// ───────── пригласить друга в сообщество
async function openInviteFriendModal(g) {
  const box = h('div.stack-s', ui.loading());
  ui.modal({ title: `Пригласить друга · ${g.name}`, body: box, actions: [{ label: 'Закрыть' }] });
  try {
    const d = await api.get(`/api/groups/${g.id}/invitable-friends`);
    if (!d.items || !d.items.length) {
      box.replaceChildren(ui.empty('Нет доступных друзей для приглашения (возможно, они уже в сообществе или запретили приглашения)', 'users'));
      return;
    }
    box.replaceChildren(...d.items.map(it => {
      const btn = ui.btn('Пригласить', {
        ic: 'user-plus', small: true, kind: 'primary',
        onclick: async () => {
          btn.disabled = true;
          try {
            await api.post(`/api/groups/${g.id}/invite`, { user_id: it.user.id });
            btn.textContent = 'Отправлено ✓';
            ui.ok(`Приглашение отправлено ${it.user.nick}`);
          } catch { btn.disabled = false; }
        },
      });
      return h('div.list-item', ui.avatar(it.user, 36), h('div.main', ui.userLink(it.user)), btn);
    }));
  } catch {
    box.replaceChildren(ui.empty('Не удалось загрузить список друзей', 'users'));
  }
}

// ───────── автовступление по уникальной ссылке /groups/join/:token
router.add('/groups/join/:token', async ctx => {
  const tok = ctx.params.token;
  ctx.title('Приглашение в сообщество');
  const d = await api.get('/api/group-join/' + encodeURIComponent(tok));
  const g = d.group;
  if (me()) {
    const r = await api.post('/api/group-join/' + encodeURIComponent(tok));
    ui.ok(r.already ? 'Вы уже состоите в этом сообществе' : `Вы вступили в «${g.name}»`);
    router.go('/groups/' + r.group_id);
    return ui.loading();
  }
  return [
    ui.pageHead('Приглашение в сообщество'),
    h('div.card.pad.stack',
      h('div.row', h('div.grp-ava-sm', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
        h('div.main', h('h2', g.name), h('div.small.muted', g.category, ' · участников: ', fmt.num(g.members)))),
      g.opis_html ? h('div.text', { html: g.opis_html }) : null,
      h('div.muted', 'Войдите или зарегистрируйтесь, чтобы автоматически вступить в это сообщество по ссылке.'),
      h('div.row', ui.btn('Войти', { kind: 'primary', href: '/login' }), ui.btn('Регистрация', { href: '/reg' }))),
  ];
});

// ───────── каталог сообществ /groups
router.add('/groups', async ctx => {
  const tab = ctx.query.tab || 'all';
  const q = ctx.query.q || '';
  const cat = ctx.query.category || '';
  const uid = +(ctx.query.user || 0);
  ctx.title('Сообщества');

  const d = await api.get('/api/groups', { tab: uid ? '' : tab, q, category: cat, user: uid || undefined, page: ctx.page });
  const searchIn = h('input.input', { value: q, placeholder: 'Поиск сообществ…' });
  const goSearch = () => router.go(`/groups?tab=${tab}&q=${encodeURIComponent(searchIn.value.trim())}&category=${encodeURIComponent(cat)}`);
  searchIn.onkeydown = e => e.key === 'Enter' && goSearch();

  const list = d.items.map(g => h('a.list-item', { href: '/groups/' + g.id },
    h('div.grp-ava-sm', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
    h('div.main',
      h('div.title', g.name,
        g.private ? h('span.badge.warn', { style: { marginLeft: '6px' } }, icon('lock', 11), ' Приватное') : null,
        h('span.badge', { style: { marginLeft: '6px' } }, g.category)),
      g.opis ? h('div.small.muted.clip', g.opis) : null,
      h('div.small.muted', `Участников: ${fmt.num(g.members)}`)),
    g.perms && g.perms.is_member ? h('span.badge.primary', 'Вы участник') : null));

  const tabs = [{ key: 'all', label: 'Все сообщества', href: '/groups?tab=all' }];
  if (me()) tabs.push({ key: 'my', label: 'Мои сообщества', href: '/groups?tab=my' });

  return [
    ui.pageHead(uid ? 'Сообщества пользователя' : 'Сообщества',
      me() ? ui.btn('Создать сообщество', { ic: 'plus', kind: 'primary', small: true, onclick: openCreateGroupModal }) : null),
    !uid ? ui.tabs(tabs, tab) : null,
    h('div.card.pad.stack-s',
      h('div.row.nowrap', searchIn, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: goSearch })),
      h('div.chips',
        h('a.chip' + (!cat ? '.active' : ''), { href: `/groups?tab=${tab}&q=${encodeURIComponent(q)}` }, 'Все'),
        ...CATS.map(c => h('a.chip' + (cat === c ? '.active' : ''), { href: `/groups?tab=${tab}&q=${encodeURIComponent(q)}&category=${encodeURIComponent(c)}` }, c)))),
    ui.card(list.length ? h('div.list', list) : ui.empty('Сообществ пока нет', 'users')),
    ui.pager(d),
  ];
});

// ───────── главная страница сообщества /groups/:id (шапка + плитки разделов)
router.add('/groups/:id', async ctx => {
  const id = +ctx.params.id;
  const g = await api.get('/api/groups/' + id);
  const reload = () => router.render();
  ctx.title(g.name);

  const p = g.perms || {};
  const banner = h('div.grp-banner', g.banner ? { style: { backgroundImage: `url(${g.banner})` } } : {});
  const joinBtn = !me() ? null
    : p.is_creator ? h('span.badge.danger', 'Создатель')
      : p.is_member
        ? ui.btn('Выйти', { small: true, onclick: async () => { await api.post(`/api/groups/${id}/leave`); ui.ok('Вы покинули сообщество'); reload(); } })
        : g.pending_request
          ? ui.btn('Отменить заявку', { small: true, onclick: async () => { await api.post(`/api/groups/${id}/leave`); ui.ok('Заявка отменена'); reload(); } })
          : ui.btn(g.private ? 'Подать заявку' : 'Вступить', {
            ic: g.private ? 'lock' : 'plus', kind: 'primary', small: true,
            onclick: async () => {
              const r = await api.post(`/api/groups/${id}/join`);
              ui.ok(r.pending ? 'Заявка отправлена администрации сообщества' : 'Вы вступили в сообщество');
              reload();
            },
          });

  const inviteBtn = me() && p.is_member ? ui.btn('Пригласить друга', { ic: 'user-plus', small: true, onclick: () => openInviteFriendModal(g) }) : null;

  const headerCard = h('div.card.grp-head',
    banner,
    h('div.grp-main',
      h('div.grp-ava', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
      h('div.grp-info',
        h('h1', g.name, g.private ? h('span.badge.warn', { style: { marginLeft: '8px', verticalAlign: 'middle' } }, icon('lock', 12), ' Приватное') : null),
        h('div.small.muted', g.category, ' · участников: ', fmt.num(g.members), ' · создатель: ', ui.userLink(g.creator))),
      h('div.grp-actions', inviteBtn, joinBtn)),
    g.opis_html ? h('div.grp-opis.text', { html: g.opis_html }) : null);

  if (!g.can_view) {
    return [
      ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { title: g.name }]),
      headerCard,
      ui.card(ui.empty(
        g.pending_request ? 'Ваша заявка на вступление рассматривается администрацией' : 'Это приватное сообщество. Подайте заявку или воспользуйтесь ссылкой-приглашением, чтобы открыть разделы.',
        'lock'
      )),
    ];
  }

  const tiles = [];
  if (g.enable_blog) tiles.push({ title: 'Блог', icon: 'book', href: `/groups/${id}/blog`, count: g.counts.blog });
  if (g.enable_forum) tiles.push({ title: 'Форум', icon: 'forum', href: `/groups/${id}/forum`, count: g.counts.forum });
  if (g.enable_chat) tiles.push({ title: 'Чат', icon: 'message-circle', href: `/groups/${id}/chat`, count: g.counts.chat });
  if (g.enable_files) tiles.push({ title: 'Файлы', icon: 'folder', href: `/groups/${id}/files`, count: g.counts.files });
  tiles.push({ title: 'Участники', icon: 'users', href: `/groups/${id}/members`, count: g.members, badge: g.requests_count > 0 ? `+${g.requests_count}` : null });
  if (p.manage) tiles.push({ title: 'Настройки', icon: 'settings', href: `/groups/${id}/settings`, count: null });

  const grid = h('div.grid-links', tiles.map(t =>
    h('a.ptile', { href: t.href }, icon(t.icon, 22), h('div.t', t.title),
      t.badge ? h('span.badge.danger', t.badge) : null,
      t.count != null ? h('div.c', fmt.num(t.count)) : null)));

  const out = [
    ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { title: g.name }]),
    headerCard,
    grid,
  ];

  // Свежие записи блога на главной сообщества
  if (g.enable_blog && g.counts.blog > 0) {
    const bd = await api.get(`/api/groups/${id}/blog`, { page: 1 });
    if (bd.items && bd.items.length) {
      out.push(ui.section('Последние записи блога',
        h('div.list', ...bd.items.slice(0, 3).map(post => h('a.list-item', { href: `/groups/${id}/blog` },
          ui.avatar(post.user, 36),
          h('div.main',
            h('div.title', post.title || 'Запись в блоге'),
            h('div.small.muted', post.user.nick, ' · ', fmt.ago(post.time))),
          h('span.badge', icon('heart', 12), ' ', post.likes)))),
        ui.btn('Все записи', { small: true, href: `/groups/${id}/blog` })));
    }
  }
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: БЛОГ (/groups/:id/blog)
router.add('/groups/:id/blog', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/blog`, { page: ctx.page })]);
  const reload = () => router.render();
  ctx.title(`Блог · ${g.name}`);
  const p = g.perms || {};
  const out = [grpCrumbs(g, { title: 'Блог' }), ui.pageHead(`Блог · ${g.name}`)];

  if (p.mod_blog || (g.open_blog && p.is_member)) {
    const ti = h('input.input', { placeholder: 'Заголовок (необязательно)', maxlength: 120 });
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Новая запись в блоге сообщества…', maxlength: 4096 });
    out.push(h('details.card.pad',
      h('summary', icon('plus', 16), ' Написать запись'),
      h('div.stack', { style: { marginTop: '10px' } }, ti, ed.el,
        ui.btn('Опубликовать', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/groups/${id}/blog`, { title: ti.value, msg: ed.value }); ui.ok('Запись опубликована'); reload(); } }))));
  }

  if (!d.items.length) {
    out.push(ui.card(ui.empty('Записей в блоге пока нет', 'book')));
  } else {
    d.items.forEach(post => {
      const kommBox = h('div');
      let kommOpen = false;
      const loadKomms = async () => {
        kommBox.replaceChildren(ui.loading());
        const kd = await api.get(`/api/group-blog/${post.id}/komms`);
        const kIn = h('input.input', { placeholder: 'Комментарий…', maxlength: 1024 });
        const sendK = async () => { if (!kIn.value.trim()) return; await api.post(`/api/group-blog/${post.id}/komms`, { msg: kIn.value }); kIn.value = ''; loadKomms(); };
        kIn.onkeydown = e => e.key === 'Enter' && sendK();
        kommBox.replaceChildren(h('div.stack-s', { style: { marginTop: '10px', paddingTop: '10px', borderTop: '1px solid var(--border-s)' } },
          kd.items.length ? h('div.posts', kd.items.map(k => ui.post({
            user: k.user, time: k.time, body: h('div.text', { html: k.msg }),
            actions: k.can_delete ? [ui.btn('', { ic: 'trash', small: true, onclick: async () => { await api.del('/api/group-blog-komm/' + k.id); loadKomms(); } })] : [],
          }))) : h('div.small.muted', 'Комментариев пока нет'),
          me() ? h('div.row.nowrap', kIn, ui.btn('', { ic: 'send', small: true, kind: 'primary', onclick: sendK })) : null));
      };
      const likeBtn = ui.reaction('heart', post.likes, post.liked, async () => {
        if (!me()) return router.go('/login');
        const r = await api.post(`/api/group-blog/${post.id}/like`);
        likeBtn.classList.toggle('active', r.liked);
        likeBtn.querySelector('span').textContent = fmt.num(r.likes);
      });
      out.push(h('div.card.pad.stack-s',
        h('div.row', ui.avatar(post.user, 34), h('div.grow', ui.userLink(post.user), h('div.small.muted', fmt.ago(post.time))),
          post.pinned ? h('span.badge.primary', icon('pin', 12), ' Закреплено') : null,
          post.can_pin ? ui.btn('', { ic: 'pin', small: true, title: post.pinned ? 'Открепить' : 'Закрепить', onclick: async () => { await api.post(`/api/group-blog/${post.id}/pin`); reload(); } }) : null,
          post.can_delete ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить запись?', { danger: true })) { await api.del('/api/group-blog/' + post.id); reload(); } } }) : null),
        post.title ? h('h3', post.title) : null,
        h('div.text', { html: post.msg }),
        h('div.row', likeBtn, ui.btn(`Комментарии (${post.komms})`, { ic: 'message-circle', small: true, onclick: () => { kommOpen = !kommOpen; if (kommOpen) loadKomms(); else kommBox.replaceChildren(); } })),
        kommBox));
    });
  }
  out.push(ui.pager(d));
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: ФОРУМ (список подфорумов) (/groups/:id/forum)
router.add('/groups/:id/forum', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/subforums`)]);
  const reload = () => router.render();
  ctx.title(`Форум · ${g.name}`);
  const out = [grpCrumbs(g, { title: 'Форум' }), ui.pageHead(`Форум · ${g.name}`)];

  if (d.can_mod) {
    const nm = h('input.input', { placeholder: 'Название подфорума', maxlength: 80 });
    const op = h('input.input', { placeholder: 'Краткое описание (необязательно)', maxlength: 256 });
    out.push(h('details.card.pad',
      h('summary', icon('plus', 16), ' Создать подфорум'),
      h('div.stack', { style: { marginTop: '10px' } }, nm, op,
        ui.btn('Создать', { ic: 'check', kind: 'primary', onclick: async () => { await api.post(`/api/groups/${id}/subforums`, { name: nm.value, opis: op.value }); ui.ok('Подфорум создан'); reload(); } }))));
  }

  out.push(ui.card(d.items.length ? h('div.list', d.items.map(s => h('div.list-item',
    h('a.row.nowrap.grow', { href: `/groups/${id}/forum/${s.id}`, style: { color: 'inherit', textDecoration: 'none' } },
      h('div.forum-ico', icon('forum', 20)),
      h('div.main', h('div.title', s.name), s.opis ? h('div.small.muted', s.opis) : null)),
    h('span.badge', `${s.topics} тем`),
    d.can_mod && d.items.length > 1 ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', title: 'Удалить подфорум', onclick: async () => { if (await ui.confirm(`Удалить подфорум «${s.name}» со всеми темами?`, { danger: true })) { await api.del('/api/group-subforum/' + s.id); reload(); } } }) : null))) : ui.empty('Подфорумов пока нет', 'forum')));
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: ПОДФОРУМ (темы раздела) (/groups/:id/forum/:subId)
router.add('/groups/:id/forum/:subId', async ctx => {
  const id = +ctx.params.id, subId = +ctx.params.subId;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/forum`, { sub: subId, page: ctx.page })]);
  const reload = () => router.render();
  const subName = (d.sub && d.sub.name) || 'Подфорум';
  ctx.title(`${subName} · ${g.name}`);
  const p = g.perms || {};
  const out = [grpCrumbs(g, { href: `/groups/${id}/forum`, title: 'Форум' }, { title: subName }), ui.pageHead(subName)];

  if (p.is_member) {
    const nm = h('input.input', { placeholder: 'Название темы', maxlength: 120 });
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Текст первого сообщения…', maxlength: 4096 });
    out.push(h('details.card.pad',
      h('summary', icon('plus', 16), ' Создать тему'),
      h('div.stack', { style: { marginTop: '10px' } }, nm, ed.el,
        ui.btn('Создать', { ic: 'check', kind: 'primary', onclick: async () => { const r = await api.post(`/api/groups/${id}/forum`, { sub: subId, name: nm.value, msg: ed.value }); router.go(`/groups/${id}/forum/topic/${r.id}`); } }))));
  }

  out.push(ui.card(d.items.length ? h('div.list', d.items.map(t => h('a.list-item', { href: `/groups/${id}/forum/topic/${t.id}` },
    h('div.forum-ico', icon(t.closed ? 'lock' : t.pinned ? 'pin' : 'forum', 20)),
    h('div.main', h('div.title', t.name), h('div.small.muted', t.user.nick, ' · ', fmt.ago(t.time_last))),
    h('span.badge', t.posts)))) : ui.empty('Тем в этом подфоруме пока нет', 'forum')), ui.pager(d));
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: ТЕМА ФОРУМА (/groups/:id/forum/topic/:tid)
router.add('/groups/:id/forum/topic/:tid', async ctx => {
  const id = +ctx.params.id, tid = +ctx.params.tid;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/group-forum/${tid}`, { page: ctx.page })]);
  const reload = () => router.render();
  const t = d.topic;
  ctx.title(`${t.name} · ${g.name}`);
  const crumbs = [
    { href: `/groups/${id}/forum`, title: 'Форум' },
    t.sub_id ? { href: `/groups/${id}/forum/${t.sub_id}`, title: t.sub_name || 'Подфорум' } : null,
    { title: t.name },
  ].filter(Boolean);

  const out = [
    grpCrumbs(g, ...crumbs),
    h('div.card.pad.stack-s',
      h('div.row', h('h2.grow', t.name),
        t.pinned ? h('span.badge.primary', 'Закреплена') : null,
        t.closed ? h('span.badge.warn', 'Закрыта') : null),
      h('div.row.small.muted', ui.avatar(t.user, 26), ui.userLink(t.user), fmt.datetime(t.time)),
      h('div.text', { html: t.msg }),
      h('div.row',
        t.can_mod ? ui.btn(t.pinned ? 'Открепить' : 'Закрепить', { ic: 'pin', small: true, onclick: async () => { await api.post(`/api/group-forum/${tid}/toggle`, { field: 'pinned' }); reload(); } }) : null,
        t.can_mod ? ui.btn(t.closed ? 'Открыть' : 'Закрыть', { ic: 'lock', small: true, onclick: async () => { await api.post(`/api/group-forum/${tid}/toggle`, { field: 'closed' }); reload(); } }) : null,
        t.can_delete ? ui.btn('Удалить тему', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить тему?', { danger: true })) { await api.del('/api/group-forum/' + tid); router.go(t.sub_id ? `/groups/${id}/forum/${t.sub_id}` : `/groups/${id}/forum`); } } }) : null)),
    ui.card(d.items.length ? h('div.posts', d.items.map(r => ui.post({
      user: r.user, time: r.time, body: h('div.text', { html: r.msg }),
      actions: r.can_delete ? [ui.btn('', { ic: 'trash', small: true, onclick: async () => { await api.del('/api/group-forum-post/' + r.id); reload(); } })] : [],
    }))) : ui.empty('Ответов пока нет', 'message-circle')),
    ui.pager(d),
  ];
  if (t.can_reply) {
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Ваш ответ в теме…', maxlength: 4096 });
    out.push(h('div.card.pad.stack', ed.el, ui.btn('Ответить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/group-forum/${tid}/posts`, { msg: ed.value }); ui.ok('Ответ добавлен'); reload(); } })));
  }
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: ЧАТ (полноэкранный мессенджер-формат) (/groups/:id/chat)
router.add('/groups/:id/chat', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/chat`)]);
  ctx.title(`Чат · ${g.name}`);
  document.body.classList.add('msg-page');

  const scrollEl = h('div.msg-scroll');
  let items = d.items || [];

  const draw = () => {
    if (!items.length) { scrollEl.replaceChildren(ui.empty('Чат пуст — начните общение!', 'message-circle')); return; }
    scrollEl.replaceChildren(...items.map(m => {
      const mine = me() && me().id === m.user.id;
      return h('div.bubble-row' + (mine ? '.mine' : ''),
        !mine ? ui.avatar(m.user, 28) : null,
        h('div.bubble' + (mine ? '.mine' : ''),
          !mine ? h('div', { style: { fontWeight: '700', fontSize: '.78rem', marginBottom: '2px' } }, ui.userLink(m.user)) : null,
          ui.html(m.msg, 'text'),
          h('div.bubble-meta',
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })),
            m.can_delete ? h('button.link', { title: 'Удалить', onclick: async () => { await api.del('/api/group-chat/' + m.id); items = items.filter(x => x.id !== m.id); draw(); } }, icon('trash', 12)) : null)));
    }));
    requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
  };

  const leaveSSE = events.join('group:' + id, (ev, data) => {
    if (ev === 'group_chat' && data && data.group_id === id && !items.some(x => x.id === data.id)) {
      items.push({ ...data, can_delete: d.can_mod || (me() && me().id === data.user.id) });
      draw();
    }
  });
  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    leaveSSE && leaveSSE();
  });

  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/groups/' + id, title: 'К сообществу' }, icon('arrow-left', 20)),
    h('div.grp-ava-sm', { style: { width: '36px', height: '36px', borderRadius: '10px', fontSize: '.95rem' } }, g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
    h('div.msg-head-info', h('b', g.name), h('div.msg-status', `Чат сообщества · участников: ${fmt.num(g.members)}`)));

  const foot = h('div.msg-foot');
  if (d.can_write) {
    const ta = h('textarea.msg-input', { rows: 1, placeholder: 'Написать в чат…', maxlength: 1024 });
    const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить', disabled: true }, icon('send', 18));
    const sync = () => {
      ta.style.height = 'auto';
      ta.style.height = Math.min(110, Math.max(38, ta.scrollHeight)) + 'px';
      sendBtn.disabled = !ta.value.trim();
    };
    const send = async () => {
      const msg = ta.value.trim();
      if (!msg || sendBtn.disabled) return;
      sendBtn.disabled = true;
      try {
        ta.value = ''; sync();
        const r = await api.post(`/api/groups/${id}/chat`, { msg });
        if (!items.some(x => x.id === r.id)) { items.push({ ...r, can_delete: true }); draw(); }
      } finally { sync(); }
    };
    sendBtn.onclick = send;
    ta.oninput = sync;
    ta.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) { e.preventDefault(); send(); } };
    foot.append(h('div.msg-compose', h('div.msg-input-wrap', ta), sendBtn));
  } else {
    foot.append(h('div.pad.small.muted.center', 'Вступите в сообщество, чтобы писать в чат.'));
  }

  draw();
  return h('div.msg-shell', head, scrollEl, foot);
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: ФАЙЛЫ (/groups/:id/files)
router.add('/groups/:id/files', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/files`, { page: ctx.page })]);
  const reload = () => router.render();
  ctx.title(`Файлы · ${g.name}`);
  const p = g.perms || {};
  const out = [grpCrumbs(g, { title: 'Файлы' }), ui.pageHead(`Файлы · ${g.name}`)];

  if (p.is_member) {
    const fi = h('input', { type: 'file' });
    const nm = h('input.input', { placeholder: 'Название (по умолчанию — имя файла)', maxlength: 120 });
    const op = h('input.input', { placeholder: 'Описание (необязательно)', maxlength: 500 });
    out.push(h('details.card.pad',
      h('summary', icon('upload', 16), ' Загрузить файл'),
      h('div.stack', { style: { marginTop: '10px' } }, fi, nm, op,
        ui.btn('Загрузить', {
          ic: 'upload', kind: 'primary',
          onclick: async () => {
            if (!fi.files[0]) return ui.toast('Выберите файл', 'warn');
            await api.upload(`/api/groups/${id}/files`, { file: fi.files[0], name: nm.value, opis: op.value });
            ui.ok('Файл загружен'); reload();
          },
        }))));
  }
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(f => h('div.list-item',
    h('div.forum-ico', icon('file', 20)),
    h('div.main',
      h('div.title', f.name + (f.ras ? '.' + f.ras : '')),
      f.opis ? h('div.small.muted', { html: f.opis }) : null,
      h('div.small.muted', f.size_str, ' · ', f.user.nick, ' · ', fmt.ago(f.time), ' · ⬇ ', f.loads)),
    ui.btn('', { ic: 'download', small: true, kind: 'primary', href: f.download }),
    f.can_delete ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить файл?', { danger: true })) { await api.del('/api/group-files/' + f.id); reload(); } } }) : null))) : ui.empty('Файлов пока нет', 'folder')), ui.pager(d));
  return out;
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: УЧАСТНИКИ (/groups/:id/members)
router.add('/groups/:id/members', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/members`, { page: ctx.page })]);
  const reload = () => router.render();
  ctx.title(`Участники · ${g.name}`);
  const p = g.perms || {};

  const openRoleModal = m => {
    const roleSel = h('select.input',
      h('option', { value: 'member', selected: m.role !== 'admin' }, 'Участник / Модератор разделов'),
      (p.is_creator || (me() && me().level >= 3)) ? h('option', { value: 'admin', selected: m.role === 'admin' }, 'Администратор (все разделы)') : null);
    const mb = h('input', { type: 'checkbox', checked: !!m.mod_blog });
    const mf = h('input', { type: 'checkbox', checked: !!m.mod_forum });
    const mc = h('input', { type: 'checkbox', checked: !!m.mod_chat });
    const mfl = h('input', { type: 'checkbox', checked: !!m.mod_files });
    const modBox = h('div.field', h('label', 'Права модератора по разделам'),
      h('div.checks',
        h('label.check', mb, ' Модератор блога'),
        h('label.check', mf, ' Модератор форума'),
        h('label.check', mc, ' Модератор чата'),
        h('label.check', mfl, ' Модератор файлов')));
    const syncVis = () => { modBox.style.display = roleSel.value === 'admin' ? 'none' : ''; };
    roleSel.onchange = syncVis;
    syncVis();

    const modal = ui.modal({
      title: `Права · ${m.user.nick}`,
      body: h('div.stack', h('div.field', h('label', 'Роль'), roleSel), modBox),
      actions: [
        { label: 'Отмена' },
        {
          label: 'Сохранить', kind: 'primary',
          onclick: () => {
            api.put(`/api/groups/${id}/members/${m.user.id}`, {
              role: roleSel.value, mod_blog: mb.checked, mod_forum: mf.checked, mod_chat: mc.checked, mod_files: mfl.checked,
            }).then(() => { modal.close(); ui.ok('Права обновлены'); reload(); });
            return false;
          },
        },
      ],
    });
  };

  let reqBlock = null;
  if (p.manage) {
    const rq = await api.get(`/api/groups/${id}/requests`).catch(() => ({ items: [] }));
    if (rq.items && rq.items.length) {
      reqBlock = ui.section(`Заявки на вступление (${rq.items.length})`, h('div.list', rq.items.map(r => h('div.list-item',
        ui.avatar(r.user, 40),
        h('div.main', ui.userLink(r.user), h('div.small.muted', 'подал заявку ', fmt.ago(r.time))),
        h('span.row',
          ui.btn('Принять', { ic: 'check', kind: 'primary', small: true, onclick: async () => { await api.post(`/api/groups/${id}/requests/${r.user.id}/approve`); ui.ok('Заявка одобрена'); reload(); } }),
          ui.btn('Отклонить', { ic: 'x', small: true, kind: 'danger', onclick: async () => { await api.post(`/api/groups/${id}/requests/${r.user.id}/reject`); ui.ok('Заявка отклонена'); reload(); } }))))));
    }
  }

  return [
    grpCrumbs(g, { title: 'Участники' }),
    ui.pageHead(`Участники · ${g.name}`),
    reqBlock,
    ui.card(d.items.length ? h('div.list', d.items.map(m => h('div.list-item',
      ui.avatar(m.user, 40),
      h('div.main', ui.userLink(m.user), roleBadges(m)),
      p.manage && m.role !== 'creator' ? h('span.row',
        ui.btn('Роль', { ic: 'shield', small: true, onclick: () => openRoleModal(m) }),
        ui.btn('', { ic: 'trash', small: true, kind: 'danger', title: 'Исключить', onclick: async () => { if (await ui.confirm(`Исключить ${m.user.nick}?`, { danger: true })) { await api.del(`/api/groups/${id}/members/${m.user.id}`); reload(); } } })) : null))) : ui.empty('Участников нет', 'users')),
    ui.pager(d),
  ];
});

// ───────── ОТДЕЛЬНАЯ СТРАНИЦА: НАСТРОЙКИ (/groups/:id/settings)
router.add('/groups/:id/settings', async ctx => {
  const id = +ctx.params.id;
  const g = await api.get('/api/groups/' + id);
  const reload = () => router.render();
  ctx.title(`Настройки · ${g.name}`);
  if (!g.perms || !g.perms.manage) return [grpCrumbs(g, { title: 'Настройки' }), ui.forbidden()];

  const name = h('input.input', { value: g.name, maxlength: 64 });
  const cat = h('select.input', CATS.map(c => h('option', { value: c, selected: c === g.category }, c)));
  const opis = h('textarea.input', { rows: 3, maxlength: 1000 }, g.opis || '');
  const cBlog = h('input', { type: 'checkbox', checked: g.enable_blog });
  const cForum = h('input', { type: 'checkbox', checked: g.enable_forum });
  const cChat = h('input', { type: 'checkbox', checked: g.enable_chat });
  const cFiles = h('input', { type: 'checkbox', checked: g.enable_files });
  const cOpenBlog = h('input', { type: 'checkbox', checked: g.open_blog });
  const cPriv = h('input', { type: 'checkbox', checked: !!g.private });
  const avaIn = h('input', { type: 'file', accept: 'image/*' });
  const banIn = h('input', { type: 'file', accept: 'image/*' });

  // Ссылки для автовступления
  const linksData = await api.get(`/api/groups/${id}/links`).catch(() => ({ items: [] }));
  const linkNote = h('input.input', { placeholder: 'Пометка (необязательно)', maxlength: 64 });
  const linkMax = h('input.input', { type: 'number', min: 0, value: 0, style: { width: '110px' } });
  const linkExp = h('select.input',
    h('option', { value: 0 }, 'Бессрочная'),
    h('option', { value: 1 }, '1 час'),
    h('option', { value: 24 }, '24 часа'),
    h('option', { value: 168 }, '7 дней'),
    h('option', { value: 720 }, '30 дней'));

  const linksList = (linksData.items || []).map(l => {
    const url = `${location.origin}/groups/join/${l.token}`;
    const limText = l.max_uses > 0 ? `${l.uses} / ${l.max_uses}` : `${l.uses} / ∞`;
    const expText = l.expires_at > 0 ? 'до ' + fmt.datetime(l.expires_at) : 'бессрочная';
    return h('div.list-item',
      h('div.main',
        h('div.title', l.note || 'Ссылка-приглашение',
          l.active ? h('span.badge.ok', { style: { marginLeft: '6px' } }, 'Активна') : h('span.badge.danger', { style: { marginLeft: '6px' } }, l.expired ? 'Истекла' : 'Исчерпана')),
        h('div.small.muted.clip', url),
        h('div.small.muted', `Вступлений: ${limText} · ${expText}`)),
      ui.btn('', { ic: 'copy', small: true, title: 'Скопировать ссылку', onclick: () => navigator.clipboard?.writeText(url).then(() => ui.ok('Ссылка скопирована')) }),
      ui.btn('', { ic: 'trash', small: true, kind: 'danger', title: 'Удалить', onclick: async () => { await api.del('/api/group-link/' + l.id); ui.ok('Ссылка удалена'); reload(); } }));
  });

  return [
    grpCrumbs(g, { title: 'Настройки' }),
    ui.pageHead(`Настройки · ${g.name}`),
    h('div.card.pad.stack',
      h('h3', 'Основные настройки и разделы'),
      h('div.field', h('label', 'Название'), name),
      h('div.field', h('label', 'Тематика'), cat),
      h('div.field', h('label', 'Описание'), opis),
      h('label.check', cPriv, ' Приватное сообщество (вступление по одобрению или уникальной ссылке)'),
      h('div.field', h('label', 'Разделы сообщества'),
        h('div.checks',
          h('label.check', cBlog, ' Блог'),
          h('label.check', cForum, ' Форум'),
          h('label.check', cChat, ' Чат'),
          h('label.check', cFiles, ' Файлы'))),
      h('label.check', cOpenBlog, ' Разрешить обычным участникам публиковать посты в блог'),
      ui.btn('Сохранить настройки', {
        ic: 'check', kind: 'primary',
        onclick: async () => {
          await api.put('/api/groups/' + id, {
            name: name.value, category: cat.value, opis: opis.value, private: cPriv.checked,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked, open_blog: cOpenBlog.checked,
          });
          ui.ok('Настройки сохранены'); reload();
        },
      })),
    h('div.card.pad.stack',
      h('h3', 'Ссылки для автовступления'),
      h('div.small.muted', 'Переходя по такой ссылке, пользователь автоматически принимается в сообщество без ожидания одобрения.'),
      h('div.row',
        h('div.field', h('label', 'Пометка'), linkNote),
        h('div.field', { style: { flex: '0 0 auto' } }, h('label', 'Лимит (0 = ∞)'), linkMax),
        h('div.field', { style: { flex: '0 0 150px' } }, h('label', 'Срок действия'), linkExp)),
      ui.btn('Создать ссылку', {
        ic: 'link', kind: 'primary', small: true,
        onclick: async () => {
          await api.post(`/api/groups/${id}/links`, { note: linkNote.value, max_uses: +linkMax.value || 0, expires_hours: +linkExp.value || 0 });
          ui.ok('Ссылка создана'); reload();
        },
      }),
      linksList.length ? h('div.list', ...linksList) : h('div.small.muted', 'Активных ссылок пока нет')),
    h('div.card.pad.stack',
      h('h3', 'Оформление (аватарка и баннер)'),
      h('div.row', h('div.field', h('label', 'Аватарка сообщества'), avaIn),
        ui.btn('Загрузить аватарку', { ic: 'upload', small: true, onclick: async () => { if (!avaIn.files[0]) return; await api.upload(`/api/groups/${id}/avatar`, { file: avaIn.files[0] }); ui.ok('Аватарка обновлена'); reload(); } })),
      h('div.row', h('div.field', h('label', 'Баннер (шапка сообщества)'), banIn),
        ui.btn('Загрузить баннер', { ic: 'upload', small: true, onclick: async () => { if (!banIn.files[0]) return; await api.upload(`/api/groups/${id}/banner`, { file: banIn.files[0] }); ui.ok('Баннер обновлён'); reload(); } }))),
    g.perms.is_creator || (me() && me().level >= 3) ? h('div.card.pad',
      ui.btn('Удалить сообщество навсегда', {
        ic: 'trash', kind: 'danger',
        onclick: async () => { if (await ui.confirm('Удалить сообщество вместе со всеми темами, блогом, чатом и файлами?', { danger: true })) { await api.del('/api/groups/' + id); ui.ok('Сообщество удалено'); router.go('/groups'); } },
      })) : null,
  ];
});

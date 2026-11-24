import { h, api, ui, router, state, icon, fmt, events, bus } from '../core.js';

if (!document.querySelector('link[href="/css/groups.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/groups.css' }));
}

const me = () => state.user;
const CATS = ['Общение', 'Игры', 'Музыка', 'Технологии', 'Спорт', 'Творчество', 'Кино', 'Образование', 'Другое'];
const initialOf = s => (Array.from((s || 'Г').trim())[0] || 'Г').toUpperCase();

state.adminLinks.push({
  group: 'Модерация',
  title: 'Модерация сообществ',
  href: '/admin/communities',
  access: 'groups_block',
  icon: 'users',
  desc: 'Скрытие, блокировка и удаление сообществ',
});

function roleBadges(m) {
  const b = [];
  if (m.role === 'creator') b.push(h('span.badge.danger', 'Создатель'));
  else if (m.role === 'admin') b.push(h('span.badge.primary', 'Администратор'));
  else {
    if (m.mod_blog) b.push(h('span.badge.ok', 'Мод. блога'));
    if (m.mod_forum) b.push(h('span.badge.ok', 'Мод. форума'));
    if (m.mod_chat) b.push(h('span.badge.ok', 'Мод. чата'));
    if (m.mod_files) b.push(h('span.badge.ok', 'Мод. файлов'));
    if (m.mod_messages) b.push(h('span.badge.ok', 'Мод. сообщений'));
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
  const cMsgs = h('input', { type: 'checkbox', checked: true });
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
          h('label.check', cFiles, ' Файлы'),
          h('label.check', cMsgs, ' Личные сообщения'))),
      err),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Создать', kind: 'primary',
        onclick: () => {
          err.textContent = '';
          api.post('/api/groups', {
            name: name.value, category: cat.value, opis: opis.value, private: cPriv.checked,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked, enable_messages: cMsgs.checked,
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
  const dmBtn = me() && g.enable_messages && !g.blocked
    ? ui.btn(g.user_msg_unread > 0 ? `Написать (+${g.user_msg_unread})` : 'Написать сообществу', { ic: 'mail', kind: p.is_member ? 'primary' : '', small: true, href: `/mail/group/${id}` })
    : null;

  const headerCard = h('div.card.grp-head',
    banner,
    h('div.grp-main',
      h('div.grp-ava', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
      h('div.grp-info',
        h('h1', g.name,
          g.private ? h('span.badge.warn', { style: { marginLeft: '8px', verticalAlign: 'middle' } }, icon('lock', 12), ' Приватное') : null,
          g.hidden ? h('span.badge.warn', { style: { marginLeft: '6px', verticalAlign: 'middle' } }, 'Скрыто') : null,
          g.blocked ? h('span.badge.danger', { style: { marginLeft: '6px', verticalAlign: 'middle' } }, 'Заблокировано') : null),
        h('div.small.muted', g.category, ' · участников: ', fmt.num(g.members), ' · создатель: ', ui.userLink(g.creator))),
      h('div.grp-actions', dmBtn, inviteBtn, joinBtn)),
    g.opis_html ? h('div.grp-opis.text', { html: g.opis_html }) : null);

  const modBar = p.can_block ? h('div.card.pad.row',
    h('span.small.muted', icon('shield', 14), ' Модерация:'),
    g.blocked && g.block_reason ? h('span.small.err', 'Причина: ', g.block_reason) : null,
    h('span.grow'),
    ui.btn(g.hidden ? 'Показать в каталоге' : 'Скрыть из каталога', {
      small: true,
      onclick: async () => {
        await api.post(`/api/groups/${id}/moderate`, { hidden: !g.hidden });
        ui.ok(g.hidden ? 'Сообщество возвращено в каталог' : 'Сообщество скрыто из каталога');
        reload();
      },
    }),
    ui.btn(g.blocked ? 'Разблокировать' : 'Заблокировать', {
      small: true, kind: g.blocked ? '' : 'danger',
      onclick: async () => {
        if (g.blocked) {
          await api.post(`/api/groups/${id}/moderate`, { blocked: false });
          ui.ok('Сообщество разблокировано');
          reload();
        } else {
          const reason = await ui.prompt('Причина блокировки (необязательно)', '', { title: 'Заблокировать сообщество' });
          if (reason === null) return;
          await api.post(`/api/groups/${id}/moderate`, { blocked: true, reason });
          ui.ok('Сообщество заблокировано');
          reload();
        }
      },
    })) : null;

  if (!g.can_view) {
    return [
      ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { title: g.name }]),
      headerCard,
      modBar,
      ui.card(g.blocked
        ? ui.empty('Сообщество заблокировано администрацией' + (g.block_reason ? `: ${g.block_reason}` : '.'), 'lock')
        : ui.empty(
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
  if (p.mod_messages && (g.enable_messages || g.inbox_unread > 0)) {
    tiles.push({ title: 'Входящие (ЛС)', icon: 'mail', href: `/groups/${id}/inbox`, count: null, badge: g.inbox_unread > 0 ? `+${g.inbox_unread}` : null });
  } else if (me() && g.enable_messages && !g.blocked) {
    tiles.push({ title: 'Написать', icon: 'mail', href: `/mail/group/${id}`, count: null, badge: g.user_msg_unread > 0 ? `+${g.user_msg_unread}` : null });
  }
  tiles.push({ title: 'Участники', icon: 'users', href: `/groups/${id}/members`, count: g.members, badge: g.requests_count > 0 ? `+${g.requests_count}` : null });
  if (p.manage) tiles.push({ title: 'Настройки', icon: 'settings', href: `/groups/${id}/settings`, count: null });

  const grid = h('div.grid-links', tiles.map(t =>
    h('a.ptile', { href: t.href }, icon(t.icon, 22), h('div.t', t.title),
      t.badge ? h('span.badge.danger', t.badge) : null,
      t.count != null ? h('div.c', fmt.num(t.count)) : null)));

  const out = [
    ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { title: g.name }]),
    headerCard,
    modBar,
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
    const mmsg = h('input', { type: 'checkbox', checked: !!m.mod_messages });
    const modBox = h('div.field', h('label', 'Права модератора по разделам'),
      h('div.checks',
        h('label.check', mb, ' Модератор блога'),
        h('label.check', mf, ' Модератор форума'),
        h('label.check', mc, ' Модератор чата'),
        h('label.check', mfl, ' Модератор файлов'),
        h('label.check', mmsg, ' Модератор личных сообщений')));
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
              role: roleSel.value, mod_blog: mb.checked, mod_forum: mf.checked, mod_chat: mc.checked, mod_files: mfl.checked, mod_messages: mmsg.checked,
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
  const cMsgs = h('input', { type: 'checkbox', checked: !!g.enable_messages });
  const cOpenBlog = h('input', { type: 'checkbox', checked: g.open_blog });
  const cPriv = h('input', { type: 'checkbox', checked: !!g.private });
  const avaIn = h('input', { type: 'file', accept: 'image/*' });
  const banIn = h('input', { type: 'file', accept: 'image/*' });

  const copyFallback = (text, okMsg) => {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(() => ui.ok(okMsg)).catch(() => {
        const ta = h('textarea', { style: { position: 'fixed', opacity: '0' } }, text);
        document.body.append(ta); ta.select();
        try { document.execCommand('copy'); ui.ok(okMsg); } catch { ui.toast('Выделите и скопируйте вручную', 'info'); }
        ta.remove();
      });
    } else {
      const ta = h('textarea', { style: { position: 'fixed', opacity: '0' } }, text);
      document.body.append(ta); ta.select();
      try { document.execCommand('copy'); ui.ok(okMsg); } catch { ui.toast('Выделите и скопируйте вручную', 'info'); }
      ta.remove();
    }
  };

  // API-токены (боты сообщества)
  const tokData = await api.get(`/api/groups/${id}/tokens`).catch(() => ({ items: [], available_scopes: [] }));
  const tokName = h('input.input', { placeholder: 'Название бота (например: Бот поддержки)', maxlength: 64 });
  const tokScopes = new Set(['messages', 'members']);
  const tokScopeChecks = (tokData.available_scopes || []).map(sc => {
    const cb = h('input', { type: 'checkbox', checked: tokScopes.has(sc.name) });
    cb.onchange = () => { if (cb.checked) tokScopes.add(sc.name); else tokScopes.delete(sc.name); };
    return h('label.check', { title: sc.desc }, cb, ' ', h('b', sc.title), ' ', h('span.badge', sc.name));
  });
  const tokList = (tokData.items || []).map(t => {
    let revealed = false;
    const tokIn = h('input.input', { value: 'dcms_grp_••••••••••••••••••••', readOnly: true, style: { fontFamily: 'ui-monospace,monospace', fontSize: '.82rem' }, onclick: e => { if (revealed) e.target.select(); } });
    const revBtn = ui.btn('Показать', {
      small: true,
      onclick: () => {
        revealed = !revealed;
        tokIn.value = revealed ? t.token : 'dcms_grp_••••••••••••••••••••';
        revBtn.textContent = revealed ? 'Скрыть' : 'Показать';
      },
    });
    const runCmd = `dcms-bot -url ${location.origin} -token ${t.token}`;
    const cmdIn = h('input.input', {
      value: runCmd,
      readOnly: true,
      style: { fontFamily: 'ui-monospace,monospace', fontSize: '.82rem' },
      onclick: e => e.target.select(),
    });
    return h('div.card.pad.stack-s', { style: { background: 'var(--bg)' } },
      h('div.row',
        h('b.grow', t.name),
        ...(t.scopes || []).map(s => h('span.badge', s)),
        ui.btn('', {
          ic: 'trash', small: true, kind: 'danger', title: 'Отозвать токен',
          onclick: async () => { if (await ui.confirm(`Отозвать токен «${t.name}»?`, { danger: true })) { await api.del('/api/group-tokens/' + t.id); ui.ok('Токен отозван'); reload(); } },
        })),
      h('div.field', h('label', 'Токен сообщества'),
        h('div.row.nowrap', tokIn, revBtn, ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => copyFallback(t.token, 'Токен скопирован') }))),
      h('div.field', h('label', 'Команда запуска тестового бота (нажмите, чтобы выделить)'),
        h('div.row.nowrap', cmdIn, ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => copyFallback(runCmd, 'Команда скопирована') }))));
  });

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
      h('div.card.pad', { style: { background: 'var(--bg)' } },
        h('label.check', cMsgs, h('b', ' Включить личные сообщения сообщества')),
        h('div.small.muted', { style: { marginTop: '4px' } }, 'Любой пользователь сможет нажать «Написать сообществу» и начать диалог. Отвечать могут создатель, администратор и модератор личных сообщений (первым сообщество написать не может).')),
      h('label.check', cOpenBlog, ' Разрешить обычным участникам публиковать посты в блог'),
      ui.btn('Сохранить настройки', {
        ic: 'check', kind: 'primary',
        onclick: async () => {
          await api.put('/api/groups/' + id, {
            name: name.value, category: cat.value, opis: opis.value, private: cPriv.checked,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked, enable_messages: cMsgs.checked, open_blog: cOpenBlog.checked,
          });
          ui.ok('Настройки сохранены'); reload();
        },
      })),
    h('div.card.pad.stack',
      h('h3', 'Боты и API-токены сообщества (Long Poll)'),
      h('div.small.muted', 'Создайте API-токен сообщества, чтобы подключать ботов к личным сообщениям, чату, форуму, блогу и авто-одобрению заявок через ', h('code', '/api/v1/bot/getUpdates'), ' (без использования личного токена пользователя). ', h('a', { href: '/developers' }, 'Документация Bot API →')),
      h('div.field', h('label', 'Название бота / токена'), tokName),
      h('div.field', h('label', 'Разрешения и события, доступные боту'), h('div.checks', ...tokScopeChecks)),
      ui.btn('Создать токен сообщества', {
        ic: 'key', kind: 'primary', small: true,
        onclick: async () => {
          await api.post(`/api/groups/${id}/tokens`, { name: tokName.value, scopes: [...tokScopes] });
          ui.ok('Токен сообщества создан'); reload();
        },
      }),
      tokList.length ? h('div.stack-s', ...tokList) : h('div.small.muted', 'Активных токенов сообщества пока нет')),
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
    g.perms.can_delete_group || g.perms.is_creator || (me() && me().level >= 3) ? h('div.card.pad',
      ui.btn('Удалить сообщество навсегда', {
        ic: 'trash', kind: 'danger',
        onclick: async () => { if (await ui.confirm('Удалить сообщество вместе со всеми темами, блогом, чатом и файлами?', { danger: true })) { await api.del('/api/groups/' + id); ui.ok('Сообщество удалено'); router.go('/groups'); } },
      })) : null,
  ];
});

// ───────── панель модерации сообществ (/admin/communities)
router.add('/admin/communities', async ctx => {
  const filter = ctx.query.filter || 'all';
  const q = ctx.query.q || '';
  ctx.title('Модерация сообществ');
  const d = await api.get('/api/admin/communities', { filter: filter === 'all' ? '' : filter, q, page: ctx.page });
  const reload = () => router.render();

  const searchIn = h('input.input', { value: q, placeholder: 'Поиск по названию или описанию…' });
  const goSearch = () => router.go(`/admin/communities?filter=${filter}&q=${encodeURIComponent(searchIn.value.trim())}`);
  searchIn.onkeydown = e => e.key === 'Enter' && goSearch();

  const tabs = [
    { key: 'all', label: 'Все', href: `/admin/communities?filter=all&q=${encodeURIComponent(q)}` },
    { key: 'hidden', label: 'Скрытые', href: `/admin/communities?filter=hidden&q=${encodeURIComponent(q)}` },
    { key: 'blocked', label: 'Заблокированные', href: `/admin/communities?filter=blocked&q=${encodeURIComponent(q)}` },
  ];

  const list = (d.items || []).map(g => h('div.list-item',
    h('div.grp-ava-sm', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
    h('div.main',
      h('div.title',
        h('a', { href: '/groups/' + g.id }, g.name),
        g.private ? h('span.badge', { style: { marginLeft: '6px' } }, icon('lock', 11), ' Приватное') : null,
        g.blocked ? h('span.badge.danger', { style: { marginLeft: '6px' } }, 'Заблокировано') : null,
        g.hidden ? h('span.badge.warn', { style: { marginLeft: '6px' } }, 'Скрыто') : null),
      h('div.small.muted', g.category, ' · участников: ', fmt.num(g.members), ' · создатель: ', ui.userLink(g.creator)),
      g.block_reason ? h('div.small.err', 'Причина: ', g.block_reason) : null),
    h('div.row',
      ui.btn(g.hidden ? 'Показать' : 'Скрыть', {
        small: true,
        onclick: async () => {
          await api.post(`/api/groups/${g.id}/moderate`, { hidden: !g.hidden });
          ui.ok(g.hidden ? 'Сообщество возвращено в каталог' : 'Сообщество скрыто');
          reload();
        },
      }),
      ui.btn(g.blocked ? 'Разблокировать' : 'Заблокировать', {
        small: true, kind: g.blocked ? '' : 'danger',
        onclick: async () => {
          if (g.blocked) {
            await api.post(`/api/groups/${g.id}/moderate`, { blocked: false });
            ui.ok('Сообщество разблокировано');
            reload();
          } else {
            const reason = await ui.prompt('Причина блокировки (необязательно)', '', { title: 'Заблокировать сообщество' });
            if (reason === null) return;
            await api.post(`/api/groups/${g.id}/moderate`, { blocked: true, reason });
            ui.ok('Сообщество заблокировано');
            reload();
          }
        },
      }),
      ui.btn('', {
        ic: 'trash', small: true, kind: 'danger', title: 'Удалить навсегда',
        onclick: async () => {
          if (await ui.confirm(`Удалить сообщество «${g.name}» навсегда?`, { danger: true })) {
            await api.del('/api/groups/' + g.id);
            ui.ok('Сообщество удалено');
            reload();
          }
        },
      }))));

  return [
    ui.breadcrumbs([{ href: '/admin', title: 'Панель управления' }, { title: 'Модерация сообществ' }]),
    ui.pageHead('Модерация сообществ'),
    ui.tabs(tabs, filter),
    h('div.card.pad', h('div.row.nowrap', searchIn, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: goSearch }))),
    ui.card(list.length ? h('div.list', list) : ui.empty('Сообществ не найдено', 'users')),
    ui.pager(d),
  ];
}, { auth: true, access: 'groups_block' });

// ───────── ЛИЧНАЯ ПЕРЕПИСКА ПОЛЬЗОВАТЕЛЯ С СООБЩЕСТВОМ → единый диалог в Почте (/mail/group/:id)
router.add('/groups/:id/message', async ctx => {
  router.go('/mail/group/' + ctx.params.id, true);
  return ui.loading();
}, { auth: true });

// ───────── ВХОДЯЩИЕ СООБЩЕНИЯ СООБЩЕСТВА (для создателя / админа / модератора сообщений) (/groups/:id/inbox)
router.add('/groups/:id/inbox', async ctx => {
  const id = +ctx.params.id;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/inbox`)]);
  ctx.title(`Сообщения · ${g.name}`);
  const items = d.items || [];

  return [
    grpCrumbs(g, { title: 'Сообщения сообщества' }),
    ui.pageHead(`Сообщения · ${g.name}`),
    h('div.card.pad.small.muted', icon('info', 14), ' Сообщество не может писать пользователям первым. Здесь появляются диалоги только с теми пользователями, которые сами написали сообществу.'),
    ui.card(items.length ? h('div.list', ...items.map(it => h('a.list-item', { href: `/groups/${id}/inbox/${it.user.id}` },
      ui.avatar(it.user, 40),
      h('div.main',
        h('div.title', it.user.nick, it.unread > 0 ? h('span.badge.danger', { style: { marginLeft: '6px' } }, `+${it.unread}`) : null),
        h('div.small.muted.clip', (it.from_group ? 'Сообщество: ' : '') + it.last_msg),
        h('div.small.muted', fmt.ago(it.last_time))))))
      : ui.empty('Входящих сообщений пока нет', 'mail')),
  ];
}, { auth: true });

// ───────── ДИАЛОГ С ПОЛЬЗОВАТЕЛЕМ ОТ ИМЕНИ СООБЩЕСТВА (/groups/:id/inbox/:uid) — полноценный мессенджер
router.add('/groups/:id/inbox/:uid', async ctx => {
  const id = +ctx.params.id, uid = +ctx.params.uid;
  const [g, d] = await Promise.all([api.get('/api/groups/' + id), api.get(`/api/groups/${id}/inbox/${uid}`)]);
  const peer = d.peer;
  ctx.title(`${peer.nick} · Сообщения ${g.name}`);

  const head = h('div.msg-head',
    h('a.icon-btn', { href: `/groups/${id}/inbox`, title: 'К диалогам' }, icon('arrow-left', 20)),
    ui.avatar(peer, 36),
    h('div.msg-head-info', ui.userLink(peer), h('div.msg-status', `Ответ от имени «${g.name}»`)));

  return state.renderRichGroupDialog({
    ctx, head,
    initialItems: d.items,
    initialPinned: d.pinned,
    canWrite: d.can_write,
    emptyText: 'Сообщений нет',
    disabledText: 'Отправка ответов недоступна.',
    placeholder: `Ответить от имени «${g.name}»…`,
    isMine: m => !!m.from_group,
    senderLabel: m => (m.from_group ? g.name : peer.nick),
    showStaff: true,
    canDeleteMsg: () => true,
    onSend: payload => api.post(`/api/groups/${id}/inbox/${uid}`, payload),
    onPin: m => api.post(`/api/group-dm/${m.id}/pin`),
    onDelete: m => api.del(`/api/group-dm/${m.id}`),
    onSubscribe: cb => events.join('group_dm:' + id, (ev, data) => {
      if (ev === 'group_dm' && data && data.group_id === id && data.user_id === uid) cb(data);
    }),
  });
}, { auth: true });

import { h, api, ui, router, state, icon, link, fmt, events } from '../core.js';

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
  const err = h('div.form-err');

  const modal = ui.modal({
    title: 'Создать сообщество',
    body: h('div.stack',
      h('div.field', h('label', 'Название'), name),
      h('div.field', h('label', 'Тематика'), cat),
      h('div.field', h('label', 'Описание'), opis),
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
            name: name.value, category: cat.value, opis: opis.value,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked,
          }).then(r => { modal.close(); ui.ok('Сообщество создано'); router.go('/groups/' + r.id); })
            .catch(e => { err.textContent = e.message || 'Ошибка'; });
          return false;
        },
      },
    ],
  });
}

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
      h('div.title', g.name, h('span.badge', { style: { marginLeft: '8px' } }, g.category)),
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

// ───────── страница сообщества /groups/:id
router.add('/groups/:id', async ctx => {
  const id = +ctx.params.id;
  const g = await api.get('/api/groups/' + id);
  const reload = () => router.render();
  ctx.title(g.name);

  const p = g.perms || {};
  const enabledTabs = [];
  if (g.enable_blog) enabledTabs.push({ key: 'blog', label: `Блог (${g.counts.blog})`, icon: 'book', href: `/groups/${id}?tab=blog` });
  if (g.enable_forum) enabledTabs.push({ key: 'forum', label: `Форум (${g.counts.forum})`, icon: 'forum', href: `/groups/${id}?tab=forum` });
  if (g.enable_chat) enabledTabs.push({ key: 'chat', label: 'Чат', icon: 'message-circle', href: `/groups/${id}?tab=chat` });
  if (g.enable_files) enabledTabs.push({ key: 'files', label: `Файлы (${g.counts.files})`, icon: 'folder', href: `/groups/${id}?tab=files` });
  enabledTabs.push({ key: 'members', label: `Участники (${g.members})`, icon: 'users', href: `/groups/${id}?tab=members` });
  if (p.manage) enabledTabs.push({ key: 'settings', label: 'Настройки', icon: 'settings', href: `/groups/${id}?tab=settings` });

  let tab = ctx.query.tab || (enabledTabs[0] ? enabledTabs[0].key : 'members');
  if (!enabledTabs.some(t => t.key === tab)) tab = enabledTabs[0].key;

  const banner = h('div.grp-banner', g.banner ? { style: { backgroundImage: `url(${g.banner})` } } : {});
  const joinBtn = !me() ? null
    : p.is_creator ? h('span.badge.danger', 'Создатель')
      : p.is_member
        ? ui.btn('Выйти', { small: true, onclick: async () => { await api.post(`/api/groups/${id}/leave`); ui.ok('Вы покинули сообщество'); reload(); } })
        : ui.btn('Вступить', { ic: 'plus', kind: 'primary', small: true, onclick: async () => { await api.post(`/api/groups/${id}/join`); ui.ok('Вы вступили в сообщество'); reload(); } });

  const headerCard = h('div.card.grp-head',
    banner,
    h('div.grp-main',
      h('div.grp-ava', g.avatar ? h('img', { src: g.avatar, alt: g.name }) : initialOf(g.name)),
      h('div.grp-info',
        h('h1', g.name),
        h('div.small.muted', g.category, ' · участников: ', fmt.num(g.members), ' · создатель: ', ui.userLink(g.creator))),
      h('div.grp-actions', joinBtn)),
    g.opis_html ? h('div.grp-opis.text', { html: g.opis_html }) : null);

  let bodyNode;
  if (tab === 'blog') bodyNode = await renderBlog(g, ctx, reload);
  else if (tab === 'forum') bodyNode = await renderForum(g, ctx, reload);
  else if (tab === 'chat') bodyNode = await renderChat(g, ctx);
  else if (tab === 'files') bodyNode = await renderFiles(g, ctx, reload);
  else if (tab === 'members') bodyNode = await renderMembers(g, ctx, reload);
  else if (tab === 'settings') bodyNode = renderSettings(g, reload);

  return [
    ui.breadcrumbs([{ href: '/groups', title: 'Сообщества' }, { title: g.name }]),
    headerCard,
    ui.tabs(enabledTabs, tab),
    bodyNode,
  ];
});

// ───────── БЛОГ СООБЩЕСТВА
async function renderBlog(g, ctx, reload) {
  const d = await api.get(`/api/groups/${g.id}/blog`, { page: ctx.page });
  const p = g.perms || {};
  const out = [];

  if (p.mod_blog || (g.open_blog && p.is_member)) {
    const ti = h('input.input', { placeholder: 'Заголовок (необязательно)', maxlength: 120 });
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Новая запись в блоге сообщества…', maxlength: 4096 });
    out.push(h('details.card.pad',
      h('summary', icon('plus', 16), ' Написать запись'),
      h('div.stack', { style: { marginTop: '10px' } }, ti, ed.el,
        ui.btn('Опубликовать', { ic: 'send', kind: 'primary', onclick: async () => { await api.post(`/api/groups/${g.id}/blog`, { title: ti.value, msg: ed.value }); ui.ok('Запись опубликована'); reload(); } }))));
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
  return h('div.stack', ...out);
}

// ───────── ФОРУМ СООБЩЕСТВА
async function renderForum(g, ctx, reload) {
  const tid = +(ctx.query.topic || 0);
  if (tid) {
    const d = await api.get(`/api/group-forum/${tid}`, { page: ctx.page });
    const t = d.topic;
    const out = [
      ui.btn('← Все темы форума', { small: true, href: `/groups/${g.id}?tab=forum` }),
      h('div.card.pad.stack-s',
        h('div.row', h('h2.grow', t.name),
          t.pinned ? h('span.badge.primary', 'Закреплена') : null,
          t.closed ? h('span.badge.warn', 'Закрыта') : null),
        h('div.row.small.muted', ui.avatar(t.user, 26), ui.userLink(t.user), fmt.datetime(t.time)),
        h('div.text', { html: t.msg }),
        h('div.row',
          t.can_mod ? ui.btn(t.pinned ? 'Открепить' : 'Закрепить', { ic: 'pin', small: true, onclick: async () => { await api.post(`/api/group-forum/${tid}/toggle`, { field: 'pinned' }); reload(); } }) : null,
          t.can_mod ? ui.btn(t.closed ? 'Открыть' : 'Закрыть', { ic: 'lock', small: true, onclick: async () => { await api.post(`/api/group-forum/${tid}/toggle`, { field: 'closed' }); reload(); } }) : null,
          t.can_delete ? ui.btn('Удалить тему', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить тему?', { danger: true })) { await api.del('/api/group-forum/' + tid); router.go(`/groups/${g.id}?tab=forum`); } } }) : null)),
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
    return h('div.stack', ...out);
  }

  const d = await api.get(`/api/groups/${g.id}/forum`, { page: ctx.page });
  const p = g.perms || {};
  const out = [];
  if (p.is_member) {
    const nm = h('input.input', { placeholder: 'Название темы', maxlength: 120 });
    const ed = ui.bbEditor({ rows: 3, placeholder: 'Текст первого сообщения…', maxlength: 4096 });
    out.push(h('details.card.pad',
      h('summary', icon('plus', 16), ' Создать тему'),
      h('div.stack', { style: { marginTop: '10px' } }, nm, ed.el,
        ui.btn('Создать', { ic: 'check', kind: 'primary', onclick: async () => { const r = await api.post(`/api/groups/${g.id}/forum`, { name: nm.value, msg: ed.value }); router.go(`/groups/${g.id}?tab=forum&topic=${r.id}`); } }))));
  }
  out.push(ui.card(d.items.length ? h('div.list', d.items.map(t => h('a.list-item', { href: `/groups/${g.id}?tab=forum&topic=${t.id}` },
    h('div.forum-ico', icon(t.closed ? 'lock' : t.pinned ? 'pin' : 'forum', 20)),
    h('div.main', h('div.title', t.name), h('div.small.muted', t.user.nick, ' · ', fmt.ago(t.time_last))),
    h('span.badge', t.posts)))) : ui.empty('Тем на форуме пока нет', 'forum')), ui.pager(d));
  return h('div.stack', ...out);
}

// ───────── ЧАТ СООБЩЕСТВА (SSE в реальном времени)
async function renderChat(g, ctx) {
  const d = await api.get(`/api/groups/${g.id}/chat`);
  const listEl = h('div.grp-chat-list');
  let items = d.items || [];

  const draw = () => {
    if (!items.length) { listEl.replaceChildren(ui.empty('Чат пуст — начните общение!', 'message-circle')); return; }
    listEl.replaceChildren(...items.map(m => h('div.grp-chat-row',
      ui.avatar(m.user, 32),
      h('div.main',
        h('div.row', ui.userLink(m.user), h('span.small.muted', fmt.ago(m.time)), h('span.grow'),
          m.can_delete ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { await api.del('/api/group-chat/' + m.id); items = items.filter(x => x.id !== m.id); draw(); } }) : null),
        h('div.text', { html: m.msg })))));
    requestAnimationFrame(() => { listEl.scrollTop = listEl.scrollHeight; });
  };

  const leaveSSE = events.join('group:' + g.id, (ev, data) => {
    if (ev === 'group_chat' && data && data.group_id === g.id && !items.some(x => x.id === data.id)) {
      items.push({ ...data, can_delete: d.can_mod || (me() && me().id === data.user.id) });
      draw();
    }
  });
  ctx.onLeave(() => leaveSSE && leaveSSE());

  let bar;
  if (d.can_write) {
    const ta = h('textarea.input', { rows: 1, placeholder: 'Написать в чат…', maxlength: 1024 });
    const send = async () => {
      const msg = ta.value.trim();
      if (!msg) return;
      ta.value = '';
      const r = await api.post(`/api/groups/${g.id}/chat`, { msg });
      if (!items.some(x => x.id === r.id)) { items.push({ ...r, can_delete: true }); draw(); }
    };
    ta.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); } };
    bar = h('div.grp-chat-bar', ta, ui.btn('', { ic: 'send', kind: 'primary', onclick: send }));
  } else {
    bar = h('div.pad.small.muted.center', 'Вступите в сообщество, чтобы писать в чат.');
  }

  draw();
  return h('div.grp-chat-box', listEl, bar);
}

// ───────── ФАЙЛЫ СООБЩЕСТВА
async function renderFiles(g, ctx, reload) {
  const d = await api.get(`/api/groups/${g.id}/files`, { page: ctx.page });
  const p = g.perms || {};
  const out = [];
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
            await api.upload(`/api/groups/${g.id}/files`, { file: fi.files[0], name: nm.value, opis: op.value });
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
  return h('div.stack', ...out);
}

// ───────── УЧАСТНИКИ И НАЗНАЧЕНИЕ РОЛЕЙ
async function renderMembers(g, ctx, reload) {
  const d = await api.get(`/api/groups/${g.id}/members`, { page: ctx.page });
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
            api.put(`/api/groups/${g.id}/members/${m.user.id}`, {
              role: roleSel.value, mod_blog: mb.checked, mod_forum: mf.checked, mod_chat: mc.checked, mod_files: mfl.checked,
            }).then(() => { modal.close(); ui.ok('Права обновлены'); reload(); });
            return false;
          },
        },
      ],
    });
  };

  return h('div.stack',
    ui.card(d.items.length ? h('div.list', d.items.map(m => h('div.list-item',
      ui.avatar(m.user, 40),
      h('div.main', ui.userLink(m.user), roleBadges(m)),
      p.manage && m.role !== 'creator' ? h('span.row',
        ui.btn('Роль', { ic: 'shield', small: true, onclick: () => openRoleModal(m) }),
        ui.btn('', { ic: 'trash', small: true, kind: 'danger', title: 'Исключить', onclick: async () => { if (await ui.confirm(`Исключить ${m.user.nick}?`, { danger: true })) { await api.del(`/api/groups/${g.id}/members/${m.user.id}`); reload(); } } })) : null))) : ui.empty('Участников нет', 'users')),
    ui.pager(d));
}

// ───────── НАСТРОЙКИ СООБЩЕСТВА (баннер, аватарка, включение/выключение разделов)
function renderSettings(g, reload) {
  const name = h('input.input', { value: g.name, maxlength: 64 });
  const cat = h('select.input', CATS.map(c => h('option', { value: c, selected: c === g.category }, c)));
  const opis = h('textarea.input', { rows: 3, maxlength: 1000 }, g.opis || '');
  const cBlog = h('input', { type: 'checkbox', checked: g.enable_blog });
  const cForum = h('input', { type: 'checkbox', checked: g.enable_forum });
  const cChat = h('input', { type: 'checkbox', checked: g.enable_chat });
  const cFiles = h('input', { type: 'checkbox', checked: g.enable_files });
  const cOpenBlog = h('input', { type: 'checkbox', checked: g.open_blog });

  const avaIn = h('input', { type: 'file', accept: 'image/*' });
  const banIn = h('input', { type: 'file', accept: 'image/*' });

  return h('div.stack',
    h('div.card.pad.stack',
      h('h3', 'Основные настройки и разделы'),
      h('div.field', h('label', 'Название'), name),
      h('div.field', h('label', 'Тематика'), cat),
      h('div.field', h('label', 'Описание'), opis),
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
          await api.put('/api/groups/' + g.id, {
            name: name.value, category: cat.value, opis: opis.value,
            enable_blog: cBlog.checked, enable_forum: cForum.checked, enable_chat: cChat.checked, enable_files: cFiles.checked, open_blog: cOpenBlog.checked,
          });
          ui.ok('Настройки сохранены'); reload();
        },
      })),
    h('div.card.pad.stack',
      h('h3', 'Оформление (аватарка и баннер)'),
      h('div.row', h('div.field', h('label', 'Аватарка сообщества'), avaIn),
        ui.btn('Загрузить аватарку', { ic: 'upload', small: true, onclick: async () => { if (!avaIn.files[0]) return; await api.upload(`/api/groups/${g.id}/avatar`, { file: avaIn.files[0] }); ui.ok('Аватарка обновлена'); reload(); } })),
      h('div.row', h('div.field', h('label', 'Баннер (шапка сообщества)'), banIn),
        ui.btn('Загрузить баннер', { ic: 'upload', small: true, onclick: async () => { if (!banIn.files[0]) return; await api.upload(`/api/groups/${g.id}/banner`, { file: banIn.files[0] }); ui.ok('Баннер обновлён'); reload(); } }))),
    g.perms && (g.perms.is_creator || (me() && me().level >= 3)) ? h('div.card.pad',
      ui.btn('Удалить сообщество навсегда', {
        ic: 'trash', kind: 'danger',
        onclick: async () => { if (await ui.confirm('Удалить сообщество вместе со всеми темами, блогом, чатом и файлами?', { danger: true })) { await api.del('/api/groups/' + g.id); ui.ok('Сообщество удалено'); router.go('/groups'); } },
      })) : null);
}

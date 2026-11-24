import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

if (!document.querySelector('link[href="/css/developers.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/developers.css' }));
}

const me = () => state.user;
const copyText = async (txt, label = 'Скопировано') => {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(txt);
      ui.ok(label);
      return;
    }
  } catch {}
  const ta = h('textarea', { style: { position: 'fixed', opacity: '0' } }, txt);
  document.body.append(ta);
  ta.select();
  try { document.execCommand('copy'); ui.ok(label); }
  catch { ui.toast(txt, 'info'); }
  ta.remove();
};

function codeBlock(title, code) {
  return h('div.dev-code-wrap',
    h('div.dev-code-head',
      h('span', title),
      h('button.dev-copy-btn', { type: 'button', onclick: () => copyText(code, 'Код скопирован') }, icon('copy', 13), ' Копировать')),
    h('pre.dev-code', code));
}

function multiLangCode(snippets) {
  const langs = Object.keys(snippets);
  let active = langs[0];
  const pre = h('pre.dev-code', snippets[active]);
  const btns = langs.map(l => h('button.dev-lang-btn' + (l === active ? '.active' : ''), {
    type: 'button',
    onclick: () => {
      active = l;
      pre.textContent = snippets[l];
      btns.forEach((b, idx) => b.classList.toggle('active', langs[idx] === l));
    },
  }, l));
  return h('div.dev-code-wrap',
    h('div.dev-code-head',
      h('div.dev-lang-tabs', ...btns),
      h('button.dev-copy-btn', { type: 'button', onclick: () => copyText(snippets[active], 'Код скопирован') }, icon('copy', 13), ' Копировать')),
    pre);
}

// ───────── ЭКРАН СОГЛАСИЯ OAuth 2.0 (/oauth/authorize)
router.add('/oauth/authorize', async ctx => {
  ctx.title('Авторизация приложения');
  const q = ctx.query;
  const client_id = q.client_id || '';
  const redirect_uri = q.redirect_uri || '';
  const response_type = q.response_type || 'code';
  const scope = q.scope || 'profile';
  const stateParam = q.state || '';
  const code_challenge = q.code_challenge || '';
  const code_challenge_method = q.code_challenge_method || '';

  let info;
  try {
    info = await api.get('/api/oauth/authorize-info', { client_id, redirect_uri, response_type, scope, state: stateParam }, { silent: true });
  } catch (e) {
    return h('div.oauth-box', h('div.card.pad.stack',
      h('h2.err', icon('lock', 20), ' Ошибка OAuth-запроса'),
      h('p.muted', e.message || 'Некорректные параметры авторизации.'),
      h('div.row', ui.btn('Документация API', { href: '/developers', ic: 'code' }), ui.btn('На главную', { href: '/' }))));
  }

  const app = info.app;
  if (!me()) {
    const ret = encodeURIComponent(location.pathname + location.search);
    return h('div.oauth-box', h('div.card.pad.stack',
      h('div.row', h('span.oauth-scope-ic', icon('key', 18)),
        h('div', h('h2', app.name), h('div.small.muted', 'запрашивает доступ к вашему аккаунту'))),
      app.description ? h('p.small.muted', app.description) : null,
      h('div.card.pad', h('p', 'Чтобы выдать доступ этому приложению, сначала войдите в свой аккаунт на сайте.')),
      h('div.row',
        ui.btn('Войти в аккаунт', { kind: 'primary', ic: 'log-in', href: `/login?return=${ret}` }),
        ui.btn('Регистрация', { href: `/reg?return=${ret}` }))));
  }

  const requestedScopes = info.scopes || [];
  const hasDanger = requestedScopes.some(sc => sc.danger);
  const selected = new Set(requestedScopes.map(sc => sc.name));

  let busy = false;
  const decide = async approve => {
    if (busy) return;
    if (approve && selected.size === 0) {
      ui.toast('Выберите хотя бы одно разрешение или нажмите «Отмена»', 'warn');
      return;
    }
    busy = true;
    try {
      const r = await api.post('/api/oauth/authorize', {
        client_id, redirect_uri, scope: [...selected].join(' '), state: stateParam,
        approve, code_challenge, code_challenge_method,
      });
      if (r.redirect) location.href = r.redirect;
    } catch {
      busy = false;
    }
  };

  let hostText = redirect_uri;
  try { hostText = new URL(redirect_uri).origin; } catch {}

  const scopeNodes = requestedScopes.map(sc => {
    const locked = sc.name === 'profile' && requestedScopes.length === 1;
    const cb = h('input', { type: 'checkbox', checked: true, disabled: locked });
    const row = h('label.oauth-scope-item' + (sc.danger ? '.danger' : ''),
      cb,
      h('div.grow',
        h('div.row',
          h('b', sc.title),
          h('span.badge', sc.name),
          sc.danger ? h('span.badge.danger', icon('lock', 11), ' Опасное разрешение') : null),
        h('div.small.muted', sc.desc),
        sc.danger ? h('div.small.err', { style: { marginTop: '3px' } }, 'Вы можете снять галочку, если не доверяете этому приложению доступ к перепискам.') : null));
    cb.onchange = () => {
      if (cb.checked) selected.add(sc.name);
      else selected.delete(sc.name);
      row.classList.toggle('unchecked', !cb.checked);
    };
    return row;
  });

  return h('div.oauth-box', h('div.card.pad.stack',
    h('div.row',
      h('span.oauth-scope-ic', icon('shield', 18)),
      h('div.grow',
        h('h2', app.name),
        h('div.small.muted', 'Автор: ', ui.userLink(app.owner), app.website ? [' · ', h('a', { href: app.website, target: '_blank', rel: 'noopener nofollow', 'data-native': '' }, app.website)] : null))),
    app.description ? h('p.small.muted', app.description) : null,
    h('div.row.small', ui.avatar(me(), 28), h('span', 'Вы вошли как '), h('b', me().nick)),
    hasDanger ? h('div.oauth-danger-banner',
      icon('shield', 20),
      h('div',
        h('b', 'Внимание: запрошены потенциально опасные разрешения!'),
        h('div', 'Приложение запрашивает доступ к вашим личным сообщениям. Вы можете снять галочку возле опасных разрешений — авторизация пройдёт без них.'))) : null,
    h('div.stack-s',
      h('div.small.muted', 'Выберите разрешения, которые вы готовы предоставить:'),
      ...scopeNodes),
    h('div.small.muted', 'После подтверждения вы будете перенаправлены на ', h('b', hostText)),
    h('div.row',
      ui.btn('Разрешить доступ', { kind: 'primary', ic: 'check', onclick: () => decide(true) }),
      ui.btn('Отмена', { ic: 'x', onclick: () => decide(false) }))));
});

// ───────── МОДАЛКА СОЗДАНИЯ / РЕДАКТИРОВАНИЯ ПРИЛОЖЕНИЯ
function openAppModal(existing, onSaved) {
  const nameIn = h('input.input', { value: existing?.name || '', placeholder: 'Например: Мой Telegram-бот или Портал', maxlength: 64 });
  const siteIn = h('input.input', { value: existing?.website || '', placeholder: 'https://example.com (необязательно)', maxlength: 200 });
  const descIn = h('textarea.input', { rows: 2, placeholder: 'Краткое описание приложения (показывается пользователю при входе)', maxlength: 500 }, existing?.description || '');
  const urisIn = h('textarea.input', {
    rows: 3,
    placeholder: 'https://example.com/oauth/callback\nhttp://localhost:3000/callback',
  }, (existing?.redirect_uris || []).join('\n'));
  const err = h('div.form-err');

  const modal = ui.modal({
    title: existing ? 'Редактировать OAuth-приложение' : 'Новое OAuth-приложение',
    body: h('div.stack',
      h('div.field', h('label', 'Название приложения'), nameIn),
      h('div.field', h('label', 'Сайт приложения'), siteIn),
      h('div.field', h('label', 'Описание'), descIn),
      h('div.field', h('label', 'Разрешённые Redirect URI (по одному на строку)'), urisIn,
        h('div.small.muted', 'Куда перенаправлять пользователя с параметром ?code=... после авторизации')),
      err),
    actions: [
      { label: 'Отмена' },
      {
        label: existing ? 'Сохранить' : 'Создать приложение', kind: 'primary',
        onclick: () => {
          err.textContent = '';
          const payload = { name: nameIn.value, website: siteIn.value, description: descIn.value, redirect_uris: urisIn.value };
          const req = existing ? api.put('/api/oauth/apps/' + existing.id, payload) : api.post('/api/oauth/apps', payload);
          req.then(res => { modal.close(); ui.ok(existing ? 'Приложение обновлено' : 'Приложение создано'); onSaved && onSaved(res); })
            .catch(e => { err.textContent = e.message || 'Ошибка сохранения'; });
          return false;
        },
      },
    ],
  });
}

// ───────── СПРАВОЧНИК МЕТОДОВ API v1 (разбитый по разделам + живой поиск)
function renderEpCard(ep) {
  return h('details.dev-ep',
    h('summary',
      h('span.dev-method.' + ep.method, ep.method),
      h('span.dev-path', ep.path),
      h('span.badge', ep.scope),
      h('span.small.muted.grow', ep.summary)),
    h('div.dev-ep-body',
      ep.params && ep.params.length ? h('div',
        h('div.small.muted', { style: { marginBottom: '6px' } }, 'Параметры:'),
        h('table.dev-table',
          h('thead', h('tr', h('th', 'Параметр'), h('th', 'Тип'), h('th', 'Описание'))),
          h('tbody', ...ep.params.map(([n, t, d]) => h('tr', h('td', h('code', n)), h('td', t), h('td', d)))))) : null,
      codeBlock('Пример ответа JSON', ep.response)));
}

function buildDocsSection(scopes, sec) {
  const origin = location.origin;

  const userEps = [
    {
      method: 'GET', path: '/api/v1/site', scope: 'Публичный',
      summary: 'Общая информация о сайте и URL OAuth-эндпоинтов',
      params: [],
      response: JSON.stringify({ site_name: state.config.site_name || 'DCMS Social', api_version: '1.0', users_total: 1240, online_now: 38, oauth_auth: '/oauth/authorize', oauth_token: '/oauth/token', oauth_revoke: '/oauth/revoke' }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/token/info', scope: 'Любой токен',
      summary: 'Проверка активности токена и списка выданных разрешений (introspection)',
      params: [],
      response: JSON.stringify({ active: true, user_id: 1, nick: 'Admin', app_id: 3, app_name: 'MyApp', scopes: ['profile', 'email', 'friends'], expires_at: 1795462000, created_at: 1795458400 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me', scope: 'profile (+ email)',
      summary: 'Данные текущего авторизованного пользователя',
      params: [],
      response: JSON.stringify({ id: 1, nick: 'Admin', name: 'Алексей', gender: 'male', avatar: origin + '/api/user/1/avatar?s=200', city: 'Москва', group: 'Создатель', level: 10, rating: 420, online: true, email: 'user@example.com (при scope=email)' }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users', scope: 'profile',
      summary: 'Поиск пользователей сайта с пагинацией',
      params: [['q', 'string', 'Поиск по нику или имени'], ['page', 'int', 'Страница (по умолчанию 1)'], ['per', 'int', 'На странице (1–100)']],
      response: JSON.stringify({ items: [{ id: 1, nick: 'Admin', online: true, rating: 420 }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users/{id}', scope: 'profile',
      summary: 'Профиль конкретного пользователя по ID (с учётом приватности)',
      params: [['id', 'int (path)', 'ID пользователя']],
      response: JSON.stringify({ id: 2, nick: 'Elena', name: 'Елена', gender: 'female', city: 'Казань', group: 'Пользователь', online: false }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/friends', scope: 'friends',
      summary: 'Список друзей авторизованного пользователя',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице (до 100)']],
      response: JSON.stringify({ items: [{ id: 2, nick: 'Elena', online: true, avatar: '/api/user/2/avatar?s=100' }], total: 1, page: 1, per: 30, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users/{id}/friends', scope: 'friends',
      summary: 'Список друзей указанного пользователя',
      params: [['id', 'int (path)', 'ID пользователя'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 5, nick: 'Max', online: false }], total: 1, page: 1, per: 30, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users/{id}/wall', scope: 'wall',
      summary: 'Записи со стены пользователя',
      params: [['id', 'int (path)', 'ID владельца стены'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 15, wall_id: 1, author: { id: 2, nick: 'Elena' }, text: 'Привет!', likes: 3, time: 1795458000 }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/me/wall', scope: 'wall_write',
      summary: 'Опубликовать новую запись на стене авторизованного пользователя',
      params: [['text', 'string (JSON body)', 'Текст записи (до 1024 символов, BBCode)']],
      response: JSON.stringify({ id: 16, wall_id: 1, author: { id: 1, nick: 'Admin' }, text: 'Запись через API!', time: 1795458500 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/notifications', scope: 'notifications',
      summary: 'Уведомления авторизованного пользователя',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 42, type: 'like', object_id: 15, from: { id: 2, nick: 'Elena' }, read: false, time: 1795458100 }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
  ];

  const msgEps = [
    {
      method: 'GET', path: '/api/v1/me/messages', scope: 'messages (опасное)',
      summary: 'Список личных диалогов авторизованного пользователя',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице (до 100)']],
      response: JSON.stringify({ items: [{ peer: { id: 2, nick: 'Elena', online: true }, last_message: 'Привет! Как дела?', last_time: 1795458300, unread: 1, favorite: false }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/messages/{id}', scope: 'messages (опасное)',
      summary: 'История личной переписки с пользователем {id}',
      params: [['id', 'int (path)', 'ID собеседника'], ['page', 'int', 'Страница'], ['per', 'int', 'Сообщений на странице (до 100)']],
      response: JSON.stringify({ peer: { id: 2, nick: 'Elena' }, items: [{ id: 105, from: { id: 2, nick: 'Elena' }, to: { id: 1, nick: 'Admin' }, text: 'Привет!', read: true, pinned: false, reply_id: 0, time: 1795458300 }], total: 1, page: 1, per: 30, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/me/messages/{id}', scope: 'messages_write (опасное)',
      summary: 'Отправить личное сообщение пользователю {id}',
      params: [['id', 'int (path)', 'ID получателя'], ['text', 'string (JSON body)', 'Текст (1–1024 символа, BBCode)'], ['reply_id', 'int (JSON body)', 'ID сообщения для ответа (необязательно)']],
      response: JSON.stringify({ id: 106, from: { id: 1, nick: 'Admin' }, to: { id: 2, nick: 'Elena' }, text: 'Отправлено через API!', reply_id: 0, time: 1795458600 }, null, 2),
    },
  ];

  const groupCatalogEps = [
    {
      method: 'GET', path: '/api/v1/groups', scope: 'groups',
      summary: 'Каталог открытых сообществ (с поиском и фильтром по тематике)',
      params: [['q', 'string', 'Поиск по названию/описанию'], ['category', 'string', 'Тематика'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 1, name: 'Разработчики', category: 'Технологии', members: 42, private: false }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/groups', scope: 'groups',
      summary: 'Список сообществ, в которых состоит пользователь',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 1, name: 'Разработчики', members: 42 }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}', scope: 'groups',
      summary: 'Карточка сообщества и счётчики всех разделов',
      params: [['id', 'int (path)', 'ID сообщества']],
      response: JSON.stringify({ id: 1, name: 'Разработчики', members: 42, enable_blog: true, enable_forum: true, enable_chat: true, enable_files: true, enable_messages: true, counts: { blog: 5, forum: 12, chat: 80, files: 4 } }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/join', scope: 'groups_write',
      summary: 'Вступить в сообщество (или подать заявку в приватное)',
      params: [['id', 'int (path)', 'ID сообщества']],
      response: JSON.stringify({ joined: true, members: 43 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/leave', scope: 'groups_write',
      summary: 'Покинуть сообщество или отменить заявку',
      params: [['id', 'int (path)', 'ID сообщества']],
      response: JSON.stringify({ ok: true }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}/members', scope: 'groups',
      summary: 'Участники сообщества и их роли (creator, admin, модераторы)',
      params: [['id', 'int (path)', 'ID сообщества'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ user: { id: 1, nick: 'Admin' }, role: 'creator', mod_blog: false, mod_forum: false, mod_chat: false, mod_files: false, mod_messages: false }], total: 1, page: 1, per: 30, pages: 1 }, null, 2),
    },
  ];

  const groupContentEps = [
    {
      method: 'GET', path: '/api/v1/groups/{id}/blog', scope: 'groups',
      summary: 'Записи блога сообщества',
      params: [['id', 'int (path)', 'ID сообщества'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 3, title: 'Анонс', text: 'Текст поста', likes: 5, komms: 2, author: { id: 1, nick: 'Admin' } }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/blog', scope: 'groups_write',
      summary: 'Опубликовать запись в блоге сообщества',
      params: [['id', 'int (path)', 'ID сообщества'], ['title', 'string (JSON body)', 'Заголовок'], ['text', 'string (JSON body)', 'Текст (1–4096 символов)']],
      response: JSON.stringify({ id: 4, title: 'Новость', text: 'Опубликовано через API', time: 1795459000 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}/subforums', scope: 'groups',
      summary: 'Подфорумы внутри форума сообщества',
      params: [['id', 'int (path)', 'ID сообщества']],
      response: JSON.stringify({ items: [{ id: 1, name: 'Общий раздел', opis: 'Основные обсуждения', topics: 12 }] }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}/forum', scope: 'groups',
      summary: 'Темы форума сообщества (все или по подфоруму ?sub=ID)',
      params: [['id', 'int (path)', 'ID сообщества'], ['sub', 'int', 'ID подфорума (необязательно)'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 7, sub_id: 1, name: 'Обсуждение', posts: 5, pinned: false, closed: false }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/forum', scope: 'groups_write',
      summary: 'Создать новую тему на форуме сообщества',
      params: [['id', 'int (path)', 'ID сообщества'], ['sub', 'int (JSON body)', 'ID подфорума (0 = первый)'], ['name', 'string (JSON body)', 'Название темы'], ['text', 'string (JSON body)', 'Первое сообщение']],
      response: JSON.stringify({ id: 8, group_id: 1, sub_id: 1, name: 'Новая тема', text: 'Привет!', time: 1795459100 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/group-topics/{tid}', scope: 'groups',
      summary: 'Открыть тему форума сообщества и ответы в ней',
      params: [['tid', 'int (path)', 'ID темы'], ['page', 'int', 'Страница'], ['per', 'int', 'Ответов на странице']],
      response: JSON.stringify({ topic: { id: 8, name: 'Новая тема', text: 'Привет!' }, items: [{ id: 12, text: 'Ответ в теме', author: { id: 2, nick: 'Elena' } }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/group-topics/{tid}/posts', scope: 'groups_write',
      summary: 'Ответить в теме форума сообщества',
      params: [['tid', 'int (path)', 'ID темы'], ['text', 'string (JSON body)', 'Текст ответа (1–4096 символов)']],
      response: JSON.stringify({ id: 13, topic_id: 8, text: 'Отличный пост!', time: 1795459200 }, null, 2),
    },
  ];

  const groupCommEps = [
    {
      method: 'GET', path: '/api/v1/groups/{id}/chat', scope: 'groups',
      summary: 'Последние сообщения чата сообщества',
      params: [['id', 'int (path)', 'ID сообщества'], ['limit', 'int', 'Количество (1–100, по умолчанию 50)']],
      response: JSON.stringify({ items: [{ id: 21, author: { id: 1, nick: 'Admin' }, text: 'Всем привет в чате!', time: 1795459300 }] }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/chat', scope: 'groups_write',
      summary: 'Отправить сообщение в чат сообщества',
      params: [['id', 'int (path)', 'ID сообщества'], ['text', 'string (JSON body)', 'Текст (1–1024 символа)']],
      response: JSON.stringify({ id: 22, group_id: 1, text: 'Привет из API!', time: 1795459400 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}/files', scope: 'groups',
      summary: 'Файлы сообщества со ссылками на скачивание',
      params: [['id', 'int (path)', 'ID сообщества'], ['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({ items: [{ id: 5, name: 'manual', ext: 'pdf', size: 245760, loads: 10, download_url: '/api/group-files/5/download' }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/dm', scope: 'messages_write (опасное)',
      summary: 'Написать личное сообщение сообществу от имени пользователя',
      params: [['id', 'int (path)', 'ID сообщества'], ['text', 'string (JSON body)', 'Текст (1–2000 символов)']],
      response: JSON.stringify({ id: 30, group_id: 1, text: 'Здравствуйте!', time: 1795459500 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/groups/{id}/inbox', scope: 'messages (опасное)',
      summary: 'Входящие диалоги сообщества (для создателя/админа/мод. сообщений)',
      params: [['id', 'int (path)', 'ID сообщества']],
      response: JSON.stringify({ items: [{ user: { id: 2, nick: 'Elena' }, last_msg: 'Здравствуйте!', unread: 1, last_time: 1795459500 }] }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/groups/{id}/inbox/{uid}', scope: 'messages_write (опасное)',
      summary: 'Ответить пользователю {uid} от лица сообщества (если он писал первым)',
      params: [['id', 'int (path)', 'ID сообщества'], ['uid', 'int (path)', 'ID пользователя'], ['text', 'string (JSON body)', 'Текст ответа']],
      response: JSON.stringify({ id: 31, group_id: 1, user_id: 2, text: 'Добрый день! Чем помочь?', time: 1795459600 }, null, 2),
    },
  ];

  const botEps = [
    {
      method: 'GET', path: '/api/v1/bot/me', scope: 'Токен сообщества',
      summary: 'Информация о сообществе, названии токена и выданных разрешениях',
      params: [],
      response: JSON.stringify({ ok: true, token_name: 'Бот поддержки', scopes: ['messages', 'members'], group: { id: 1, name: 'Разработчики', members: 42 } }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/bot/getUpdates', scope: 'По правам токена',
      summary: 'Long Polling: получение новых событий сообщества (ждёт до timeout сек)',
      params: [
        ['offset', 'int', 'ID первого нужного события (передавайте last_update_id + 1)'],
        ['limit', 'int', 'Максимум событий за запрос (1–100, по умолчанию 50)'],
        ['timeout', 'int', 'Таймаут ожидания новых событий в секундах (0–30, рекомендуется 25)'],
      ],
      response: JSON.stringify({ ok: true, result: [{ update_id: 15, type: 'message_new', time: 1795460000, data: { id: 42, from: { id: 2, nick: 'Elena' }, text: '/start', time: 1795460000 } }] }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/sendMessage', scope: 'messages',
      summary: 'Ответить пользователю в ЛС от лица сообщества (поддерживает inline-кнопки и нижнюю клавиатуру)',
      params: [
        ['user_id', 'int (JSON body)', 'ID пользователя-получателя'],
        ['text', 'string (JSON body)', 'Текст ответа (1–2000 символов, BBCode)'],
        ['inline_keyboard', '[][]Button', 'Кнопки под сообщением: [[{"text":"Ок","callback_data":"ok","color":"primary|positive|negative|default","url":"..."}]]. Событие: inline_button_click'],
        ['keyboard', '[][]Button', 'Нижняя клавиатура под полем ввода: [[{"text":"Меню","callback_data":"menu","color":"primary"}]]. Событие: keyboard_button_click'],
        ['one_time', 'bool', 'Скрыть нижнюю клавиатуру после первого нажатия'],
        ['remove_keyboard', 'bool', 'Удалить нижнюю клавиатуру у пользователя (true)'],
      ],
      response: JSON.stringify({ ok: true, message: { id: 43, group_id: 1, user_id: 2, text: 'Выберите действие:', time: 1795460005 } }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/setMyCommands', scope: 'Токен сообщества',
      summary: 'Установить список команд бота для быстрого меню «/» у пользователей',
      params: [['commands', '[]Command (JSON body)', 'Массив команд: [{"command":"start","description":"Главное меню"}] (до 30 команд)']],
      response: JSON.stringify({ ok: true, commands: [{ command: 'start', description: 'Главное меню' }] }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/bot/getMyCommands', scope: 'Токен сообщества',
      summary: 'Получить текущий список команд бота',
      params: [],
      response: JSON.stringify({ ok: true, commands: [{ command: 'start', description: 'Главное меню' }] }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/bot/inbox', scope: 'messages',
      summary: 'Список входящих личных диалогов сообщества',
      params: [],
      response: JSON.stringify({ ok: true, items: [{ user: { id: 2, nick: 'Elena' }, last_msg: '/start', unread: 1, last_time: 1795460000 }] }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/bot/requests', scope: 'members',
      summary: 'Список заявок на вступление в приватное сообщество',
      params: [],
      response: JSON.stringify({ ok: true, items: [{ user: { id: 5, nick: 'Max' }, time: 1795460100 }] }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/approveRequest', scope: 'members',
      summary: 'Автоматически одобрить заявку на вступление',
      params: [['user_id', 'int (JSON body)', 'ID пользователя']],
      response: JSON.stringify({ ok: true }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/rejectRequest', scope: 'members',
      summary: 'Отклонить заявку на вступление',
      params: [['user_id', 'int (JSON body)', 'ID пользователя']],
      response: JSON.stringify({ ok: true }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/sendChat', scope: 'chat',
      summary: 'Отправить сообщение в чат сообщества',
      params: [['text', 'string (JSON body)', 'Текст сообщения (1–1024 символа)']],
      response: JSON.stringify({ ok: true, message: { id: 88, group_id: 1, text: 'Бот на связи!', time: 1795460200 } }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/createPost', scope: 'blog',
      summary: 'Опубликовать запись в блоге сообщества',
      params: [['title', 'string (JSON body)', 'Заголовок (необязательно)'], ['text', 'string (JSON body)', 'Текст (1–4096 символов)']],
      response: JSON.stringify({ ok: true, post: { id: 12, title: 'Дайджест', text: 'Новости дня…', time: 1795460300 } }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/createTopic', scope: 'forum',
      summary: 'Создать тему на форуме сообщества',
      params: [['sub', 'int (JSON body)', 'ID подфорума (0 = первый)'], ['name', 'string (JSON body)', 'Название'], ['text', 'string (JSON body)', 'Текст']],
      response: JSON.stringify({ ok: true, topic: { id: 19, group_id: 1, sub_id: 1, name: 'Обсуждение', text: 'Текст темы', time: 1795460400 } }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/bot/replyTopic', scope: 'forum',
      summary: 'Ответить в теме форума сообщества',
      params: [['topic_id', 'int (JSON body)', 'ID темы'], ['text', 'string (JSON body)', 'Текст ответа (1–4096 символов)']],
      response: JSON.stringify({ ok: true, post: { id: 54, topic_id: 19, text: 'Принято!', time: 1795460500 } }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/bot/files', scope: 'files',
      summary: 'Список файлов сообщества со ссылками на скачивание',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице (до 100)']],
      response: JSON.stringify({ items: [{ id: 5, name: 'rules', ext: 'pdf', size: 124000, download_url: '/api/group-files/5/download' }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
  ];

  const allGroupEps = [...groupCatalogEps, ...groupContentEps, ...groupCommEps];
  const allEndpoints = [...userEps, ...msgEps, ...allGroupEps, ...botEps];

  const SECTIONS = [
    { key: 'oauth', title: 'OAuth 2.0: старт', icon: 'key' },
    { key: 'scopes', title: 'Разрешения (Scopes)', icon: 'shield', cnt: scopes.length },
    { key: 'users', title: 'Пользователи и стена', icon: 'user', cnt: userEps.length },
    { key: 'messages', title: 'Личные сообщения', icon: 'mail', cnt: msgEps.length },
    { key: 'groups', title: 'Сообщества', icon: 'users', cnt: allGroupEps.length },
    { key: 'bots', title: 'Боты (Long Poll)', icon: 'message-circle', cnt: botEps.length },
    { key: 'playground', title: 'Песочница API', icon: 'play' },
  ];
  const curIdx = Math.max(0, SECTIONS.findIndex(s => s.key === sec));
  const curSec = SECTIONS[curIdx].key;

  const sideNav = h('nav.dev-sidenav',
    h('div.dev-sidenav-title', 'Разделы API'),
    ...SECTIONS.map(s => h('a.dev-nav-link' + (s.key === curSec ? '.active' : ''), { href: `/developers?tab=docs&sec=${s.key}` },
      icon(s.icon, 16), h('span', s.title), s.cnt ? h('span.dev-nav-cnt', String(s.cnt)) : null)));

  // Живой поиск по всем методам
  const searchIn = h('input.input', { placeholder: 'Быстрый поиск метода по пути или описанию (например: getUpdates, wall, messages)…' });
  const searchResults = h('div.stack-s');
  const sectionHost = h('div.stack');

  searchIn.oninput = () => {
    const q = searchIn.value.trim().toLowerCase();
    if (!q) {
      searchResults.replaceChildren();
      sectionHost.style.display = '';
      return;
    }
    sectionHost.style.display = 'none';
    const found = allEndpoints.filter(ep =>
      ep.path.toLowerCase().includes(q) || ep.summary.toLowerCase().includes(q) || ep.scope.toLowerCase().includes(q));
    searchResults.replaceChildren(
      h('div.small.muted', `Найдено методов: ${found.length}`),
      ...(found.length ? found.map(renderEpCard) : [ui.card(ui.empty('Методы по вашему запросу не найдены', 'search'))]));
  };

  // Контент выбранного раздела
  if (curSec === 'oauth') {
    sectionHost.append(
      h('div.card.pad.stack',
        h('h2', 'Быстрый старт: OAuth 2.0 авторизация'),
        h('p.muted', 'Подключайте вход через наш сайт на сторонних сайтах, в мобильных приложениях и сервисах по стандарту OAuth 2.0 (Authorization Code + опциональный PKCE). Все методы API поддерживают CORS и возвращают JSON (UTF-8).'),
        h('div.stack-s',
          h('b', 'Шаг 1. Создайте приложение'),
          h('div.small.muted', 'Откройте вкладку «Мои приложения», укажите Redirect URI и получите ', h('code', 'client_id'), ' и ', h('code', 'client_secret'), '. Там же есть удобный конструктор ссылки авторизации.'),
          h('b', 'Шаг 2. Перенаправьте пользователя на экран согласия'),
          codeBlock('GET URL авторизации', `${origin}/oauth/authorize?client_id=ВАШ_CLIENT_ID&redirect_uri=https://your-site.ru/callback&response_type=code&scope=profile%20email%20friends&state=random123`),
          h('div.small.muted', 'После подтверждения пользователь вернётся на ', h('code', 'redirect_uri?code=...&state=...'), ' (код одноразовый, действует 5 минут). При отказе — ', h('code', '?error=access_denied'), '.'),
          h('b', 'Шаг 3. Обменяйте code на access_token'),
          multiLangCode({
            'cURL': `curl -X POST ${origin}/oauth/token \\\n  -H "Content-Type: application/x-www-form-urlencoded" \\\n  -d "grant_type=authorization_code" \\\n  -d "client_id=ВАШ_CLIENT_ID" \\\n  -d "client_secret=ВАШ_CLIENT_SECRET" \\\n  -d "redirect_uri=https://your-site.ru/callback" \\\n  -d "code=ПОЛУЧЕННЫЙ_CODE"`,
            'JavaScript': `const res = await fetch('${origin}/oauth/token', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({\n    grant_type: 'authorization_code',\n    client_id: 'ВАШ_CLIENT_ID',\n    client_secret: 'ВАШ_CLIENT_SECRET',\n    redirect_uri: 'https://your-site.ru/callback',\n    code: 'ПОЛУЧЕННЫЙ_CODE'\n  })\n});\nconst { access_token, refresh_token, scope } = await res.json();`,
            'PHP': `$ch = curl_init('${origin}/oauth/token');\ncurl_setopt_array($ch, [\n  CURLOPT_POST => true,\n  CURLOPT_RETURNTRANSFER => true,\n  CURLOPT_POSTFIELDS => http_build_query([\n    'grant_type'    => 'authorization_code',\n    'client_id'     => 'ВАШ_CLIENT_ID',\n    'client_secret' => 'ВАШ_CLIENT_SECRET',\n    'redirect_uri'  => 'https://your-site.ru/callback',\n    'code'          => $_GET['code'],\n  ]),\n]);\n$data = json_decode(curl_exec($ch), true);\n$token = $data['access_token'];`,
            'Python': `import requests\n\nr = requests.post('${origin}/oauth/token', json={\n    'grant_type': 'authorization_code',\n    'client_id': 'ВАШ_CLIENT_ID',\n    'client_secret': 'ВАШ_CLIENT_SECRET',\n    'redirect_uri': 'https://your-site.ru/callback',\n    'code': code,\n})\ntoken = r.json()['access_token']`,
          }),
          h('b', 'Шаг 4. Запросы к API с заголовком Authorization: Bearer'),
          multiLangCode({
            'cURL': `curl ${origin}/api/v1/me \\\n  -H "Authorization: Bearer ВАШ_ACCESS_TOKEN"`,
            'JavaScript': `const me = await fetch('${origin}/api/v1/me', {\n  headers: { Authorization: 'Bearer ' + access_token }\n}).then(r => r.json());`,
            'Python': `me = requests.get('${origin}/api/v1/me', headers={'Authorization': f'Bearer {token}'}).json()`,
          }),
          h('b', 'Шаг 5. Продление (refresh_token) и отзыв токена'),
          codeBlock('POST /oauth/token и POST /oauth/revoke', `# Продлить токен:\ncurl -X POST ${origin}/oauth/token -H "Content-Type: application/json" \\\n  -d '{"grant_type":"refresh_token","client_id":"...","client_secret":"...","refresh_token":"dcms_rt_..."}'\n\n# Отозвать токен:\ncurl -X POST ${origin}/oauth/revoke -H "Content-Type: application/json" \\\n  -d '{"token":"dcms_at_..."}'`))),
      h('div.card.pad.stack-s',
        h('h3', 'Формат ошибок'),
        h('p.small.muted', 'При любой ошибке сервер возвращает JSON ', h('code', '{"error": "код_ошибки", "error_description": "описание"}'), ' со стандартными HTTP-статусами: ', h('code', '400'), ' (неверные параметры), ', h('code', '401'), ' (неверный/истёкший токен), ', h('code', '403'), ' (недостаточно прав или scope), ', h('code', '404'), ' (объект не найден).')));
  } else if (curSec === 'scopes') {
    const botScopes = [
      ['messages', 'Личные сообщения', 'События message_new, inline_button_click, keyboard_button_click и ответы от лица сообщества'],
      ['chat', 'Чат сообщества', 'Событие chat_message_new и отправка сообщений в чат'],
      ['forum', 'Форум сообщества', 'События forum_topic_new, forum_post_new и публикация тем/ответов'],
      ['blog', 'Блог сообщества', 'Событие blog_post_new и публикация записей в блог'],
      ['members', 'Участники и заявки', 'События join_request, member_join, member_leave и одобрение/отклонение заявок'],
      ['files', 'Файлы сообщества', 'Просмотр списка файлов сообщества'],
    ];
    sectionHost.append(
      h('div.card.pad.stack',
        h('h2', 'Разрешения OAuth 2.0 (пользовательские токены)'),
        h('p.small.muted', 'Передавайте нужные разрешения через пробел в параметре ', h('code', 'scope'), '. Опасные разрешения выделяются красным на экране согласия, и пользователь может снять с них галочку — проверяйте фактически выданный список в поле ', h('code', 'scope'), ' ответа ', h('code', 'POST /oauth/token'), '.'),
        h('div', { style: { overflowX: 'auto' } },
          h('table.dev-table',
            h('thead', h('tr', h('th', 'Scope'), h('th', 'Название'), h('th', 'Что даёт'))),
            h('tbody', ...scopes.map(s => h('tr',
              h('td', h('code', s.name), s.danger ? [' ', h('span.badge.danger', 'Опасное')] : null),
              h('td', h('b', s.title)), h('td', s.desc))))))),
      h('div.card.pad.stack',
        h('h2', 'Разрешения токенов сообществ (Bot API)'),
        h('p.small.muted', 'Выбираются создателем или администратором при выпуске токена ', h('code', 'dcms_grp_...'), ' в настройках сообщества.'),
        h('div', { style: { overflowX: 'auto' } },
          h('table.dev-table',
            h('thead', h('tr', h('th', 'Scope'), h('th', 'Название'), h('th', 'Доступные события и методы'))),
            h('tbody', ...botScopes.map(([k, t, d]) => h('tr', h('td', h('code', k)), h('td', h('b', t)), h('td', d))))))));
  } else if (curSec === 'users') {
    sectionHost.append(
      h('div.card.pad.stack-s',
        h('h2', 'Пользователи, друзья, стена и уведомления'),
        h('p.small.muted', 'Базовый URL: ', h('code', origin), '. Нажмите на любой метод, чтобы раскрыть параметры и пример JSON-ответа.'),
        h('div.stack-s', ...userEps.map(renderEpCard))));
  } else if (curSec === 'messages') {
    sectionHost.append(
      h('div.card.pad.stack-s',
        h('h2', 'Личные сообщения'),
        h('div.oauth-danger-banner', icon('lock', 18), h('div', h('b', 'Опасные разрешения: '), 'для методов ниже требуются ', h('code', 'messages'), ' и ', h('code', 'messages_write'), '. Пользователь может снять с них галочку при OAuth-авторизации.')),
        h('div.stack-s', ...msgEps.map(renderEpCard))));
  } else if (curSec === 'groups') {
    sectionHost.append(
      h('div.card.pad.stack-s',
        h('h2', 'API Сообществ (от имени пользователя)'),
        h('p.small.muted', 'Для чтения требуется ', h('code', 'groups'), ', для публикаций и вступления — ', h('code', 'groups_write'), '. Для автоматизации без пользовательского токена используйте раздел ', h('a', { href: '/developers?tab=docs&sec=bots' }, 'Боты (Long Poll)'), '.'),
        h('div.dev-group-h', 'Каталог, участие и роли'),
        h('div.stack-s', ...groupCatalogEps.map(renderEpCard)),
        h('div.dev-group-h', 'Блог и Форум'),
        h('div.stack-s', ...groupContentEps.map(renderEpCard)),
        h('div.dev-group-h', 'Чат, Файлы и Личные сообщения сообщества'),
        h('div.stack-s', ...groupCommEps.map(renderEpCard))));
  } else if (curSec === 'bots') {
    const evTable = [
      ['message_new', 'messages', 'Пользователь отправил новое ЛС сообществу'],
      ['inline_button_click', 'messages', 'Нажатие inline-кнопки, прикреплённой к сообщению бота'],
      ['keyboard_button_click', 'messages', 'Нажатие кнопки в нижней клавиатуре под полем ввода'],
      ['chat_message_new', 'chat', 'Новое сообщение в чате сообщества'],
      ['forum_topic_new', 'forum', 'Создана новая тема на форуме сообщества'],
      ['forum_post_new', 'forum', 'Новый ответ в теме форума сообщества'],
      ['blog_post_new', 'blog', 'Опубликована новая запись в блоге сообщества'],
      ['join_request', 'members', 'Новая заявка на вступление в приватное сообщество'],
      ['member_join', 'members', 'Пользователь вступил в сообщество'],
      ['member_leave', 'members', 'Пользователь покинул сообщество'],
    ];
    sectionHost.append(
      h('div.card.pad.stack',
        h('h2', 'Боты сообществ (Bot API · Long Polling)'),
        h('p.small.muted', 'Создайте токен сообщества ', h('code', 'dcms_grp_...'), ' в разделе ', h('b', 'Сообщество → Настройки → Боты и API-токены'), '. Передавайте его в заголовке ', h('code', 'Authorization: Bearer dcms_grp_...'), '.'),
        h('div.dev-group-h', 'Методы Bot API v1'),
        h('div.stack-s', ...botEps.map(renderEpCard))),
      h('div.card.pad.stack-s',
        h('h3', 'События Long Poll (getUpdates)'),
        h('div', { style: { overflowX: 'auto' } },
          h('table.dev-table',
            h('thead', h('tr', h('th', 'Тип события'), h('th', 'Нужный scope'), h('th', 'Описание'))),
            h('tbody', ...evTable.map(([t, s, d]) => h('tr', h('td', h('code', t)), h('td', h('code', s)), h('td', d))))))),
      h('div.card.pad.stack-s',
        h('h3', 'Примеры ботов на Long Polling'),
        multiLangCode({
          'Python (кнопки + команды)': `import requests\n\nURL = '${origin}'\nH = {'Authorization': 'Bearer dcms_grp_ВАШ_ТОКЕН'}\n\n# 1. Устанавливаем команды меню "/"\nrequests.post(f'{URL}/api/v1/bot/setMyCommands', json={\n    'commands': [{'command': 'start', 'description': 'Главное меню'}]\n}, headers=H)\n\n# 2. Цикл Long Polling\noffset = 0\nwhile True:\n    r = requests.get(f'{URL}/api/v1/bot/getUpdates', params={'offset': offset, 'timeout': 25}, headers=H).json()\n    for u in r.get('result', []):\n        offset = u['update_id'] + 1\n        if u['type'] == 'message_new':\n            uid = u['data']['from']['id']\n            requests.post(f'{URL}/api/v1/bot/sendMessage', json={\n                'user_id': uid,\n                'text': 'Привет! Выберите кнопку:',\n                'inline_keyboard': [[{'text': '👍 Ок', 'callback_data': 'ok', 'color': 'positive'}]],\n                'keyboard': [[{'text': '📋 Инфо', 'callback_data': 'info', 'color': 'primary'}]],\n            }, headers=H)\n        elif u['type'] == 'inline_button_click':\n            uid = u['data']['from']['id']\n            requests.post(f'{URL}/api/v1/bot/sendMessage', json={'user_id': uid, 'text': 'Нажата inline-кнопка!'}, headers=H)\n        elif u['type'] == 'keyboard_button_click':\n            uid = u['data']['from']['id']\n            requests.post(f'{URL}/api/v1/bot/sendMessage', json={'user_id': uid, 'text': 'Нажата нижняя кнопка!'}, headers=H)`,
          'cURL': `# Получить события (Long Poll 25с):\ncurl "${origin}/api/v1/bot/getUpdates?offset=0&timeout=25" \\\n  -H "Authorization: Bearer dcms_grp_ВАШ_ТОКЕН"\n\n# Ответить с inline-кнопкой:\ncurl -X POST ${origin}/api/v1/bot/sendMessage \\\n  -H "Authorization: Bearer dcms_grp_ВАШ_ТОКЕН" \\\n  -H "Content-Type: application/json" \\\n  -d '{"user_id":2,"text":"Подтвердить?","inline_keyboard":[[{"text":"Да","callback_data":"yes","color":"positive"}]]}'`,
        })));
  } else if (curSec === 'playground') {
    const tokIn = h('input.input', { placeholder: 'Вставьте access_token (dcms_at_...) или токен бота (dcms_grp_...)' });
    const pathIn = h('input.input', { value: '/api/v1/site', placeholder: '/api/v1/...' });
    const presetSel = h('select.input',
      h('option', { value: '/api/v1/site' }, 'GET /api/v1/site (публичный)'),
      h('option', { value: '/api/v1/me' }, 'GET /api/v1/me (OAuth: profile)'),
      h('option', { value: '/api/v1/token/info' }, 'GET /api/v1/token/info (OAuth)'),
      h('option', { value: '/api/v1/me/friends' }, 'GET /api/v1/me/friends (OAuth: friends)'),
      h('option', { value: '/api/v1/groups' }, 'GET /api/v1/groups (OAuth: groups)'),
      h('option', { value: '/api/v1/me/groups' }, 'GET /api/v1/me/groups (OAuth: groups)'),
      h('option', { value: '/api/v1/me/messages' }, 'GET /api/v1/me/messages (OAuth: messages)'),
      h('option', { value: '/api/v1/bot/me' }, 'GET /api/v1/bot/me (Bot token)'),
      h('option', { value: '/api/v1/bot/getUpdates?offset=0&timeout=0' }, 'GET /api/v1/bot/getUpdates (Bot token)'),
      h('option', { value: '/api/v1/bot/inbox' }, 'GET /api/v1/bot/inbox (Bot token)'));
    presetSel.onchange = () => { pathIn.value = presetSel.value; };
    const resPre = h('pre.dev-code', '// Выберите метод и нажмите «Выполнить запрос»');
    const statusSpan = h('span', 'Готов к тесту');
    const runTest = async () => {
      statusSpan.textContent = 'Выполняется…';
      const headers = {};
      if (tokIn.value.trim()) headers.Authorization = 'Bearer ' + tokIn.value.trim();
      try {
        const resp = await fetch(pathIn.value.trim() || '/api/v1/site', { headers });
        const txt = await resp.text();
        statusSpan.textContent = `HTTP ${resp.status} ${resp.statusText}`;
        try { resPre.textContent = JSON.stringify(JSON.parse(txt), null, 2); }
        catch { resPre.textContent = txt; }
      } catch (e) {
        statusSpan.textContent = 'Ошибка сети';
        resPre.textContent = String(e);
      }
    };
    sectionHost.append(
      h('div.card.pad.stack',
        h('h2', 'Песочница API (живой тест)'),
        h('div.field', h('label', 'Быстрый выбор метода'), presetSel),
        h('div.field', h('label', 'Путь GET-запроса (можно отредактировать вручную)'), pathIn),
        h('div.field', h('label', 'Bearer Token (OAuth или Bot)'), tokIn),
        h('div.row', ui.btn('Выполнить запрос', { ic: 'play', kind: 'primary', onclick: runTest })),
        h('div.dev-code-wrap', h('div.dev-code-head', statusSpan), resPre)));
  }

  const prevSec = curIdx > 0 ? SECTIONS[curIdx - 1] : null;
  const nextSec = curIdx < SECTIONS.length - 1 ? SECTIONS[curIdx + 1] : null;
  const pager = h('div.dev-pager',
    prevSec ? ui.btn(`← ${prevSec.title}`, { href: `/developers?tab=docs&sec=${prevSec.key}` }) : h('span'),
    nextSec ? ui.btn(`${nextSec.title} →`, { kind: 'primary', href: `/developers?tab=docs&sec=${nextSec.key}` }) : h('span'));

  return h('div.dev-layout',
    sideNav,
    h('div.dev-content',
      h('div.card.pad', searchIn),
      searchResults,
      sectionHost,
      pager));
}

// ───────── КОНСТРУКТОР ССЫЛКИ OAuth-АВТОРИЗАЦИИ
function buildAuthUrlBuilder(app, scopes) {
  const uris = app.redirect_uris || [];
  if (!uris.length) return null;

  const chosen = new Set(['profile']);
  const uriSel = uris.length > 1
    ? h('select.input', ...uris.map(u => h('option', { value: u }, u)))
    : h('input.input', { value: uris[0], readOnly: true });
  const stateIn = h('input.input', { placeholder: 'Необязательно (например: random123)', maxlength: 128 });
  const urlBox = h('div.dev-url-box');
  let curPath = '';
  let curFull = '';

  const rebuild = () => {
    const scList = scopes.map(s => s.name).filter(n => chosen.has(n));
    const params = new URLSearchParams({
      client_id: app.client_id,
      redirect_uri: uriSel.value,
      response_type: 'code',
      scope: scList.length ? scList.join(' ') : 'profile',
    });
    const st = stateIn.value.trim();
    if (st) params.set('state', st);
    curPath = '/oauth/authorize?' + params.toString();
    curFull = location.origin + curPath;
    urlBox.textContent = curFull;
  };

  uriSel.onchange = rebuild;
  stateIn.oninput = rebuild;

  const pills = scopes.map(sc => {
    const cb = h('input', { type: 'checkbox', checked: chosen.has(sc.name) });
    cb.onchange = () => {
      if (cb.checked) chosen.add(sc.name);
      else chosen.delete(sc.name);
      rebuild();
    };
    return h('label.dev-scope-pill' + (sc.danger ? '.danger' : ''), { title: sc.desc },
      cb,
      h('span', sc.title),
      h('code', { style: { fontSize: '.74rem', opacity: '.75' } }, sc.name),
      sc.danger ? h('span.badge.danger', 'опасное') : null);
  });

  rebuild();

  return h('details.dev-builder',
    h('summary', icon('link', 16), ' Конструктор ссылки авторизации'),
    h('div.dev-builder-body',
      h('div.field', h('label', 'Выберите разрешения (scopes)'), h('div.dev-scope-grid', ...pills)),
      h('div.row',
        h('div.field.grow', h('label', 'Redirect URI'), uriSel),
        h('div.field', { style: { minWidth: '180px' } }, h('label', 'Параметр state'), stateIn)),
      h('div.field', h('label', 'Готовая ссылка для перехода пользователя'), urlBox),
      h('div.row',
        ui.btn('Скопировать ссылку', { ic: 'copy', kind: 'primary', small: true, onclick: () => copyText(curFull, 'Ссылка авторизации скопирована') }),
        ui.btn('Открыть и проверить', { ic: 'key', small: true, onclick: () => router.go(curPath) }))));
}

// ───────── РАЗДЕЛ ДЛЯ РАЗРАБОТЧИКОВ (/developers)
router.add('/developers', async ctx => {
  const tab = ctx.query.tab || (me() ? 'docs' : 'docs');
  ctx.title('Разработчикам · OAuth и API');
  const reload = () => router.render();

  const scData = await api.get('/api/oauth/scopes');
  const scopes = scData.scopes || [];

  const tabs = [{ key: 'docs', label: 'Документация API', href: '/developers?tab=docs' }];
  if (me()) {
    tabs.push(
      { key: 'apps', label: 'Мои приложения', href: '/developers?tab=apps' },
      { key: 'grants', label: 'Выданные доступы', href: '/developers?tab=grants' },
    );
  }

  let content;
  if (tab === 'apps' && me()) {
    const d = await api.get('/api/oauth/apps');
    const apps = d.items || [];
    content = h('div.stack',
      h('div.row',
        h('div.small.muted.grow', 'Создавайте OAuth-приложения для авторизации пользователей на ваших сайтах и в сервисах.'),
        ui.btn('Создать приложение', { ic: 'plus', kind: 'primary', onclick: () => openAppModal(null, reload) })),
      apps.length ? h('div.stack', ...apps.map(app => {
        let shown = false;
        const secVal = h('span.dev-cred-val', '••••••••••••••••••••••••••••••••');
        const toggleBtn = ui.btn('Показать', {
          small: true,
          onclick: () => {
            shown = !shown;
            secVal.textContent = shown ? app.client_secret : '••••••••••••••••••••••••••••••••';
            toggleBtn.textContent = shown ? 'Скрыть' : 'Показать';
          },
        });
        return h('div.card.pad.stack-s',
          h('div.row',
            h('h3.grow', app.name),
            h('span.badge', `Пользователей: ${fmt.num(app.active_users)}`),
            ui.btn('Изменить', { ic: 'edit', small: true, onclick: () => openAppModal(app, reload) }),
            ui.btn('', {
              ic: 'trash', small: true, kind: 'danger', title: 'Удалить приложение',
              onclick: async () => {
                if (await ui.confirm(`Удалить приложение «${app.name}» и отозвать все его токены?`, { danger: true })) {
                  await api.del('/api/oauth/apps/' + app.id);
                  ui.ok('Приложение удалено');
                  reload();
                }
              },
            })),
          app.description ? h('div.small.muted', app.description) : null,
          h('div.field', h('label', 'Client ID'),
            h('div.dev-cred-row',
              h('span.dev-cred-val', app.client_id),
              ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => copyText(app.client_id, 'Client ID скопирован') }))),
          h('div.field', h('label', 'Client Secret (храните в секрете на сервере)'),
            h('div.dev-cred-row',
              secVal,
              toggleBtn,
              ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => copyText(app.client_secret, 'Client Secret скопирован') }),
              ui.btn('Обновить', {
                ic: 'refresh', small: true,
                onclick: async () => {
                  if (await ui.confirm('Сгенерировать новый Client Secret? Старый перестанет работать.', { danger: true })) {
                    await api.post(`/api/oauth/apps/${app.id}/secret`);
                    ui.ok('Client Secret обновлён');
                    reload();
                  }
                },
              }))),
          h('div.small.muted', 'Redirect URI: ', (app.redirect_uris || []).join(', ')),
          buildAuthUrlBuilder(app, scopes));
      })) : ui.card(ui.empty('У вас пока нет OAuth-приложений', 'key',
        ui.btn('Создать первое приложение', { ic: 'plus', kind: 'primary', onclick: () => openAppModal(null, reload) }))));
  } else if (tab === 'grants' && me()) {
    const d = await api.get('/api/oauth/grants');
    const items = d.items || [];
    content = ui.card(items.length ? h('div.list', ...items.map(g => h('div.list-item',
      h('span.oauth-scope-ic', icon('key', 18)),
      h('div.main',
        h('div.title', g.name),
        h('div.small.muted', 'Автор: ', ui.userLink(g.owner), ' · выдано ', fmt.ago(g.created_at), g.last_used ? [' · активность ', fmt.ago(g.last_used)] : null),
        h('div.row', { style: { marginTop: '4px' } }, ...(g.scopes || []).map(sc => h('span.badge', sc.title)))),
      ui.btn('Отозвать', {
        ic: 'x', small: true, kind: 'danger',
        onclick: async () => {
          if (await ui.confirm(`Отозвать доступ у приложения «${g.name}»?`, { danger: true })) {
            await api.del('/api/oauth/grants/' + g.app_id);
            ui.ok('Доступ отозван');
            reload();
          }
        },
      })))) : ui.empty('Вы ещё не выдавали доступ сторонним приложениям', 'shield'));
  } else {
    content = buildDocsSection(scopes, ctx.query.sec || 'oauth');
  }

  return [
    ui.pageHead('Разработчикам · OAuth 2.0 и API',
      me() ? ui.btn('Новое приложение', { ic: 'plus', kind: 'primary', small: true, onclick: () => openAppModal(null, () => router.go('/developers?tab=apps')) })
        : ui.btn('Войти для создания приложения', { ic: 'log-in', small: true, href: '/login?return=%2Fdevelopers%3Ftab%3Dapps' })),
    ui.tabs(tabs, tab),
    content,
  ];
});

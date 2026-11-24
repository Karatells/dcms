import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

if (!document.querySelector('link[href="/css/developers.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/developers.css' }));
}

const me = () => state.user;
const copyText = async (txt, label = 'Скопировано') => {
  try { await navigator.clipboard.writeText(txt); ui.ok(label); }
  catch { ui.toast(txt, 'info'); }
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

// ───────── СПРАВОЧНИК МЕТОДОВ API v1
function buildDocsSection(scopes) {
  const origin = location.origin;

  const endpoints = [
    {
      method: 'GET', path: '/api/v1/site', scope: 'Публичный (без токена)',
      summary: 'Общая информация о сайте и URL OAuth-эндпоинтов',
      params: [],
      response: JSON.stringify({
        site_name: state.config.site_name || 'DCMS Social',
        api_version: '1.0',
        users_total: 1240,
        online_now: 38,
        oauth_auth: '/oauth/authorize',
        oauth_token: '/oauth/token',
        oauth_revoke: '/oauth/revoke',
      }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/token/info', scope: 'Любой валидный токен',
      summary: 'Проверка активности токена и списка выданных разрешений (introspection)',
      params: [],
      response: JSON.stringify({
        active: true, user_id: 1, nick: 'Admin', app_id: 3, app_name: 'MyApp',
        scopes: ['profile', 'email', 'friends'], expires_at: 1795462000, created_at: 1795458400,
      }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me', scope: 'profile (+ email опционально)',
      summary: 'Данные текущего авторизованного пользователя',
      params: [],
      response: JSON.stringify({
        id: 1, nick: 'Admin', name: 'Алексей', gender: 'male',
        avatar: origin + '/api/user/1/avatar?s=200', city: 'Москва', about: 'Разработчик',
        group: 'Создатель', level: 10, rating: 420, medal: 3, online: true, last_seen: 1795458400, reg_date: 1700000000,
        email: 'user@example.com (только при scope=email)', telegram: '@username',
      }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users', scope: 'profile',
      summary: 'Поиск пользователей сайта с пагинацией',
      params: [
        ['q', 'string', 'Поисковый запрос по нику или имени (необязательно)'],
        ['page', 'int', 'Номер страницы (по умолчанию 1)'],
        ['per', 'int', 'Количество на странице (1–100, по умолчанию 20)'],
      ],
      response: JSON.stringify({ items: [{ id: 1, nick: 'Admin', online: true, rating: 420 }], total: 1, page: 1, per: 20, pages: 1 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/users/{id}', scope: 'profile',
      summary: 'Профиль конкретного пользователя по ID (учитывает настройки приватности)',
      params: [['id', 'int (path)', 'ID пользователя']],
      response: JSON.stringify({ id: 2, nick: 'Elena', name: 'Елена', gender: 'female', city: 'Казань', group: 'Пользователь', online: false }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/friends', scope: 'friends',
      summary: 'Список друзей авторизованного пользователя',
      params: [['page', 'int', 'Номер страницы'], ['per', 'int', 'Элементов на странице (до 100)']],
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
      response: JSON.stringify({
        items: [{ id: 15, wall_id: 1, author: { id: 2, nick: 'Elena' }, text: 'Привет!', html: 'Привет!', likes: 3, time: 1795458000 }],
        total: 1, page: 1, per: 20, pages: 1,
      }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/me/wall', scope: 'wall_write',
      summary: 'Опубликовать новую запись на стене авторизованного пользователя',
      params: [['text', 'string (JSON body)', 'Текст записи (до 1024 символов, поддерживается BBCode)']],
      response: JSON.stringify({ id: 16, wall_id: 1, author: { id: 1, nick: 'Admin' }, text: 'Запись через API!', time: 1795458500 }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/notifications', scope: 'notifications',
      summary: 'Уведомления авторизованного пользователя',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице']],
      response: JSON.stringify({
        items: [{ id: 42, type: 'like', object_id: 15, from: { id: 2, nick: 'Elena' }, read: false, time: 1795458100 }],
        total: 1, page: 1, per: 20, pages: 1,
      }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/messages', scope: 'messages (опасное)',
      summary: 'Список личных диалогов авторизованного пользователя',
      params: [['page', 'int', 'Страница'], ['per', 'int', 'На странице (до 100)']],
      response: JSON.stringify({
        items: [{ peer: { id: 2, nick: 'Elena', online: true }, last_message: 'Привет! Как дела?', last_time: 1795458300, unread: 1, favorite: false }],
        total: 1, page: 1, per: 20, pages: 1,
      }, null, 2),
    },
    {
      method: 'GET', path: '/api/v1/me/messages/{id}', scope: 'messages (опасное)',
      summary: 'История личной переписки с пользователем {id}',
      params: [['id', 'int (path)', 'ID собеседника'], ['page', 'int', 'Страница'], ['per', 'int', 'Сообщений на странице (до 100)']],
      response: JSON.stringify({
        peer: { id: 2, nick: 'Elena' },
        items: [{ id: 105, from: { id: 2, nick: 'Elena' }, to: { id: 1, nick: 'Admin' }, text: 'Привет!', html: 'Привет!', read: true, pinned: false, reply_id: 0, time: 1795458300 }],
        total: 1, page: 1, per: 30, pages: 1,
      }, null, 2),
    },
    {
      method: 'POST', path: '/api/v1/me/messages/{id}', scope: 'messages_write (опасное)',
      summary: 'Отправить личное сообщение пользователю {id} от имени авторизованного пользователя',
      params: [
        ['id', 'int (path)', 'ID получателя'],
        ['text', 'string (JSON body)', 'Текст сообщения (1–1024 символа, поддерживается BBCode)'],
        ['reply_id', 'int (JSON body)', 'ID сообщения для ответа (необязательно)'],
      ],
      response: JSON.stringify({
        id: 106, from: { id: 1, nick: 'Admin' }, to: { id: 2, nick: 'Elena' },
        text: 'Отправлено через API!', html: 'Отправлено через API!', reply_id: 0, time: 1795458600,
      }, null, 2),
    },
  ];

  // Интерактивная песочница API
  const tokIn = h('input.input', { placeholder: 'Вставьте access_token (dcms_at_...) или оставьте пустым для /api/v1/site' });
  const epSel = h('select.input',
    h('option', { value: '/api/v1/site' }, 'GET /api/v1/site (публичный)'),
    h('option', { value: '/api/v1/me' }, 'GET /api/v1/me (profile)'),
    h('option', { value: '/api/v1/token/info' }, 'GET /api/v1/token/info'),
    h('option', { value: '/api/v1/me/friends' }, 'GET /api/v1/me/friends (friends)'),
    h('option', { value: '/api/v1/users' }, 'GET /api/v1/users (profile)'),
    h('option', { value: '/api/v1/me/notifications' }, 'GET /api/v1/me/notifications (notifications)'),
    h('option', { value: '/api/v1/me/messages' }, 'GET /api/v1/me/messages (messages · опасное)'));
  const resPre = h('pre.dev-code', '// Нажмите «Выполнить запрос», чтобы увидеть реальный ответ сервера');
  const statusSpan = h('span', 'Готов к тесту');
  const runTest = async () => {
    statusSpan.textContent = 'Выполняется…';
    const headers = {};
    if (tokIn.value.trim()) headers.Authorization = 'Bearer ' + tokIn.value.trim();
    try {
      const resp = await fetch(epSel.value, { headers });
      const txt = await resp.text();
      statusSpan.textContent = `HTTP ${resp.status} ${resp.statusText}`;
      try { resPre.textContent = JSON.stringify(JSON.parse(txt), null, 2); }
      catch { resPre.textContent = txt; }
    } catch (e) {
      statusSpan.textContent = 'Ошибка сети';
      resPre.textContent = String(e);
    }
  };

  return h('div.stack',
    h('div.card.pad.stack',
      h('h2', 'Как работает OAuth 2.0 авторизация'),
      h('p.muted', 'Вы можете подключить вход через наш сайт на любом стороннем ресурсе, боте или мобильном приложении по стандарту OAuth 2.0 (Authorization Code + опциональный PKCE). Все методы API поддерживают CORS.'),
      h('div.stack-s',
        h('b', 'Шаг 1. Создайте приложение'),
        h('div.small.muted', 'Во вкладке «Мои приложения» создайте приложение, укажите ваш Redirect URI и получите ', h('code', 'client_id'), ' и ', h('code', 'client_secret'), '.'),
        h('b', 'Шаг 2. Перенаправьте пользователя на экран согласия'),
        codeBlock('GET URL авторизации', `${origin}/oauth/authorize?client_id=ВАШ_CLIENT_ID&redirect_uri=https://your-site.ru/callback&response_type=code&scope=profile%20email%20friends&state=random123`),
        h('div.small.muted', 'После нажатия «Разрешить доступ» пользователь вернётся на ваш ', h('code', 'redirect_uri'), ' с параметрами ', h('code', '?code=...&state=random123'), ' (или ', h('code', '?error=access_denied'), ' при отказе). Код действует 5 минут и используется один раз.'),
        h('b', 'Шаг 3. Обменяйте code на access_token'),
        multiLangCode({
          'cURL': `curl -X POST ${origin}/oauth/token \\\n  -H "Content-Type: application/x-www-form-urlencoded" \\\n  -d "grant_type=authorization_code" \\\n  -d "client_id=ВАШ_CLIENT_ID" \\\n  -d "client_secret=ВАШ_CLIENT_SECRET" \\\n  -d "redirect_uri=https://your-site.ru/callback" \\\n  -d "code=ПОЛУЧЕННЫЙ_CODE"`,
          'JavaScript': `const res = await fetch('${origin}/oauth/token', {\n  method: 'POST',\n  headers: { 'Content-Type': 'application/json' },\n  body: JSON.stringify({\n    grant_type: 'authorization_code',\n    client_id: 'ВАШ_CLIENT_ID',\n    client_secret: 'ВАШ_CLIENT_SECRET',\n    redirect_uri: 'https://your-site.ru/callback',\n    code: 'ПОЛУЧЕННЫЙ_CODE'\n  })\n});\nconst { access_token, refresh_token, expires_in } = await res.json();`,
          'PHP': `$ch = curl_init('${origin}/oauth/token');\ncurl_setopt_array($ch, [\n  CURLOPT_POST => true,\n  CURLOPT_RETURNTRANSFER => true,\n  CURLOPT_POSTFIELDS => http_build_query([\n    'grant_type'    => 'authorization_code',\n    'client_id'     => 'ВАШ_CLIENT_ID',\n    'client_secret' => 'ВАШ_CLIENT_SECRET',\n    'redirect_uri'  => 'https://your-site.ru/callback',\n    'code'          => $_GET['code'],\n  ]),\n]);\n$data = json_decode(curl_exec($ch), true);\n$token = $data['access_token'];`,
          'Python': `import requests\n\nr = requests.post('${origin}/oauth/token', json={\n    'grant_type': 'authorization_code',\n    'client_id': 'ВАШ_CLIENT_ID',\n    'client_secret': 'ВАШ_CLIENT_SECRET',\n    'redirect_uri': 'https://your-site.ru/callback',\n    'code': code,\n})\ntoken = r.json()['access_token']`,
        }),
        h('b', 'Шаг 4. Выполняйте запросы к API v1'),
        multiLangCode({
          'cURL': `curl ${origin}/api/v1/me \\\n  -H "Authorization: Bearer ВАШ_ACCESS_TOKEN"`,
          'JavaScript': `const me = await fetch('${origin}/api/v1/me', {\n  headers: { Authorization: 'Bearer ' + access_token }\n}).then(r => r.json());`,
          'PHP': `$ch = curl_init('${origin}/api/v1/me');\ncurl_setopt_array($ch, [\n  CURLOPT_RETURNTRANSFER => true,\n  CURLOPT_HTTPHEADER => ['Authorization: Bearer ' . $token],\n]);\n$user = json_decode(curl_exec($ch), true);`,
          'Python': `me = requests.get(\n    '${origin}/api/v1/me',\n    headers={'Authorization': f'Bearer {token}'}\n).json()`,
        }),
        h('b', 'Шаг 5. Продление (refresh_token) и отзыв токена'),
        codeBlock('POST /oauth/token (refresh) и POST /oauth/revoke', `# Продлить истёкший токен:\ncurl -X POST ${origin}/oauth/token -H "Content-Type: application/json" \\\n  -d '{"grant_type":"refresh_token","client_id":"...","client_secret":"...","refresh_token":"dcms_rt_..."}'\n\n# Отозвать токен:\ncurl -X POST ${origin}/oauth/revoke -H "Content-Type: application/json" \\\n  -d '{"token":"dcms_at_..."}'`),
      )),

    h('div.card.pad.stack',
      h('h2', 'Разрешения (Scopes)'),
      h('p.small.muted', 'Передавайте нужные разрешения через пробел в параметре ', h('code', 'scope'), '. Пользователь может снять галочку с любого опасного или необязательного разрешения на экране согласия — проверяйте фактически выданный список в поле ', h('code', 'scope'), ' ответа ', h('code', 'POST /oauth/token'), '.'),
      h('div', { style: { overflowX: 'auto' } },
        h('table.dev-table',
          h('thead', h('tr', h('th', 'Scope'), h('th', 'Название'), h('th', 'Что даёт'))),
          h('tbody', ...scopes.map(s => h('tr',
            h('td', h('code', s.name), s.danger ? [' ', h('span.badge.danger', 'Опасное')] : null),
            h('td', h('b', s.title)),
            h('td', s.desc))))))),

    h('div.card.pad.stack',
      h('h2', 'Справочник методов REST API v1'),
      h('p.small.muted', 'Базовый URL: ', h('code', origin), '. Все ответы возвращаются в формате JSON (UTF-8). При ошибке возвращается объект ', h('code', '{"error":"...","error_description":"..."}'), ' с кодом HTTP 400 / 401 / 403 / 404.'),
      h('div.stack-s', ...endpoints.map(ep => h('details.dev-ep',
        h('summary',
          h('span.dev-method.' + ep.method, ep.method),
          h('span.dev-path', ep.path),
          h('span.badge', ep.scope),
          h('span.small.muted.grow', ep.summary)),
        h('div.dev-ep-body',
          ep.params.length ? h('div',
            h('div.small.muted', { style: { marginBottom: '6px' } }, 'Параметры:'),
            h('table.dev-table',
              h('thead', h('tr', h('th', 'Параметр'), h('th', 'Тип'), h('th', 'Описание'))),
              h('tbody', ...ep.params.map(([n, t, d]) => h('tr', h('td', h('code', n)), h('td', t), h('td', d)))))) : null,
          codeBlock('Пример ответа JSON', ep.response)))))),

    h('div.card.pad.stack',
      h('h2', 'Песочница API (быстрый тест)'),
      h('div.field', h('label', 'Эндпоинт'), epSel),
      h('div.field', h('label', 'Bearer Access Token'), tokIn),
      h('div.row', ui.btn('Выполнить запрос', { ic: 'play', kind: 'primary', onclick: runTest })),
      h('div.dev-code-wrap', h('div.dev-code-head', statusSpan), resPre)));
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
        const firstUri = (app.redirect_uris && app.redirect_uris[0]) || '';
        const testHref = firstUri
          ? `/oauth/authorize?client_id=${encodeURIComponent(app.client_id)}&redirect_uri=${encodeURIComponent(firstUri)}&response_type=code&scope=profile%20email%20messages%20messages_write&state=test`
          : '';
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
          testHref ? h('div.row', ui.btn('Проверить экран OAuth', { ic: 'key', small: true, href: testHref })) : null);
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
    content = buildDocsSection(scopes);
  }

  return [
    ui.pageHead('Разработчикам · OAuth 2.0 и API',
      me() ? ui.btn('Новое приложение', { ic: 'plus', kind: 'primary', small: true, onclick: () => openAppModal(null, () => router.go('/developers?tab=apps')) })
        : ui.btn('Войти для создания приложения', { ic: 'log-in', small: true, href: '/login?return=%2Fdevelopers%3Ftab%3Dapps' })),
    ui.tabs(tabs, tab),
    content,
  ];
});

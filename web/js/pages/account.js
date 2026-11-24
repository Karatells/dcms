import { h, api, ui, router, state, icon, link, bus, events, refreshMe, returnTo, fmt } from '../core.js';

const authBox = (title, sub, ...c) => h('div.auth-box', h('div.card.pad', h('div.stack', h('div', h('h1', title), sub ? h('p.muted', sub) : null), ...c)));
async function afterLogin() { await refreshMe(); events.restart(); router.go(returnTo()); }

// ───────── вход
router.add('/login', async ctx => {
  ctx.title('Вход');
  if (state.user) return router.go('/', true);
  let need2fa = false;
  const codeWrap = h('div.field', { style: { display: 'none' } },
    h('label', 'Код двухфакторной аутентификации (6 цифр)'),
    h('input.input', { name: 'code', inputMode: 'numeric', autoComplete: 'one-time-code', maxLength: 6, placeholder: '123456' }));
  const form = ui.form([
    { name: 'nick', label: 'Логин', required: true, autocomplete: 'username' },
    { name: 'pass', label: 'Пароль', type: 'password', required: true, autocomplete: 'current-password' },
  ], {
    submit: 'Войти',
    onSubmit: async v => {
      const codeIn = codeWrap.querySelector('input');
      const r = await api.post('/api/login', { ...v, code: need2fa ? codeIn.value.trim() : '' }, { silent: true });
      if (r && r.need_2fa) {
        need2fa = true;
        codeWrap.style.display = '';
        codeIn.focus();
        ui.toast('Введите 6-значный код из приложения-аутентификатора', 'info');
        return;
      }
      await afterLogin();
    },
  });
  form.querySelector('button[type=submit]').before(codeWrap);
  return authBox('Вход', 'Рады видеть вас снова', form,
    h('div.row.small', link('/pass', 'Забыли пароль?'), h('span.grow'), h('span.muted', 'Нет аккаунта? '), link('/reg', 'Регистрация')));
});

// ───────── регистрация
router.add('/reg', async ctx => {
  ctx.title('Регистрация');
  if (state.user) return router.go('/', true);
  const mode = state.config.reg_select;
  if (mode === 'close') return authBox('Регистрация', null, h('div.err', 'Регистрация временно приостановлена'));
  const cap = h('img.captcha', { src: '/api/captcha?' + Date.now(), alt: 'код', style: { borderRadius: '10px', height: '56px', width: 'auto', alignSelf: 'flex-start', cursor: 'pointer' }, title: 'Обновить' });
  cap.onclick = () => (cap.src = '/api/captcha?' + Date.now());
  const days = [{ value: '', label: 'День' }, ...Array.from({ length: 31 }, (_, i) => ({ value: i + 1, label: i + 1 }))];
  const months = ['Месяц', 'Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'].map((l, i) => ({ value: i || '', label: l }));
  const y = new Date().getFullYear();
  const years = [{ value: '', label: 'Год' }, ...Array.from({ length: y - 7 - 1930 + 1 }, (_, i) => ({ value: y - 7 - i, label: y - 7 - i }))];
  const form = ui.form([
    { name: 'nick', label: 'Логин', required: true, hint: '3–32 символа: буквы только одного алфавита, цифры, пробел, дефис, подчёркивание', maxlength: 32, autocomplete: 'username' },
    ...(mode === 'open_mail' ? [{ name: 'email', label: 'E-mail', type: 'email', required: true, hint: 'Для активации аккаунта' }] : []),
    { name: 'pass1', label: 'Пароль', type: 'password', required: true, hint: 'От 6 до 32 символов', autocomplete: 'new-password' },
    { name: 'pass2', label: 'Повторите пароль', type: 'password', required: true, autocomplete: 'new-password' },
    { name: 'pol', label: 'Пол', type: 'radio', value: '1', options: [{ value: '1', label: 'Мужской' }, { value: '0', label: 'Женский' }] },
    { name: 'name', label: 'Имя (необязательно)' },
    { name: 'd', label: 'День рождения', type: 'select', options: days }, { name: 'm', label: 'Месяц', type: 'select', options: months }, { name: 'y', label: 'Год', type: 'select', options: years },
    { name: 'captcha', label: 'Проверочное число', required: true, hint: 'Введите цифры с картинки', autocomplete: 'off' },
  ], {
    submit: 'Зарегистрироваться',
    onSubmit: async v => {
      const body = { ...v, pol: +v.pol, d: +v.d || 0, m: +v.m || 0, y: +v.y || 0 };
      try { const r = await api.post('/api/register', body, { silent: true }); if (r.activate) { form.replaceWith(h('div.ok', icon('check'), ' ', r.message)); return; } await afterLogin(); }
      catch (e) { cap.onclick(); throw e; }
    },
  });
  // компактная сетка даты рождения и капча рядом с полем
  const fields = [...form.querySelectorAll('.field')];
  const dr = fields.filter(f => ['d', 'm', 'y'].includes(f.querySelector('[name]')?.name));
  if (dr.length === 3) { const row = h('div.row.nowrap'); dr[0].before(row); row.append(...dr); dr.forEach(f => (f.style.minWidth = 0)); }
  const cf = fields.find(f => f.querySelector('[name=captcha]')); cf && cf.prepend(cap);
  return authBox('Регистрация', 'Создайте аккаунт за минуту', form, h('div.small.muted', 'Уже есть аккаунт? ', link('/login', 'Войти')));
});
router.add('/reg/activate/:id/:code', async ctx => {
  ctx.title('Активация аккаунта');
  try { const r = await api.post('/api/register/activate', { id: +ctx.params.id, code: ctx.params.code }, { silent: true }); await refreshMe(); events.restart(); return authBox('Готово', null, h('div.ok', icon('check'), ' ', r.message), link('/', h('span.btn.primary', 'На главную'))); }
  catch (e) { return authBox('Ошибка', null, h('div.err', e.message), link('/reg', 'Регистрация')); }
});

// ───────── восстановление пароля
router.add('/pass', async ctx => {
  ctx.title('Восстановление пароля');
  const form = ui.form([{ name: 'nick', label: 'Логин', required: true }, { name: 'mail', label: 'E-mail, указанный в анкете', type: 'email', required: true }],
    { submit: 'Отправить ссылку', onSubmit: async v => { const r = await api.post('/api/pass/request', v, { silent: true }); form.replaceWith(h('div.ok', icon('check'), ' ', r.message)); } });
  return authBox('Восстановление пароля', 'Мы вышлем ссылку для установки нового пароля', form, link('/login', '← Ко входу'));
});
router.add('/pass/:id/:token', async ctx => {
  ctx.title('Новый пароль');
  let info; try { info = await api.get('/api/pass/check', { id: ctx.params.id, token: ctx.params.token }, { silent: true }); } catch (e) { return authBox('Ссылка недействительна', null, h('div.err', e.message), link('/pass', 'Запросить заново')); }
  const form = ui.form([{ name: 'pass1', label: 'Новый пароль', type: 'password', required: true }, { name: 'pass2', label: 'Подтверждение', type: 'password', required: true }],
    { submit: 'Сохранить пароль', onSubmit: async v => { await api.post('/api/pass/reset', { id: +ctx.params.id, token: ctx.params.token, ...v }, { silent: true }); ui.ok('Пароль успешно изменен'); router.go('/login'); } });
  return authBox('Новый пароль', 'Для пользователя ' + info.nick, form);
});

// ───────── блокировка
router.add('/ban', async ctx => {
  ctx.title('Блокировка');
  const r = await api.get('/api/ban/status', null, { silent: true });
  const items = [];
  if (r.ip_banned) items.push(h('div.card.pad', h('h2.err', icon('ban'), ' Ваш IP-адрес заблокирован'), h('p.muted', 'Доступ к сайту с этого адреса ограничен администрацией.')));
  (r.bans || []).forEach(b => items.push(h('div.card.pad', h('h3', icon('gavel'), ' Блокировка: ', b.razdel === 'all' ? 'весь сайт' : b.razdel),
    h('p', b.reason || 'Причина не указана'), h('p.muted.small', b.forever ? 'Бессрочно' : 'До ' + fmt.datetime(b.until), b.by && b.by.id ? [' · Заблокировал ', ui.userLink(b.by)] : null))));
  if (!items.length) items.push(ui.empty('Активных блокировок нет', 'check'));
  return [ui.pageHead('Блокировка'), ...items];
});

// ───────── внешняя ссылка (go.php)
router.add('/go', async ctx => {
  const url = ctx.query.url || '';
  if (!/^https?:\/\//i.test(url)) return ui.notFound('Некорректная ссылка');
  return authBox('Переход по внешней ссылке', 'Вы покидаете сайт', h('div.card.pad', { style: { wordBreak: 'break-all' } }, url), h('div.row', h('a.btn.primary', { href: url, rel: 'noopener nofollow', target: '_blank', 'data-native': '' }, 'Перейти'), h('button.btn', { onclick: () => history.back() }, 'Назад')));
});

// ───────── настройки
const SET_TABS = [['general', 'Общие', 'settings'], ['view', 'Вид', 'monitor'], ['privacy', 'Приватность', 'lock'], ['password', 'Безопасность', 'key']];
const setTabs = a => ui.tabs(SET_TABS.map(([k, l, i]) => ({ key: k, label: l, icon: i, href: '/user/settings/' + k })), a);

const fixUrl = u => u.replace(/\.php.*$/, '').replace('/user/settings/settings.privacy', '/user/settings/privacy').replace('/user/settings/settings', '/user/settings/general').replace('/user/settings/index', '/user/settings');
const mdi = { 'cog-outline': 'settings', 'tooltip-image-outline': 'monitor', 'receipt-text-edit-outline': 'rss', 'message-cog': 'message-circle', 'bell-cog': 'bell', 'account-lock-outline': 'lock', 'form-textbox-password': 'key', 'animation-outline': 'settings' };
router.add('/user/settings', async ctx => {
  ctx.title('Мои настройки');
  const menu = await api.get('/api/settings-menu');
  return [ui.pageHead('Мои настройки'), ui.card(h('div.list', menu.map(m => m.type === 'razd' ? h('div.card-title', m.name) : h('a.list-item', { href: fixUrl(m.url) }, icon(mdi[m.img] || 'settings', 20), h('div.main', h('div.title', m.name)), icon('chevron-right', 16)))))];
}, { auth: true });

router.add('/user/settings/general', async ctx => {
  ctx.title('Общие настройки');
  const s = await api.get('/api/settings');
  const zones = Array.from({ length: 25 }, (_, i) => ({ value: i - 12, label: (i - 12 >= 0 ? '+' : '') + (i - 12) + ' ч.' }));
  const form = ui.form([
    { name: 'add_konts', label: 'Кто может добавлять меня в контакты', type: 'select', value: s.add_konts, options: [{ value: 0, label: 'Все' }, { value: 1, label: 'Только друзья' }, { value: 2, label: 'Никто' }] },
    { name: 'set_time_chat', label: 'Автообновление чата, сек (0 — выкл.)', type: 'number', min: 0, max: 900, value: s.set_time_chat },
    { name: 'set_timesdvig', label: 'Часовой пояс (сдвиг времени)', type: 'select', value: s.set_timesdvig, options: zones },
    ...(s.has_email ? [{ name: 'set_news_to_mail', type: 'checkbox', label: 'Получать новости на e-mail', value: s.set_news_to_mail }] : []),
  ], { onSubmit: async v => { await api.put('/api/settings/general', { ...v, add_konts: +v.add_konts, set_time_chat: +v.set_time_chat, set_timesdvig: +v.set_timesdvig }); ui.ok('Изменения сохранены'); } });
  return [ui.pageHead('Настройки'), setTabs('general'), ui.card(h('div.pad', form))];
}, { auth: true });

router.add('/user/settings/view', async ctx => {
  ctx.title('Вид и отображение');
  const s = await api.get('/api/settings');
  const form = ui.form([
    { name: 'theme', label: 'Тема оформления', type: 'select', value: s.theme || 'auto', options: [{ value: 'auto', label: 'Как в системе' }, { value: 'light', label: 'Светлая' }, { value: 'dark', label: 'Тёмная' }] },
    { name: 'accent', label: 'Акцентный цвет', type: 'color', value: s.accent || state.config.accent_default || '#6366f1' },
    { name: 'set_p_str', label: 'Пунктов на странице (1–100)', type: 'number', min: 1, max: 100, value: s.set_p_str || state.config.p_str || 10 },
    { name: 'show_url', type: 'checkbox', label: 'Показывать, на какой странице я нахожусь', value: s.show_url },
    { name: 'abuld', type: 'checkbox', label: 'Показывать материалы с меткой 18+', value: s.abuld },
    { name: 'set_files', type: 'checkbox', label: 'Показывать файлы и вложения в сообщениях', value: s.set_files },
  ], { onSubmit: async v => { await api.put('/api/settings/view', { ...v, set_p_str: +v.set_p_str }); await refreshMe(); ui.ok('Настройки обновлены'); } });
  return [ui.pageHead('Настройки'), setTabs('view'), ui.card(h('div.pad', form))];
}, { auth: true });

router.add('/user/settings/privacy', async ctx => {
  ctx.title('Приватность');
  const s = await api.get('/api/settings');
  const opts = [{ value: 1, label: 'Все' }, { value: 2, label: 'Только друзья' }, { value: 0, label: 'Только я' }];
  const form = ui.form([
    { name: 'privat_str', label: 'Кто видит мою страницу', type: 'select', value: s.privat_str, options: opts },
    { name: 'privat_mail', label: 'Кто может писать мне личные сообщения', type: 'select', value: s.privat_mail, options: opts },
    { name: 'ocenka', label: 'Кто может оценивать мой профиль', type: 'select', value: s.ocenka, options: opts },
    { name: 'block_group_invites', label: 'Запретить приглашения в сообщества', type: 'checkbox', value: !!s.block_group_invites },
  ], { onSubmit: async v => { await api.put('/api/settings/privacy', { privat_str: +v.privat_str, privat_mail: +v.privat_mail, ocenka: +v.ocenka, block_group_invites: !!v.block_group_invites }); ui.ok('Изменения успешно приняты'); } });
  return [ui.pageHead('Настройки'), setTabs('privacy'), ui.card(h('div.pad', form))];
}, { auth: true });

router.add('/user/settings/password', async ctx => {
  ctx.title('Безопасность');
  const [sess, tfa] = await Promise.all([api.get('/api/settings/sessions'), api.get('/api/settings/2fa')]);
  const form = ui.form([
    { name: 'pass', label: 'Старый пароль', type: 'password', required: true }, { name: 'pass1', label: 'Новый пароль', type: 'password', required: true }, { name: 'pass2', label: 'Подтверждение', type: 'password', required: true },
  ], { submit: 'Сменить пароль', onSubmit: async (v, f) => { await api.post('/api/settings/password', v); ui.ok('Пароль успешно изменен'); f.reset(); } });

  let tfaBox;
  if (tfa.enabled) {
    const passIn = h('input.input', { type: 'password', placeholder: 'Текущий пароль для отключения 2FA' });
    tfaBox = h('div.pad.stack',
      h('div.row', h('span.badge.ok', icon('check', 13), ' 2FA включена'), h('span.small.muted', 'При входе запрашивается 6-значный код из приложения-аутентификатора.')),
      h('div.row.nowrap', passIn, ui.btn('Отключить 2FA', {
        kind: 'danger', small: true,
        onclick: async () => {
          await api.post('/api/settings/2fa/disable', { pass: passIn.value });
          ui.ok('Двухфакторная аутентификация отключена');
          router.render();
        },
      })));
  } else {
    const codeIn = h('input.input', { inputMode: 'numeric', maxLength: 6, placeholder: '6-значный код из приложения' });
    tfaBox = h('div.pad.stack',
      h('div.small.muted', 'Добавьте ключ вручную в Google Authenticator, Яндекс Ключ, Authy или 1Password (тип: по времени / TOTP), затем введите 6-значный код:'),
      h('div.row.nowrap',
        h('input.input', { value: tfa.secret || '', readOnly: true, style: { fontFamily: 'ui-monospace,monospace', letterSpacing: '.08em' }, onclick: e => e.target.select() }),
        ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => navigator.clipboard?.writeText(tfa.secret).then(() => ui.ok('Ключ скопирован')) })),
      h('div.row.nowrap', codeIn, ui.btn('Включить 2FA', {
        ic: 'shield', kind: 'primary', small: true,
        onclick: async () => {
          await api.post('/api/settings/2fa/enable', { code: codeIn.value.trim() });
          ui.ok('Двухфакторная аутентификация включена');
          router.render();
        },
      })));
  }

  return [ui.pageHead('Настройки'), setTabs('password'),
    ui.section('Двухфакторная аутентификация (2FA)', tfaBox),
    ui.card(h('div.pad', form)),
    ui.section('Активные сессии', h('div.list', sess.sessions.map(s => h('div.list-item', icon(s.current ? 'check' : 'monitor', 18), h('div.main', h('div.title', s.ip, s.current ? ui.badge(' эта', 'ok') : null), h('div.meta', s.ua)), ui.time(s.last)))),
      h('div.pad', h('button.btn.sm', { onclick: async () => { await api.post('/api/settings/sessions/close-others'); ui.ok('Другие сессии закрыты'); router.render(); } }, 'Закрыть все остальные сессии'))),
    ui.section('Последние входы', h('div.list', sess.log.slice().reverse().map(l => h('div.list-item', h('div.main', h('div.meta', l.ua || '—')), ui.time(l.time)))))];
}, { auth: true });

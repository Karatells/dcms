import { h, api, ui, router, state, icon, isNativeClient, canAndroidSign2FA, link, nativePlatform, bus, events, refreshMe, returnTo, fmt } from '../core.js';

async function changeNativeServer() {
  const cur = location.origin;
  const next = await ui.prompt('Введите адрес сервера соцсети (https://...)', cur, { title: 'Сменить сервер' });
  if (!next || !next.trim()) return;
  if (window.dcmsNative && typeof window.dcmsNative.setServerUrl === 'function') {
    window.dcmsNative.setServerUrl(next.trim());
  }
}

function toggleClientTheme() {
  const cur = document.documentElement.dataset.theme;
  const dark = cur === 'dark' || (cur === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches);
  const next = dark ? 'light' : 'dark';
  localStorage.setItem('theme', next);
  document.documentElement.dataset.theme = next;
}

const authBox = (title, sub, ...c) => {
  if (!isNativeClient()) {
    return h('div.auth-box', h('div.card.pad', h('div.stack', h('div', h('h1', title), sub ? h('p.muted', sub) : null), ...c)));
  }
  const siteName = state.config.site_name || 'DCMS Social';
  const initial = (Array.from(siteName.trim())[0] || 'D').toUpperCase();
  const plat = nativePlatform();
  const platLabel = plat === 'android' ? 'Android' : 'Desktop';
  const p = location.pathname;
  const isLogin = p === '/login' || p === '/';
  const isReg = p === '/reg';
  const regOpen = state.config.reg_select !== 'close';

  const hero = h('section.na-hero',
    h('div.na-brand',
      h('div.na-logo', initial),
      h('div',
        h('div.na-title', siteName),
        h('div.na-sub', platLabel + ' клиент'))),
    h('p.na-lead', 'Единое пространство для общения, ленты, музыки и сообществ — быстро и всегда под рукой.'),
    h('ul.na-feats',
      h('li', icon('message-circle', 18), h('span', 'Живые личные чаты, беседы и голосовые сообщения')),
      h('li', icon('video', 18), h('span', 'Умная лента, истории, фото и короткие видео')),
      h('li', icon('music', 18), h('span', 'Музыкальный плеер, сообщества и мини-игры'))),
    h('div.na-server',
      h('span.na-server-url', icon('globe', 14), ' ', location.host),
      window.dcmsNative && typeof window.dcmsNative.setServerUrl === 'function'
        ? h('button.na-server-btn', { type: 'button', onclick: changeNativeServer }, 'Сменить')
        : null));

  const tabs = (isLogin || isReg) ? h('div.na-tabs',
    h('a.na-tab' + (isLogin ? '.active' : ''), { href: '/login' }, 'Вход'),
    regOpen ? h('a.na-tab' + (isReg ? '.active' : ''), { href: '/reg' }, 'Регистрация') : null) : null;

  const card = h('section.na-card',
    h('div.na-card-top',
      tabs || h('a.na-back', { href: '/login' }, icon('chevron-left', 16), ' Ко входу'),
      h('button.icon-btn', { type: 'button', title: 'Переключить тему', onclick: toggleClientTheme }, icon('sun', 18))),
    h('div.na-head',
      h('h1', title),
      sub ? h('p.muted', sub) : null),
    h('div.stack', ...c),
    window.dcmsNative && typeof window.dcmsNative.setServerUrl === 'function'
      ? h('div.na-mob-server', h('span', location.host), h('button', { type: 'button', onclick: changeNativeServer }, 'Сменить сервер'))
      : null);

  return h('div.native-auth-shell', h('div.na-frame', hero, card));
};
async function afterLogin() { await refreshMe(); events.restart(); router.go(returnTo(), true); }

// ───────── вход
router.add('/login', async ctx => {
  ctx.title('Вход');
  if (state.user) return router.go('/', true);
  let need2fa = false, pollTimer = null;
  const stopPoll = () => { if (pollTimer) { clearInterval(pollTimer); pollTimer = null; } };
  ctx.onLeave(stopPoll);

  const appWaitWrap = h('div.mfa-wait-box', { style: { display: 'none' } });
  const codeWrap = h('div.field', { style: { display: 'none' } },
    h('label', 'Код двухфакторной аутентификации (6 цифр)'),
    h('input.input', { name: 'code', inputMode: 'numeric', autoComplete: 'one-time-code', maxLength: 6, placeholder: '123456' }));

  const startAppPoll = (challengeId, matchCode, hasTotp) => {
    stopPoll();
    const statusEl = h('div.small.muted', 'Ожидание подтверждения…');
    appWaitWrap.style.display = '';
    appWaitWrap.replaceChildren(
      h('div.row', icon('smartphone', 18), h('b', 'Подтвердите вход в мобильном приложении')),
      h('div.small.muted', 'Откройте приложение на привязанном Android-устройстве и выберите это число:'),
      h('div.mfa-code-pill', String(matchCode || '')),
      statusEl,
      hasTotp ? h('div.small.muted', 'Или введите 6-значный код из аутентификатора ниже.') : null);

    pollTimer = setInterval(async () => {
      try {
        const p = await api.post('/api/login/poll', { challenge_id: challengeId }, { silent: true });
        if (p.status === 'approved') {
          stopPoll();
          await afterLogin();
        } else if (p.status === 'denied' || p.status === 'expired') {
          stopPoll();
          statusEl.textContent = p.error || 'Подтверждение не выполнено';
          statusEl.style.color = '#ef4444';
        }
      } catch {}
    }, 2000);
  };

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
        const m = r.methods || { totp: true, app: false };
        if (m.app && r.challenge_id) {
          startAppPoll(r.challenge_id, r.match_code, !!m.totp);
        }
        if (m.totp) {
          codeWrap.style.display = '';
          if (!m.app) {
            codeIn.focus();
            ui.toast('Введите 6-значный код из приложения-аутентификатора', 'info');
          }
        }
        return;
      }
      stopPoll();
      await afterLogin();
    },
  });
  form.querySelector('button[type=submit]').before(appWaitWrap, codeWrap);
  return authBox('Вход', 'Рады видеть вас снова', form,
    h('div.row.small', link('/pass', 'Забыли пароль?'), h('span.grow'), h('span.muted', 'Нет аккаунта? '), link('/reg', 'Регистрация')));
});

// ───────── регистрация
router.add('/reg', async ctx => {
  ctx.title('Регистрация');
  if (state.user) return router.go('/', true);
  const mode = state.config.reg_select;
  if (mode === 'close') return authBox('Регистрация', null, h('div.err', 'Регистрация временно приостановлена'));
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
    { name: 'captcha', label: 'Проверка', type: 'captcha', required: true },
  ], {
    submit: 'Зарегистрироваться',
    onSubmit: async v => {
      const body = { ...v, pol: +v.pol, d: +v.d || 0, m: +v.m || 0, y: +v.y || 0 };
      try { const r = await api.post('/api/register', body, { silent: true }); if (r.activate) { form.replaceWith(h('div.ok', icon('check'), ' ', r.message)); return; } await afterLogin(); }
      catch (e) { if (String(e.message || '').includes('робот')) form.inputs.captcha?.reset(); throw e; }
    },
  });
  // компактная сетка даты рождения
  const fields = [...form.querySelectorAll('.field')];
  const dr = fields.filter(f => ['d', 'm', 'y'].includes(f.querySelector('[name]')?.name));
  if (dr.length === 3) { const row = h('div.row.nowrap'); dr[0].before(row); row.append(...dr); dr.forEach(f => (f.style.minWidth = 0)); }
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
  const extOpts = [{ value: 0, label: 'Все' }, { value: 1, label: 'Только друзья' }, { value: 2, label: 'Только я' }];
  const form = ui.form([
    { name: 'privat_str', label: 'Кто видит мою страницу', type: 'select', value: s.privat_str, options: opts },
    { name: 'privat_mail', label: 'Кто может писать мне личные сообщения', type: 'select', value: s.privat_mail, options: opts },
    { name: 'privat_wall', label: 'Кто может писать на моей стене', type: 'select', value: s.privat_wall || 0, options: extOpts },
    { name: 'privat_friends', label: 'Кто видит список моих друзей', type: 'select', value: s.privat_friends || 0, options: extOpts },
    { name: 'privat_stories', label: 'Кто видит мои истории (Сторис)', type: 'select', value: s.privat_stories || 0, options: extOpts },
    { name: 'privat_duels', label: 'Кто может вызывать меня на дуэли в мини-играх', type: 'select', value: s.privat_duels || 0, options: [{ value: 0, label: 'Все' }, { value: 1, label: 'Только друзья' }, { value: 2, label: 'Никто' }] },
    { name: 'ocenka', label: 'Кто может оценивать мой профиль', type: 'select', value: s.ocenka, options: opts },
    { name: 'hide_online', label: 'Скрыть статус «онлайн» от других пользователей', type: 'checkbox', value: !!s.hide_online },
    { name: 'block_group_invites', label: 'Запретить приглашения в сообщества', type: 'checkbox', value: !!s.block_group_invites },
  ], {
    onSubmit: async v => {
      await api.put('/api/settings/privacy', {
        privat_str: +v.privat_str, privat_mail: +v.privat_mail, ocenka: +v.ocenka,
        privat_wall: +v.privat_wall, privat_friends: +v.privat_friends, privat_stories: +v.privat_stories, privat_duels: +v.privat_duels,
        hide_online: !!v.hide_online, block_group_invites: !!v.block_group_invites,
      });
      ui.ok('Изменения успешно приняты');
    },
  });
  const pushSt = await api.get('/api/push/status', null, { silent: true }).catch(() => ({ enabled: false }));
  const pushBox = h('div.pad.row',
    h('div.grow',
      h('b', 'Системные Push-уведомления (PWA)'),
      h('div.small.muted', 'Получайте уведомления о новых сообщениях и событиях, даже когда вкладка свёрнута.')),
    pushSt.enabled && ('Notification' in window) && Notification.permission === 'granted'
      ? ui.btn('Отключить Push', {
        small: true, kind: 'danger',
        onclick: async () => { await api.del('/api/push/subscribe'); ui.ok('Push-уведомления отключены'); router.render(); },
      })
      : ui.btn('Включить Push', {
        ic: 'bell', small: true, kind: 'primary',
        onclick: async () => {
          if (!('Notification' in window)) return ui.toast('Ваш браузер не поддерживает системные уведомления', 'warn');
          const perm = await Notification.requestPermission();
          if (perm !== 'granted') return ui.toast('Разрешение на уведомления не выдано', 'warn');
          await api.post('/api/push/subscribe', { endpoint: 'browser-notification:' + state.user.id });
          ui.ok('Push-уведомления включены!');
          router.render();
        },
      }));
  return [ui.pageHead('Настройки'), setTabs('privacy'), ui.card(pushBox), ui.card(h('div.pad', form))];
}, { auth: true });

router.add('/user/settings/password', async ctx => {
  ctx.title('Безопасность');
  const [sess, tfa, appDevs] = await Promise.all([
    api.get('/api/settings/sessions'),
    api.get('/api/settings/2fa'),
    api.get('/api/2fa/app/devices').catch(() => ({ items: [] })),
  ]);
  const form = ui.form([
    { name: 'pass', label: 'Старый пароль', type: 'password', required: true }, { name: 'pass1', label: 'Новый пароль', type: 'password', required: true }, { name: 'pass2', label: 'Подтверждение', type: 'password', required: true },
  ], { submit: 'Сменить пароль', onSubmit: async (v, f) => { await api.post('/api/settings/password', v); ui.ok('Пароль успешно изменен'); f.reset(); } });

  // ── Подтверждение входа через Android-приложение (аппаратный AndroidKeyStore ECDSA)
  const devItems = appDevs.items || [];
  const curDevId = canAndroidSign2FA() ? window.dcmsNative.getDeviceId() : '';
  const alreadyBound = curDevId && devItems.some(d => d.device_id === curDevId);
  let appMfaAction;
  if (canAndroidSign2FA()) {
    if (alreadyBound) {
      appMfaAction = h('div.row', h('span.badge.ok', icon('check', 13), ' Это Android-устройство привязано'),
        h('span.small.muted', 'Запросы на вход с других устройств будут приходить сюда.'));
    } else {
      const bindPass = h('input.input', { type: 'password', placeholder: 'Текущий пароль для привязки устройства' });
      appMfaAction = h('div.stack-s',
        h('div.small.muted', 'Привяжите этот телефон: приватный ключ создаётся в защищённом AndroidKeyStore и никогда не покидает устройство.'),
        h('div.row.nowrap', bindPass, ui.btn('Привязать телефон', {
          ic: 'smartphone', kind: 'primary', small: true,
          onclick: async () => {
            if (!bindPass.value) return ui.toast('Введите текущий пароль', 'error');
            const st = await api.post('/api/2fa/app/enroll/start', {});
            const devId = window.dcmsNative.getDeviceId();
            const devName = window.dcmsNative.getDeviceName();
            const pubKey = window.dcmsNative.ensure2FAKey();
            const sig = window.dcmsNative.sign2FA(`dcms-enroll-v1:${devId}:${st.nonce}`);
            await api.post('/api/2fa/app/enroll/finish', {
              pass: bindPass.value,
              device_id: devId,
              device_name: devName,
              pub_key: pubKey,
              nonce: st.nonce,
              signature: sig,
            });
            ui.ok('Android-устройство привязано для подтверждения 2FA');
            router.render();
          },
        })));
    }
  } else {
    appMfaAction = h('div.small.muted',
      'Привязать телефон можно только внутри мобильного приложения Android (ключ хранится в аппаратном хранилище устройства — подтвердить вход через браузер или десктоп невозможно). ',
      !isNativeClient() ? link('/apps', 'Скачать приложение') : null);
  }

  const appMfaBox = h('div.pad.stack',
    appMfaAction,
    devItems.length ? h('div.list', devItems.map(d => h('div.list-item',
      icon('smartphone', 18),
      h('div.main',
        h('div.title', d.device_name, d.device_id === curDevId ? ui.badge(' это устройство', 'ok') : null),
        h('div.meta', 'Привязано ', ui.time(d.created_at), d.last_used_at ? [' · использовано ', ui.time(d.last_used_at)] : null)),
      ui.btn('Отвязать', {
        ic: 'trash', small: true, kind: 'danger',
        onclick: async () => {
          const p = await ui.prompt('Введите текущий пароль для отвязки устройства', '', { title: 'Отвязать ' + d.device_name });
          if (!p) return;
          await api.post(`/api/2fa/app/devices/${d.id}/delete`, { pass: p });
          ui.ok('Устройство отвязано');
          router.render();
        },
      })))) : null);

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
      h('div.small.muted', 'Добавьте ключ вручную в Google Authenticator, Authy или 1Password (тип: по времени / TOTP), затем введите 6-значный код:'),
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
    ui.section('Подтверждение входа через Android-приложение', appMfaBox),
    ui.section('Двухфакторная аутентификация (TOTP)', tfaBox),
    ui.card(h('div.pad', form)),
    ui.section('Активные сессии', h('div.list', (sess.sessions || []).map(s => h('div.list-item',
      icon(s.mobile ? 'smartphone' : 'monitor', 18),
      h('div.main',
        h('div.title', s.ip, s.current ? ui.badge(' текущее устройство', 'ok') : null),
        h('div.meta', s.ua || 'Неизвестное устройство', ' · ', ui.time(s.last))),
      !s.current ? ui.btn('Завершить', {
        ic: 'x', small: true, kind: 'danger',
        onclick: async () => {
          await api.del('/api/settings/sessions/' + s.id);
          ui.ok('Сессия завершена');
          router.render();
        },
      }) : null))),
      h('div.pad', h('button.btn.sm', { onclick: async () => { await api.post('/api/settings/sessions/close-others'); ui.ok('Другие сессии закрыты'); router.render(); } }, 'Закрыть все остальные сессии'))),
    ui.section('Последние входы', h('div.list', sess.log.slice().reverse().map(l => h('div.list-item', h('div.main', h('div.meta', l.ua || '—')), ui.time(l.time)))))];
}, { auth: true });

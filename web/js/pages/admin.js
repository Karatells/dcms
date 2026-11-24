import { h, api, ui, router, state, icon, fmt, can, bus } from '../core.js';

const crumbs = (...c) => ui.breadcrumbs([{ href: '/admin', title: 'Панель управления' }, ...c]);
const S = s => String(s ?? '');

// ───────── учёт посещаемости (замена счётчиков старого движка)
let firstHit = true;
bus.on('route', () => {
  let ref = '';
  if (firstHit && document.referrer) { try { const u = new URL(document.referrer); if (u.host !== location.host) ref = u.host + u.pathname.replace(/\/$/, ''); } catch {} }
  firstHit = false;
  api.post('/api/hit', { ref }).catch(() => {});
});

// ───────── пункты панели управления
const BUILTIN = [
  { group: 'Пользователи и доступ', title: 'Управление аккаунтами', href: '/admin/users', access: 'adm_panel_show', icon: 'users', desc: 'Живой поиск по никам, верификация, редактирование, 2FA и баны' },
  { group: 'Пользователи и доступ', title: 'Администрация', href: '/admin/staff', access: 'adm_show_adm', icon: 'shield', desc: 'Список сотрудников и модераторов' },
  { group: 'Пользователи и доступ', title: 'Группы и привилегии', href: '/admin/groups', access: 'adm_accesses', icon: 'key', desc: 'Права каждой группы пользователей' },
  { group: 'Пользователи и доступ', title: 'Настройки пользователей', href: '/admin/settings/user', access: 'adm_set_user', icon: 'settings', desc: 'Регистрация, гости, статусы' },
  { group: 'Пользователи и доступ', title: 'Массовое удаление', href: '/admin/mass-delete', access: 'user_mass_delete', icon: 'trash', desc: 'Удалить давно не заходивших пользователей' },

  { group: 'Модерация', title: 'Жалобы', href: '/admin/spam', access: 'adm_panel_show', icon: 'flag', desc: 'Жалобы пользователей на нарушения' },
  { group: 'Модерация', title: 'Истории (24 часа)', href: '/admin/stories', access: 'adm_panel_show', icon: 'zap', desc: 'Просмотр и удаление активных историй' },
  { group: 'Модерация', title: 'Список банов', href: '/admin/banlist', access: 'adm_banlist', icon: 'gavel', desc: 'Все действующие блокировки' },
  { group: 'Модерация', title: 'Бан по IP', href: '/admin/ban-ip', access: 'adm_ban_ip', icon: 'globe', desc: 'Блокировка диапазонов адресов' },

  { group: 'API и разработчики', title: 'OAuth-приложения', href: '/admin/oauth', access: 'adm_set_sys', icon: 'key', desc: 'Все зарегистрированные приложения, токены и удаление' },

  { group: 'Аналитика', title: 'Общая информация', href: '/admin/info', access: 'adm_info', icon: 'info', desc: 'Состояние сервера, объём данных, нагрузка' },
  { group: 'Аналитика', title: 'Статистика', href: '/admin/stats', access: 'adm_statistic', icon: 'chart', desc: 'Посещаемость, регистрации, аудитория' },
  { group: 'Аналитика', title: 'Рефералы', href: '/admin/refs', access: 'adm_ref', icon: 'link', desc: 'Переходы с других сайтов' },
  { group: 'Аналитика', title: 'Журнал действий', href: '/admin/log', access: 'adm_log_read', icon: 'list', desc: 'Что делала администрация' },

  { group: 'Система', title: 'Системные настройки', href: '/admin/settings/sys', access: 'adm_set_sys', icon: 'settings', desc: 'Название, оформление, защита, почта' },
  { group: 'Система', title: 'Лимиты загрузок', href: '/admin/settings/uploads', access: 'adm_set_sys', icon: 'upload', desc: 'Макс. размеры фото, файлов, клипов, форума' },
  { group: 'Система', title: 'BBCode и форматирование', href: '/admin/settings/bbcode', access: 'adm_set_sys', icon: 'edit', desc: 'Включение и отключение всех тегов BBCode' },
  { group: 'Система', title: 'Главное меню', href: '/admin/menu', access: 'adm_menu', icon: 'menu', desc: 'Разделы и ссылки на главной' },
  { group: 'Система', title: 'Меню настроек', href: '/admin/settings-menu', access: 'user_settings', icon: 'menu', desc: 'Пункты в настройках пользователя' },
  { group: 'Система', title: 'Резервная копия', href: '/admin/backup', access: 'adm_mysql', icon: 'database', desc: 'Скачать SQL-дамп базы данных' },
];
state.adminLinks.push(...BUILTIN);

router.add('/admin', async ctx => {
  ctx.title('Панель управления');
  const ov = await api.get('/api/admin/overview', null, { silent: true }).catch(() => null);
  const links = state.adminLinks.filter(l => !l.access || can(l.access));
  const groups = new Map();
  for (const l of links) { if (!groups.has(l.group)) groups.set(l.group, []); groups.get(l.group).push(l); }
  const order = ['Пользователи и доступ', 'Модерация', 'API и разработчики', 'Аналитика', 'Система'];
  const names = [...order.filter(g => groups.has(g)), ...[...groups.keys()].filter(g => !order.includes(g))];

  const kpi = ov ? h('div.stat-grid',
    h('a.card.pad.stat', { href: '/admin/users' }, h('div.small.muted', 'Пользователей'), h('div.stat-num', fmt.num(ov.users))),
    h('a.card.pad.stat', { href: '/admin/users?filter=online' }, h('div.small.muted', 'Онлайн'), h('div.stat-num', { style: { color: 'var(--ok)' } }, fmt.num(ov.online))),
    h('a.card.pad.stat', { href: '/admin/users?filter=verified' }, h('div.small.muted', 'Верифицировано'), h('div.stat-num', fmt.num(ov.verified))),
    h('a.card.pad.stat', { href: '/admin/users?filter=2fa' }, h('div.small.muted', 'С защитой 2FA'), h('div.stat-num', fmt.num(ov.totp))),
    h('a.card.pad.stat', { href: '/admin/banlist' }, h('div.small.muted', 'Активных банов'), h('div.stat-num', fmt.num(ov.bans))),
    h('a.card.pad.stat', { href: '/admin/spam' }, h('div.small.muted', 'Жалоб'), h('div.stat-num', { style: ov.spam > 0 ? { color: 'var(--danger)' } : null }, fmt.num(ov.spam)))) : null;

  return [ui.pageHead('Панель управления'), kpi, ...names.map(g => h('div.admin-sec', h('h3.admin-h', g), h('div.admin-grid', groups.get(g).map(l =>
    h('a.card.pad.admin-tile', { href: l.href }, h('span.admin-ic', icon(l.icon || 'settings', 22)), h('div', h('b', l.title), h('div.small.muted', l.desc || ''))))))),
    links.length ? null : ui.card(ui.empty('Нет доступных разделов', 'lock'))];
}, { auth: true, access: 'adm_panel_show' });

// ───────── управление аккаунтами (живой поиск без перезагрузки)
router.add('/admin/users', async ctx => {
  ctx.title('Управление аккаунтами');
  let filter = ctx.query.filter || '';
  let query = ctx.query.q || '';
  let page = ctx.page || 1;
  let timer = null;
  let reqSeq = 0;

  const searchIn = h('input.input', {
    value: query,
    placeholder: 'Начните вводить ник, ID, имя, e-mail или IP…',
    autocomplete: 'off',
  });
  const countEl = h('div.small.muted', 'Загрузка…');
  const listEl = h('div.card');
  const pagerEl = h('div');

  const FILTERS = [
    ['', 'Все'],
    ['online', 'Онлайн'],
    ['verified', 'Верифицированные'],
    ['2fa', 'С 2FA'],
    ['banned', 'Забаненные'],
    ['staff', 'Персонал'],
  ];
  const tabsWrap = h('div.tabs');
  const drawTabs = () => {
    tabsWrap.replaceChildren(...FILTERS.map(([k, lbl]) => h('button.tab' + (filter === k ? '.active' : ''), {
      type: 'button',
      onclick: () => { filter = k; page = 1; drawTabs(); load(); },
    }, lbl)));
  };

  const canVerify = state.user && (state.user.level >= 3 || can('user_prof_edit') || can('adm_set_user'));

  const load = async () => {
    const seq = ++reqSeq;
    const d = await api.get('/api/admin/users', { q: query, filter, page }, { silent: true }).catch(() => null);
    if (seq !== reqSeq || !d) return;
    countEl.textContent = `Найдено аккаунтов: ${fmt.num(d.total)}`;
    if (!d.items.length) {
      listEl.replaceChildren(ui.empty('Аккаунты не найдены', 'users'));
      pagerEl.replaceChildren();
      return;
    }
    listEl.replaceChildren(h('div.list', ...d.items.map(it => {
      const u = it.user;
      return h('div.list-item',
        ui.avatar(u, 42),
        h('div.main',
          h('div.title',
            ui.userLink(u),
            ' ',
            h('span.small.muted', '#' + u.id),
            it.totp_enabled ? h('span.badge.ok', { style: { marginLeft: '6px' } }, '2FA') : null,
            it.banned ? h('span.badge.danger', { style: { marginLeft: '6px' } }, 'БАН') : null),
          h('div.small.muted',
            (u.group || 'Пользователь'),
            it.email ? ' · ' + it.email : '',
            it.ip ? ' · IP: ' + it.ip : ''),
          h('div.small.muted', 'Рег: ', fmt.date(it.date_reg), ' · Визит: ', fmt.ago(it.date_last), ' · Баллы: ', fmt.num(it.balls))),
        h('div.row',
          canVerify ? ui.btn('', {
            ic: 'check', small: true, kind: it.verified ? 'primary' : '',
            title: it.verified ? 'Снять верификацию' : 'Верифицировать аккаунт',
            onclick: async () => {
              const r = await api.post(`/api/admin/user/${u.id}/verify`);
              ui.ok(r.verified ? `${u.nick}: галочка выдана` : `${u.nick}: галочка снята`);
              load();
            },
          }) : null,
          it.totp_enabled && it.can_edit ? ui.btn('2FA', {
            ic: 'shield', small: true, kind: 'danger', title: 'Сбросить 2FA пользователя',
            onclick: async () => {
              if (await ui.confirm(`Сбросить двухфакторную аутентификацию у «${u.nick}»?`, { danger: true })) {
                await api.post(`/api/admin/user/${u.id}/reset-2fa`);
                ui.ok('2FA сброшена');
                load();
              }
            },
          }) : null,
          it.can_edit ? ui.btn('Ред.', { ic: 'edit', small: true, href: '/admin/user/' + u.id }) : null,
          (can('user_ban_set') || can('user_ban_set_h')) && u.id !== state.user.id ? ui.btn('', { ic: 'gavel', small: true, title: 'Бан', href: '/admin/ban/' + u.id }) : null));
    })));
    pagerEl.replaceChildren(ui.pager(d, { onPage: p => { page = p; load(); } }));
  };

  searchIn.oninput = () => {
    query = searchIn.value.trim();
    page = 1;
    clearTimeout(timer);
    timer = setTimeout(load, 180);
  };

  drawTabs();
  await load();
  return [
    crumbs({ title: 'Управление аккаунтами' }),
    ui.pageHead('Управление аккаунтами'),
    tabsWrap,
    h('div.card.pad.stack-s',
      h('div.row.nowrap', icon('search', 18), searchIn),
      countEl),
    listEl,
    pagerEl,
  ];
}, { auth: true, access: 'adm_panel_show' });

// ───────── модерация историй (24 часа)
router.add('/admin/stories', async ctx => {
  ctx.title('Истории (24 часа)');
  const reload = () => router.render();
  const d = await api.get('/api/stories');
  const flat = [];
  for (const g of (d.groups || [])) {
    for (const s of (g.stories || [])) flat.push({ ...s, user: g.user });
  }
  flat.sort((a, b) => b.time - a.time);

  return [
    crumbs({ title: 'Истории (24 часа)' }),
    ui.pageHead(`Активные истории (${flat.length})`),
    flat.length ? ui.card(h('div.list', ...flat.map(s => h('div.list-item',
      s.media
        ? h('img', { src: s.media, alt: '', style: { width: '48px', height: '64px', objectFit: 'cover', borderRadius: '8px', flex: 'none' } })
        : h('div', { style: { width: '48px', height: '64px', borderRadius: '8px', flex: 'none', background: s.bg || 'var(--accent)', display: 'grid', placeItems: 'center', color: '#fff', fontSize: '11px', padding: '4px', textAlign: 'center', overflow: 'hidden' } }, (s.caption || '').slice(0, 24)),
      h('div.main',
        h('div.title', ui.userLink(s.user)),
        s.caption ? h('div.small', s.caption) : h('div.small.muted', 'Без подписи (фото)'),
        h('div.small.muted', 'Опубликовано: ', fmt.ago(s.time), ' · Просмотров: ', fmt.num(s.views || 0))),
      ui.btn('', {
        ic: 'trash', small: true, kind: 'danger', title: 'Удалить историю',
        onclick: async () => {
          if (await ui.confirm('Удалить эту историю?', { danger: true })) {
            await api.del('/api/stories/' + s.id);
            ui.ok('История удалена');
            reload();
          }
        },
      }))))) : ui.card(ui.empty('Активных историй сейчас нет', 'zap')),
  ];
}, { auth: true, access: 'adm_panel_show' });

// ───────── управление OAuth-приложениями
router.add('/admin/oauth', async ctx => {
  ctx.title('OAuth-приложения');
  const reload = () => router.render();
  const d = await api.get('/api/admin/oauth-apps');
  const items = d.items || [];
  return [
    crumbs({ title: 'OAuth-приложения' }),
    ui.pageHead(`OAuth-приложения (${items.length})`),
    items.length ? ui.card(h('div.list', ...items.map(app => h('div.list-item',
      h('div.forum-ico', icon('key', 20)),
      h('div.main',
        h('div.title', app.name, ' ', h('span.badge', `токенов: ${app.active_tokens}`)),
        app.description ? h('div.small', app.description) : null,
        h('div.small.muted', 'Владелец: ', ui.userLink(app.owner), ' · ', fmt.datetime(app.created_at)),
        h('div.small.muted', 'Client ID: ', h('code', app.client_id)),
        app.redirect_uris && app.redirect_uris.length ? h('div.small.muted.clip', 'Redirect: ', app.redirect_uris.join(', ')) : null),
      ui.btn('', {
        ic: 'trash', small: true, kind: 'danger', title: 'Удалить приложение и отозвать токены',
        onclick: async () => {
          if (await ui.confirm(`Удалить OAuth-приложение «${app.name}» и отозвать все его токены?`, { danger: true })) {
            await api.del('/api/admin/oauth-apps/' + app.id);
            ui.ok('Приложение удалено');
            reload();
          }
        },
      }))))) : ui.card(ui.empty('OAuth-приложений пока нет', 'key')),
  ];
}, { auth: true, access: 'adm_set_sys' });

// ───────── информация
router.add('/admin/info', async ctx => {
  ctx.title('Общая информация');
  const d = await api.get('/api/admin/info');
  const up = d.uptime, upS = `${Math.floor(up / 86400)} д ${Math.floor(up % 86400 / 3600)} ч ${Math.floor(up % 3600 / 60)} мин`;
  const rows = [['Пользователей', fmt.num(d.users)], ['Активных сессий', fmt.num(d.sessions)], ['Действующих банов', fmt.num(d.bans)], ['Жалоб', fmt.num(d.spam)],
    ['СУБД (MySQL/MariaDB)', d.db_version || '—'], ['Размер БД', fmt.size(d.db_size || 0)], ['Таблиц в БД', fmt.num(d.db_tables || 0)],
    ['Загруженные файлы', fmt.size(d.data_size)], ['Память процесса', fmt.size(d.mem)], ['Горутин', d.goroutines], ['Процессоров', d.cpus], ['Платформа', d.os], ['Версия Go', d.go], ['Работает', upS]];
  return [crumbs({ title: 'Информация' }), ui.pageHead('Общая информация'), ui.card(h('div.list', rows.map(([k, v]) => h('div.list-item', h('div.main', k), h('b', S(v))))))];
}, { auth: true, access: 'adm_info' });

// ───────── статистика
router.add('/admin/stats', async ctx => {
  ctx.title('Статистика');
  const d = await api.get('/api/admin/stats');
  const chart = (title, key, color) => {
    const mx = Math.max(1, ...d.days.map(x => x[key]));
    return ui.card(h('div.pad', h('b', title), h('div.bars', d.days.map(x => h('div.bar', { title: `${new Date(x.day * 1000).toLocaleDateString('ru-RU')}: ${x[key]}` },
      h('i', { style: `height:${Math.max(2, x[key] / mx * 100)}%;background:${color}` }))))));
  };
  const sum = k => d.days.reduce((s, x) => s + x[k], 0);
  return [crumbs({ title: 'Статистика' }), ui.pageHead('Статистика'),
    h('div.stat-grid', [['Пользователей', d.users], ['Онлайн', d.online], ['Гостей', d.guests], ['Парней', d.male], ['Девушек', d.female], ['Хитов за 30 дн.', sum('hit')], ['Хостов за 30 дн.', sum('host')], ['Регистраций за 30 дн.', sum('reg')]]
      .map(([k, v]) => h('div.card.pad.stat', h('div.small.muted', k), h('div.stat-num', fmt.num(v))))),
    chart('Просмотры страниц по дням', 'hit', 'var(--accent)'), chart('Уникальные посетители по дням', 'host', '#10b981'), chart('Регистрации по дням', 'reg', '#f59e0b')];
}, { auth: true, access: 'adm_statistic' });

// ───────── рефералы
router.add('/admin/refs', async ctx => {
  ctx.title('Рефералы');
  const d = await api.get('/api/admin/refs', { page: ctx.page });
  return [crumbs({ title: 'Рефералы' }), ui.pageHead('Рефералы'), d.items.length ? ui.card(h('div.list', d.items.map(r => h('div.list-item', h('div.main', h('a', { href: 'https://' + r.url, target: '_blank', rel: 'noopener noreferrer' }, r.url), h('div.small.muted', 'Последний: ' + fmt.ago(r.time))), h('span.badge', r.count))))) : ui.card(ui.empty('Переходов пока нет', 'link')), ui.pager(d)];
}, { auth: true, access: 'adm_ref' });

// ───────── журнал
router.add('/admin/log', async ctx => {
  ctx.title('Журнал действий');
  const d = await api.get('/api/admin/log', { page: ctx.page, mod: ctx.query.mod || 0, user: ctx.query.user || 0 });
  const sel = h('select.input', { onchange: () => router.go('/admin/log' + (sel.value ? '?mod=' + sel.value : '')) }, h('option', { value: '' }, 'Все разделы'), d.mods.map(m => h('option', { value: m.id, selected: String(m.id) === ctx.query.mod }, m.name)));
  return [crumbs({ title: 'Журнал' }), ui.pageHead('Журнал действий', can('adm_log_delete') ? ui.btn('Очистить', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Очистить весь журнал?', { danger: true })) { await api.del('/api/admin/log'); router.go('/admin/log'); } } }) : null),
    h('div.card.pad', sel),
    d.items.length ? ui.card(h('div.list', d.items.map(l => h('div.list-item', h('div.main', h('div.title', l.mod + ' → ' + l.act), h('div.small', l.text), h('div.small.muted', ui.userLink(l.user), ' · ', fmt.datetime(l.time)))))))
      : ui.card(ui.empty('Записей нет', 'list')), ui.pager(d)];
}, { auth: true, access: 'adm_log_read' });

router.add('/admin/staff', async ctx => {
  ctx.title('Администрация');
  const d = await api.get('/api/admin/staff');
  return [crumbs({ title: 'Администрация' }), ui.pageHead('Администрация'), ui.card(h('div.list', d.items.map(x => h('div.list-item', ui.avatar(x.user, 40), h('div.main', h('div.title', ui.userLink(x.user)), h('div.small.muted', x.user.group + ' · ' + fmt.ago(x.last)))))))];
}, { auth: true, access: 'adm_show_adm' });

// ───────── жалобы
const SPAM_NAMES = { mail: 'Почта', guest: 'Гостевая', forum: 'Форум', notes_komm: 'Дневники', obmen_komm: 'Файлы', foto_komm: 'Фото', stena: 'Стена', status_komm: 'Статус' };
const spamLink = s => ({ forum: `/forum/theme/${s.obj}`, notes_komm: `/notes/${s.obj}`, obmen_komm: `/files/file/${s.obj}`, foto_komm: `/foto/photo/${s.obj}`, mail: `/mail/${s.from.id}`, guest: '/guest' })[s.razdel];
router.add('/admin/spam', async ctx => {
  ctx.title('Жалобы');
  const d = await api.get('/api/admin/spam', { page: ctx.page });
  return [crumbs({ title: 'Жалобы' }), ui.pageHead('Жалобы'), d.items.length ? h('div.card.pad.warn', icon('info', 16), ' После рассмотрения жалобы не забывайте её удалить.') : null,
    d.items.length ? d.items.map(s => ui.card(h('div.pad.stack-s',
      h('div', h('span.badge', SPAM_NAMES[s.razdel] || s.razdel), ' ', spamLink(s) ? h('a', { href: spamLink(s) }, 'Перейти к материалу') : null),
      h('div', h('b', 'Жалоба от: '), ui.userLink(s.from), h('span.muted.small', ' · ' + fmt.ago(s.time))),
      s.text ? h('div.quote', { html: s.text }) : null, h('div', h('b', 'Комментарий: '), h('span', { html: s.msg })),
      h('div', h('b', 'Нарушитель: '), ui.userLink(s.violator)),
      h('div.row', can('user_ban_set') || can('user_ban_set_h') ? ui.btn('Забанить', { ic: 'gavel', href: '/admin/ban/' + s.violator.id }) : null,
        ui.btn('Удалить жалобу', { ic: 'check', onclick: async () => { await api.del('/api/admin/spam/' + s.id); router.reload ? router.reload() : router.go('/admin/spam'); } }))))) : ui.card(ui.empty('Жалоб нет', 'check')), ui.pager(d)];
}, { auth: true, access: 'adm_panel_show' });

// ───────── редактирование пользователя
router.add('/admin/user/:id', async ctx => {
  const u = await api.get('/api/admin/user/' + ctx.params.id);
  ctx.title('Редактирование: ' + u.nick);
  const f = (name, label, extra = {}) => ({ name, label, value: u[name], ...extra });
  const form = ui.form([
    f('nick', 'Логин', { disabled: !u.can_nick, maxlength: 32 }), f('name', 'Имя', { maxlength: 32 }), f('city', 'Город', { maxlength: 32 }),
    { name: 'pol', label: 'Пол', type: 'select', value: u.pol, options: [{ value: 1, label: 'Мужской' }, { value: 0, label: 'Женский' }] },
    f('day', 'День рождения', { type: 'number', min: 0, max: 31 }), f('month', 'Месяц', { type: 'number', min: 0, max: 12 }), f('year', 'Год', { type: 'number', min: 0 }),
    f('skype', 'Skype'), f('phone', 'Телефон'), f('email', 'E-mail', { type: 'email' }), f('about', 'О себе', { type: 'textarea', rows: 3 }),
    { name: 'new_pass', label: 'Новый пароль', type: 'password', hint: 'Оставьте пустым, чтобы не менять (минимум 6 символов)', autocomplete: 'new-password' },
    { name: 'group', label: 'Группа', type: 'select', value: u.group, disabled: !u.can_group, options: u.groups.filter(g => g.allowed || g.id === u.group).map(g => ({ value: g.id, label: g.name })) },
    f('balls', 'Баллы', { type: 'number', disabled: !u.can_balls }), f('set_time_chat', 'Автообновление, сек', { type: 'number', min: 0, max: 900 }), f('set_p_str', 'Пунктов на страницу', { type: 'number', min: 0, max: 100 }),
    { name: 'set_files', label: 'Показывать файлы в ленте', type: 'checkbox', value: u.set_files },
    { name: 'verified', label: 'Верифицированный аккаунт (галочка)', type: 'checkbox', value: !!u.verified }],
    { submit: 'Сохранить', onSubmit: async v => {
      const b = { ...v, group: +v.group, pol: +v.pol, balls: v.balls ?? 0 }; if (!u.can_nick) delete b.nick; if (!u.can_group) delete b.group; if (!u.can_balls) delete b.balls;
      for (const k of ['day', 'month', 'year', 'set_time_chat', 'set_p_str']) b[k] = v[k] ?? 0;
      await api.put('/api/admin/user/' + u.id, b); ui.ok('Изменения сохранены'); } });
  const info = h('div.card.pad.small.muted', 'Регистрация: ', fmt.datetime(u.date_reg), ' · Последний визит: ', fmt.datetime(u.date_last), u.ip ? ' · IP: ' + u.ip : '', u.ua ? h('div', 'UA: ' + u.ua) : null);
  const out = [crumbs({ href: '/admin/users', title: 'Аккаунты' }, { href: '/id' + u.id, title: u.nick }, { title: 'Редактирование' }), ui.pageHead('Редактирование: ' + u.nick,
    u.totp_enabled ? ui.btn('Сбросить 2FA', { ic: 'shield', kind: 'danger', onclick: async () => { if (await ui.confirm('Сбросить двухфакторную аутентификацию для «' + u.nick + '»?', { danger: true })) { await api.post('/api/admin/user/' + u.id + '/reset-2fa'); ui.ok('2FA сброшена'); router.render(); } } }) : null,
    ui.btn('Бан', { ic: 'gavel', href: '/admin/ban/' + u.id }), u.can_delete ? ui.btn('Удалить', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить пользователя «' + u.nick + '» со всеми данными?', { danger: true })) { await api.del('/api/admin/user/' + u.id); ui.ok('Удалён'); router.go('/admin/users'); } } }) : null), info, ui.card(h('div.pad', form))];
  if (can('user_collisions')) {
    const box = h('div.card.pad', h('b', 'Совпадения по IP'), h('div.small.muted', 'Загрузка…'));
    api.get('/api/admin/collisions/' + u.id).then(r => box.replaceChildren(h('b', 'Совпадения по IP'), r.items.length ? h('div.list', r.items.map(x => h('div.list-item', h('div.main', ui.userLink(x.user)), h('span.badge', x.count)))) : h('div.small.muted', 'Совпадений нет'))).catch(() => box.remove());
    out.push(box);
  }
  return out;
}, { auth: true, access: 'user_prof_edit' });

// ───────── бан пользователя
const banRow = (b, canUnset, reload) => h('div.list-item', h('div.main', h('div.title', b.razdel_name + ' — ' + b.why), h('div.small', b.reason), h('div.small.muted', 'Выдал: ', ui.userLink(b.by), ' · ', b.forever ? 'навсегда' : 'до ' + fmt.datetime(b.until))),
  canUnset ? ui.btn('', { ic: 'trash', small: true, title: 'Снять бан', onclick: async () => { if (await ui.confirm('Снять бан?')) { await api.del('/api/admin/ban-item/' + b.id); ui.ok('Бан снят'); reload(); } } }) : null);

router.add('/admin/ban/:uid', async ctx => {
  const d = await api.get('/api/admin/ban/' + ctx.params.uid);
  ctx.title('Бан: ' + d.user.nick);
  const reload = () => router.go('/admin/ban/' + d.user.id);
  const out = [crumbs({ href: '/id' + d.user.id, title: d.user.nick }, { title: 'Бан' }), ui.pageHead('Бан: ' + d.user.nick)];
  if (d.bans.length) out.push(ui.section('Действующие блокировки', ui.card(h('div.list', d.bans.map(b => banRow(b, d.can_unset, reload))))));
  if (!d.can_set) { out.push(ui.card(ui.empty(d.user.id === state.user.id ? 'Нельзя банить себя' : 'Нет прав блокировать этого пользователя', 'lock'))); return out; }
  const units = [{ value: 'min', label: 'минут' }, { value: 'chas', label: 'часов' }, { value: 'sut', label: 'суток' }, ...(d.full ? [{ value: 'mes', label: 'месяцев' }] : [])];
  const fields = [
    { name: 'razdel', label: 'Раздел', type: 'select', options: d.razdels.map(r => ({ value: r.id, label: r.name })) },
    { name: 'time', label: 'Срок', type: 'number', value: 1, min: 1, hint: d.full ? '' : 'Вам доступен бан максимум на 1 сутки' }, { name: 'unit', label: 'Единица', type: 'select', value: 'chas', options: units },
    ...(d.full ? [{ name: 'forever', label: 'Бессрочно', type: 'checkbox' }] : []),
    { name: 'why', label: 'Причина', type: 'select', value: 6, options: d.reasons.map(r => ({ value: r.id, label: r.name })) },
    { name: 'reason', label: 'Комментарий', type: 'textarea', maxlength: 1024, hint: 'От 10 до 1024 символов; увидит пользователь', required: true }];
  out.push(ui.section('Новая блокировка', ui.card(h('div.pad', ui.form(fields, { submit: 'Забанить', onSubmit: async v => {
    await api.post('/api/admin/ban/' + d.user.id, { ...v, why: +v.why, time: v.time || 1, forever: !!v.forever }); ui.ok('Пользователь заблокирован'); reload(); } })))));
  return out;
}, { auth: true });

router.add('/admin/banlist', async ctx => {
  ctx.title('Список банов');
  const d = await api.get('/api/admin/banlist', { page: ctx.page, razdel: ctx.query.razdel || '' });
  const razdels = [['', 'Все'], ['all', 'Везде'], ['guest', 'Гостевая'], ['notes', 'Дневники'], ['forum', 'Форум'], ['files', 'Файлы'], ['chat', 'Чат'], ['foto', 'Фото']];
  return [crumbs({ title: 'Список банов' }), ui.pageHead('Список банов'), ui.tabs(razdels.map(([k, n]) => ({ key: k, label: n, href: '/admin/banlist' + (k ? '?razdel=' + k : '') })), ctx.query.razdel || ''),
    d.items.length ? ui.card(h('div.list', d.items.map(b => h('div.list-item', ui.avatar(b.user, 40), h('div.main', h('div.title', ui.userLink(b.user), ' — ', b.razdel_name), h('div.small', b.why + ': ' + b.reason), h('div.small.muted', 'Выдал: ', ui.userLink(b.by), ' · ', b.forever ? 'навсегда' : 'до ' + fmt.datetime(b.until))),
      can('user_ban_unset') ? ui.btn('', { ic: 'trash', small: true, title: 'Снять', onclick: async () => { if (await ui.confirm('Снять бан?')) { await api.del('/api/admin/ban-item/' + b.id); router.go('/admin/banlist'); } } }) : null)))) : ui.card(ui.empty('Забаненных нет', 'check')), ui.pager(d)];
}, { auth: true, access: 'adm_banlist' });

// ───────── диапазоны IP (бан и операторы)
function ipPage(path, title, access, withName) {
  router.add(path, async ctx => {
    ctx.title(title);
    const d = await api.get('/api' + path);
    const fields = [{ name: 'min', label: 'Начало диапазона', placeholder: '192.168.0.1', required: true }, { name: 'max', label: 'Конец диапазона', placeholder: '192.168.0.255', required: true }, ...(withName ? [{ name: 'opsos', label: 'Оператор', maxlength: 64, required: true }] : [])];
    return [crumbs({ title }), ui.pageHead(title), d.my_ip ? h('div.card.pad.small.muted', 'Ваш IP: ', h('b', d.my_ip)) : null,
      ui.card(h('div.pad', ui.form(fields, { submit: 'Добавить', onSubmit: async v => { await api.post('/api' + path, v); ui.ok('Добавлено'); router.go(path); } }))),
      d.items.length ? ui.card(h('div.list', d.items.map(x => h('div.list-item', h('div.main', h('div.title', x.min + ' – ' + x.max), x.opsos ? h('div.small.muted', x.opsos) : null),
        ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить диапазон?', { danger: true })) { await api.del(`/api${path}/${x.id}`); router.go(path); } } }))))) : ui.card(ui.empty('Список пуст', 'globe'))];
  }, { auth: true, access });
}
ipPage('/admin/ban-ip', 'Бан по IP', 'adm_ban_ip', false);
ipPage('/admin/opsos', 'Операторы связи', 'adm_ip_edit', true);

// ───────── массовое удаление
router.add('/admin/mass-delete', async ctx => {
  ctx.title('Массовое удаление');
  const res = h('div');
  const form = ui.form([{ name: 'n', label: 'Не заходили более', type: 'number', value: 6, min: 1 }, { name: 'unit', label: 'Единица', type: 'select', value: 'mes', options: [{ value: 'sut', label: 'суток' }, { value: 'mes', label: 'месяцев' }] }],
    { submit: 'Найти', onSubmit: async v => {
      const q = { n: v.n, unit: v.unit }, d = await api.get('/api/admin/mass-delete', q);
      res.replaceChildren(h('div.card.pad.stack-s', h('div', 'Будет удалено пользователей: ', h('b', S(d.count))), d.sample.length ? h('div.small.muted', d.sample.join(', ') + (d.count > d.sample.length ? '…' : '')) : null,
        d.count ? ui.btn('Удалить безвозвратно', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить ${d.count} пользователей со всеми данными?`, { danger: true })) { const r = await api.post(`/api/admin/mass-delete?n=${v.n}&unit=${v.unit}`, {}); ui.ok('Удалено: ' + r.deleted); res.replaceChildren(); } } }) : null)); } });
  return [crumbs({ title: 'Массовое удаление' }), ui.pageHead('Массовое удаление'), h('div.card.pad.warn', icon('info', 16), ' Удаляются пользователи ниже вашего уровня, давно не бывавшие на сайте. Действие необратимо.'), ui.card(h('div.pad', form)), res];
}, { auth: true, access: 'user_mass_delete' });

// ───────── настройки (формируются по описанию с сервера)
router.add('/admin/settings/:sec', async ctx => {
  const d = await api.get('/api/admin/settings/' + ctx.params.sec);
  ctx.title(d.title);
  const fields = d.fields.map(f => ({ name: f.key, label: f.label, hint: f.hint, value: f.type === 'bool' ? d.values[f.key] === '1' : d.values[f.key], type: f.type === 'bool' ? 'checkbox' : f.type === 'int' ? 'number' : f.type, min: f.min, max: f.max, options: f.opts ? f.opts.map(([value, label]) => ({ value, label })) : undefined }));
  return [crumbs({ title: d.title }), ui.pageHead(d.title), ui.card(h('div.pad', ui.form(fields, { submit: 'Сохранить', onSubmit: async v => {
    const b = {}; for (const f of d.fields) b[f.key] = f.type === 'int' ? String(v[f.key] ?? '') : v[f.key]; await api.put('/api/admin/settings/' + ctx.params.sec, b); ui.ok('Настройки сохранены'); } })))];
}, { auth: true, access: 'adm_panel_show' });

// ───────── меню (главное и пользовательских настроек)
function menuPage(path, api0, title, access, settings) {
  router.add(path, async ctx => {
    ctx.title(title);
    const d = await api.get(api0), reload = () => router.go(path);
    const fields = (m = {}) => [{ name: 'name', label: 'Название', value: m.name, required: true, maxlength: 64 }, ...(settings ? [] : [{ name: 'type', label: 'Тип', type: 'select', value: m.type || 'link', options: [{ value: 'link', label: 'Ссылка' }, { value: 'razd', label: 'Заголовок раздела' }], disabled: !!m.id }]),
      { name: 'url', label: 'Ссылка', value: m.url, maxlength: 255 }, { name: 'counter', label: 'Счётчик', value: m.counter, hint: 'Ключ счётчика (например forum, mail)' },
      { name: 'icon', label: 'Иконка', value: settings ? m.img : m.icon, hint: 'Имя иконки (например home, users)' }, ...(settings ? [{ name: 'accesses', label: 'Требуемая привилегия', value: m.accesses }] : [])];
    const edit = m => { const mo = ui.modal(m.id ? 'Изменить пункт' : 'Новый пункт', ui.form(fields(m), { submit: 'Сохранить', onSubmit: async v => { await (m.id ? api.put(`${api0}/${m.id}`, v) : api.post(api0, v)); mo.close(); reload(); } })); };
    const move = async (m, dir) => { await api.post(`${api0}/${m.id}/move`, { dir }); reload(); };
    return [crumbs({ title }), ui.pageHead(title, ui.btn('Добавить', { ic: 'plus', kind: 'primary', onclick: () => edit({}) })),
      d.items.length ? ui.card(h('div.list', d.items.map((m, i) => h('div.list-item', icon(settings ? 'settings' : m.icon || 'link', 18), h('div.main', h('div.title', (m.type === 'razd' ? '▸ ' : '') + m.name), h('div.small.muted', S(m.url) + (m.counter ? ' · счётчик: ' + m.counter : ''))),
        h('div.row', ui.btn('', { ic: 'chevron-up', small: true, disabled: i === 0, onclick: () => move(m, 'up') }), ui.btn('', { ic: 'chevron-down', small: true, disabled: i === d.items.length - 1, onclick: () => move(m, 'down') }),
          ui.btn('', { ic: 'edit', small: true, onclick: () => edit(m) }), ui.btn('', { ic: 'trash', small: true, onclick: async () => { if (await ui.confirm('Удалить пункт «' + m.name + '»?', { danger: true })) { await api.del(`${api0}/${m.id}`); reload(); } } })))))) : ui.card(ui.empty('Меню пусто', 'menu'))];
  }, { auth: true, access });
}
menuPage('/admin/menu', '/api/admin/menu', 'Главное меню', 'adm_menu', false);
menuPage('/admin/settings-menu', '/api/admin/settings-menu', 'Меню настроек', 'user_settings', true);

// ───────── группы и привилегии
router.add('/admin/groups', async ctx => {
  ctx.title('Группы и привилегии');
  const d = await api.get('/api/admin/groups');
  const out = [crumbs({ title: 'Группы' }), ui.pageHead('Группы и привилегии', ui.btn('Новая группа', { ic: 'plus', onclick: () => {
    const mo = ui.modal('Новая группа', ui.form([{ name: 'name', label: 'Название', required: true, maxlength: 32 }, { name: 'level', label: 'Уровень (0–10)', type: 'number', value: 1, min: 0, max: 10 }],
      { submit: 'Создать', onSubmit: async v => { await api.post('/api/admin/groups', { name: v.name, level: v.level }); mo.close(); router.go('/admin/groups'); } })); } })),
    h('div.card.pad.warn', icon('info', 16), ' Изменение привилегий применяется немедленно. Будьте осторожны с правами уровня «!!!».')];
  for (const g of d.groups) {
    const set = new Set(g.accesses);
    const name = h('input.input', { value: g.name, maxlength: 32 });
    const boxes = d.accesses.map(ac => { const cb = h('input', { type: 'checkbox', checked: set.has(ac.type), onchange: () => (cb.checked ? set.add(ac.type) : set.delete(ac.type)) }); return h('label.check', cb, h('span', ac.name, h('small.muted', ' ' + ac.type))); });
    out.push(h('details.card.pad', h('summary', icon('key', 16), ' ', h('b', g.name), h('span.muted.small', ` · уровень ${g.level} · пользователей: ${g.users} · прав: ${g.accesses.length}`)),
      h('div.stack', h('div.field', h('label', 'Название'), name), h('div.access-list', boxes),
        h('div.row', ui.btn('Сохранить', { ic: 'check', kind: 'primary', onclick: async () => { await api.put('/api/admin/groups/' + g.id, { name: name.value, accesses: [...set] }); ui.ok('Привилегии сохранены'); } }),
          g.users === 0 ? ui.btn('Удалить группу', { ic: 'trash', kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить группу «' + g.name + '»?', { danger: true })) { await api.del('/api/admin/groups/' + g.id); router.go('/admin/groups'); } } }) : null))));
  }
  return out;
}, { auth: true, access: 'adm_accesses' });

// ───────── резервная копия
router.add('/admin/backup', async ctx => {
  ctx.title('Резервная копия');
  return [crumbs({ title: 'Резервная копия' }), ui.pageHead('Резервная копия'), h('div.card.pad.stack', h('p', 'Архив содержит полный SQL-дамп базы данных MySQL / MariaDB (загруженные файлы в него не входят).'),
    h('a.btn.primary', { href: '/api/admin/backup', download: '' }, icon('download', 16), h('span', 'Скачать SQL-дамп (.zip)')))];
}, { auth: true, access: 'adm_mysql' });

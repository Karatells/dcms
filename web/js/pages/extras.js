import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

const me = () => state.user;
const crumbs = (...c) => ui.breadcrumbs(c);
state.adminLinks.push({ group: 'Контент', title: 'Подарки', href: '/admin/gifts', access: 'adm_panel_show', icon: 'gift', desc: 'Категории и каталог подарков, цены' });
const cur = d => d.currency || 'монет';
const wrap = (...c) => c.filter(Boolean);

// ───────── закладки
const BM_ICONS = { people: 'users', file: 'file', foto: 'camera', forum: 'message', notes: 'book' };
const bmIndex = async ctx => {
  const uid = ctx.params.uid || (me() && me().id);
  const d = await api.get('/api/bookmarks/' + uid);
  ctx.title('Закладки ' + d.owner.nick);
  return [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { title: 'Закладки' }), ui.pageHead('Закладки ' + d.owner.nick),
    d.mine ? h('div.card.pad.small.muted', 'С помощью закладок вы можете сохранить ссылку на интересного человека, файл, фотографию, тему форума или блог.') : null,
    ui.card(h('div.list', d.items.map(x => h('a.list-item', { href: `/user/bookmark/${uid}/${x.type}` }, h('div.forum-ico', icon(BM_ICONS[x.type] || 'star', 22)), h('div.main', h('div.title', x.name)), h('span.badge', String(x.count))))))];
};
router.add('/user/bookmark', bmIndex, { auth: true });
router.add('/user/bookmark/:uid', bmIndex);
router.add('/user/bookmark/:uid/:type', async ctx => {
  const { uid, type } = ctx.params, reload = () => router.render();
  const d = await api.get(`/api/bookmarks/${uid}/${type}`, { page: ctx.page });
  const names = { people: 'Люди', file: 'Файлы', foto: 'Фотографии', forum: 'Форум', notes: 'Блоги' };
  ctx.title('Закладки — ' + names[type]);
  const del = it => d.mine ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { await api.post('/api/bookmark/toggle', { type, id: it.id }); ui.ok('Закладка удалена'); reload(); } }) : null;
  return [crumbs({ href: '/user/bookmark/' + uid, title: 'Закладки' }, { title: names[type] }), ui.pageHead('Закладки — ' + names[type]),
    ui.card(d.items.length ? h('div.list', d.items.map(it => type === 'people'
      ? h('div.list-item', ui.avatar(it.user, 40), h('div.main', ui.userLink(it.user), h('div.small.muted', fmt.ago(it.time))), del(it))
      : h('div.list-item', h('a.main', { href: it.link }, h('div.title', it.title), it.text ? h('div.small.muted.clip', it.text) : null, h('div.small.muted', it.owner && it.owner.nick ? it.owner.nick + ' · ' : '', fmt.ago(it.time))), del(it)))) : ui.empty('Нет результатов', 'star')), ui.pager(d)];
});

// ───────── лидеры (формируется из VIP-подписчиков с включённым выводом в Топ)
router.add('/user/liders', async ctx => {
  ctx.title('Лидеры');
  const reload = () => router.render();
  const d = await api.get('/api/liders', { page: ctx.page });
  return [ui.pageHead('Лидеры', me() ? ui.btn('Попасть в Топ (VIP)', { ic: 'crown', kind: 'primary', small: true, href: '/vip' }) : null),
    ui.card(d.items.length ? h('div.list', d.items.map(l => h('div.list-item', ui.avatar(l.user, 44), h('div.main', h('div.row', ui.userLink(l.user), h('span.grow'), h('span.small.muted', 'до ' + fmt.datetime(l.time))),
      h('div', { html: l.msg })),
      d.admin ? ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить пользователя из лидеров?', { danger: true })) { await api.del('/api/liders/' + l.user.id); ui.ok('Пользователь удалён из списка лидеров'); reload(); } } }) : null))) : ui.empty('В топе пока никого нет', 'crown')), ui.pager(d),
    me() ? h('div.card.pad.row', link('/vip', icon('crown', 16), ' Подключить VIP-подписку и вывести анкету в Топ')) : null];
});

// ───────── VIP-подписка (/vip)
const NICK_STYLE_NAMES = { '': 'Стандартный', gold: 'Золотой', neon: 'Неоновый', sunset: 'Закат', ocean: 'Океан', emerald: 'Изумруд', rainbow: 'Радужный' };
const COVER_NAMES = { '': 'Стандартный', aurora: 'Аврора', royal: 'Королевский', cyber: 'Киберпанк', sunset: 'Закат', emerald: 'Изумруд', midnight: 'Космос' };
const VIP_PERKS = [
  ['👑', 'Уникальная иконка возле ника', 'Выбирайте из 10 премиальных значков — видна в профиле, чатах, комментариях и ленте'],
  ['🎨', 'Градиентный цвет ника', '6 эксклюзивных цветовых стилей для вашего логина по всему сайту'],
  ['🖼️', '6 уникальных дизайнов всего профиля', 'Полное преображение вашей страницы: анимированный баннер, цветовая палитра карточек, рамок, плиток и свечение аватара'],
  ['✨', 'Золотая рамка аватара', 'Элегантное золотое свечение вокруг аватара в профиле'],
  ['📈', 'Продвижение анкеты в Топ и Знакомства', 'Включайте и выключайте показ своей анкеты со своей подписью в разделах «Лидеры» и «Знакомства» бесплатно'],
  ['👻', 'Режим невидимки в гостях', 'Просматривайте чужие страницы, не оставляя следов в их списке гостей'],
  ['⚡', 'До 25 историй за 24 часа', 'Увеличенный лимит активных историй (25 вместо 10)'],
  ['⭐', 'Бесплатная «Оценка 5+»', 'Автоматически активна на весь срок действия VIP-подписки'],
  ['💰', 'Выгодный обмен баллов (-20%)', '400 баллов за монету вместо 500 при обмене в магазине услуг'],
  ['💎', 'Достижение «VIP-клуб»', 'Особый золотой бейдж в блоке достижений вашего профиля'],
];

async function renderVIPPage(ctx) {
  ctx.title('VIP-подписка');
  const d = await api.get('/api/vip');
  const reload = () => router.render();
  const s = d.settings || {};

  const hero = h('div.card.vip-hero',
    h('div.row',
      h('div.grow',
        h('h2', { style: { margin: '0 0 4px' } }, '👑 VIP-подписка'),
        h('div', { style: { opacity: '.92', fontSize: '.92rem' } },
          d.active ? `Активна до ${fmt.datetime(d.until)}` : '10 эксклюзивных возможностей для выделения вашего аккаунта')),
      h('div.vip-bal',
        h('div', { style: { fontSize: '.74rem', opacity: '.82' } }, 'Ваш баланс'),
        h('b', { style: { fontSize: '1.15rem' } }, `${fmt.num(d.money)} ${cur(d)}`))),
    h('div.row', { style: { marginTop: '10px' } },
      ui.btn('Обменять баллы на монеты', { small: true, href: '/user/money' })));

  const plans = h('div.vip-plans', ...(d.plans || []).map(p => h('div.card.pad.vip-plan' + (p.months === 12 ? '.best' : ''),
    p.discount > 0 ? h('span.vip-disc', `−${p.discount}%`) : null,
    h('div.vip-plan-t', p.label),
    h('div.vip-plan-p',
      h('b', `${fmt.num(p.price)} ${cur(d)}`),
      p.discount > 0 ? h('span.vip-old', `${fmt.num(p.old_price)}`) : null),
    h('div.small.muted', `${Math.round(p.price / p.months)} ${cur(d)} / мес`),
    ui.btn(d.active ? 'Продлить' : 'Подключить', {
      ic: 'crown', kind: p.months === 12 || p.months === 3 ? 'primary' : '', small: true,
      onclick: async () => {
        if (d.money < p.price) return ui.toast(`Недостаточно монет (нужно ${p.price}, у вас ${d.money})`, 'warn');
        if (!(await ui.confirm(`${d.active ? 'Продлить' : 'Оформить'} VIP-подписку на ${p.label} за ${p.price} ${cur(d)}?`))) return;
        await api.post('/api/vip/buy', { months: p.months });
        ui.ok(`VIP-подписка оформлена на ${p.label}!`);
        reload();
      },
    }))));

  let settingsCard = null;
  if (d.active) {
    let badge = s.badge || '👑';
    let nickStyle = s.nick_style || '';
    let cover = s.cover || '';
    const stealthCb = h('input', { type: 'checkbox', checked: !!s.stealth });
    const topCb = h('input', { type: 'checkbox', checked: !!s.top });
    const topMsgIn = h('textarea.input', { rows: 2, maxlength: 215, value: s.top_msg || '', placeholder: 'Короткая подпись для разделов «Лидеры» и «Знакомства» (до 215 символов)…' });

    const previewNick = h('span');
    const previewWrap = h('div.card.pad', { style: { background: 'var(--bg-sunken)', display: 'flex', alignItems: 'center', gap: '8px' } },
      h('span.small.muted', 'Превью ника: '), previewNick);
    const updatePreview = () => {
      previewNick.replaceChildren(
        h('b' + (nickStyle ? '.vip-nick-' + nickStyle : ''), state.user ? state.user.nick : 'Ваш ник'),
        ' ',
        h('span.ub.vip-ic', badge));
    };

    const badgeWrap = h('div.row');
    const drawBadges = () => {
      badgeWrap.replaceChildren(...(d.badges || []).map(b => h('button.vip-pick' + (badge === b ? '.active' : ''), {
        type: 'button', onclick: () => { badge = b; drawBadges(); updatePreview(); },
      }, b)));
    };

    const styleWrap = h('div.row');
    const drawStyles = () => {
      styleWrap.replaceChildren(...(d.nick_styles || []).map(st => h('button.vip-style-btn' + (nickStyle === st ? '.active' : ''), {
        type: 'button', onclick: () => { nickStyle = st; drawStyles(); updatePreview(); },
      }, h('span' + (st ? '.vip-nick-' + st : ''), NICK_STYLE_NAMES[st] || st))));
    };

    const coverWrap = h('div.row');
    const drawCovers = () => {
      coverWrap.replaceChildren(...(d.covers || []).map(cv => h('button.vip-cov-btn' + (cv ? '.vip-cov-' + cv : '') + (cover === cv ? '.active' : ''), {
        type: 'button', onclick: () => { cover = cv; drawCovers(); },
      }, COVER_NAMES[cv] || cv)));
    };

    drawBadges();
    drawStyles();
    drawCovers();
    updatePreview();

    settingsCard = h('div.card.pad.stack',
      h('h3', { style: { margin: 0 } }, 'Настройка ваших VIP-привилегий'),
      previewWrap,
      h('div.field', h('label', 'Иконка возле ника'), badgeWrap),
      h('div.field', h('label', 'Стиль ника'), styleWrap),
      h('div.field', h('label', 'Дизайн всего профиля'), h('div.small.muted', 'Меняет оформление всей вашей страницы: баннер, рамки карточек, кнопки, плитки и свечение аватара.'), coverWrap),
      h('div.card.pad.stack-s', { style: { background: 'var(--bg-sunken)' } },
        h('label.check', topCb, h('b', ' Выводить анкету в «Лидеры» и «Знакомства»')),
        h('div.small.muted', 'Ваша анкета будет показываться в топе раздела знакомств и в списке лидеров, пока включён этот переключатель.'),
        topMsgIn),
      h('label.check', stealthCb, h('b', ' Режим невидимки в гостях'), ' ', h('span.small.muted', '(ваши визиты не отображаются в чужих гостях)')),
      ui.btn('Сохранить VIP-настройки', {
        ic: 'check', kind: 'primary',
        onclick: async () => {
          await api.put('/api/vip/settings', {
            badge, nick_style: nickStyle, cover, stealth: stealthCb.checked, top: topCb.checked, top_msg: topMsgIn.value.trim(),
          });
          ui.ok('VIP-настройки сохранены');
          reload();
        },
      }));
  }

  const perksCard = ui.section('Что даёт VIP-подписка (10 привилегий)',
    h('div.list', ...VIP_PERKS.map(([em, t, desc]) => h('div.list-item',
      h('span', { style: { fontSize: '1.35rem', width: '30px', textAlign: 'center', flex: 'none' } }, em),
      h('div.main', h('div.title', t), h('div.small.muted', desc))))));

  return [
    crumbs({ href: '/user/money', title: 'Услуги' }, { title: 'VIP-подписка' }),
    hero,
    h('div.create-sub', d.active ? 'Продлить подписку со скидкой' : 'Выберите период подписки'),
    plans,
    settingsCard,
    perksCard,
  ];
}
router.add('/vip', renderVIPPage, { auth: true });
router.add('/user/vip', renderVIPPage, { auth: true });

// ───────── магазин услуг
const badgeOn = on => h('span.badge' + (on ? '.ok' : '.danger'), on ? 'Активна' : 'Купить');
router.add('/user/money', async ctx => {
  ctx.title('Магазин услуг');
  const reload = () => { router.render(); };
  const d = await api.get('/api/money');
  const n = h('select.input', { style: { width: 'auto', maxWidth: '100%' } }, Array.from({ length: 20 }, (_, k) => h('option', { value: k + 1 }, `${k + 1} ${cur(d)} — ${(k + 1) * d.balls_per_coin} баллов`)));
  return [ui.pageHead('Магазин услуг'),
    h('div.card.pad', h('div.row', h('div.grow', h('div.small.muted', 'Активность'), h('div.h2', String(d.balls), ' ', h('small.muted', 'баллов'))), h('div.grow', h('div.small.muted', 'Баланс'), h('div.h2', String(d.money), ' ', h('small.muted', cur(d))))),
      h('div.row.nowrap', n, ui.btn('Обменять баллы', { ic: 'plus', kind: 'primary', onclick: async () => { await api.post('/api/money/exchange', { n: +n.value }); ui.ok('Пополнение счёта произведено'); reload(); } })),
      d.is_vip ? h('div.small', { style: { marginTop: '6px', color: 'var(--ok)' } }, '👑 Действует VIP-курс обмена: 400 баллов за монету (-20%)') : null),
    ui.card(h('div.list',
      h('a.list-item', { href: '/vip' }, h('span', { style: { fontSize: '1.35rem' } }, '👑'),
        h('div.main', h('div.title', 'VIP-подписка'), h('div.small.muted', d.is_vip ? 'Активна до ' + fmt.datetime(d.vip_until) + ' · Настроить привилегии' : 'Иконка, цвет ника, обложки, Топ знакомств, невидимка и др. (скидки до 30%)')),
        badgeOn(d.is_vip)),
      h('a.list-item', { href: '/user/money/plus5' }, icon('star', 22), h('div.main', h('div.title', 'Оценка 5+'), d.is_plus5 ? h('div.small.muted', 'до ' + fmt.datetime(d.plus5_until)) : h('div.small.muted', 'Также входит бесплатно в VIP-подписку')), badgeOn(d.is_plus5)),
      h('a.list-item', { href: '/user/money/translate' }, icon('send', 22), h('div.main', h('div.title', 'Перевод другому пользователю')), h('span.badge', 'карта')))),
    h('div.card.pad.small.muted', 'Номер вашей карты для получения переводов: ', h('b', d.card.replace(/(\d{4})(?=\d)/g, '$1 ')))];
}, { auth: true });

const stavSelect = d => h('select.input', Array.from({ length: 7 }, (_, k) => h('option', { value: k + 1 }, `${k + 1} ${cur(d)} — ${k + 1} ${fmt.plural ? fmt.plural(k + 1, ['день', 'дня', 'дней']) : 'дн.'}`)));
router.add('/user/money/liders', async () => { router.go('/vip', true); return ui.loading(); }, { auth: true });
router.add('/user/money/plus5', async ctx => {
  ctx.title('Оценка 5+');
  const d = await api.get('/api/money');
  const st = stavSelect(d);
  return [crumbs({ href: '/user/money', title: 'Услуги' }, { title: 'Оценка 5+' }), ui.pageHead('Услуга «Оценка 5+»'), h('div.card.pad.small', '1 монета = 1 день пользования привилегией. Позволяет ставить фотографиям оценку «5+».'),
    d.is_plus5 ? h('div.card.pad', 'Услуга подключена до ' + fmt.datetime(d.plus5_until))
      : h('div.card.pad.stack', h('div.field', h('label', 'Ставка'), st), ui.btn('Купить услугу', { ic: 'star', kind: 'primary', onclick: async () => { await api.post('/api/money/plus5', { stav: +st.value }); ui.ok('Услуга успешно подключена'); router.go('/user/money'); } }))];
}, { auth: true });
router.add('/user/money/translate', async ctx => {
  ctx.title('Перевод');
  const d = await api.get('/api/money');
  const card = h('input.input', { placeholder: '4444 0000 0000 0000', maxlength: 19, inputmode: 'numeric' }), amount = h('input.input', { type: 'number', min: 1, placeholder: 'Сумма' });
  return [crumbs({ href: '/user/money', title: 'Услуги' }, { title: 'Перевод' }), ui.pageHead('Перевод монет'), h('div.card.pad.small.muted', 'Ваш баланс: ', h('b', String(d.money), ' ', cur(d))),
    h('div.card.pad.stack', h('div.field', h('label', 'Номер карты получателя'), card), h('div.field', h('label', 'Сумма'), amount),
      ui.btn('Отправить', { ic: 'send', kind: 'primary', onclick: async () => { await api.post('/api/money/transfer', { card: card.value, amount: +amount.value }); ui.ok('Перевод успешно выполнен'); router.go('/user/money'); } }))];
}, { auth: true });

// ───────── подарки
const giftPic = (g, size = 72) => g.img ? h('img', { src: g.img, alt: g.name, style: { maxWidth: size + 'px', maxHeight: size + 'px' } }) : h('span', { style: { fontSize: size * .8 + 'px', lineHeight: 1 } }, g.emoji || '🎁');
const giftsUser = async ctx => {
  const uid = ctx.params.uid || me().id, reload = () => router.render();
  const d = await api.get('/api/gifts/user/' + uid, { page: ctx.page });
  ctx.title('Подарки ' + d.owner.nick);
  return [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { title: 'Подарки' }), ui.pageHead('Подарки ' + d.owner.nick, !d.mine ? ui.btn('Подарить', { ic: 'gift', kind: 'primary', small: true, href: '/user/gift/send/' + uid }) : null),
    ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', { style: { flexDirection: 'column', alignItems: 'center', textAlign: 'center' } },
      h('a', { href: '/user/gift/item/' + x.id }, giftPic(x.gift, 96)), h('div.small', x.gift.name, x.status === 0 ? h('span.badge.danger', { style: { marginLeft: '6px' } }, 'NEW') : null),
      h('div.small.muted', ui.userLink(x.from), ' · ', fmt.ago(x.time)),
      !d.mine || !x.can_reply ? null : ui.btn('Подарить в ответ', { ic: 'gift', small: true, href: '/user/gift/send/' + x.from.id }),
      x.status === 0 && d.mine ? h('div.row', ui.btn('Принять', { small: true, kind: 'primary', onclick: async () => { await api.post(`/api/gifts/${x.id}/accept`); reload(); } }), ui.btn('Отказаться', { small: true, onclick: async () => { await api.post(`/api/gifts/${x.id}/decline`); reload(); } })) : null))) : ui.empty('Нет подарков', 'gift')), ui.pager(d)];
};
router.add('/user/gift', giftsUser, { auth: true });
router.add('/user/gift/send/:uid', async ctx => {
  const d = await api.get('/api/gifts/catalog');
  const to = await api.get('/api/user/' + ctx.params.uid).catch(() => null);
  ctx.title('Выбор подарка');
  return [ui.pageHead('Подарок' + (to ? ' для ' + (to.user ? to.user.nick : to.nick) : '')), ui.card(h('div.list', d.items.map(k => h('a.list-item', { href: `/user/gift/send/${ctx.params.uid}/${k.id}` }, h('div.forum-ico', icon('gift', 22)), h('div.main', h('div.title', k.name)), h('span.badge', String(k.count)))))),
    d.admin ? h('div.card.pad', link('/admin/gifts', icon('settings', 16), ' Управление подарками')) : null];
}, { auth: true });
router.add('/user/gift/send/:uid/:cat', async ctx => {
  const { uid, cat } = ctx.params;
  const d = await api.get('/api/gifts/catalog/' + cat);
  ctx.title(d.category.name);
  return [crumbs({ href: '/user/gift/send/' + uid, title: 'Категории' }, { title: d.category.name }), ui.pageHead(d.category.name),
    d.items.length ? h('div.grid-gifts', d.items.map(g => h('a.card.pad.gift-tile', { href: `/user/gift/send/${uid}/${cat}/${g.id}` }, giftPic(g), h('div.title', g.name), h('div.small', { style: { color: 'var(--success,#16a34a)' } }, g.money, ' ', cur(d))))) : ui.card(ui.empty('Нет подарков', 'gift'))];
}, { auth: true });
router.add('/user/gift/send/:uid/:cat/:gift', async ctx => {
  const { uid, cat, gift } = ctx.params;
  const d = await api.get('/api/gifts/catalog/' + cat), m = await api.get('/api/money');
  const g = d.items.find(x => x.id == gift);
  if (!g) return ui.empty('Подарок не найден', 'gift');
  ctx.title('Подарок');
  const ed = ui.bbEditor({ rows: 3, maxlength: 150, placeholder: 'Комментарий (необязательно)' });
  return [crumbs({ href: `/user/gift/send/${uid}`, title: 'Категории' }, { href: `/user/gift/send/${uid}/${cat}`, title: d.category.name }, { title: g.name }), ui.pageHead('Отправка подарка'),
    h('div.card.pad.stack', h('div', { style: { textAlign: 'center' } }, giftPic(g, 120), h('div.title', g.name)), h('div', 'Цена: ', h('b', g.money, ' ', cur(d)), ' (у вас ', m.money, ')'), ed.el,
      h('div.row', ui.btn('Подарить', { ic: 'gift', kind: 'primary', onclick: async () => { await api.post('/api/gifts/send', { to: +uid, gift: g.id, msg: ed.value }); ui.ok('Ваш подарок успешно отправлен'); router.go('/user/gift/' + uid); } }), ui.btn('Отмена', { href: '/id' + uid })))];
}, { auth: true });
router.add('/user/gift/item/:id', async ctx => {
  const d = await api.get('/api/gifts/' + ctx.params.id), reload = () => router.render();
  ctx.title(d.gift.name);
  return [crumbs({ href: '/id' + d.to.id, title: d.to.nick }, { href: '/user/gift/' + d.to.id, title: 'Подарки' }, { title: d.gift.name }), ui.pageHead(d.gift.name),
    h('div.card.pad.stack', { style: { textAlign: 'center' } }, giftPic(d.gift, 140), h('div.small.muted', fmt.datetime(d.time)), h('div.row', { style: { justifyContent: 'center' } }, ui.avatar(d.from, 32), ui.userLink(d.from)), d.msg ? h('div.muted', { html: d.msg }) : null,
      d.mine && d.status === 0 ? h('div.row', { style: { justifyContent: 'center' } }, ui.btn('Принять', { kind: 'primary', onclick: async () => { await api.post(`/api/gifts/${d.id}/accept`); reload(); } }), ui.btn('Отказаться', { onclick: async () => { await api.post(`/api/gifts/${d.id}/decline`); router.go('/user/gift/' + d.to.id); } })) : null,
      d.can_delete && d.status === 1 ? ui.btn('Удалить подарок', { ic: 'trash', kind: 'danger', small: true, onclick: async () => { if (await ui.confirm('Удалить подарок?', { danger: true })) { await api.del('/api/gifts/' + d.id); router.go('/user/gift/' + d.to.id); } } }) : null)];
}, { auth: true });
router.add('/user/gift/:uid', giftsUser, { auth: true });

// управление каталогом подарков
router.add('/admin/gifts', async ctx => {
  ctx.title('Подарки'); const reload = () => router.render();
  const d = await api.get('/api/gifts/catalog');
  const name = h('input.input', { maxlength: 128, placeholder: 'Название категории' });
  return [crumbs({ href: '/admin', title: 'Панель управления' }, { title: 'Подарки' }), ui.pageHead('Категории подарков'),
    ui.card(d.items.length ? h('div.list', d.items.map(k => h('div.list-item', h('a.row.nowrap.grow', { href: '/admin/gifts/' + k.id }, h('div.forum-ico', icon('gift', 22)), h('div.main', h('div.title', k.name))), h('span.badge', String(k.count)),
      ui.btn('', { ic: 'edit', small: true, onclick: async () => { const v = await ui.prompt('Название категории', k.name, { title: 'Переименовать' }); if (v) { await api.put('/api/gifts/cat/' + k.id, { name: v }); reload(); } } }),
      ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm(`Удалить категорию «${k.name}» со всеми подарками?`, { danger: true })) { await api.del('/api/gifts/cat/' + k.id); ui.ok('Категория удалена'); reload(); } } })))) : ui.empty('Нет категорий', 'gift')),
    h('div.card.pad.row.nowrap', name, ui.btn('Добавить', { ic: 'plus', kind: 'primary', onclick: async () => { await api.post('/api/gifts/cat', { name: name.value }); ui.ok('Категория добавлена'); reload(); } }))];
}, { auth: true, access: 'adm_panel_show' });
router.add('/admin/gifts/:cat', async ctx => {
  const cat = ctx.params.cat, reload = () => router.render();
  const d = await api.get('/api/gifts/catalog/' + cat);
  ctx.title(d.category.name);
  const name = h('input.input', { maxlength: 128, placeholder: 'Название' }), money = h('input.input', { type: 'number', min: 1, placeholder: 'Цена', style: { width: '110px' } }), emoji = h('input.input', { maxlength: 8, placeholder: 'Эмодзи', style: { width: '110px' } }), file = h('input', { type: 'file', accept: 'image/*' });
  return [crumbs({ href: '/admin/gifts', title: 'Подарки' }, { title: d.category.name }), ui.pageHead(d.category.name),
    ui.card(d.items.length ? h('div.list', d.items.map(g => h('div.list-item', giftPic(g, 40), h('div.main', h('div.title', g.name), h('div.small.muted', g.money, ' ', cur(d))),
      ui.btn('', { ic: 'edit', small: true, onclick: async () => {
        const n = await ui.prompt('Название', g.name, { title: 'Редактирование подарка' }); if (!n) return;
        const p = await ui.prompt('Цена', String(g.money), { title: 'Редактирование подарка' }); if (!p) return;
        await api.put('/api/gifts/item/' + g.id, { name: n, money: +p, emoji: g.emoji, category: +cat }); ui.ok('Подарок отредактирован'); reload(); } }),
      ui.btn('', { ic: 'trash', small: true, kind: 'danger', onclick: async () => { if (await ui.confirm('Удалить подарок?', { danger: true })) { await api.del('/api/gifts/item/' + g.id); ui.ok('Подарок удалён'); reload(); } } })))) : ui.empty('Нет подарков', 'gift')),
    h('div.card.pad.stack', h('h3', 'Добавить подарок'), name, h('div.row', money, emoji), h('div.field', h('label', 'Картинка (png/gif, необязательно, если указано эмодзи)'), file),
      ui.btn('Добавить', { ic: 'plus', kind: 'primary', onclick: async () => { await api.upload('/api/gifts/item', { category: cat, name: name.value, money: money.value, emoji: emoji.value, gift: file.files[0] || '' }); ui.ok('Подарок добавлен'); reload(); } }))];
}, { auth: true, access: 'adm_panel_show' });

// ───────── музыка
router.add('/user/music/:uid', async ctx => {
  const uid = ctx.params.uid, reload = () => router.render();
  const d = await api.get('/api/music/' + uid, { page: ctx.page });
  ctx.title('Музыка ' + d.owner.nick);
  const audio = h('audio', { controls: true, style: { width: '100%' } });
  let idx = -1; const rows = [];
  const play = k => { if (k < 0 || k >= d.items.length) return; idx = k; audio.src = d.items[k].src; audio.play().catch(() => {}); rows.forEach((r, j) => r.classList.toggle('active', j === k)); };
  audio.onended = () => play(idx + 1);
  const list = d.items.map((t, k) => { const r = h('div.list-item', ui.btn('', { ic: 'play', small: true, onclick: () => play(k) }), h('div.main', h('a.title', { href: t.link }, t.name), h('div.small.muted', t.size_str, ' · ', t.ras)),
    h('a.btn.small', { href: t.download, title: 'Скачать' }, icon('download', 16)), d.mine ? ui.btn('', { ic: 'trash', small: true, onclick: async () => { await api.post('/api/music/toggle/' + t.file); ui.ok('Трек удалён из плейлиста'); reload(); } }) : null); rows.push(r); return r; });
  return [crumbs({ href: '/id' + d.owner.id, title: d.owner.nick }, { title: 'Музыка' }), ui.pageHead('Музыка ' + d.owner.nick), d.items.length ? h('div.card.pad', audio) : null,
    ui.card(d.items.length ? h('div.list', list) : ui.empty('Нет треков в плейлисте', 'music')), ui.pager(d)];
});

// ───────── знакомства
router.add('/user/love', async ctx => {
  ctx.title('Знакомства');
  const o = ctx.query.orders || 'all';
  const d = await api.get('/api/love', { page: ctx.page, orders: o });
  return [ui.pageHead('Знакомства'), h('div.chips', [['all', 'Все'], ['woman', 'Девушки'], ['man', 'Парни']].map(([v, l]) => h('a.chip' + (o === v ? '.active' : ''), { href: '/user/love?orders=' + v }, l))),
    d.lider ? h('div.card.pad.row', ui.avatar(d.lider.user, 44), h('div.main', h('div.row', icon('crown', 16), ui.userLink(d.lider.user)), h('div', { html: d.lider.msg }), link('/user/liders', 'Все лидеры (' + d.lider.total + ')'))) : null,
    ui.card(d.items.length ? h('div.list', d.items.map(x => h('div.list-item', ui.avatar(x.user, 44), h('div.main', ui.userLink(x.user), h('div.small', { html: x.msg }))))) : ui.empty('Список анкет пуст', 'heart')), ui.pager(d)];
});
document.head.append(h('style', '.grid-gifts{display:grid;grid-template-columns:repeat(auto-fill,minmax(130px,1fr));gap:10px}.gift-tile{text-align:center;display:flex;flex-direction:column;align-items:center;gap:6px}.list-item.active{background:var(--accent-soft,rgba(99,102,241,.12))}.badge.ok{background:#dcfce7;color:#166534}'));

import { h, api, ui, router, state, icon, fmt, bus, refreshMe } from '../core.js';

const me = () => state.user;

// ───────── Ежедневные задания и серии (Streak)
router.add('/quests', async ctx => {
  ctx.title('Ежедневные задания');
  const d = await api.get('/api/quests');
  const reload = () => router.render();

  const days = (d.rewards || []).map(r => {
    const cur = (d.streak % 7);
    const done = d.claimed_today ? r.day <= (cur === 0 && d.streak > 0 ? 7 : cur) : r.day <= cur;
    const isNext = !d.claimed_today && r.day === (cur + 1);
    return h('div.card.pad.center.stack-s' + (isNext ? '.quest-next' : done ? '.quest-done' : ''),
      h('div.small.muted', `День ${r.day}`),
      h('b', `+${r.balls} б.`),
      r.coins ? h('span.badge.primary', `+${r.coins} монет`) : null,
      done ? h('span.badge.ok', '✓') : null);
  });

  const checkinBtn = d.claimed_today
    ? h('span.badge.ok', icon('check', 14), ' Сегодняшняя награда получена')
    : ui.btn('Забрать ежедневную награду', {
      ic: 'award', kind: 'primary',
      onclick: async () => {
        const r = await api.post('/api/quests/checkin');
        ui.ok(`Получено +${r.balls} баллов${r.coins ? ` и +${r.coins} монет` : ''}! Серия: ${r.streak} дн.`);
        await refreshMe();
        reload();
      },
    });

  const qRows = (d.quests || []).map(q => h('div.list-item',
    icon('award', 20),
    h('div.main',
      h('div.title', q.title, h('span.badge', { style: { marginLeft: '6px' } }, `+${q.balls} б.`)),
      h('div.meta', q.desc)),
    q.claimed
      ? h('span.badge.ok', 'Выполнено')
      : ui.btn('Забрать', {
        small: true, kind: 'primary',
        onclick: async () => {
          await api.post('/api/quests/claim', { key: q.key });
          ui.ok(`+${q.balls} баллов!`);
          await refreshMe();
          reload();
        },
      })));

  return [
    ui.pageHead('Ежедневные задания', h('div.row',
      ui.btn('Мини-игры', { ic: 'dice', small: true, href: '/games' }),
      ui.btn('Магазин', { ic: 'shopping-bag', small: true, href: '/shop' }))),
    h('div.card.pad.stack',
      h('div.row',
        h('div.grow',
          h('h3', { style: { margin: 0 } }, `Серия входов: ${d.streak} дн.`),
          h('div.small.muted', d.is_vip ? 'VIP активен: +50% к баллам за вход!' : 'Заходите каждый день, чтобы дойти до награды 7-го дня (+5 монет).')),
        checkinBtn),
      h('div.quest-grid', ...days)),
    ui.section('Задания на сегодня', h('div.list', ...qRows)),
  ];
}, { auth: true });

// ───────── Мини-игры и дуэли
const RPS_MAP = { rock: '🪨 Камень', paper: '📄 Бумага', scissors: '✂️ Ножницы' };

router.add('/games', async ctx => {
  ctx.title('Мини-игры и дуэли');
  const d = await api.get('/api/games/duels');
  const reload = () => router.render();

  const onUpd = bus.on('sse:duel_update', () => reload());
  ctx.onLeave(() => onUpd && onUpd());

  const openCreate = () => {
    const gameSel = h('select.input',
      h('option', { value: 'rps' }, 'Камень-ножницы-бумага'),
      h('option', { value: 'ttt' }, 'Крестики-нолики'));
    const betIn = h('input.input', { type: 'number', min: 0, max: 500, value: 10 });
    const oppIn = h('input.input', { placeholder: 'Оставьте пустым для открытой дуэли' });
    const moveSel = h('select.input',
      h('option', { value: 'rock' }, '🪨 Камень'),
      h('option', { value: 'scissors' }, '✂️ Ножницы'),
      h('option', { value: 'paper' }, '📄 Бумага'));
    const moveField = h('div.field', h('label', 'Ваш ход'), moveSel);
    gameSel.onchange = () => { moveField.style.display = gameSel.value === 'rps' ? '' : 'none'; };

    let modal;
    modal = ui.modal({
      title: 'Создать дуэль',
      body: h('div.stack',
        h('div.field', h('label', 'Игра'), gameSel),
        h('div.field', h('label', 'Ставка (баллы, 0–500)'), betIn),
        h('div.field', h('label', 'Вызвать игрока по логину'), oppIn),
        moveField),
      actions: [
        { label: 'Отмена' },
        {
          label: 'Создать', kind: 'primary',
          onclick: async () => {
            await api.post('/api/games/duels', {
              game: gameSel.value, bet: +betIn.value || 0, opponent_nick: oppIn.value.trim(), move: moveSel.value,
            });
            modal.close();
            ui.ok('Дуэль создана!');
            reload();
            return false;
          },
        },
      ],
    });
  };

  const renderDuel = g => {
    const isMine = me() && (g.p1.id === me().id || (g.p2 && g.p2.id === me().id));
    const title = g.game === 'rps' ? 'Камень-ножницы-бумага' : 'Крестики-нолики';
    const header = h('div.row',
      h('b', title),
      g.bet ? h('span.badge.primary', `Ставка: ${g.bet} б.`) : h('span.badge', 'Без ставки'),
      h('span.grow'),
      h('span.small.muted', fmt.ago(g.created)));

    const vs = h('div.row', ui.userLink(g.p1, { avatar: true }), h('span.muted', 'vs'),
      g.p2 ? ui.userLink(g.p2, { avatar: true }) : h('span.muted', 'Ожидание соперника…'));

    let body = null;
    if (g.status === 'open' && me() && g.p1.id !== me().id) {
      if (g.game === 'rps') {
        body = h('div.row', ...Object.entries(RPS_MAP).map(([mv, lbl]) => ui.btn(lbl, {
          small: true, kind: 'primary',
          onclick: async () => { await api.post(`/api/games/duels/${g.id}/join`, { move: mv }); ui.ok('Ход сделан!'); reload(); },
        })));
      } else {
        body = ui.btn('Принять вызов', {
          ic: 'dice', small: true, kind: 'primary',
          onclick: async () => { await api.post(`/api/games/duels/${g.id}/join`, {}); reload(); },
        });
      }
    } else if (g.status === 'playing' && g.game === 'ttt') {
      const board = (g.state && g.state.board) || Array(9).fill(0);
      const myTurn = me() && g.turn === me().id;
      body = h('div.stack-s',
        h('div.small', myTurn ? h('b.ok', 'Ваш ход!') : h('span.muted', 'Ход соперника…')),
        h('div.ttt-board', ...board.map((cell, idx) => h('button.ttt-cell', {
          type: 'button',
          disabled: !myTurn || cell !== 0,
          onclick: async () => { await api.post(`/api/games/duels/${g.id}/move`, { cell: idx }); reload(); },
        }, cell === 1 ? '✕' : cell === 2 ? '◯' : ''))));
    } else if (g.status === 'done') {
      const resText = g.winner ? `Победитель: ${g.winner === g.p1.id ? g.p1.nick : (g.p2 ? g.p2.nick : '')}` : 'Ничья!';
      const rpsInfo = g.game === 'rps' && g.state ? ` (${RPS_MAP[g.state.m1] || ''} vs ${RPS_MAP[g.state.m2] || ''})` : '';
      body = h('div.small', h('b.ok', resText), rpsInfo);
    } else if (isMine) {
      body = h('div.small.muted', 'Ожидаем соперника…');
    }

    return h('div.card.pad.stack-s', header, vs, body);
  };

  return [
    ui.pageHead('Мини-игры и дуэли', h('div.row',
      ui.btn('Задания', { ic: 'award', small: true, href: '/quests' }),
      me() ? ui.btn('Создать дуэль', { ic: 'plus', kind: 'primary', small: true, onclick: openCreate }) : null)),
    (d.mine && d.mine.length) ? ui.section('Мои активные дуэли', h('div.stack', ...d.mine.map(renderDuel))) : null,
    ui.section('Открытые дуэли', (d.open && d.open.length) ? h('div.stack', ...d.open.map(renderDuel)) : ui.empty('Открытых дуэлей пока нет — создайте первую!', 'dice')),
    (d.recent && d.recent.length) ? ui.section('Последние матчи', h('div.stack', ...d.recent.map(renderDuel))) : null,
    (d.leaderboard && d.leaderboard.length) ? ui.section('Таблица лидеров', h('div.list', ...d.leaderboard.map((r, idx) => h('div.list-item',
      h('b', `#${idx + 1}`), ui.avatar(r.user, 32), h('div.main', ui.userLink(r.user)), h('span.badge.primary', `${r.wins} побед`))))) : null,
  ];
});

// ───────── Магазин оформления (рамки аватаров)
router.add('/shop', async ctx => {
  ctx.title('Магазин оформления');
  const d = await api.get('/api/shop');
  const reload = async () => { await refreshMe(); router.render(); };

  const cards = (d.items || []).map(it => {
    const previewRef = me() ? { ...me(), avatar_frame: it.class } : { nick: 'A', pol: 1, avatar_frame: it.class };
    let actionBtn;
    if (!me()) {
      actionBtn = ui.btn('Войти', { small: true, href: '/login' });
    } else if (it.equipped) {
      actionBtn = ui.btn('Снять рамку', {
        small: true,
        onclick: async () => { await api.post('/api/shop/equip', { item_id: '' }); ui.ok('Рамка снята'); reload(); },
      });
    } else if (it.owned) {
      actionBtn = ui.btn('Надеть', {
        small: true, kind: 'primary',
        onclick: async () => { await api.post('/api/shop/equip', { item_id: it.id }); ui.ok('Рамка надета!'); reload(); },
      });
    } else {
      actionBtn = ui.btn(`Купить за ${it.price} мон.`, {
        small: true, kind: 'primary',
        onclick: async () => {
          if (await ui.confirm(`Купить «${it.name}» за ${it.price} монет?`)) {
            await api.post('/api/shop/buy', { item_id: it.id });
            ui.ok('Покупка успешна! Рамка надета.');
            reload();
          }
        },
      });
    }

    return h('div.card.pad.stack-s.center',
      h('div', { style: { padding: '8px 0' } }, ui.avatar(previewRef, 68)),
      h('b', it.name),
      h('div.small.muted', it.desc),
      d.is_vip && !it.owned ? h('div.small', h('s.muted', `${it.base_price} `), h('b.ok', `${it.price} монет (-30% VIP)`)) : null,
      actionBtn);
  });

  return [
    ui.pageHead('Магазин оформления', me() ? h('span.badge.primary', `Баланс: ${fmt.num(d.money)} монет`) : null),
    d.is_vip ? h('div.card.pad.small', '👑 Для вас действует ', h('b', 'VIP-скидка 30%'), ' на все рамки аватара!') : null,
    h('div.shop-grid', ...cards),
  ];
});

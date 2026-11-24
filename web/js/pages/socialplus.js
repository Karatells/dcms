import { h, api, ui, router, state, icon, link, fmt } from '../core.js';

const REACTION_EMOJIS = ['🔥', '❤️', '😂', '👍', '😮'];

// ───────── универсальная панель эмодзи-реакций
export function emojiReactionsBar(type, id, initial = []) {
  let list = Array.isArray(initial) ? initial : [];
  const wrap = h('div.react-bar');
  const draw = () => {
    const byE = new Map(list.map(x => [x.emoji, x]));
    wrap.replaceChildren(...REACTION_EMOJIS.map(em => {
      const it = byE.get(em);
      const cnt = it ? it.count : 0;
      const mine = !!(it && it.mine);
      return h('button.react-pill' + (mine ? '.mine' : ''), {
        type: 'button',
        title: state.user ? 'Поставить реакцию ' + em : 'Войдите, чтобы поставить реакцию',
        onclick: async e => {
          e.stopPropagation();
          if (!state.user) { router.go('/login'); return; }
          const r = await api.post('/api/reactions/toggle', { type, id, emoji: em }, { silent: true });
          list = r.reactions || [];
          draw();
        },
      }, em, cnt > 0 ? h('span', String(cnt)) : null);
    }));
  };
  draw();
  return wrap;
}
ui.emojiReactions = emojiReactionsBar;

// ───────── глобальное автодополнение @ник в любых textarea
let mentionPop = null;
let mentionTimer = null;
const closeMention = () => { if (mentionPop) { mentionPop.remove(); mentionPop = null; } };
document.addEventListener('click', e => { if (mentionPop && !mentionPop.contains(e.target)) closeMention(); });
document.addEventListener('input', e => {
  const ta = e.target;
  if (!ta || ta.tagName !== 'TEXTAREA') return;
  const pos = ta.selectionStart || 0;
  const before = ta.value.slice(0, pos);
  const m = before.match(/(?:^|\s)@([A-Za-zА-Яа-яЁё0-9_\-\.]{2,24})$/);
  if (!m) { closeMention(); return; }
  const q = m[1];
  clearTimeout(mentionTimer);
  mentionTimer = setTimeout(async () => {
    try {
      const r = await api.get('/api/users/mention', { q }, { silent: true });
      const users = r.items || [];
      if (!users.length) { closeMention(); return; }
      closeMention();
      const rect = ta.getBoundingClientRect();
      mentionPop = h('div.mention-pop', {
        style: { left: Math.max(8, Math.min(window.innerWidth - 230, rect.left)) + 'px', top: Math.max(8, rect.top - Math.min(220, users.length * 42 + 12)) + 'px' },
      }, ...users.map(u => h('button.mention-item', {
        type: 'button',
        onclick: () => {
          const start = pos - q.length - 1;
          const ins = '@' + u.nick + ' ';
          ta.value = ta.value.slice(0, start) + ins + ta.value.slice(pos);
          ta.focus();
          ta.selectionStart = ta.selectionEnd = start + ins.length;
          closeMention();
        },
      }, ui.avatar(u, 24), h('span', u.nick))));
      document.body.append(mentionPop);
    } catch { closeMention(); }
  }, 180);
});

// ───────── Умная главная лента (/feed)
router.add('/feed', async ctx => {
  const tab = ctx.query.tab || (state.user ? 'following' : 'popular');
  ctx.title('Умная лента');
  const d = await api.get('/api/smart-feed', { tab, page: ctx.page });
  const tabs = h('div.tabs',
    state.user ? h('a.tab' + (tab === 'following' ? '.active' : ''), { href: '/feed?tab=following' }, icon('users', 15), 'Подписки') : null,
    h('a.tab' + (tab === 'popular' ? '.active' : ''), { href: '/feed?tab=popular' }, icon('trending', 15), 'Популярное'));

  let quickBox = null;
  if (state.user) {
    const ta = h('textarea.input', { rows: 2, placeholder: 'Что нового? Упомяните друзей через @ник…', maxlength: 1024 });
    quickBox = h('div.card.pad.stack',
      ta,
      h('div.row',
        h('span.small.muted', 'Опубликуется на вашей стене и попадёт в ленту'),
        h('span.grow'),
        ui.btn('Опубликовать', {
          ic: 'send', kind: 'primary', small: true,
          onclick: async () => {
            const msg = ta.value.trim();
            if (msg.length < 2) return ui.toast('Напишите хотя бы пару слов', 'warn');
            await api.post(`/api/user/${state.user.id}/stena`, { msg });
            ta.value = '';
            ui.ok('Опубликовано!');
            router.render();
          },
        })));
  }

  const cards = (d.items || []).map(it => h('article.card.pad.stack',
    h('div.row',
      ui.avatar(it.user, 38),
      h('div.grow',
        h('div', ui.userLink(it.user, { badges: true }),
          it.wall_id !== it.user.id ? h('span.small.muted', ' → ', ui.userLink(it.wall_user)) : null),
        h('div.small.muted', fmt.ago(it.time))),
      link(it.link, 'Открыть →', 'small muted')),
    ui.html(it.html, 'text'),
    emojiReactionsBar(it.type, it.id, it.reactions || [])));

  return [
    ui.pageHead('Умная лента', h('div.row',
      ui.btn('Клипы', { ic: 'video', small: true, href: '/shorts' }),
      ui.btn('Задания', { ic: 'award', small: true, href: '/quests' }))),
    tabs,
    quickBox,
    cards.length ? h('div.stack', ...cards) : ui.empty(tab === 'following' ? 'В подписках пока тихо — загляните в «Популярное»!' : 'Записей пока нет', 'flame'),
    ui.pager(d),
  ];
});

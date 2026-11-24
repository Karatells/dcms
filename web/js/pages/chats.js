import { h, api, ui, router, state, icon, fmt, bus } from '../core.js';

const me = () => state.user;

// ───────── список групповых бесед
router.add('/chats', async ctx => {
  ctx.title('Групповые беседы');
  const d = await api.get('/api/chats');
  const items = d.items || [];

  const openCreate = async () => {
    const titleIn = h('input.input', { placeholder: 'Название беседы', maxlength: 64 });
    const friendsBox = h('div.stack-s', ui.loading());
    const chosen = new Set();
    let modal;
    modal = ui.modal({
      title: 'Создать групповую беседу',
      body: h('div.stack',
        h('div.field', h('label', 'Название'), titleIn),
        h('div.field', h('label', 'Пригласить друзей (необязательно)'), friendsBox)),
      actions: [
        { label: 'Отмена' },
        {
          label: 'Создать', kind: 'primary',
          onclick: async () => {
            const title = titleIn.value.trim();
            if (title.length < 2) { ui.toast('Введите название (от 2 символов)', 'warn'); return false; }
            const r = await api.post('/api/chats', { title, member_ids: [...chosen] });
            modal.close();
            ui.ok('Беседа создана');
            router.go('/chats/' + r.id);
            return false;
          },
        },
      ],
    });
    try {
      const f = await api.get(`/api/user/${me().id}/frends`, { page: 1 }, { silent: true });
      const list = f.items || [];
      if (!list.length) { friendsBox.replaceChildren(h('div.small.muted', 'Список друзей пуст — друзей можно будет пригласить по ссылке позже.')); return; }
      friendsBox.replaceChildren(h('div.list', { style: { maxHeight: '220px', overflowY: 'auto' } }, ...list.map(x => {
        const chk = h('input', { type: 'checkbox', onchange: () => { if (chk.checked) chosen.add(x.user.id); else chosen.delete(x.user.id); } });
        return h('label.list-item', { style: { cursor: 'pointer' } }, chk, ui.avatar(x.user, 32), h('div.main', h('div.title', x.user.nick)));
      })));
    } catch { friendsBox.replaceChildren(h('div.small.muted', 'Пригласить участников можно будет по ссылке.')); }
  };

  const rows = items.map(c => h('a.list-item', { href: '/chats/' + c.id },
    h('div.grp-ava-sm', { style: { width: '44px', height: '44px', borderRadius: '12px', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#06b6d4)', color: '#fff', fontWeight: '800', flexShrink: '0' } },
      Array.from(c.title || 'Б')[0].toUpperCase()),
    h('div.main',
      h('div.title', c.title, h('span.badge', { style: { marginLeft: '6px' } }, `${c.members} уч.`)),
      c.last_text ? h('div.small.muted.clip', c.last_nick ? `${c.last_nick}: ${c.last_text}` : c.last_text) : h('div.small.muted', 'Сообщений пока нет')),
    c.last_time ? ui.time(c.last_time) : null));

  return [
    ui.pageHead('Групповые беседы', h('div.row',
      ui.btn('К почте', { ic: 'mail', small: true, href: '/mail' }),
      ui.btn('Создать беседу', { ic: 'plus', kind: 'primary', small: true, onclick: openCreate }))),
    ui.card(rows.length ? h('div.list', rows) : ui.empty('У вас пока нет групповых бесед', 'users',
      ui.btn('Создать первую беседу', { ic: 'plus', kind: 'primary', onclick: openCreate }))),
  ];
}, { auth: true });

// ───────── вступление по инвайт-ссылке
router.add('/chats/join/:code', async ctx => {
  ctx.title('Приглашение в беседу');
  const r = await api.post('/api/chats/join/' + encodeURIComponent(ctx.params.code));
  ui.ok('Вы присоединились к беседе!');
  router.go('/chats/' + r.id, true);
}, { auth: true });

// ───────── окно групповой беседы
router.add('/chats/:id', async ctx => {
  const cid = +ctx.params.id;
  const d = await api.get('/api/chats/' + cid, { page: 1 });
  ctx.title(d.title);
  document.body.classList.add('msg-page');

  let curPage = 1;
  const totalPages = d.pages || 1;
  let items = [...(d.items || [])].reverse();
  let pinned = d.pinned || null;
  let replyTo = null;
  const isAdm = d.my_role === 'owner' || d.my_role === 'admin';
  const isOwner = d.my_role === 'owner';

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
  const chipHost = h('div');
  const reload = () => router.render();

  const renderPinned = () => {
    if (!pinned) { pinnedHost.replaceChildren(); return; }
    pinnedHost.replaceChildren(h('div.msg-pinned',
      icon('pin', 16),
      h('div.msg-pinned-body',
        h('div.msg-pinned-title', `Закреплено · ${pinned.nick}`),
        h('div.msg-pinned-text', pinned.text || 'Сообщение')),
      isAdm ? h('button.icon-btn', {
        type: 'button', title: 'Открепить',
        onclick: async () => { await api.post(`/api/chats/${cid}/pin`, { msg_id: pinned.id }); pinned = null; renderPinned(); },
      }, icon('x', 16)) : null));
  };

  const renderChips = () => {
    if (!replyTo) { chipHost.replaceChildren(); return; }
    chipHost.replaceChildren(h('div.msg-bar-chip',
      icon('corner-up-left', 16),
      h('div.chip-main', h('div.chip-title', `Ответ · ${replyTo.nick}`), h('div.chip-text', replyTo.text)),
      h('button.icon-btn', { type: 'button', onclick: () => { replyTo = null; renderChips(); } }, icon('x', 16))));
  };

  const renderMessages = (keepBottom = true) => {
    const nodes = [];
    if (curPage < totalPages) {
      nodes.push(ui.btn('Загрузить предыдущие', {
        small: true, cls: 'msg-older',
        onclick: async () => {
          const next = await api.get('/api/chats/' + cid, { page: curPage + 1 });
          curPage++;
          items = [...(next.items || []).reverse(), ...items];
          renderMessages(false);
        },
      }));
    }
    if (!items.length) {
      nodes.push(ui.empty('Напишите первое сообщение в беседу!', 'message-circle'));
    } else {
      items.forEach(m => {
        const bubble = h('div.bubble' + (m.mine ? '.mine' : ''),
          !m.mine ? h('div.small', { style: { fontWeight: '700', marginBottom: '2px', opacity: '.9' } }, ui.userLink(m.user)) : null,
          m.reply ? h('div.bubble-reply', h('b', m.reply.nick), h('span', m.reply.text)) : null,
          ui.html(m.msg, 'text'),
          h('div.bubble-meta',
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })),
            h('button.icon-btn', {
              type: 'button', title: 'Ответить', style: { width: '22px', height: '22px', opacity: '.75' },
              onclick: e => { e.stopPropagation(); replyTo = { id: m.id, nick: m.user.nick, text: (m.raw || '').slice(0, 70) }; renderChips(); },
            }, icon('corner-up-left', 12)),
            isAdm ? h('button.icon-btn', {
              type: 'button', title: 'Закрепить', style: { width: '22px', height: '22px', opacity: '.75' },
              onclick: async e => {
                e.stopPropagation();
                const r = await api.post(`/api/chats/${cid}/pin`, { msg_id: m.id });
                pinned = r.pinned_msg_id ? { id: m.id, nick: m.user.nick, text: (m.raw || '').slice(0, 80) } : null;
                renderPinned();
              },
            }, icon('pin', 12)) : null,
            (m.mine || isAdm) ? h('button.icon-btn', {
              type: 'button', title: 'Удалить', style: { width: '22px', height: '22px', opacity: '.75' },
              onclick: async e => {
                e.stopPropagation();
                await api.del('/api/chats/msg/' + m.id);
                items = items.filter(x => x.id !== m.id);
                renderMessages(false);
              },
            }, icon('trash', 12)) : null));
        nodes.push(h('div.bubble-row' + (m.mine ? '.mine' : ''),
          !m.mine ? ui.avatar(m.user, 30) : null,
          bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
  };

  const openMembersModal = () => {
    const inviteURL = `${location.origin}/chats/join/${d.invite_code}`;
    const addIn = h('input.input', { placeholder: 'Логин пользователя' });
    const body = h('div.stack',
      h('div.field', h('label', 'Ссылка-приглашение'),
        h('div.row.nowrap',
          h('input.input', { value: inviteURL, readOnly: true, onclick: e => e.target.select() }),
          ui.btn('Копировать', { ic: 'copy', small: true, onclick: () => navigator.clipboard?.writeText(inviteURL).then(() => ui.ok('Ссылка скопирована')) }))),
      isAdm ? h('div.field', h('label', 'Добавить участника'),
        h('div.row.nowrap', addIn, ui.btn('Добавить', {
          ic: 'user-plus', kind: 'primary', small: true,
          onclick: async () => {
            await api.post(`/api/chats/${cid}/members`, { nick: addIn.value.trim() });
            ui.ok('Участник добавлен');
            modal.close(); reload();
          },
        }))) : null,
      h('div.list', ...(d.members || []).map(mem => h('div.list-item',
        ui.avatar(mem.user, 34),
        h('div.main',
          h('div.title', ui.userLink(mem.user), mem.role !== 'member' ? h('span.badge.primary', { style: { marginLeft: '6px' } }, mem.role === 'owner' ? 'Создатель' : 'Админ') : null)),
        isOwner && mem.role !== 'owner' ? ui.btn(mem.role === 'admin' ? 'Снять' : 'Админ', {
          small: true,
          onclick: async () => {
            await api.post(`/api/chats/${cid}/role`, { user_id: mem.user.id, role: mem.role === 'admin' ? 'member' : 'admin' });
            ui.ok('Роль обновлена'); modal.close(); reload();
          },
        }) : null,
        (isAdm && mem.role !== 'owner' && mem.user.id !== me().id) ? ui.btn('', {
          ic: 'x', small: true, kind: 'danger', title: 'Исключить',
          onclick: async () => {
            await api.del(`/api/chats/${cid}/members/${mem.user.id}`);
            ui.ok('Участник исключён'); modal.close(); reload();
          },
        }) : null))));
    const modal = ui.modal({
      title: `Участники (${(d.members || []).length})`,
      body,
      actions: [
        {
          label: 'Выйти из беседы', kind: 'danger',
          onclick: async () => {
            if (await ui.confirm('Выйти из беседы?', { danger: true })) {
              await api.del(`/api/chats/${cid}/members/${me().id}`);
              modal.close(); router.go('/chats');
            }
          },
        },
        { label: 'Закрыть' },
      ],
    });
  };

  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/chats', title: 'К списку бесед' }, icon('arrow-left', 20)),
    h('div.msg-head-info',
      h('b', d.title),
      h('div.msg-status', `${(d.members || []).length} участников`)),
    ui.btn('Участники', { ic: 'users', small: true, onclick: openMembersModal }));

  const ta = h('textarea.msg-input', { rows: 1, placeholder: 'Сообщение… (можно @ник)', maxlength: 1024 });
  const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить' }, icon('send', 18));
  const doSend = async () => {
    const msg = ta.value.trim();
    if (!msg) return;
    sendBtn.disabled = true;
    try {
      await api.post(`/api/chats/${cid}/msg`, { msg, reply_id: replyTo ? replyTo.id : 0 });
      ta.value = ''; replyTo = null; renderChips();
    } finally { sendBtn.disabled = false; }
  };
  sendBtn.onclick = doSend;
  ta.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) { e.preventDefault(); doSend(); } };
  const foot = h('div.msg-foot', chipHost, h('div.msg-compose', h('div.msg-input-wrap', ta), sendBtn));

  const onMsg = bus.on('sse:gchat_msg', m => {
    if (m && m.chat === cid && !items.some(x => x.id === m.id)) {
      items.push(m);
      renderMessages(true);
    }
  });
  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    onMsg && onMsg();
  });

  renderPinned();
  renderMessages(true);
  return h('div.msg-shell', head, pinnedHost, scrollEl, foot);
}, { auth: true });

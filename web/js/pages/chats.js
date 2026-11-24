import { h, api, ui, router, state, icon, fmt, bus } from '../core.js';
import { getSmiles, dayLabel, openAttachPicker, renderBubbleAttach, renderVoicePlayer, openForwardModal, openMsgMenu } from './feeds.js';

const me = () => state.user;

// ───────── модалка создания беседы (доступна и из /mail, и из /chats)
export async function openCreateChatModal() {
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
}

// ───────── список групповых бесед (редирект/просмотр)
router.add('/chats', async ctx => {
  ctx.title('Групповые беседы');
  const d = await api.get('/api/chats');
  const items = d.items || [];
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
      ui.btn('Создать беседу', { ic: 'plus', kind: 'primary', small: true, onclick: openCreateChatModal }))),
    ui.card(rows.length ? h('div.list', rows) : ui.empty('У вас пока нет групповых бесед', 'users',
      ui.btn('Создать первую беседу', { ic: 'plus', kind: 'primary', onclick: openCreateChatModal }))),
  ];
}, { auth: true });

// ───────── вступление по инвайт-ссылке
router.add('/chats/join/:code', async ctx => {
  ctx.title('Приглашение в беседу');
  const r = await api.post('/api/chat-join/' + encodeURIComponent(ctx.params.code));
  ui.ok('Вы присоединились к беседе!');
  router.go('/chats/' + r.id, true);
}, { auth: true });

// ───────── окно групповой беседы (100% паритет с личной перепиской /mail/:id)
router.add('/chats/:id', async ctx => {
  const cid = +ctx.params.id;
  const d = await api.get('/api/chats/' + cid, { page: 1 });
  ctx.title(d.title);
  document.body.classList.add('msg-page');

  let curPage = 1;
  const totalPages = d.pages || 1;
  let items = [...(d.items || [])].reverse();
  const seenIds = new Set(items.map(x => x.id));
  let pinnedMsg = d.pinned || null;
  let replyTo = null;
  let editMsg = null;
  let attach = null;
  let inputTa = null;
  let myMutedUntil = d.my_muted_until || 0;
  let denyBots = !!d.deny_bots;
  const isAdm = d.my_role === 'owner' || d.my_role === 'admin';
  const isOwner = d.my_role === 'owner';

  const scrollEl = h('div.msg-scroll');
  const pinnedHost = h('div');
  const searchHost = h('div');
  const chipHost = h('div');
  const muteBannerHost = h('div');
  const reload = () => router.render();

  const renderMuteBanner = () => {
    const now = Math.floor(Date.now() / 1000);
    if (myMutedUntil > now) {
      const rem = Math.max(1, Math.ceil((myMutedUntil - now) / 60));
      muteBannerHost.replaceChildren(h('div.pad.small.err.center', { style: { background: 'color-mix(in srgb,var(--danger) 10%,var(--bg-card))', borderTop: '1px solid var(--border-s)' } },
        `Вам временно запрещено писать в этой беседе (осталось ~${rem} мин.)`));
    } else {
      muteBannerHost.replaceChildren();
    }
  };

  const upsertMsg = m => {
    if (!m || !m.id) return false;
    const idx = items.findIndex(x => x.id === m.id);
    if (idx >= 0) { items[idx] = m; return false; }
    items.push(m);
    return true;
  };

  const jumpToMsg = id => {
    const el = scrollEl.querySelector(`.bubble[data-mid="${id}"]`);
    if (!el) return;
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.classList.add('flash');
    setTimeout(() => el.classList.remove('flash'), 1200);
  };

  const renderPinned = () => {
    if (!pinnedMsg) { pinnedHost.replaceChildren(); return; }
    pinnedHost.replaceChildren(h('div.msg-pinned', { onclick: () => jumpToMsg(pinnedMsg.id) },
      icon('pin', 16),
      h('div.msg-pinned-body',
        h('div.msg-pinned-title', `Закреплено · ${pinnedMsg.nick}`),
        h('div.msg-pinned-text', pinnedMsg.text || 'Сообщение')),
      isAdm ? h('button.icon-btn', {
        type: 'button', title: 'Открепить',
        onclick: async e => {
          e.stopPropagation();
          await api.post(`/api/chats/${cid}/pin`, { msg_id: pinnedMsg.id });
          pinnedMsg = null; items.forEach(x => { x.pinned = false; });
          renderPinned(); renderMessages(false);
        },
      }, icon('x', 16)) : null));
  };

  let syncTa = () => {};
  const renderChips = () => {
    const nodes = [];
    if (editMsg) {
      nodes.push(h('div.msg-bar-chip',
        icon('edit', 16),
        h('div.chip-main', h('div.chip-title', 'Редактирование сообщения'), h('div.chip-text', editMsg.raw)),
        h('button.icon-btn', { type: 'button', onclick: () => { editMsg = null; if (inputTa) inputTa.value = ''; renderChips(); syncTa(); } }, icon('x', 16))));
    }
    if (replyTo) {
      nodes.push(h('div.msg-bar-chip',
        icon('corner-up-left', 16),
        h('div.chip-main', h('div.chip-title', `Ответ · ${replyTo.nick}`), h('div.chip-text', replyTo.text)),
        h('button.icon-btn', { type: 'button', onclick: () => { replyTo = null; renderChips(); } }, icon('x', 16))));
    }
    if (attach) {
      const lbl = { foto: 'Фотография', files: 'Личный файл', shorts: 'Клип' }[attach.type] || 'Вложение';
      nodes.push(h('div.msg-bar-chip',
        attach.image ? h('img', { src: attach.image, alt: '' }) : icon('paperclip', 16),
        h('div.chip-main', h('div.chip-title', lbl), h('div.chip-text', attach.title)),
        h('button.icon-btn', { type: 'button', onclick: () => { attach = null; renderChips(); syncTa(); } }, icon('x', 16))));
    }
    chipHost.replaceChildren(...nodes);
  };

  const renderMessages = (keepBottom = true) => {
    const prevH = scrollEl.scrollHeight;
    const prevTop = scrollEl.scrollTop;
    const nodes = [];
    if (curPage < totalPages) {
      nodes.push(ui.btn('Загрузить предыдущие сообщения', {
        small: true, cls: 'msg-older',
        onclick: async () => {
          const next = await api.get('/api/chats/' + cid, { page: curPage + 1 });
          curPage++;
          const older = (next.items || []).reverse();
          older.forEach(x => seenIds.add(x.id));
          items = [...older, ...items];
          renderMessages(false);
        },
      }));
    }
    if (!items.length) {
      nodes.push(ui.empty('Сообщений пока нет — напишите первым!', 'message-circle'));
    } else {
      let lastDay = '';
      items.forEach(m => {
        const dl = dayLabel(m.time);
        if (dl !== lastDay) { lastDay = dl; nodes.push(h('div.msg-day', dl)); }
        if (m.system) {
          seenIds.add(m.id);
          nodes.push(h('div.msg-sys', m.raw || ''));
          return;
        }
        const doReact = async em => {
          const r = await api.post(`/api/chats/msg/${m.id}/react`, { emoji: em }, { silent: true });
          m.reactions = r.reactions || [];
          renderMessages(false);
        };
        const previewTxt = (m.raw || '').slice(0, 80) || (m.voice ? '🎤 Голосовое' : m.sticker ? m.sticker + ' Стикер' : (m.attach ? m.attach.title : 'Сообщение'));
        const openMenu = e => {
          if (e.target.closest('a, .bubble-reply, .bubble-react-pill, .voice-msg')) return;
          e.preventDefault();
          openMsgMenu(m, {
            onReact: doReact,
            onReply: () => { editMsg = null; replyTo = { id: m.id, nick: m.user.nick, text: previewTxt }; renderChips(); inputTa && inputTa.focus(); },
            onEdit: m.mine ? () => { replyTo = null; attach = null; editMsg = { id: m.id, raw: m.raw }; renderChips(); if (inputTa) { inputTa.value = m.raw; syncTa(); inputTa.focus(); } } : null,
            onForward: () => openForwardModal(m, m.user),
            onPin: isAdm ? async () => {
              const r = await api.post(`/api/chats/${cid}/pin`, { msg_id: m.id });
              items.forEach(x => { x.pinned = false; });
              m.pinned = !!r.pinned_msg_id;
              pinnedMsg = m.pinned ? { id: m.id, nick: m.user.nick, text: previewTxt } : null;
              renderPinned(); renderMessages(false);
            } : () => ui.toast('Закреплять могут только администраторы беседы', 'warn'),
            onSpam: () => ui.ok('Жалоба передана администраторам беседы'),
            onDelete: (m.mine || isAdm) ? async () => {
              await api.del('/api/chats/msg/' + m.id);
              items = items.filter(x => x.id !== m.id);
              if (pinnedMsg && pinnedMsg.id === m.id) { pinnedMsg = null; renderPinned(); }
              renderMessages(false);
            } : () => ui.toast('Можно удалять только свои сообщения', 'warn'),
          });
        };
        const reactPills = m.reactions && m.reactions.length
          ? h('div.bubble-reacts', ...m.reactions.map(rx => h('button.bubble-react-pill' + (rx.mine ? '.mine' : ''), {
            type: 'button',
            onclick: e => { e.stopPropagation(); doReact(rx.emoji); },
          }, rx.emoji, rx.count > 1 ? h('span', String(rx.count)) : null)))
          : null;
        const senderHeader = !m.mine
          ? h('div.small', { style: { fontWeight: '700', marginBottom: '2px', opacity: '.92', display: 'flex', alignItems: 'center', gap: '6px' } },
            m.bot ? h('a', { href: '/groups/' + m.bot.id, style: { color: 'inherit' } }, m.bot.name) : ui.userLink(m.user),
            m.bot ? h('span.badge.primary', { style: { fontSize: '.66rem', padding: '1px 5px' } }, 'БОТ') : null)
          : null;
        const bubble = h('div.bubble' + (m.mine ? '.mine' : '') + (m.sticker ? '.is-sticker' : ''), {
          dataset: { mid: String(m.id) },
          onclick: openMenu,
          oncontextmenu: openMenu,
        },
          senderHeader,
          m.fwd ? h('div.bubble-fwd', icon('share', 12), 'Переслано от ', m.fwd.nick) : null,
          m.reply ? h('div.bubble-reply', { onclick: e => { e.stopPropagation(); jumpToMsg(m.reply.id); } }, h('b', m.reply.nick), h('span', m.reply.text)) : null,
          m.sticker ? h('div.bubble-sticker', m.sticker) : null,
          m.voice ? renderVoicePlayer(m.voice) : null,
          renderBubbleAttach(m.attach),
          m.msg ? ui.html(m.msg, 'text') : null,
          h('div.bubble-meta',
            m.pinned ? icon('pin', 11) : null,
            m.edited_at ? h('span', 'изм. ') : null,
            h('span', new Date(m.time * 1000).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }))));
        // Свайп вправо для ответа
        let swX = 0, swY = 0, swMoved = 0;
        bubble.addEventListener('touchstart', e => {
          if (e.touches.length !== 1) return;
          swX = e.touches[0].clientX; swY = e.touches[0].clientY; swMoved = 0;
        }, { passive: true });
        bubble.addEventListener('touchmove', e => {
          if (!swX) return;
          const dx = e.touches[0].clientX - swX, dy = Math.abs(e.touches[0].clientY - swY);
          if (dx > 12 && dx < 72 && dy < 24) { swMoved = dx; bubble.style.transform = `translateX(${dx}px)`; }
        }, { passive: true });
        bubble.addEventListener('touchend', () => {
          bubble.style.transform = '';
          if (swMoved > 46) {
            if (navigator.vibrate) try { navigator.vibrate(15); } catch {}
            editMsg = null; replyTo = { id: m.id, nick: m.user.nick, text: previewTxt };
            renderChips(); inputTa && inputTa.focus();
          }
          swX = 0; swMoved = 0;
        });
        const isNew = !seenIds.has(m.id);
        if (isNew) seenIds.add(m.id);
        nodes.push(h('div.bubble-row' + (m.mine ? '.mine' : '') + (isNew ? '.new-msg' : ''),
          !m.mine ? ui.avatar(m.user, 30) : null,
          reactPills ? h('div.msg-bubble-col', bubble, reactPills) : bubble));
      });
    }
    scrollEl.replaceChildren(...nodes);
    if (keepBottom) requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight; });
    else requestAnimationFrame(() => { scrollEl.scrollTop = scrollEl.scrollHeight - prevH + prevTop; });
  };

  const openMembersModal = async () => {
    const inviteURL = `${location.origin}/chats/join/${d.invite_code}`;
    const addIn = h('input.input', { placeholder: 'Логин пользователя' });
    const botPickerWrap = h('div.stack-s');
    const botsList = d.bots || [];

    const denyChk = isOwner ? h('input', {
      type: 'checkbox', checked: denyBots,
      onchange: async () => {
        const r = await api.post(`/api/chats/${cid}/settings`, { deny_bots: denyChk.checked });
        denyBots = !!r.deny_bots;
        ui.ok(denyBots ? 'Приглашение ботов запрещено' : 'Приглашение ботов разрешено');
        modal.close(); reload();
      },
    }) : null;

    const botsSection = h('div.stack-s',
      h('b', `Боты сообществ (${botsList.length})`),
      isOwner ? h('label.check', denyChk, ' Запретить приглашать ботов в беседу') : null,
      denyBots ? h('div.small.warn', 'Создатель беседы запретил приглашать новых ботов.') : null,
      h('div.small.muted', 'Боты реагируют ТОЛЬКО когда к ним обращаются: @тег, /команда@тег или ответ на сообщение бота.'),
      botsList.length ? h('div.list', ...botsList.map(b => h('div.list-item',
        h('div.grp-ava-sm', { style: { width: '34px', height: '34px', borderRadius: '10px', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#ec4899)', color: '#fff', fontWeight: '800', overflow: 'hidden', flexShrink: '0' } },
          b.avatar ? h('img', { src: b.avatar, alt: '', style: { width: '100%', height: '100%', objectFit: 'cover' } }) : Array.from(b.name || 'B')[0].toUpperCase()),
        h('div.main',
          h('div.title', b.name, h('span.badge.primary', { style: { marginLeft: '6px' } }, '@' + b.alias)),
          b.commands && b.commands.length ? h('div.small.muted', b.commands.map(c => `/${c.command}@${b.alias}`).join(', ')) : h('div.small.muted', 'Обращение: @' + b.alias)),
        isAdm ? ui.btn('', {
          ic: 'trash', small: true, kind: 'danger', title: 'Убрать бота',
          onclick: async () => {
            await api.del(`/api/chats/${cid}/bots/${b.group_id}`);
            ui.ok('Бот удалён из беседы'); modal.close(); reload();
          },
        }) : null))) : h('div.small.muted', 'В беседе пока нет ботов.'),
      isAdm && !denyBots ? botPickerWrap : null);

    const openMutePicker = mem => {
      const now = Math.floor(Date.now() / 1000);
      const isMuted = mem.muted_until && mem.muted_until > now;
      const setMute = async mins => {
        await api.post(`/api/chats/${cid}/mute`, { user_id: mem.user.id, minutes: mins });
        ui.ok(mins > 0 ? `${mem.user.nick} получил(а) мут на ${mins} мин.` : `Мут с ${mem.user.nick} снят`);
        mMod.close(); modal.close(); reload();
      };
      const mMod = ui.modal({
        title: `Ограничить: ${mem.user.nick}`,
        body: h('div.stack',
          h('div.small.muted', 'Выберите, на сколько запретить участнику писать в этой беседе:'),
          h('div.row',
            ui.btn('5 мин', { small: true, onclick: () => setMute(5) }),
            ui.btn('10 мин', { small: true, onclick: () => setMute(10) }),
            ui.btn('30 мин', { small: true, onclick: () => setMute(30) }),
            ui.btn('1 час', { small: true, onclick: () => setMute(60) }),
            ui.btn('24 часа', { small: true, onclick: () => setMute(1440) })),
          isMuted ? ui.btn('Снять мут', { ic: 'check', kind: 'primary', small: true, onclick: () => setMute(0) }) : null),
        actions: [{ label: 'Отмена' }],
      });
    };

    const nowSec = Math.floor(Date.now() / 1000);
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
            ui.ok('Участник добавлен'); modal.close(); reload();
          },
        }))) : null,
      botsSection,
      h('b', `Участники (${(d.members || []).length})`),
      h('div.list', ...(d.members || []).map(mem => {
        const isMuted = mem.muted_until && mem.muted_until > nowSec;
        const canModTarget = isAdm && mem.role !== 'owner' && mem.user.id !== me().id && (isOwner || mem.role !== 'admin');
        return h('div.list-item',
          ui.avatar(mem.user, 34),
          h('div.main',
            h('div.title', ui.userLink(mem.user),
              mem.role !== 'member' ? h('span.badge.primary', { style: { marginLeft: '6px' } }, mem.role === 'owner' ? 'Создатель' : 'Админ') : null,
              isMuted ? h('span.badge.danger', { style: { marginLeft: '6px' } }, 'Мут') : null)),
          isOwner && mem.role !== 'owner' ? ui.btn(mem.role === 'admin' ? 'Снять' : 'Админ', {
            small: true,
            onclick: async () => {
              await api.post(`/api/chats/${cid}/role`, { user_id: mem.user.id, role: mem.role === 'admin' ? 'member' : 'admin' });
              ui.ok('Роль обновлена'); modal.close(); reload();
            },
          }) : null,
          canModTarget ? ui.btn(isMuted ? 'Мут✓' : 'Мут', {
            ic: 'volume-x', small: true, title: 'Запретить писать временно',
            onclick: () => openMutePicker(mem),
          }) : null,
          canModTarget ? ui.btn('', {
            ic: 'x', small: true, kind: 'danger', title: 'Выгнать из беседы',
            onclick: async () => {
              if (await ui.confirm(`Выгнать ${mem.user.nick} из беседы?`, { danger: true })) {
                await api.del(`/api/chats/${cid}/members/${mem.user.id}`);
                ui.ok('Участник исключён'); modal.close(); reload();
              }
            },
          }) : null);
      })));
    const modal = ui.modal({
      title: `Участники и боты`,
      body,
      actions: [
        {
          label: 'Выйти из беседы', kind: 'danger',
          onclick: async () => {
            if (await ui.confirm('Выйти из беседы?', { danger: true })) {
              await api.del(`/api/chats/${cid}/members/${me().id}`);
              modal.close(); router.go('/mail');
            }
          },
        },
        { label: 'Закрыть' },
      ],
    });

    if (isAdm) {
      const inChat = new Set(botsList.map(x => x.group_id));
      const qIn = h('input.input', { placeholder: 'Поиск бота по названию или ID сообщества…' });
      const aliasIn = h('input.input', { placeholder: 'Свой тег (необяз., напр. helper)', maxlength: 24 });
      const resBox = h('div.stack-s');
      let tmr = null;
      qIn.oninput = () => {
        clearTimeout(tmr);
        const q = qIn.value.trim();
        if (!q) { resBox.replaceChildren(); return; }
        tmr = setTimeout(async () => {
          try {
            const av = await api.get('/api/chat-bots', { q }, { silent: true });
            const avail = (av.items || []).filter(x => !inChat.has(x.id));
            if (!avail.length) { resBox.replaceChildren(h('div.small.muted', 'Подходящих ботов не найдено.')); return; }
            resBox.replaceChildren(h('div.list', ...avail.map(g => h('div.list-item',
              h('div.main', h('div.title', g.name), h('div.small.muted', `@bot${g.id}`)),
              ui.btn('Пригласить', {
                ic: 'plus', kind: 'primary', small: true,
                onclick: async () => {
                  await api.post(`/api/chats/${cid}/bots`, { group_id: g.id, alias: aliasIn.value.trim() });
                  ui.ok('Бот приглашён в беседу!'); modal.close(); reload();
                },
              })))));
          } catch { resBox.replaceChildren(); }
        }, 250);
      };
      botPickerWrap.replaceChildren(
        h('div.small.muted', 'Также бота можно пригласить прямо со страницы нужного сообщества кнопкой «В беседу».'),
        qIn, aliasIn, resBox);
    }
  };

  let searchOpen = false;
  const toggleSearch = () => {
    searchOpen = !searchOpen;
    if (!searchOpen) { searchHost.replaceChildren(); return; }
    const sIn = h('input.input', { placeholder: 'Поиск по беседе…' });
    const sRes = h('div.msg-search-res');
    let st = null;
    sIn.oninput = () => {
      clearTimeout(st);
      const q = sIn.value.trim();
      if (!q) { sRes.replaceChildren(); return; }
      st = setTimeout(async () => {
        const r = await api.get(`/api/chats/${cid}/search`, { q }, { silent: true });
        const hits = r.items || [];
        if (!hits.length) { sRes.replaceChildren(h('div.pad.small.muted', 'Ничего не найдено')); return; }
        sRes.replaceChildren(h('div.list', ...hits.map(hm => h('button.list-item', {
          type: 'button', style: { width: '100%', background: 'none', border: 0, textAlign: 'left', cursor: 'pointer' },
          onclick: () => { jumpToMsg(hm.id); toggleSearch(); },
        }, h('div.main', h('div.small.muted', hm.user.nick, ' · ', fmt.datetime(hm.time)), h('div.clip', hm.raw || 'Сообщение'))))));
      }, 250);
    };
    searchHost.replaceChildren(
      h('div.msg-search-bar', icon('search', 16), sIn, h('button.icon-btn', { type: 'button', onclick: toggleSearch }, icon('x', 18))),
      sRes);
    sIn.focus();
  };

  const statusNormal = `${(d.members || []).length} участников`;
  const statusEl = h('div.msg-status', statusNormal);
  const head = h('div.msg-head',
    h('a.icon-btn', { href: '/mail', title: 'Назад к почте' }, icon('arrow-left', 20)),
    h('div.grp-ava-sm', { style: { width: '38px', height: '38px', borderRadius: '10px', display: 'grid', placeItems: 'center', background: 'linear-gradient(135deg,var(--accent),#06b6d4)', color: '#fff', fontWeight: '800', flexShrink: '0' } },
      Array.from(d.title || 'Б')[0].toUpperCase()),
    h('div.msg-head-info', h('b', d.title), statusEl),
    h('button.icon-btn', { type: 'button', title: 'Поиск по беседе', onclick: toggleSearch }, icon('search', 19)),
    ui.btn('', { ic: 'users', small: true, title: 'Участники и ссылка', onclick: openMembersModal }));

  // ───────── Полный композер как в личной переписке (+, вложения, стикеры, BB, смайлы, голос)
  const ta = h('textarea.msg-input', { rows: 1, placeholder: (d.bots && d.bots.length) ? 'Сообщение… (@тег или /команда@тег для бота)' : 'Сообщение… (можно @ник)', maxlength: 1024 });
  inputTa = ta;
  const charEl = h('span.msg-char');
  const sendBtn = h('button.msg-send', { type: 'button', title: 'Отправить', disabled: true }, icon('send', 18));
  const micBtn = h('button.msg-tool', { type: 'button', title: 'Записать голосовое сообщение' }, icon('mic', 19));
  const plusBtn = h('button.msg-tool', { type: 'button', title: 'Вложения, стикеры, формат и смайлы' }, icon('plus', 20));
  const chatBots = d.bots || [];
  let cmdBtn = null;
  if (chatBots.length) {
    cmdBtn = h('button.msg-tool', {
      type: 'button', title: 'Команды ботов в беседе',
      onclick: () => {
        activeDrawer = activeDrawer === 'cmds' ? '' : 'cmds';
        plusBtn.classList.remove('active');
        cmdBtn.classList.toggle('active', activeDrawer === 'cmds');
        if (activeDrawer !== 'cmds') { drawerHost.replaceChildren(); return; }
        const rows = [];
        chatBots.forEach(b => {
          rows.push(h('button.msg-cmd-item', {
            type: 'button',
            onclick: () => { ta.value = `@${b.alias} `; ta.focus(); syncTa(); activeDrawer = ''; cmdBtn.classList.remove('active'); drawerHost.replaceChildren(); },
          }, h('span.msg-cmd-name', '@' + b.alias), h('span.msg-cmd-desc', `Обратиться к боту «${b.name}»`)));
          (b.commands || []).forEach(c => {
            rows.push(h('button.msg-cmd-item', {
              type: 'button',
              onclick: () => { ta.value = `/${c.command}@${b.alias} `; ta.focus(); syncTa(); activeDrawer = ''; cmdBtn.classList.remove('active'); drawerHost.replaceChildren(); },
            }, h('span.msg-cmd-name', `/${c.command}@${b.alias}`), h('span.msg-cmd-desc', c.description || b.name)));
          });
        });
        drawerHost.replaceChildren(h('div.msg-drawer', h('div.msg-cmd-list', ...rows)));
      },
    }, h('b', '/'));
  }
  const recHost = h('div');
  const drawerHost = h('div');
  let activeDrawer = '';
  let subTab = 'bb';

  syncTa = () => {
    ta.style.height = 'auto';
    const sh = ta.scrollHeight;
    ta.style.height = Math.min(120, Math.max(36, sh)) + 'px';
    ta.style.overflowY = sh > 120 ? 'auto' : 'hidden';
    sendBtn.disabled = ta.value.trim().length < 1 && !attach;
    if (ta.value.length > 850) {
      charEl.textContent = `${ta.value.length}/1024`;
      charEl.classList.toggle('warn', ta.value.length > 980);
    } else charEl.textContent = '';
  };

  const wrap = (a, b = '') => {
    const s = ta.selectionStart, e = ta.selectionEnd, v = ta.value;
    ta.value = v.slice(0, s) + a + v.slice(s, e) + b + v.slice(e);
    ta.focus();
    ta.selectionStart = s + a.length;
    ta.selectionEnd = e + a.length;
    syncTa();
  };

  const drawToolsDrawer = async () => {
    if (activeDrawer !== 'tools') { drawerHost.replaceChildren(); return; }
    const body = h('div');
    const tabsRow = h('div.msg-tools-tabs',
      h('button.msg-tools-tab', { type: 'button', onclick: () => { activeDrawer = ''; plusBtn.classList.remove('active'); drawerHost.replaceChildren(); openAttachPicker(a => { attach = a; renderChips(); syncTa(); }); } }, icon('paperclip', 15), 'Прикрепить'),
      h('button.msg-tools-tab' + (subTab === 'stickers' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'stickers'; drawToolsDrawer(); } }, icon('star', 15), 'Стикеры'),
      h('button.msg-tools-tab' + (subTab === 'bb' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'bb'; drawToolsDrawer(); } }, h('b', 'Aa'), 'Формат'),
      h('button.msg-tools-tab' + (subTab === 'smiles' ? '.active' : ''), { type: 'button', onclick: () => { subTab = 'smiles'; drawToolsDrawer(); } }, icon('smile', 15), 'Смайлы'));
    drawerHost.replaceChildren(h('div.msg-drawer', tabsRow, body));

    if (subTab === 'stickers') {
      body.replaceChildren(h('div.pad.small.muted', 'Загрузка стикеров…'));
      const stData = await api.get('/api/stickers', null, { silent: true });
      const packs = stData.packs || [];
      let curPack = packs[0]?.id || '';
      const pTabs = h('div.sticker-tabs');
      const pGrid = h('div.sticker-grid');
      const drawPack = () => {
        pTabs.replaceChildren(...packs.map(pk => h('button.btn.sm' + (pk.id === curPack ? '.primary' : ''), {
          type: 'button', onclick: () => { curPack = pk.id; drawPack(); },
        }, pk.vip ? '👑 ' + pk.title : pk.title)));
        const pk = packs.find(x => x.id === curPack) || packs[0];
        const locked = pk && pk.vip && !stData.is_vip;
        pGrid.replaceChildren(...((pk && pk.stickers) || []).map(st => h('button.sticker-btn' + (locked ? '.locked' : ''), {
          type: 'button',
          title: locked ? 'Доступно только для VIP' : 'Отправить стикер',
          onclick: async () => {
            if (locked) { ui.toast('Этот набор стикеров доступен только для VIP-подписчиков', 'warn'); return; }
            const sent = await api.post(`/api/chats/${cid}/msg`, { sticker: st, reply_id: replyTo ? replyTo.id : 0 });
            if (upsertMsg(sent)) renderMessages(true);
            replyTo = null; renderChips();
            activeDrawer = ''; plusBtn.classList.remove('active'); drawerHost.replaceChildren();
          },
        }, st)));
      };
      drawPack();
      body.replaceChildren(pTabs, pGrid);
    } else if (subTab === 'bb') {
      const ribbon = h('div.msg-bb-ribbon');
      const addBtn = (lbl, title, fn) => ribbon.append(h('button.msg-bb-btn', { type: 'button', title, onclick: fn }, lbl));
      addBtn(h('b', 'B'), 'Жирный', () => wrap('[b]', '[/b]'));
      addBtn(h('i', 'I'), 'Курсив', () => wrap('[i]', '[/i]'));
      addBtn(h('u', 'U'), 'Подчёркнутый', () => wrap('[u]', '[/u]'));
      addBtn(h('del', 'S'), 'Зачёркнутый', () => wrap('[s]', '[/s]'));
      addBtn(icon('eye-off', 14), 'Спойлер', () => wrap('[spoiler]', '[/spoiler]'));
      addBtn(icon('quote', 14), 'Цитата', () => wrap('[quote]', '[/quote]'));
      addBtn(icon('hash', 14), 'Код', () => wrap('[code]', '[/code]'));
      [['red', '#e11d48', 'Красный'], ['yellow', '#eab308', 'Жёлтый'], ['green', '#16a34a', 'Зелёный'], ['blue', '#2563eb', 'Синий']].forEach(([tag, col, title]) => {
        ribbon.append(h('button.msg-color-dot', { type: 'button', title, style: { background: col }, onclick: () => wrap(`[${tag}]`, `[/${tag}]`) }));
      });
      body.replaceChildren(ribbon);
    } else {
      const box = h('div.msg-smiles-drawer', ui.loading());
      body.replaceChildren(box);
      const list = await getSmiles();
      if (!list.length) box.replaceChildren(h('span.small.muted', 'Смайлов нет'));
      else box.replaceChildren(...list.map(s => h('button.smile-btn', { type: 'button', title: s.code, onclick: () => wrap(' ' + s.code + ' ') }, h('img', { src: s.url, alt: s.code }))));
    }
  };

  plusBtn.onclick = () => {
    activeDrawer = activeDrawer === 'tools' ? '' : 'tools';
    plusBtn.classList.toggle('active', activeDrawer === 'tools');
    drawToolsDrawer();
  };

  const doSend = async () => {
    const msg = ta.value.trim();
    if ((msg.length < 1 && !attach) || sendBtn.disabled) return;
    sendBtn.disabled = true;
    try {
      if (editMsg) {
        const upd = await api.put('/api/chats/msg/' + editMsg.id, { msg });
        const idx = items.findIndex(x => x.id === editMsg.id);
        if (idx >= 0) items[idx] = upd;
        editMsg = null; ta.value = ''; renderChips(); syncTa(); renderMessages(false);
        return;
      }
      const sent = await api.post(`/api/chats/${cid}/msg`, {
        msg,
        reply_id: replyTo ? replyTo.id : 0,
        att_type: attach ? attach.type : '',
        att_id: attach ? attach.id : 0,
      });
      if (upsertMsg(sent)) renderMessages(true);
      ta.value = ''; replyTo = null; attach = null;
      renderChips(); syncTa();
    } finally { syncTa(); }
  };

  let recorder = null, recChunks = [], recTimer = null, recSec = 0, recStream = null;
  const stopRec = sendVoice => {
    clearInterval(recTimer);
    recHost.replaceChildren();
    if (recorder && recorder.state !== 'inactive') {
      recorder.onstop = async () => {
        recStream && recStream.getTracks().forEach(t => t.stop());
        if (!sendVoice || !recChunks.length) return;
        const mime = recorder.mimeType || 'audio/webm';
        const ext = mime.includes('ogg') ? 'ogg' : mime.includes('mp4') ? 'm4a' : 'webm';
        const blob = new Blob(recChunks, { type: mime });
        const fd = new FormData();
        fd.append('file', blob, 'voice.' + ext);
        fd.append('dur', String(Math.max(1, recSec)));
        try {
          const res = await fetch(`/api/chats/${cid}/voice`, { method: 'POST', body: fd, headers: { 'X-DCMS': '1' } });
          if (!res.ok) throw new Error();
          const sent = await res.json();
          if (upsertMsg(sent)) renderMessages(true);
        } catch { ui.toast('Не удалось отправить голосовое', 'err'); }
      };
      recorder.stop();
    } else if (recStream) recStream.getTracks().forEach(t => t.stop());
    recorder = null;
  };
  micBtn.onclick = async () => {
    if (recorder) { stopRec(true); return; }
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof MediaRecorder === 'undefined') {
      ui.toast('Запись голоса не поддерживается вашим браузером', 'warn'); return;
    }
    try {
      recStream = await navigator.mediaDevices.getUserMedia({ audio: true });
      recChunks = []; recSec = 0;
      recorder = new MediaRecorder(recStream);
      recorder.ondataavailable = e => { if (e.data && e.data.size) recChunks.push(e.data); };
      recorder.start();
      const tEl = h('span', '0:00');
      recTimer = setInterval(() => { recSec++; tEl.textContent = `${Math.floor(recSec / 60)}:${String(recSec % 60).padStart(2, '0')}`; }, 1000);
      recHost.replaceChildren(h('div.msg-rec-bar',
        h('span.msg-rec-dot'), h('span', 'Запись… '), tEl, h('span.grow'),
        ui.btn('Отмена', { small: true, onclick: () => stopRec(false) }),
        ui.btn('Отправить', { ic: 'send', small: true, kind: 'primary', onclick: () => stopRec(true) })));
    } catch { ui.toast('Нет доступа к микрофону', 'err'); }
  };

  let lastTypingSent = 0;
  sendBtn.onclick = doSend;
  ta.oninput = () => {
    syncTa();
    const now = Date.now();
    if (ta.value.trim() && now - lastTypingSent > 2500) {
      lastTypingSent = now;
      api.post(`/api/chats/${cid}/typing`, {}, { silent: true }).catch(() => {});
    }
  };
  ta.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey && window.innerWidth > 700) { e.preventDefault(); doSend(); } };
  const foot = h('div.msg-foot', muteBannerHost, chipHost, recHost, drawerHost, h('div.msg-compose', plusBtn, cmdBtn, h('div.msg-input-wrap', ta, charEl), micBtn, sendBtn));

  let typingTimer = null;
  const onTyping = bus.on('sse:gchat_typing', ev => {
    if (ev && ev.chat === cid) {
      statusEl.className = 'msg-status typing';
      statusEl.textContent = `${ev.nick} печатает…`;
      clearTimeout(typingTimer);
      typingTimer = setTimeout(() => { statusEl.className = 'msg-status'; statusEl.textContent = statusNormal; }, 3500);
    }
  });
  const onMsg = bus.on('sse:gchat_msg', m => {
    if (m && m.chat === cid && upsertMsg(m)) {
      renderMessages(true);
    }
  });
  const onEdit = bus.on('sse:gchat_edit', ev => {
    if (ev && ev.chat === cid) {
      const m = items.find(x => x.id === ev.id);
      if (m) { m.msg = ev.msg; m.raw = ev.raw; m.edited_at = ev.edited_at; renderMessages(false); }
    }
  });
  const onReact = bus.on('sse:gchat_react', ev => {
    if (ev && ev.chat === cid) {
      const m = items.find(x => x.id === ev.id);
      if (m) { m.reactions = ev.reactions || []; renderMessages(false); }
    }
  });
  const onDel = bus.on('sse:gchat_del', ev => {
    if (ev && ev.chat === cid) {
      const prevLen = items.length;
      items = items.filter(x => x.id !== ev.id);
      if (items.length !== prevLen) renderMessages(false);
    }
  });

  // Лёгкий фоновый дозапрос новых сообщений каждые 3.5 сек на случай буферизации SSE
  let syncing = false;
  const syncTimer = setInterval(async () => {
    if (syncing || document.visibilityState !== 'visible') return;
    syncing = true;
    try {
      const lastId = items.length ? items[items.length - 1].id : 0;
      const res = await api.get(`/api/chats/${cid}`, { after: lastId }, { silent: true });
      if (res.my_muted_until !== undefined && res.my_muted_until !== myMutedUntil) {
        myMutedUntil = res.my_muted_until;
        renderMuteBanner();
      }
      if (res.deny_bots !== undefined) denyBots = !!res.deny_bots;
      let added = false;
      (res.items || []).forEach(nm => { if (upsertMsg(nm)) added = true; });
      if (added) renderMessages(true);
    } catch {} finally { syncing = false; }
  }, 3500);

  ctx.onLeave(() => {
    document.body.classList.remove('msg-page');
    clearTimeout(typingTimer);
    clearInterval(syncTimer);
    onMsg && onMsg();
    onEdit && onEdit();
    onReact && onReact();
    onDel && onDel();
    onTyping && onTyping();
  });

  renderMuteBanner();
  renderPinned();
  renderMessages(true);
  return h('div.msg-shell', head, searchHost, pinnedHost, scrollEl, foot);
}, { auth: true });

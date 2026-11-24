import { h, api, ui, router, state, icon, link, fmt, bus } from '../core.js';

if (!document.querySelector('link[href="/css/shorts.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/shorts.css' }));
}

const me = () => state.user;
let globalMuted = false;
const syncMuteUI = muted => {
  document.querySelectorAll('.short-mute').forEach(b => b.replaceChildren(icon(muted ? 'volume-x' : 'volume', 18)));
};

state.adminLinks.push({
  group: 'Модерация',
  title: 'Модерация клипов',
  href: '/admin/shorts',
  access: 'shorts_block',
  icon: 'video',
  desc: 'Скрытие, блокировка и удаление клипов',
});
const fmtDur = s => {
  s = Math.max(0, Math.round(s || 0));
  const m = Math.floor(s / 60), r = s % 60;
  return `${m}:${r < 10 ? '0' : ''}${r}`;
};

// ───────── загрузка клипа с прогрессом (вызывается только на своей странице клипов)
export function openUploadModal(maxSec = 60, maxMB = 100, onDone) {
  if (!me()) { router.go('/login'); return; }
  let pickedFile = null;
  let previewURL = '';

  const fileInput = h('input', { type: 'file', accept: 'video/*,.mp4,.mov,.webm,.mkv,.avi,.3gp', style: { display: 'none' } });
  const titleIn = h('input.input', { placeholder: 'Название клипа', maxlength: 100 });
  const opisIn = h('textarea.input', { rows: 2, placeholder: 'Описание, #хештеги (необязательно)', maxlength: 500 });
  const previewBox = h('div');
  const errBox = h('div.form-err');
  const progFill = h('i');
  const progText = h('div.small.muted');
  const progWrap = h('div.stack-s', { style: { display: 'none' } }, h('div.upload-bar', progFill), progText);

  const drop = h('div.upload-drop', {
    onclick: () => fileInput.click(),
    ondragover: e => { e.preventDefault(); drop.classList.add('drag'); },
    ondragleave: () => drop.classList.remove('drag'),
    ondrop: e => {
      e.preventDefault();
      drop.classList.remove('drag');
      const f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
      if (f) handleFile(f);
    },
  },
    icon('upload', 26),
    h('b', 'Выберите или перетащите видео'),
    h('span.small.muted', `До ${maxSec} секунд · до ${maxMB} МБ · вертикальный формат 9:16`));

  let liveTitleEl = null;
  let liveOpisEl = null;
  const syncLiveOverlay = () => {
    if (liveTitleEl) liveTitleEl.textContent = titleIn.value.trim() || 'Название клипа';
    if (liveOpisEl) liveOpisEl.textContent = opisIn.value.trim();
  };
  titleIn.oninput = syncLiveOverlay;
  opisIn.oninput = syncLiveOverlay;

  const handleFile = f => {
    errBox.textContent = '';
    if (!f) return;
    if (f.size > maxMB * 1024 * 1024) {
      errBox.textContent = `Файл слишком большой (максимум ${maxMB} МБ)`;
      return;
    }
    pickedFile = f;
    if (!titleIn.value) titleIn.value = f.name.replace(/\.[^.]+$/, '').slice(0, 100);
    if (previewURL) URL.revokeObjectURL(previewURL);
    previewURL = URL.createObjectURL(f);
    drop.style.display = 'none';

    const v = h('video.short-prev-vid', { src: previewURL, autoplay: true, loop: true, muted: true, playsinline: true, disablePictureInPicture: true });
    v.setAttribute('playsinline', '');
    v.setAttribute('webkit-playsinline', '');
    v.oncontextmenu = e => e.preventDefault();

    const playMark = h('div.short-prev-play', icon('play', 34));
    const soundBtn = h('button.short-prev-btn', {
      type: 'button', title: 'Включить/выключить звук',
      onclick: e => {
        e.stopPropagation();
        v.muted = !v.muted;
        soundBtn.replaceChildren(icon(v.muted ? 'volume-x' : 'volume-2', 16));
      },
    }, icon('volume-x', 16));
    const changeBtn = h('button.short-prev-btn', {
      type: 'button', title: 'Выбрать другое видео',
      onclick: e => { e.stopPropagation(); fileInput.click(); },
    }, icon('edit', 15), h('span', 'Заменить'));

    const seek = h('input.short-prev-seek', { type: 'range', min: 0, max: 100, step: 0.1, value: 0 });
    const timeEl = h('span.short-prev-time', '0:00 / 0:00');
    const metaEl = h('div.small.muted.center', { style: { marginTop: '8px' } }, `${f.name} · ${fmt.size(f.size)}`);

    liveTitleEl = h('div.short-prev-title');
    liveOpisEl = h('div.short-prev-opis');
    syncLiveOverlay();

    const togglePlay = () => {
      if (v.paused) { v.play().catch(() => {}); playMark.classList.remove('show'); }
      else { v.pause(); playMark.classList.add('show'); }
    };
    v.onclick = togglePlay;
    playMark.onclick = togglePlay;

    v.onloadedmetadata = () => {
      const dur = v.duration || 0;
      const wh = v.videoWidth && v.videoHeight ? `${v.videoWidth}×${v.videoHeight} · ` : '';
      metaEl.textContent = `${wh}${fmtDur(Math.round(dur))} · ${fmt.size(f.size)}`;
      timeEl.textContent = `0:00 / ${fmtDur(Math.round(dur))}`;
      if (dur > maxSec + 1.5) {
        errBox.textContent = `Видео длиннее ${maxSec} сек (${Math.round(dur)} сек). Загрузите ролик до ${maxSec} сек.`;
        pickedFile = null;
      }
      v.play().catch(() => playMark.classList.add('show'));
    };
    v.ontimeupdate = () => {
      if (!v.duration) return;
      seek.value = (v.currentTime / v.duration) * 100;
      timeEl.textContent = `${fmtDur(Math.floor(v.currentTime))} / ${fmtDur(Math.round(v.duration))}`;
    };
    seek.oninput = e => {
      e.stopPropagation();
      if (v.duration) v.currentTime = (+seek.value / 100) * v.duration;
    };

    const card = h('div.short-prev-card',
      v,
      playMark,
      h('div.short-prev-top', soundBtn, changeBtn),
      h('div.short-prev-foot',
        h('div.short-prev-user', ui.avatar(me(), 24), h('b', me().nick)),
        liveTitleEl,
        liveOpisEl,
        h('div.short-prev-bar', seek, timeEl)));

    previewBox.replaceChildren(card, metaEl);
  };

  fileInput.onchange = () => {
    const f = fileInput.files && fileInput.files[0];
    if (f) handleFile(f);
  };

  let uploading = false;
  let modal;
  const doUpload = () => {
    if (uploading) return false;
    if (!pickedFile) { errBox.textContent = 'Выберите видео до 1 минуты'; return false; }
    errBox.textContent = '';
    uploading = true;
    progWrap.style.display = '';
    progText.textContent = 'Загрузка на сервер: 0%';

    const fd = new FormData();
    fd.append('file', pickedFile);
    fd.append('title', titleIn.value.trim());
    fd.append('opis', opisIn.value.trim());

    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/shorts', true);
    xhr.setRequestHeader('X-DCMS', '1');
    xhr.upload.onprogress = e => {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        progFill.style.width = pct + '%';
        progText.textContent = pct < 100 ? `Загрузка на сервер: ${pct}%` : 'Файл загружен, запускаю сжатие…';
      }
    };
    xhr.onload = () => {
      uploading = false;
      let res = {};
      try { res = JSON.parse(xhr.responseText || '{}'); } catch {}
      if (xhr.status >= 200 && xhr.status < 300) {
        if (previewURL) URL.revokeObjectURL(previewURL);
        modal && modal.close();
        ui.ok('Видео загружено и отправлено в обработку');
        onDone && onDone(res.id);
      } else {
        progWrap.style.display = 'none';
        errBox.textContent = res.error || 'Ошибка загрузки';
      }
    };
    xhr.onerror = () => {
      uploading = false;
      progWrap.style.display = 'none';
      errBox.textContent = 'Ошибка сети при загрузке';
    };
    xhr.send(fd);
    return false;
  };

  const body = h('div.stack', fileInput, drop, previewBox,
    h('div.field', h('label', 'Название'), titleIn),
    h('div.field', h('label', 'Описание'), opisIn),
    progWrap, errBox);

  modal = ui.modal({
    title: 'Загрузить короткое видео',
    body,
    onClose: () => { if (previewURL) URL.revokeObjectURL(previewURL); },
    actions: [
      { label: 'Отмена' },
      { label: 'Загрузить клип', kind: 'primary', onclick: doUpload },
    ],
  });
}

// ───────── комментарии к клипу (выезжающая снизу панель как в TikTok / YouTube Shorts)
function openCommentsModal(s, onCountChange) {
  const titleCnt = h('span.badge', fmt.num(s.komms));
  const listWrap = h('div.posts');
  const bodyEl = h('div.sheet-body', listWrap);
  const replyBar = h('div.sheet-reply', { style: { display: 'none' } });
  const footEl = h('div.sheet-foot');
  let replyTo = 0;
  let curPage = 1;
  let totalPages = 1;
  let allItems = [];

  const setReply = (uid, nick) => {
    replyTo = uid || 0;
    if (replyTo) {
      replyBar.style.display = '';
      replyBar.replaceChildren(
        h('span', `Ответ для ${nick}`),
        h('button', { type: 'button', title: 'Отменить ответ', onclick: () => setReply(0, '') }, icon('x', 14))
      );
    } else {
      replyBar.style.display = 'none';
      replyBar.replaceChildren();
    }
  };

  function closeSheet() {
    back.classList.remove('open');
    document.removeEventListener('keydown', onEsc);
    setTimeout(() => back.remove(), 260);
  }
  const onEsc = e => { if (e.key === 'Escape') closeSheet(); };

  const renderList = () => {
    if (!allItems.length) {
      listWrap.replaceChildren(ui.empty('Комментариев пока нет — будьте первым!', 'message-circle'));
      return;
    }
    const nodes = allItems.map(k => ui.post({
      user: k.user, time: k.time,
      body: h('div.text', { html: k.msg }),
      actions: [
        k.can_reply ? ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => { setReply(k.user.id, k.user.nick); input.value = `[b]${k.user.nick}[/b], `; input.focus(); } }) : null,
        k.can_delete ? ui.btn('', { ic: 'trash', small: true, title: 'Удалить', onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/shorts/komm/' + k.id); load(1, false); } } }) : null,
      ].filter(Boolean),
    }));
    if (curPage < totalPages) {
      nodes.push(ui.btn('Показать ещё', { cls: 'sheet-more', onclick: () => load(curPage + 1, true) }));
    }
    listWrap.replaceChildren(...nodes);
  };

  async function load(page = 1, append = false) {
    if (!append) listWrap.replaceChildren(ui.loading());
    const d = await api.get(`/api/shorts/${s.id}/komms`, { page });
    curPage = d.page || page;
    totalPages = d.pages || 1;
    titleCnt.textContent = fmt.num(d.total);
    onCountChange && onCountChange(d.total);
    allItems = append ? allItems.concat(d.items || []) : (d.items || []);
    renderList();
  }

  const input = h('textarea.input', { rows: 1, placeholder: 'Добавьте комментарий…', maxlength: 1024 });
  if (me()) {
    const sendBtn = h('button.sheet-send', { type: 'button', title: 'Отправить', 'aria-label': 'Отправить' }, icon('send', 18));
    const doSend = async () => {
      const msg = input.value.trim();
      if (!msg || sendBtn.disabled) return;
      sendBtn.disabled = true;
      try {
        const r = await api.post(`/api/shorts/${s.id}/komms`, { msg, reply_to: replyTo });
        input.value = '';
        input.style.height = '';
        setReply(0, '');
        titleCnt.textContent = fmt.num(r.komms);
        onCountChange && onCountChange(r.komms);
        await load(1, false);
        bodyEl.scrollTop = 0;
      } finally { sendBtn.disabled = false; }
    };
    sendBtn.onclick = doSend;
    input.oninput = () => { input.style.height = 'auto'; input.style.height = Math.min(110, input.scrollHeight) + 'px'; };
    input.onkeydown = e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); doSend(); } };
    footEl.append(replyBar, h('div.sheet-compose', input, sendBtn));
  } else {
    footEl.append(h('div.small.muted.center', link('/login', 'Войдите'), ', чтобы оставлять комментарии.'));
  }

  const topEl = h('div.sheet-top',
    h('div.sheet-handle'),
    h('div.sheet-head',
      h('h3', 'Комментарии ', titleCnt),
      h('button.icon-btn', { type: 'button', onclick: closeSheet, 'aria-label': 'Закрыть' }, icon('x', 20))));

  const sheet = h('div.sheet', { role: 'dialog', 'aria-label': 'Комментарии' }, topEl, bodyEl, footEl);
  const back = h('div.sheet-back', { onclick: e => { if (e.target === back) closeSheet(); } }, sheet);

  // Свайп вниз за шапку панели для закрытия
  let sy = 0, dy = 0, dragging = false;
  topEl.addEventListener('touchstart', e => {
    if (e.touches.length !== 1) return;
    sy = e.touches[0].clientY; dy = 0; dragging = true;
    sheet.style.transition = 'none';
  }, { passive: true });
  topEl.addEventListener('touchmove', e => {
    if (!dragging) return;
    dy = Math.max(0, e.touches[0].clientY - sy);
    sheet.style.transform = `translateY(${dy}px)`;
  }, { passive: true });
  topEl.addEventListener('touchend', () => {
    if (!dragging) return;
    dragging = false;
    sheet.style.transition = '';
    sheet.style.transform = '';
    if (dy > 90) closeSheet();
  });

  document.body.append(back);
  document.addEventListener('keydown', onEsc);
  requestAnimationFrame(() => back.classList.add('open'));
  load(1, false);
}

// ───────── поделиться (ЛС / Стена / Ссылка)
async function openShareModal(s) {
  const fullURL = `${location.origin}/shorts/${s.id}`;
  const bbLink = `[url=/shorts/${s.id}]🎬 ${s.title}[/url]`;
  const noteIn = h('input.input', { placeholder: 'Добавить комментарий (необязательно)', maxlength: 300 });

  const copyBtn = ui.btn('Скопировать ссылку', {
    ic: 'copy',
    onclick: async () => {
      try { await navigator.clipboard.writeText(fullURL); ui.ok('Ссылка скопирована'); }
      catch { ui.toast(fullURL, 'info'); }
    },
  });

  const wallBtn = me() ? ui.btn('Поделиться на своей стене', {
    ic: 'share', kind: 'primary',
    onclick: async () => {
      const text = (noteIn.value.trim() ? noteIn.value.trim() + '\n' : 'Поделился клипом: ') + bbLink;
      wallBtn.disabled = true;
      try {
        await api.post(`/api/user/${me().id}/stena`, { msg: text });
        ui.ok('Опубликовано на вашей стене');
      } finally { wallBtn.disabled = false; }
    },
  }) : null;

  const usersBox = h('div.share-users', me() ? ui.loading() : h('div.small.muted', link('/login', 'Войдите'), ', чтобы отправить в личные сообщения.'));

  ui.modal({
    title: 'Поделиться клипом',
    body: h('div.stack',
      h('div.field', h('label', 'Подпись к репосту'), noteIn),
      h('div.row', wallBtn, copyBtn),
      me() ? h('div.field', h('label', 'Отправить в личные сообщения'), usersBox) : null),
    actions: [{ label: 'Закрыть' }],
  });

  if (me()) {
    try {
      const d = await api.get('/api/shorts/share-targets');
      if (!d.users || !d.users.length) {
        usersBox.replaceChildren(h('div.small.muted', 'Нет доступных получателей'));
        return;
      }
      usersBox.replaceChildren(...d.users.map(u => {
        const btn = ui.btn('Отправить', {
          ic: 'send', small: true, kind: 'primary',
          onclick: async () => {
            const text = (noteIn.value.trim() ? noteIn.value.trim() + '\n' : 'Посмотри клип: ') + bbLink;
            btn.disabled = true;
            try {
              await api.post(`/api/mail/${u.id}`, { msg: text });
              btn.textContent = 'Отправлено ✓';
              ui.ok(`Отправлено пользователю ${u.nick}`);
            } catch { btn.disabled = false; }
          },
        });
        return h('div.share-user', ui.avatar(u, 34), h('div.grow', ui.userLink(u)), btn);
      }));
    } catch {
      usersBox.replaceChildren(h('div.small.muted', 'Не удалось загрузить контакты'));
    }
  }
}

// ───────── карточка вертикального видео в полноэкранной ленте
function buildShortSlide(s, { onDeleted, onUpdated }) {
  const video = h('video', {
    src: s.video_url,
    poster: s.poster_url,
    loop: true,
    playsinline: true,
    muted: globalMuted,
    preload: 'metadata',
    controlsList: 'nodownload noplaybackrate',
    disablePictureInPicture: true,
    oncontextmenu: e => e.preventDefault(),
  });
  const card = h('article.short-card.paused', {
    dataset: { id: String(s.id) },
    oncontextmenu: e => e.preventDefault(),
  });
  const progFill = h('div.short-prog-fill');
  const prog = h('div.short-prog', h('div.short-prog-bg', progFill));
  const muteBtn = h('button.short-mute', { title: 'Звук', 'aria-label': 'Звук' }, icon(globalMuted ? 'volume-x' : 'volume', 18));
  const viewsSpan = h('span', `${fmt.num(s.views)} просм.`);
  let viewedSent = false;

  muteBtn.onclick = e => {
    e.stopPropagation();
    globalMuted = !video.muted;
    document.querySelectorAll('.short-card video').forEach(v => { v.muted = globalMuted; });
    syncMuteUI(globalMuted);
  };

  video.onplay = () => {
    card.classList.remove('paused');
    if (!viewedSent) {
      viewedSent = true;
      api.post(`/api/shorts/${s.id}/view`, {}, { silent: true }).then(r => {
        if (r && r.views != null) viewsSpan.textContent = `${fmt.num(r.views)} просм.`;
      }).catch(() => {});
    }
  };
  video.onpause = () => card.classList.add('paused');
  video.ontimeupdate = () => {
    if (video.duration) progFill.style.width = ((video.currentTime / video.duration) * 100) + '%';
  };
  card.onclick = e => {
    if (e.target.closest('button, a, .short-prog, .short-desc, .dropdown-menu')) return;
    if (!globalMuted && video.muted && !video.paused) {
      video.muted = false;
      syncMuteUI(false);
      return;
    }
    if (video.paused) {
      video.muted = globalMuted;
      video.play().catch(() => { video.muted = true; syncMuteUI(true); video.play().catch(() => {}); });
    } else {
      video.pause();
    }
  };

  prog.onclick = e => {
    e.stopPropagation();
    if (!video.duration) return;
    const rect = prog.getBoundingClientRect();
    video.currentTime = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)) * video.duration;
  };

  // Правая панель действий
  const likeCnt = h('span.cnt', fmt.num(s.likes));
  const likeBtn = h('button.short-act' + (s.liked ? '.liked' : ''), {
    title: 'Нравится',
    onclick: async e => {
      e.stopPropagation();
      if (!me()) { router.go('/login'); return; }
      const r = await api.post(`/api/shorts/${s.id}/like`);
      s.liked = r.liked; s.likes = r.likes;
      likeBtn.classList.toggle('liked', !!r.liked);
      likeCnt.textContent = fmt.num(r.likes);
    },
  }, h('span.ic-wrap', icon('heart', 20)), likeCnt);

  const kommCnt = h('span.cnt', fmt.num(s.komms));
  const kommBtn = h('button.short-act', {
    title: 'Комментарии',
    onclick: e => {
      e.stopPropagation();
      openCommentsModal(s, n => { s.komms = n; kommCnt.textContent = fmt.num(n); });
    },
  }, h('span.ic-wrap', icon('message-circle', 20)), kommCnt);

  const bmBtn = h('button.short-act' + (s.bookmarked ? '.saved' : ''), {
    title: 'В закладки',
    onclick: async e => {
      e.stopPropagation();
      if (!me()) { router.go('/login'); return; }
      const r = await api.post('/api/bookmark/toggle', { type: 'shorts', id: s.id });
      s.bookmarked = !!(r.bookmarked ?? r.on ?? !s.bookmarked);
      bmBtn.classList.toggle('saved', s.bookmarked);
      ui.ok(s.bookmarked ? 'Добавлено в закладки' : 'Удалено из закладок');
    },
  }, h('span.ic-wrap', icon('bookmark', 20)), h('span.cnt', 'Сохр.'));

  const shareBtn = h('button.short-act', {
    title: 'Поделиться',
    onclick: e => { e.stopPropagation(); openShareModal(s); },
  }, h('span.ic-wrap', icon('share', 20)), h('span.cnt', 'Подел.'));

  const moreItems = [
    { label: 'Все клипы автора', href: '/shorts/user/' + s.user.id, icon: 'video' },
    { label: 'Скопировать ссылку', icon: 'copy', onclick: () => navigator.clipboard?.writeText(`${location.origin}/shorts/${s.id}`).then(() => ui.ok('Ссылка скопирована')) },
  ];
  if (s.can_block) {
    moreItems.push({ sep: true }, {
      label: s.hidden ? 'Показать в ленте' : 'Скрыть из ленты', icon: s.hidden ? 'eye' : 'eye-off',
      onclick: async () => {
        const upd = await api.post(`/api/shorts/${s.id}/moderate`, { hidden: !s.hidden });
        ui.ok(upd.hidden ? 'Клип скрыт из ленты' : 'Клип возвращён в ленту');
        onUpdated && onUpdated(upd);
      },
    }, {
      label: s.blocked ? 'Разблокировать клип' : 'Заблокировать клип', icon: s.blocked ? 'check' : 'lock', danger: !s.blocked,
      onclick: async () => {
        if (s.blocked) {
          const upd = await api.post(`/api/shorts/${s.id}/moderate`, { blocked: false });
          ui.ok('Клип разблокирован');
          onUpdated && onUpdated(upd);
        } else {
          const reason = await ui.prompt('Причина блокировки (необязательно)', '', { title: 'Заблокировать клип' });
          if (reason === null) return;
          const upd = await api.post(`/api/shorts/${s.id}/moderate`, { blocked: true, reason });
          ui.ok('Клип заблокирован');
          onUpdated && onUpdated(upd);
        }
      },
    });
  }
  if (s.can_edit) {
    moreItems.push({ sep: true }, {
      label: 'Редактировать', icon: 'edit',
      onclick: () => {
        const ti = h('input.input', { value: s.title, maxlength: 100 });
        const op = h('textarea.input', { rows: 3, maxlength: 500 }, s.opis || '');
        const modal = ui.modal({
          title: 'Редактировать клип',
          body: h('div.stack', h('div.field', h('label', 'Название'), ti), h('div.field', h('label', 'Описание'), op)),
          actions: [
            { label: 'Отмена' },
            {
              label: 'Сохранить', kind: 'primary',
              onclick: () => {
                api.put('/api/shorts/' + s.id, { title: ti.value, opis: op.value }).then(upd => {
                  modal.close(); ui.ok('Сохранено'); onUpdated && onUpdated(upd);
                });
                return false;
              },
            },
          ],
        });
      },
    }, {
      label: 'Удалить клип', icon: 'trash', danger: true,
      onclick: async () => {
        if (await ui.confirm('Удалить этот клип навсегда?', { danger: true })) {
          await api.del('/api/shorts/' + s.id);
          ui.ok('Клип удалён');
          onDeleted && onDeleted(s.id);
        }
      },
    });
  } else if (me()) {
    moreItems.push({ sep: true }, {
      label: 'Пожаловаться', icon: 'flag', danger: true,
      onclick: async () => {
        const msg = await ui.prompt('Опишите нарушение', '', { title: 'Жалоба на клип', multiline: true });
        if (msg) { await api.post(`/api/shorts/${s.id}/spam`, { msg }); ui.ok('Жалоба отправлена'); }
      },
    });
  }
  const moreBtn = ui.dropdown(
    h('button.short-act', { title: 'Ещё', onclick: e => e.stopPropagation() }, h('span.ic-wrap', icon('more', 20))),
    moreItems
  );

  const descEl = s.opis_html ? h('div.short-desc', { html: s.opis_html, onclick: e => { e.stopPropagation(); descEl.classList.toggle('open'); } }) : null;

  const statusPill = s.blocked
    ? h('span.short-pill', { style: { background: 'rgba(220,38,38,.85)' } }, icon('lock', 13), ' Заблокирован')
    : s.hidden
      ? h('span.short-pill', { style: { background: 'rgba(217,119,6,.85)' } }, 'Скрыт')
      : null;

  card.append(
    video,
    h('div.short-play-flash', h('span', icon('play', 28))),
    h('div.short-top-bar', h('div.row', h('span.short-pill', icon('video', 14), fmtDur(s.duration)), statusPill), muteBtn),
    h('div.short-rail', likeBtn, kommBtn, bmBtn, shareBtn, moreBtn),
    h('div.short-info',
      h('div.short-author', ui.avatar(s.user, 32), ui.userLink(s.user)),
      h('div.short-title', s.title),
      descEl,
      h('div.short-meta', viewsSpan, h('span', '· ' + fmt.ago(s.time)))),
    prog
  );

  const slide = h('section.short-slide', card);
  return { slide, card, video };
}

// ───────── плитка для страницы клипов пользователя
function shortTile(s, uid) {
  const href = uid ? `/shorts/${s.id}?user=${uid}` : `/shorts/${s.id}`;
  const modBadge = s.blocked
    ? h('span.badge.danger', { style: { position: 'absolute', top: '6px', left: '6px', zIndex: 2 } }, 'Заблокирован')
    : s.hidden
      ? h('span.badge.warn', { style: { position: 'absolute', top: '6px', left: '6px', zIndex: 2 } }, 'Скрыт')
      : null;
  return h('a.short-tile', { href },
    h('img', { src: s.poster_url, alt: s.title, loading: 'lazy' }),
    modBadge,
    h('span.short-tile-dur', fmtDur(s.duration)),
    h('div.short-tile-ov',
      h('div.short-tile-title', s.title),
      h('div.short-tile-meta', icon('heart', 12), fmt.num(s.likes), icon('eye', 12), fmt.num(s.views))));
}

// ───────── полноэкранная TikTok-лента (/shorts и /shorts/:id)
async function renderShortsFeed(ctx, startID = 0) {
  const tab = ctx.query.tab || 'new';
  const filterUID = +(ctx.query.user || 0);
  ctx.title('Клипы');

  document.body.classList.add('shorts-fs');
  let curPage = 1;
  let totalPages = 1;
  let loadingMore = false;

  const d = await api.get('/api/shorts', { tab, user: filterUID || undefined, page: 1, start: startID || undefined });
  totalPages = d.pages || 1;
  const reload = () => router.render();

  const backHref = filterUID ? `/shorts/user/${filterUID}` : '/';
  const tabs = [['new', 'Новые'], ['popular', 'Популярные']];
  if (me()) tabs.push(['friends', 'Друзья']);

  const headCenter = filterUID && d.owner
    ? h('div.shorts-fs-tabs', h('span.shorts-fs-tab.active', `Клипы · ${d.owner.nick}`))
    : h('div.shorts-fs-tabs', ...tabs.map(([k, l]) => h('a.shorts-fs-tab' + (tab === k ? '.active' : ''), { href: `/shorts?tab=${k}` }, l)));

  const header = h('div.shorts-fs-head',
    h('a.shorts-fs-back', { href: backHref, title: 'Назад', 'aria-label': 'Назад' }, icon('arrow-left', 20)),
    headCenter,
    h('span.shorts-fs-spacer'));

  const wrap = h('div.shorts-fs-wrap', header);

  if (!d.items || !d.items.length) {
    wrap.append(h('div.shorts-fs-empty', ui.empty(
      tab === 'friends' ? 'У друзей пока нет клипов' : 'Клипов пока нет',
      'video',
      me() ? ui.btn('Перейти в мои клипы', { ic: 'video', kind: 'primary', href: `/shorts/user/${me().id}` }) : null
    )));
    ctx.onLeave(() => document.body.classList.remove('shorts-fs'));
    return wrap;
  }

  const snap = h('div.shorts-snap');
  const slides = [];
  const io = new IntersectionObserver(entries => {
    entries.forEach(en => {
      const v = en.target.querySelector('video');
      if (!v) return;
      if (en.isIntersecting && en.intersectionRatio >= 0.6) {
        v.muted = globalMuted;
        syncMuteUI(globalMuted);
        v.play().catch(() => {
          if (!v.muted) {
            v.muted = true;
            syncMuteUI(true);
            v.play().catch(() => {});
          }
        });
        const idx = slides.findIndex(x => x.slide === en.target);
        if (idx >= slides.length - 2 && curPage < totalPages && !loadingMore) {
          loadMore();
        }
      } else {
        v.pause();
      }
    });
  }, { root: snap, threshold: [0.2, 0.6] });

  const appendItems = items => {
    items.forEach(s => {
      const item = buildShortSlide(s, { onDeleted: reload, onUpdated: reload });
      slides.push(item);
      snap.append(item.slide);
      io.observe(item.slide);
    });
  };

  async function loadMore() {
    loadingMore = true;
    try {
      const next = await api.get('/api/shorts', { tab, user: filterUID || undefined, page: curPage + 1 }, { silent: true });
      curPage++;
      totalPages = next.pages || totalPages;
      if (next.items && next.items.length) appendItems(next.items);
    } finally { loadingMore = false; }
  }

  appendItems(d.items);
  wrap.append(snap);

  // Клавиатура: стрелки вверх/вниз, пробел, M, Esc
  const onKey = e => {
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) return;
    if (document.querySelector('.modal-back, .sheet-back')) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const curIdx = slides.findIndex(c => !c.video.paused);
      const next = Math.max(0, Math.min(slides.length - 1, (curIdx < 0 ? 0 : curIdx) + (e.key === 'ArrowDown' ? 1 : -1)));
      if (slides[next]) {
        e.preventDefault();
        slides[next].slide.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }
    } else if (e.key === ' ') {
      const active = slides.find(c => !c.video.paused) || slides[0];
      if (active) { e.preventDefault(); active.video.paused ? active.video.play().catch(() => {}) : active.video.pause(); }
    } else if (e.key.toLowerCase() === 'm') {
      globalMuted = !globalMuted;
      slides.forEach(c => { c.video.muted = globalMuted; });
      document.querySelectorAll('.short-mute').forEach(b => b.replaceChildren(icon(globalMuted ? 'volume-x' : 'volume', 18)));
    }
  };
  document.addEventListener('keydown', onKey);
  ctx.onLeave(() => {
    document.body.classList.remove('shorts-fs');
    io.disconnect();
    document.removeEventListener('keydown', onKey);
    slides.forEach(c => c.video.pause());
  });

  return wrap;
}

router.add('/shorts', ctx => renderShortsFeed(ctx, 0));
router.add('/shorts/:id', ctx => renderShortsFeed(ctx, +ctx.params.id));

// ───────── клипы конкретного пользователя (загрузка доступна ТОЛЬКО на своей странице)
router.add('/shorts/user/:uid', async ctx => {
  const uid = +ctx.params.uid;
  const d = await api.get('/api/shorts', { user: uid, page: ctx.page });
  const isMe = !!(me() && me().id === uid);
  const nick = (d.owner && d.owner.nick) || (isMe ? me().nick : 'Пользователь');
  const reload = () => router.render();
  ctx.title(isMe ? 'Мои клипы' : `Клипы · ${nick}`);

  const out = [
    ui.breadcrumbs([{ href: '/id' + uid, title: nick }, { title: 'Клипы' }]),
    ui.pageHead(isMe ? 'Мои клипы' : `Клипы · ${nick}`,
      isMe ? ui.btn('Загрузить клип', { ic: 'upload', kind: 'primary', small: true, onclick: () => openUploadModal(d.max_sec || 60, d.max_mb || 100, reload) }) : null),
  ];

  // Показываем статус и прогресс обработки только владельцу на его странице
  if (isMe && d.pending && d.pending.length) {
    const pCard = h('div.card.pad.stack');
    const progMap = new Map();
    d.pending.forEach(p => {
      const badgeEl = h('span.badge' + (p.status === 'failed' ? '.danger' : '.primary'));
      const pctEl = h('span.small.muted');
      const fillEl = h('div.short-proc-fill');
      const barEl = p.status === 'processing' ? h('div.short-proc-bar', fillEl) : null;
      const hintEl = h('div.small' + (p.err_msg ? '.err' : '.muted'));

      const applyState = (st, pct, qPos, errMsg) => {
        if (st === 'failed' || errMsg) {
          badgeEl.className = 'badge danger';
          badgeEl.textContent = 'Ошибка';
          pctEl.textContent = '';
          if (barEl) barEl.style.display = 'none';
          hintEl.className = 'small err';
          hintEl.textContent = errMsg || 'Не удалось обработать видео';
          return;
        }
        if (st === 'queued' && qPos > 0) {
          badgeEl.className = 'badge';
          badgeEl.textContent = `В очереди (#${qPos})`;
          pctEl.textContent = 'Ожидание…';
          fillEl.style.width = '8%';
          fillEl.classList.add('queued');
          hintEl.textContent = qPos === 1 ? 'Ваше видео следующее в очереди на обработку' : `Перед вами в очереди: ${qPos - 1}`;
        } else {
          const val = Math.max(1, Math.min(99, pct || 0));
          badgeEl.className = 'badge primary';
          badgeEl.textContent = 'Обработка';
          pctEl.textContent = val + '%';
          fillEl.classList.remove('queued');
          fillEl.style.width = val + '%';
          hintEl.textContent = 'Видео обрабатывается и скоро появится в ленте';
        }
      };

      applyState(p.status === 'failed' ? 'failed' : (p.proc_state || 'processing'), p.progress || 0, p.queue_pos || 0, p.err_msg);
      progMap.set(p.id, applyState);

      pCard.append(h('div.stack-s',
        h('div.row',
          badgeEl,
          h('b', p.title),
          h('span.grow'),
          pctEl,
          p.status === 'failed' ? ui.btn('Убрать', { ic: 'trash', small: true, onclick: async () => { await api.del('/api/shorts/' + p.id); reload(); } }) : null),
        barEl,
        hintEl));
    });
    out.push(pCard);

    if (d.pending.some(p => p.status === 'processing')) {
      const offProg = bus.on('sse:shorts_progress', ev => {
        if (!ev || !ev.id) return;
        const fn = progMap.get(ev.id);
        if (fn) fn(ev.state, ev.progress, ev.queue_pos, ev.err);
      });
      const tm = setTimeout(() => router.render(), 10000);
      ctx.onLeave(() => { offProg(); clearTimeout(tm); });
    }
  }

  out.push(
    d.items && d.items.length
      ? h('div.shorts-grid', ...d.items.map(s => shortTile(s, uid)))
      : ui.card(ui.empty(
        isMe ? 'У вас пока нет клипов' : 'У пользователя пока нет клипов',
        'video',
        isMe ? ui.btn('Загрузить первый клип', { ic: 'upload', kind: 'primary', onclick: () => openUploadModal(d.max_sec || 60, d.max_mb || 100, reload) }) : null
      )),
    ui.pager(d)
  );
  return out;
});

// ───────── панель модерации клипов (/admin/shorts)
router.add('/admin/shorts', async ctx => {
  const filter = ctx.query.filter || 'all';
  const q = ctx.query.q || '';
  ctx.title('Модерация клипов');
  const d = await api.get('/api/admin/shorts', { filter: filter === 'all' ? '' : filter, q, page: ctx.page });
  const reload = () => router.render();

  const searchIn = h('input.input', { value: q, placeholder: 'Поиск по названию или описанию…' });
  const goSearch = () => router.go(`/admin/shorts?filter=${filter}&q=${encodeURIComponent(searchIn.value.trim())}`);
  searchIn.onkeydown = e => e.key === 'Enter' && goSearch();

  const tabs = [
    { key: 'all', label: 'Все', href: `/admin/shorts?filter=all&q=${encodeURIComponent(q)}` },
    { key: 'hidden', label: 'Скрытые', href: `/admin/shorts?filter=hidden&q=${encodeURIComponent(q)}` },
    { key: 'blocked', label: 'Заблокированные', href: `/admin/shorts?filter=blocked&q=${encodeURIComponent(q)}` },
  ];

  const list = (d.items || []).map(s => h('div.list-item',
    h('img', { src: s.poster_url, alt: s.title, style: { width: '44px', height: '64px', objectFit: 'cover', borderRadius: '6px', flexShrink: '0' } }),
    h('div.main',
      h('div.title',
        link('/shorts/' + s.id, s.title),
        s.blocked ? h('span.badge.danger', { style: { marginLeft: '6px' } }, 'Заблокирован') : null,
        s.hidden ? h('span.badge.warn', { style: { marginLeft: '6px' } }, 'Скрыт') : null),
      h('div.small.muted', ui.userLink(s.user), ' · ', fmtDur(s.duration), ' · ', fmt.num(s.views), ' просм. · ', fmt.ago(s.time)),
      s.block_reason ? h('div.small.err', 'Причина: ', s.block_reason) : null),
    h('div.row',
      ui.btn(s.hidden ? 'Показать' : 'Скрыть', {
        small: true,
        onclick: async () => {
          await api.post(`/api/shorts/${s.id}/moderate`, { hidden: !s.hidden });
          ui.ok(s.hidden ? 'Клип возвращён в ленту' : 'Клип скрыт');
          reload();
        },
      }),
      ui.btn(s.blocked ? 'Разблокировать' : 'Заблокировать', {
        small: true, kind: s.blocked ? '' : 'danger',
        onclick: async () => {
          if (s.blocked) {
            await api.post(`/api/shorts/${s.id}/moderate`, { blocked: false });
            ui.ok('Клип разблокирован');
            reload();
          } else {
            const reason = await ui.prompt('Причина блокировки (необязательно)', '', { title: 'Заблокировать клип' });
            if (reason === null) return;
            await api.post(`/api/shorts/${s.id}/moderate`, { blocked: true, reason });
            ui.ok('Клип заблокирован');
            reload();
          }
        },
      }),
      ui.btn('', {
        ic: 'trash', small: true, kind: 'danger', title: 'Удалить навсегда',
        onclick: async () => {
          if (await ui.confirm(`Удалить клип «${s.title}» навсегда?`, { danger: true })) {
            await api.del('/api/shorts/' + s.id);
            ui.ok('Клип удалён');
            reload();
          }
        },
      }))));

  return [
    ui.breadcrumbs([{ href: '/admin', title: 'Панель управления' }, { title: 'Модерация клипов' }]),
    ui.pageHead('Модерация клипов'),
    ui.tabs(tabs, filter),
    h('div.card.pad', h('div.row.nowrap', searchIn, ui.btn('Найти', { ic: 'search', kind: 'primary', onclick: goSearch }))),
    ui.card(list.length ? h('div.list', list) : ui.empty('Клипов не найдено', 'video')),
    ui.pager(d),
  ];
}, { auth: true, access: 'shorts_block' });

// Уведомление в реальном времени о завершении обработки ffmpeg
bus.on('sse:shorts_ready', d => {
  ui.ok(`Клип «${d.title || 'Видео'}» обработан и опубликован!`);
  if (location.pathname.startsWith('/shorts')) router.render();
});

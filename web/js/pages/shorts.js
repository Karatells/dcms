import { h, api, ui, router, state, icon, link, fmt, bus } from '../core.js';

if (!document.querySelector('link[href="/css/shorts.css"]')) {
  document.head.append(h('link', { rel: 'stylesheet', href: '/css/shorts.css' }));
}

const me = () => state.user;
let globalMuted = true;
const fmtDur = s => {
  s = Math.max(0, Math.round(s || 0));
  const m = Math.floor(s / 60), r = s % 60;
  return `${m}:${r < 10 ? '0' : ''}${r}`;
};

// ───────── загрузка клипа с прогрессом
function openUploadModal(maxSec = 60, maxMB = 100, onDone) {
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

  const drop = h('div.upload-drop', { onclick: () => fileInput.click() },
    icon('upload', 26),
    h('b', 'Выберите видеофайл'),
    h('span.small.muted', `До ${maxSec} секунд · до ${maxMB} МБ · видео будет автоматически сжато на сервере`));

  fileInput.onchange = () => {
    errBox.textContent = '';
    const f = fileInput.files && fileInput.files[0];
    if (!f) return;
    if (f.size > maxMB * 1024 * 1024) {
      errBox.textContent = `Файл слишком большой (максимум ${maxMB} МБ)`;
      return;
    }
    pickedFile = f;
    if (!titleIn.value) titleIn.value = f.name.replace(/\.[^.]+$/, '').slice(0, 100);
    if (previewURL) URL.revokeObjectURL(previewURL);
    previewURL = URL.createObjectURL(f);
    const v = h('video.upload-preview', { src: previewURL, controls: true, muted: true, playsinline: true });
    v.onloadedmetadata = () => {
      if (v.duration && v.duration > maxSec + 1.5) {
        errBox.textContent = `Видео длиннее ${maxSec} сек (${Math.round(v.duration)} сек). Загрузите ролик до 1 минуты.`;
        pickedFile = null;
      }
    };
    previewBox.replaceChildren(v, h('div.small.muted.center', { style: { marginTop: '6px' } }, `${f.name} · ${fmt.size(f.size)}`));
  };

  let uploading = false;
  const submitBtn = ui.btn('Загрузить клип', {
    ic: 'upload', kind: 'primary',
    onclick: () => {
      if (uploading) return;
      if (!pickedFile) { errBox.textContent = 'Выберите видео до 1 минуты'; return; }
      errBox.textContent = '';
      uploading = true;
      submitBtn.disabled = true;
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
        submitBtn.disabled = false;
        let res = {};
        try { res = JSON.parse(xhr.responseText || '{}'); } catch {}
        if (xhr.status >= 200 && xhr.status < 300) {
          if (previewURL) URL.revokeObjectURL(previewURL);
          close();
          ui.ok('Видео загружено и отправлено в обработку (ffmpeg)');
          onDone && onDone(res.id);
        } else {
          progWrap.style.display = 'none';
          errBox.textContent = res.error || 'Ошибка загрузки';
        }
      };
      xhr.onerror = () => {
        uploading = false;
        submitBtn.disabled = false;
        progWrap.style.display = 'none';
        errBox.textContent = 'Ошибка сети при загрузке';
      };
      xhr.send(fd);
    },
  });

  const body = h('div.stack', fileInput, drop, previewBox,
    h('div.field', h('label', 'Название'), titleIn),
    h('div.field', h('label', 'Описание'), opisIn),
    progWrap, errBox);

  const close = ui.modal({
    title: 'Загрузить короткое видео',
    body,
    actions: [{ label: 'Отмена' }, submitBtn],
  });
}

// ───────── комментарии к клипу
async function openCommentsModal(s, onCountChange) {
  const listWrap = h('div.posts');
  const foot = h('div.stack-s');
  let replyTo = 0;

  async function load(page = 1) {
    listWrap.replaceChildren(ui.loading());
    const d = await api.get(`/api/shorts/${s.id}/komms`, { page });
    onCountChange && onCountChange(d.total);
    if (!d.items.length) {
      listWrap.replaceChildren(ui.empty('Комментариев пока нет', 'message-circle'));
      return;
    }
    listWrap.replaceChildren(...d.items.map(k => ui.post({
      user: k.user, time: k.time,
      body: h('div.text', { html: k.msg }),
      actions: [
        k.can_reply ? ui.btn('', { ic: 'corner-up-left', small: true, title: 'Ответить', onclick: () => { replyTo = k.user.id; input.value = `[b]${k.user.nick}[/b], `; input.focus(); } }) : null,
        k.can_delete ? ui.btn('', { ic: 'trash', small: true, title: 'Удалить', onclick: async () => { if (await ui.confirm('Удалить комментарий?', { danger: true })) { await api.del('/api/shorts/komm/' + k.id); load(page); } } }) : null,
      ].filter(Boolean),
    })));
  }

  const input = h('textarea.input', { rows: 2, placeholder: 'Напишите комментарий…', maxlength: 1024 });
  if (me()) {
    const sendBtn = ui.btn('Отправить', {
      ic: 'send', kind: 'primary', small: true,
      onclick: async () => {
        const msg = input.value.trim();
        if (!msg) return;
        sendBtn.disabled = true;
        try {
          const r = await api.post(`/api/shorts/${s.id}/komms`, { msg, reply_to: replyTo });
          input.value = ''; replyTo = 0;
          onCountChange && onCountChange(r.komms);
          await load(1);
        } finally { sendBtn.disabled = false; }
      },
    });
    foot.append(input, h('div.row', h('span.grow'), sendBtn));
  } else {
    foot.append(h('div.small.muted', link('/login', 'Войдите'), ', чтобы оставлять комментарии.'));
  }

  ui.modal({
    title: `Комментарии · ${s.title}`,
    wide: true,
    body: h('div.stack', foot, listWrap),
    actions: [{ label: 'Закрыть' }],
  });
  load(1);
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

// ───────── карточка вертикального видео в ленте
function buildShortCard(s, { onDeleted, onUpdated }) {
  const video = h('video', {
    src: s.video_url,
    poster: s.poster_url,
    loop: true,
    playsinline: true,
    muted: globalMuted,
    preload: 'metadata',
  });
  const card = h('article.short-card.paused', { dataset: { id: String(s.id) } });
  const progFill = h('div.short-prog-fill');
  const prog = h('div.short-prog', h('div.short-prog-bg', progFill));
  const muteBtn = h('button.short-mute', { title: 'Звук', 'aria-label': 'Звук' }, icon(globalMuted ? 'volume-x' : 'volume', 18));
  const viewsSpan = h('span', `${fmt.num(s.views)} просм.`);
  let viewedSent = false;

  const syncMuteIcon = () => {
    muteBtn.replaceChildren(icon(video.muted ? 'volume-x' : 'volume', 18));
  };
  muteBtn.onclick = e => {
    e.stopPropagation();
    globalMuted = !video.muted;
    document.querySelectorAll('.short-card video').forEach(v => { v.muted = globalMuted; });
    document.querySelectorAll('.short-mute').forEach(b => b.replaceChildren(icon(globalMuted ? 'volume-x' : 'volume', 18)));
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
  video.onclick = () => { if (video.paused) video.play().catch(() => {}); else video.pause(); };

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
  if (s.can_edit) {
    moreItems.push({ sep: true }, {
      label: 'Редактировать', icon: 'edit',
      onclick: () => {
        const ti = h('input.input', { value: s.title, maxlength: 100 });
        const op = h('textarea.input', { rows: 3, maxlength: 500 }, s.opis || '');
        const close = ui.modal({
          title: 'Редактировать клип',
          body: h('div.stack', h('div.field', h('label', 'Название'), ti), h('div.field', h('label', 'Описание'), op)),
          actions: [{ label: 'Отмена' }, ui.btn('Сохранить', {
            ic: 'check', kind: 'primary',
            onclick: async () => {
              const upd = await api.put('/api/shorts/' + s.id, { title: ti.value, opis: op.value });
              close(); ui.ok('Сохранено'); onUpdated && onUpdated(upd);
            },
          })],
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

  card.append(
    video,
    h('div.short-play-flash', h('span', icon('play', 28))),
    h('div.short-top-bar', h('span.short-pill', icon('video', 14), fmtDur(s.duration)), muteBtn),
    h('div.short-rail', likeBtn, kommBtn, bmBtn, shareBtn, moreBtn),
    h('div.short-info',
      h('div.short-author', ui.avatar(s.user, 32), ui.userLink(s.user)),
      h('div.short-title', s.title),
      descEl,
      h('div.short-meta', viewsSpan, h('span', '· ' + fmt.ago(s.time)))),
    prog
  );
  syncMuteIcon();
  return { card, video };
}

// ───────── плитка для режима сетки
function shortTile(s) {
  return h('a.short-tile', { href: '/shorts/' + s.id },
    h('img', { src: s.poster_url, alt: s.title, loading: 'lazy' }),
    h('span.short-tile-dur', fmtDur(s.duration)),
    h('div.short-tile-ov',
      h('div.short-tile-title', s.title),
      h('div.short-tile-meta', icon('heart', 12), fmt.num(s.likes), icon('eye', 12), fmt.num(s.views))));
}

// ───────── основная страница /shorts и /shorts/:id
async function renderShorts(ctx, startID = 0) {
  const tab = ctx.query.tab || 'new';
  const viewMode = ctx.query.view || 'feed'; // feed | grid
  ctx.title('Клипы');

  const d = await api.get('/api/shorts', { tab, page: ctx.page, start: startID || undefined });
  const reload = () => router.render();

  const tabs = [['new', 'Новые'], ['popular', 'Популярные']];
  if (me()) tabs.push(['friends', 'Друзья']);

  const topBar = h('div.shorts-top',
    h('div.tabs', ...tabs.map(([k, l]) => h('a.tab' + (tab === k ? '.active' : ''), { href: `/shorts?tab=${k}&view=${viewMode}` }, l))),
    h('div.row',
      h('a.btn.sm' + (viewMode === 'feed' ? '.primary' : ''), { href: `/shorts?tab=${tab}&view=feed`, title: 'Лента' }, icon('video', 15), ' Лента'),
      h('a.btn.sm' + (viewMode === 'grid' ? '.primary' : ''), { href: `/shorts?tab=${tab}&view=grid`, title: 'Сетка' }, icon('grid', 15), ' Сетка'),
      me() ? ui.btn('Загрузить', { ic: 'plus', kind: 'primary', small: true, onclick: () => openUploadModal(d.max_sec || 60, d.max_mb || 100, reload) }) : null));

  const out = [ui.pageHead('Клипы', null), topBar];

  // Блок обрабатываемых видео текущего пользователя
  if (d.pending && d.pending.length) {
    const pCard = h('div.card.pad.stack-s');
    d.pending.forEach(p => {
      pCard.append(h('div.row',
        p.status === 'processing' ? h('span.badge.primary', 'Обработка ffmpeg…') : h('span.badge.danger', 'Ошибка'),
        h('b', p.title),
        p.err_msg ? h('span.small.err', p.err_msg) : h('span.small.muted', 'Видео сжимается и скоро появится в ленте'),
        h('span.grow'),
        p.status === 'failed' ? ui.btn('Убрать', { ic: 'trash', small: true, onclick: async () => { await api.del('/api/shorts/' + p.id); reload(); } }) : null));
    });
    out.push(pCard);
    if (d.pending.some(p => p.status === 'processing')) {
      const tm = setTimeout(() => router.render(), 4000);
      ctx.onLeave(() => clearTimeout(tm));
    }
  }

  if (!d.items || !d.items.length) {
    out.push(ui.card(ui.empty(
      tab === 'friends' ? 'У друзей пока нет клипов' : 'Клипов пока нет — загрузите первый!',
      'video',
      me() ? ui.btn('Загрузить клип', { ic: 'upload', kind: 'primary', onclick: () => openUploadModal(d.max_sec || 60, d.max_mb || 100, reload) }) : null
    )));
    return out;
  }

  if (viewMode === 'grid' && !startID) {
    out.push(h('div.shorts-grid', ...d.items.map(shortTile)), ui.pager(d));
    return out;
  }

  // Вертикальная лента с автовоспроизведением видимого ролика
  const feed = h('div.shorts-feed');
  const cards = [];
  d.items.forEach(s => {
    const item = buildShortCard(s, {
      onDeleted: () => reload(),
      onUpdated: () => reload(),
    });
    cards.push(item);
    feed.append(item.card);
  });

  const io = new IntersectionObserver(entries => {
    entries.forEach(en => {
      const v = en.target.querySelector('video');
      if (!v) return;
      if (en.isIntersecting && en.intersectionRatio >= 0.65) {
        v.muted = globalMuted;
        v.play().catch(() => {});
      } else {
        v.pause();
      }
    });
  }, { threshold: [0.2, 0.65] });
  cards.forEach(c => io.observe(c.card));

  // Навигация с клавиатуры (стрелки вверх/вниз, пробел, M)
  const onKey = e => {
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const curIdx = cards.findIndex(c => !c.video.paused);
      const next = Math.max(0, Math.min(cards.length - 1, (curIdx < 0 ? 0 : curIdx) + (e.key === 'ArrowDown' ? 1 : -1)));
      if (cards[next]) {
        e.preventDefault();
        cards[next].card.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    } else if (e.key === ' ') {
      const active = cards.find(c => !c.video.paused) || cards[0];
      if (active) { e.preventDefault(); active.video.paused ? active.video.play().catch(() => {}) : active.video.pause(); }
    } else if (e.key.toLowerCase() === 'm') {
      globalMuted = !globalMuted;
      cards.forEach(c => { c.video.muted = globalMuted; });
      document.querySelectorAll('.short-mute').forEach(b => b.replaceChildren(icon(globalMuted ? 'volume-x' : 'volume', 18)));
    }
  };
  document.addEventListener('keydown', onKey);
  ctx.onLeave(() => {
    io.disconnect();
    document.removeEventListener('keydown', onKey);
    cards.forEach(c => c.video.pause());
  });

  out.push(feed, ui.pager(d));
  return out;
}

router.add('/shorts', ctx => renderShorts(ctx, 0));
router.add('/shorts/:id', ctx => renderShorts(ctx, +ctx.params.id));

// ───────── клипы конкретного пользователя
router.add('/shorts/user/:uid', async ctx => {
  const uid = +ctx.params.uid;
  ctx.title('Клипы пользователя');
  const d = await api.get('/api/shorts', { user: uid, page: ctx.page });
  const isMe = me() && me().id === uid;
  return [
    ui.breadcrumbs([{ href: '/shorts', title: 'Клипы' }, { href: '/id' + uid, title: 'Профиль' }, { title: 'Клипы' }]),
    ui.pageHead('Клипы пользователя', isMe ? ui.btn('Загрузить клип', { ic: 'plus', kind: 'primary', small: true, onclick: () => openUploadModal(d.max_sec || 60, d.max_mb || 100, () => router.render()) }) : null),
    d.items && d.items.length
      ? h('div.shorts-grid', ...d.items.map(shortTile))
      : ui.card(ui.empty('У пользователя пока нет клипов', 'video')),
    ui.pager(d),
  ];
});

// Уведомление в реальном времени о завершении обработки ffmpeg
bus.on('sse:shorts_ready', d => {
  ui.ok(`Клип «${d.title || 'Видео'}» обработан и опубликован!`);
  if (location.pathname.startsWith('/shorts')) router.render();
});

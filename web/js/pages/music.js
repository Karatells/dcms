import { h, api, ui, router, state, icon, fmt } from '../core.js';

const me = () => state.user;

// ───────── Глобальный непрерывный SPA мини-плеер (живёт поверх переходов между страницами)
const audio = new Audio();
let queue = [];
let curIdx = -1;
let playerBar = null;
let playBtnEl = null, titleEl = null, artistEl = null, seekEl = null, timeEl = null;

const fmtSec = s => {
  s = Math.max(0, Math.floor(s || 0));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
};

function ensureMiniPlayer() {
  if (playerBar) return;
  playBtnEl = h('button.icon-btn', {
    type: 'button', title: 'Пауза / Воспроизвести',
    onclick: () => { if (audio.paused) audio.play().catch(() => {}); else audio.pause(); },
  }, icon('play', 18));
  const prevBtn = h('button.icon-btn', { type: 'button', title: 'Предыдущий трек', onclick: () => playAt(curIdx - 1) }, icon('skip-back', 16));
  const nextBtn = h('button.icon-btn', { type: 'button', title: 'Следующий трек', onclick: () => playAt(curIdx + 1) }, icon('skip-forward', 16));
  titleEl = h('div.mp-title');
  artistEl = h('div.mp-artist');
  seekEl = h('input.mp-seek', {
    type: 'range', min: 0, max: 100, step: 0.2, value: 0,
    oninput: () => { if (audio.duration) audio.currentTime = (+seekEl.value / 100) * audio.duration; },
  });
  timeEl = h('span.mp-time', '0:00');
  const closeBtn = h('button.icon-btn', {
    type: 'button', title: 'Закрыть плеер',
    onclick: () => { audio.pause(); audio.src = ''; curIdx = -1; playerBar.remove(); playerBar = null; },
  }, icon('x', 16));

  playerBar = h('div.mini-player',
    prevBtn, playBtnEl, nextBtn,
    h('div.mp-meta', titleEl, artistEl),
    seekEl, timeEl, closeBtn);
  document.body.append(playerBar);
}

function playAt(idx) {
  if (!queue.length) return;
  if (idx < 0) idx = queue.length - 1;
  if (idx >= queue.length) idx = 0;
  curIdx = idx;
  const tr = queue[curIdx];
  ensureMiniPlayer();
  titleEl.textContent = tr.title;
  artistEl.textContent = tr.artist;
  audio.src = tr.url;
  audio.play().catch(() => {});
}

audio.addEventListener('play', () => { if (playBtnEl) playBtnEl.replaceChildren(icon('pause2', 17)); });
audio.addEventListener('pause', () => { if (playBtnEl) playBtnEl.replaceChildren(icon('play', 18)); });
audio.addEventListener('ended', () => playAt(curIdx + 1));
audio.addEventListener('timeupdate', () => {
  if (!audio.duration || !seekEl || !timeEl) return;
  seekEl.value = (audio.currentTime / audio.duration) * 100;
  timeEl.textContent = `${fmtSec(audio.currentTime)} / ${fmtSec(audio.duration)}`;
});

window.dcmsPlayer = { playQueue: (list, startIdx) => { queue = list; playAt(startIdx); } };

// ───────── Музыкальный раздел (/music)
router.add('/music', async ctx => {
  ctx.title('Музыка');
  const tab = ctx.query.tab || 'all';
  const q = ctx.query.q || '';
  const params = { page: ctx.page, q: q || undefined };
  if (tab === 'my' && me()) params.user = me().id;
  const d = await api.get('/api/music', params);
  const items = d.items || [];
  const reload = () => router.render();

  const openUpload = () => {
    const fileIn = h('input.input', { type: 'file', accept: 'audio/*,.mp3,.ogg,.wav,.m4a,.flac' });
    const titleIn = h('input.input', { placeholder: 'Название трека', maxlength: 100 });
    const artistIn = h('input.input', { placeholder: 'Исполнитель', maxlength: 80 });
    fileIn.onchange = () => {
      const f = fileIn.files && fileIn.files[0];
      if (f && !titleIn.value) titleIn.value = f.name.replace(/\.[^.]+$/, '').slice(0, 100);
    };
    let modal;
    modal = ui.modal({
      title: 'Загрузить трек',
      body: h('div.stack',
        h('div.field', h('label', 'Аудиофайл (MP3, OGG, WAV, M4A, FLAC, до 25 МБ)'), fileIn),
        h('div.field', h('label', 'Название'), titleIn),
        h('div.field', h('label', 'Исполнитель'), artistIn)),
      actions: [
        { label: 'Отмена' },
        {
          label: 'Загрузить', kind: 'primary',
          onclick: async () => {
            const f = fileIn.files && fileIn.files[0];
            if (!f) { ui.toast('Выберите аудиофайл', 'warn'); return false; }
            const fd = new FormData();
            fd.append('file', f);
            fd.append('title', titleIn.value.trim());
            fd.append('artist', artistIn.value.trim());
            const res = await fetch('/api/music', { method: 'POST', body: fd, headers: { 'X-DCMS': '1' } });
            if (!res.ok) { const e = await res.json().catch(() => ({})); ui.toast(e.error || 'Ошибка загрузки', 'err'); return false; }
            modal.close();
            ui.ok('Трек загружен!');
            reload();
            return false;
          },
        },
      ],
    });
  };

  const searchIn = h('input.input', { value: q, placeholder: 'Поиск треков и исполнителей…' });
  const searchForm = h('form.row.nowrap', {
    onsubmit: e => { e.preventDefault(); router.go('/music?tab=' + encodeURIComponent(tab) + (searchIn.value.trim() ? '&q=' + encodeURIComponent(searchIn.value.trim()) : '')); },
  }, searchIn, ui.btn('Найти', { ic: 'search', small: true, type: 'submit' }));

  const tabs = h('div.tabs',
    h('a.tab' + (tab === 'all' ? '.active' : ''), { href: '/music?tab=all' }, icon('music', 15), 'Вся музыка'),
    me() ? h('a.tab' + (tab === 'my' ? '.active' : ''), { href: '/music?tab=my' }, icon('user', 15), 'Моя музыка') : null);

  const rows = items.map((tr, idx) => h('div.list-item',
    h('button.icon-btn', {
      type: 'button', title: 'Воспроизвести',
      style: { background: 'var(--accent-soft)', color: 'var(--accent)' },
      onclick: () => { queue = items; playAt(idx); },
    }, icon('play', 18)),
    h('div.main',
      h('div.title', tr.title),
      h('div.meta', tr.artist, ' · ', fmt.num(tr.plays), ' просл. · ', fmt.size(tr.size))),
    tr.can_delete ? ui.btn('', {
      ic: 'trash', small: true, kind: 'danger', title: 'Удалить',
      onclick: async () => {
        if (await ui.confirm('Удалить трек?', { danger: true })) {
          await api.del('/api/music/' + tr.id);
          ui.ok('Трек удалён');
          reload();
        }
      },
    }) : null));

  return [
    ui.pageHead('Музыка', me() ? ui.btn('Загрузить трек', { ic: 'upload', kind: 'primary', small: true, onclick: openUpload }) : null),
    tabs,
    searchForm,
    ui.card(rows.length ? h('div.list', ...rows) : ui.empty('Треков пока нет — загрузите первый!', 'music')),
    ui.pager(d),
  ];
});

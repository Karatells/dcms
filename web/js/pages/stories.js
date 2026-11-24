import { h, api, ui, state, icon, fmt, router } from '../core.js';

const GRADIENTS = [
  'linear-gradient(135deg,#6366f1,#ec4899)',
  'linear-gradient(135deg,#0ea5e9,#10b981)',
  'linear-gradient(135deg,#f59e0b,#ef4444)',
  'linear-gradient(135deg,#8b5cf6,#06b6d4)',
  'linear-gradient(135deg,#1e293b,#475569)',
];

function openCreateStoryModal(onDone) {
  let file = null;
  let bg = GRADIENTS[0];
  const fileIn = h('input', { type: 'file', accept: 'image/jpeg,image/png,image/webp,image/gif', style: { display: 'none' } });
  const preview = h('div.story-preview', { style: { background: bg } }, h('span.muted', { style: { color: '#fff', opacity: '.9' } }, 'Выберите фото или напишите текст ниже'));
  const capIn = h('textarea.input', { rows: 2, maxlength: 200, placeholder: 'Текст истории или подпись к фото…' });

  const updatePreview = () => {
    if (file) {
      const url = URL.createObjectURL(file);
      preview.style.background = '#000';
      preview.replaceChildren(h('img', { src: url, style: { width: '100%', height: '100%', objectFit: 'cover' } }),
        capIn.value.trim() ? h('div.story-preview-cap', capIn.value.trim()) : null);
    } else {
      preview.style.background = bg;
      preview.replaceChildren(h('div.story-preview-txt', capIn.value.trim() || 'Текстовая история на 24 часа'));
    }
  };

  fileIn.onchange = () => { file = fileIn.files[0] || null; updatePreview(); };
  capIn.oninput = updatePreview;

  const swatches = h('div.row', ...GRADIENTS.map(g => h('button.story-swatch', {
    type: 'button', style: { background: g },
    onclick: () => { file = null; fileIn.value = ''; bg = g; updatePreview(); },
  })));

  const modal = ui.modal({
    title: 'Новая история (24 часа)',
    body: h('div.stack',
      preview,
      h('div.row',
        ui.btn('Выбрать фото', { ic: 'image', small: true, onclick: () => fileIn.click() }),
        fileIn,
        h('span.small.muted', 'или фон:'),
        swatches),
      capIn),
    actions: [
      { label: 'Отмена' },
      {
        label: 'Опубликовать', kind: 'primary',
        onclick: async () => {
          if (!file && !capIn.value.trim()) { ui.toast('Выберите фото или введите текст', 'warn'); return false; }
          await api.upload('/api/stories', { ...(file ? { file } : {}), caption: capIn.value.trim(), bg });
          modal.close();
          ui.ok('История опубликована на 24 часа');
          onDone && onDone();
          return false;
        },
      },
    ],
  });
  updatePreview();
}

function openStoryViewer(groups, startGroupIdx, onRefresh) {
  let gIdx = startGroupIdx;
  let sIdx = 0;
  let timer = null;
  const DURATION = 6000;

  const overlay = h('div.story-viewer');
  const barsEl = h('div.story-bars');
  const headEl = h('div.story-head');
  const stageEl = h('div.story-stage');
  const footEl = h('div.story-foot');

  const close = () => {
    clearTimeout(timer);
    document.removeEventListener('keydown', onKey);
    overlay.remove();
  };

  const next = () => {
    const g = groups[gIdx];
    if (sIdx + 1 < g.stories.length) { sIdx++; render(); }
    else if (gIdx + 1 < groups.length) { gIdx++; sIdx = 0; render(); }
    else close();
  };
  const prev = () => {
    if (sIdx > 0) { sIdx--; render(); }
    else if (gIdx > 0) { gIdx--; sIdx = groups[gIdx].stories.length - 1; render(); }
  };

  const onKey = e => {
    if (e.key === 'Escape') close();
    else if (e.key === 'ArrowRight' || e.key === ' ') { e.preventDefault(); next(); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); prev(); }
  };
  document.addEventListener('keydown', onKey);

  const render = () => {
    clearTimeout(timer);
    const g = groups[gIdx];
    const st = g.stories[sIdx];
    if (!st) return close();

    if (state.user && !st.seen) {
      st.seen = true;
      api.post(`/api/stories/${st.id}/view`, {}, { silent: true }).then(r => { if (r && r.views) st.views = r.views; }).catch(() => {});
    }

    barsEl.replaceChildren(...g.stories.map((_, idx) => {
      const fill = h('div.story-bar-fill');
      if (idx < sIdx) fill.style.width = '100%';
      else if (idx === sIdx) {
        fill.style.width = '0%';
        fill.style.transition = `width ${DURATION}ms linear`;
        requestAnimationFrame(() => requestAnimationFrame(() => { fill.style.width = '100%'; }));
      }
      return h('div.story-bar', fill);
    }));

    headEl.replaceChildren(
      h('a.row.grow', { href: '/id' + g.user.id, style: { color: '#fff', textDecoration: 'none' }, onclick: close },
        ui.avatar(g.user, 34),
        h('div',
          h('b', { style: { fontSize: '.9rem' } }, g.user.nick),
          h('div', { style: { fontSize: '.74rem', opacity: '.8' } }, fmt.ago(st.time)))),
      st.is_owner ? h('button.icon-btn', {
        title: 'Кто посмотрел', style: { color: '#fff' },
        onclick: async e => {
          e.stopPropagation();
          clearTimeout(timer);
          const v = await api.get(`/api/stories/${st.id}/viewers`);
          ui.modal({
            title: `Просмотры (${v.items.length})`,
            body: v.items.length
              ? h('div.list', ...v.items.map(x => h('a.list-item', { href: '/id' + x.user.id, onclick: close }, ui.avatar(x.user, 32), h('div.main', h('b', x.user.nick), h('div.small.muted', fmt.ago(x.time))))))
              : h('div.muted.small', 'Пока никто не посмотрел'),
            actions: [{ label: 'Закрыть' }],
          });
        },
      }, icon('eye', 18), h('span', { style: { fontSize: '.78rem', marginLeft: '3px' } }, String(st.views || 0))) : null,
      st.can_delete ? h('button.icon-btn', {
        title: 'Удалить историю', style: { color: '#fff' },
        onclick: async e => {
          e.stopPropagation();
          clearTimeout(timer);
          if (await ui.confirm('Удалить эту историю?', { danger: true })) {
            await api.del(`/api/stories/${st.id}`);
            ui.ok('История удалена');
            close();
            onRefresh && onRefresh();
          }
        },
      }, icon('trash', 18)) : null,
      h('button.icon-btn', { title: 'Закрыть', style: { color: '#fff' }, onclick: e => { e.stopPropagation(); close(); } }, icon('x', 22)));

    if (st.media) {
      stageEl.style.background = '#000';
      stageEl.replaceChildren(h('img.story-img', { src: st.media, alt: '' }));
      footEl.replaceChildren(st.caption ? h('div.story-cap', st.caption) : null);
    } else {
      stageEl.style.background = st.bg || GRADIENTS[0];
      stageEl.replaceChildren(h('div.story-text-only', st.caption));
      footEl.replaceChildren();
    }

    timer = setTimeout(next, DURATION);
  };

  const tapL = h('div.story-tap-l', { onclick: prev });
  const tapR = h('div.story-tap-r', { onclick: next });
  const box = h('div.story-box', barsEl, headEl, stageEl, tapL, tapR, footEl);
  overlay.append(box);
  overlay.onclick = e => { if (e.target === overlay) close(); };
  document.body.append(overlay);
  render();
}

export async function renderStoriesBar() {
  const wrap = h('div.stories-wrap');
  const load = async () => {
    const d = await api.get('/api/stories', null, { silent: true }).catch(() => ({ groups: [] }));
    const groups = d.groups || [];
    if (!state.user && !groups.length) { wrap.replaceChildren(); return; }

    const items = [];
    if (state.user) {
      items.push(h('button.story-item', { type: 'button', onclick: () => openCreateStoryModal(load) },
        h('div.story-ring.add',
          ui.avatar(state.user, 52),
          h('span.story-plus', '+')),
        h('span.story-name', 'Добавить')));
    }
    groups.forEach((g, idx) => {
      items.push(h('button.story-item', { type: 'button', onclick: () => openStoryViewer(groups, idx, load) },
        h('div.story-ring' + (g.has_unseen ? '.unseen' : '.seen'), ui.avatar(g.user, 52)),
        h('span.story-name', g.is_me ? 'Ваша' : g.user.nick)));
    });
    wrap.replaceChildren(h('div.card.stories-bar', ...items));
  };
  await load();
  return wrap;
}

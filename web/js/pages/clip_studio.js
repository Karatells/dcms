// Уникальная мобильная студия создания и редактирования клипов для Android-приложения.
// Поддерживает: съёмку с фронтальной/основной камеры, выбор из галереи, предпросмотр,
// стикеры-эмодзи, текстовые плашки, цветовые фильтры, обрезку по времени и запекание правок в видео.
import { h, ui, icon } from '../core.js';

const FILTERS = [
  { id: 'none', name: 'Обычный', css: 'none' },
  { id: 'warm', name: 'Тёплый', css: 'sepia(0.28) saturate(1.35) brightness(1.04)' },
  { id: 'cool', name: 'Холодный', css: 'hue-rotate(18deg) saturate(1.15) contrast(1.06)' },
  { id: 'vivid', name: 'Яркий', css: 'contrast(1.18) saturate(1.45)' },
  { id: 'bw', name: 'Ч/Б', css: 'grayscale(1) contrast(1.12)' },
  { id: 'vintage', name: 'Винтаж', css: 'sepia(0.55) contrast(0.92) brightness(0.96)' },
];

const STICKERS = [
  '🔥', '❤️', '😂', '😍', '🤯', '🎉', '👏', '💯',
  '⚡', '👑', '🌟', '🎵', '🚀', '💎', '😎', '🫶',
  '🎬', '🐾', '🍕', '☕', '🌈', '💀', '👀', '✨',
];

const TEXT_STYLES = [
  { id: 'white', label: 'Белый', fg: '#ffffff', bg: '' },
  { id: 'dark', label: 'Плашка', fg: '#ffffff', bg: 'rgba(15,18,25,0.78)' },
  { id: 'accent', label: 'Неон', fg: '#ffffff', bg: '#6366f1' },
];

const BLANK_POSTER = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';

const fmtSec = s => {
  s = Math.max(0, Math.floor(s || 0));
  const m = Math.floor(s / 60), r = s % 60;
  return `${m}:${r < 10 ? '0' : ''}${r}`;
};

function pickRecorderMime() {
  const list = ['video/webm;codecs=vp8,opus', 'video/webm;codecs=vp9,opus', 'video/webm', 'video/mp4'];
  for (const m of list) {
    if (window.MediaRecorder && MediaRecorder.isTypeSupported(m)) return m;
  }
  return '';
}

export function openAndroidClipStudio(maxSec = 60, maxMB = 100, onDone, opts = {}) {
  const isStory = opts.target === 'story';
  let mode = 'camera'; // camera | edit
  let facing = 'user';
  let stream = null;
  let recorder = null;
  let recChunks = [];
  let recStartAt = 0;
  let recTick = null;
  let isRecording = false;

  let sourceBlob = null;
  let sourceKind = 'video'; // video | image
  let sourceName = 'clip.webm';
  let sourceURL = '';
  let vidDuration = 0;
  let trimStart = 0;
  let trimEnd = 0;
  let filterIdx = 0;
  let muteOutput = false;
  let overlays = []; // { id, kind:'emoji'|'text', text, x:0..1, y:0..1, size:1..3, style:'white'|'dark'|'accent' }
  let selectedOverlayId = 0;
  let nextOvlId = 1;
  let activeTool = ''; // '' | 'stickers' | 'text' | 'filters' | 'trim'
  let busyPublish = false;

  const fileInput = h('input', {
    type: 'file',
    accept: isStory ? 'video/*,image/*,.mp4,.mov,.webm,.jpg,.jpeg,.png,.webp,.gif' : 'video/*,.mp4,.mov,.webm,.mkv,.3gp',
    style: { display: 'none' },
  });
  const camCaptureInput = h('input', {
    type: 'file',
    accept: 'video/*',
    capture: 'environment',
    style: { display: 'none' },
  });
  camCaptureInput.onchange = () => {
    const f = camCaptureInput.files && camCaptureInput.files[0];
    camCaptureInput.value = '';
    if (f) openEditorWithBlob(f, f.name || ('clip-' + Date.now() + '.mp4'));
  };
  const root = h('div.acs-root');
  document.body.append(root);
  document.body.classList.add('acs-open');

  const stopCamera = () => {
    if (recTick) { clearInterval(recTick); recTick = null; }
    if (recorder && recorder.state !== 'inactive') { try { recorder.stop(); } catch {} }
    recorder = null;
    isRecording = false;
    if (stream) { stream.getTracks().forEach(t => t.stop()); stream = null; }
  };

  const close = () => {
    stopCamera();
    if (sourceURL) { URL.revokeObjectURL(sourceURL); sourceURL = ''; }
    document.body.classList.remove('acs-open');
    root.remove();
  };

  const openEditorWithBlob = (blob, name) => {
    const isImg = (blob.type && blob.type.startsWith('image/')) || /\.(jpe?g|png|webp|gif)$/i.test(name || '');
    const limMB = isImg ? 10 : maxMB;
    if (blob.size > limMB * 1024 * 1024) {
      ui.toast(`Файл слишком большой (максимум ${limMB} МБ)`, 'error');
      return;
    }
    stopCamera();
    if (sourceURL) URL.revokeObjectURL(sourceURL);
    sourceBlob = blob;
    sourceKind = isImg ? 'image' : 'video';
    sourceName = name || (isImg ? 'story.png' : 'clip.webm');
    sourceURL = URL.createObjectURL(blob);
    overlays = [];
    selectedOverlayId = 0;
    activeTool = '';
    muteOutput = false;
    trimStart = 0;
    trimEnd = 0;
    vidDuration = 0;
    mode = 'edit';
    render();
  };

  fileInput.onchange = () => {
    const f = fileInput.files && fileInput.files[0];
    fileInput.value = '';
    if (f) openEditorWithBlob(f, f.name);
  };

  // ─────────────────────────────── ЭКРАН 1: КАМЕРА
  async function renderCamera() {
    const progFill = h('div.acs-prog-fill');
    const timerEl = h('span.acs-pill', `0:00 / ${fmtSec(maxSec)}`);
    const filterNameEl = h('span.acs-pill', FILTERS[filterIdx].name);
    const camVid = h('video.acs-video', { autoplay: true, muted: true, playsinline: true, poster: BLANK_POSTER });
    camVid.setAttribute('playsinline', '');
    camVid.setAttribute('webkit-playsinline', '');
    camVid.setAttribute('muted', '');
    const errBox = h('div.acs-cam-err', { style: { display: 'none' } });

    const applyCamVisuals = () => {
      camVid.style.filter = FILTERS[filterIdx].css;
      camVid.style.transform = facing === 'user' ? 'scaleX(-1)' : 'none';
      filterNameEl.textContent = FILTERS[filterIdx].name;
    };

    const openCameraStream = async () => {
      const attempts = [
        { video: { facingMode: { ideal: facing } }, audio: true },
        { video: { facingMode: { ideal: facing } }, audio: false },
        { video: true, audio: true },
        { video: true, audio: false },
      ];
      let lastErr = null;
      for (const c of attempts) {
        try {
          return await navigator.mediaDevices.getUserMedia(c);
        } catch (e) {
          lastErr = e;
        }
      }
      throw lastErr || new Error('Camera unavailable');
    };

    const startCam = async () => {
      stopCamera();
      errBox.style.display = 'none';
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        errBox.style.display = '';
        errBox.replaceChildren(
          h('b', { style: { fontSize: '1.08rem' } }, isStory ? 'Снять историю или выбрать файл' : 'Снять клип или выбрать видео'),
          h('p', { class: 'small muted', style: { margin: 0, maxWidth: '300px' } }, 'Снимите видео системной камерой телефона — оно сразу откроется в редакторе со стикерами, текстом и фильтрами.'),
          h('div.row',
            h('button.btn.primary', { type: 'button', onclick: () => camCaptureInput.click() }, icon('video', 16), ' Снять на камеру'),
            h('button.btn', { type: 'button', onclick: () => fileInput.click() }, icon('folder', 16), ' Галерея')));
        return;
      }
      try {
        stream = await openCameraStream();
        camVid.srcObject = stream;
        applyCamVisuals();
        await camVid.play().catch(() => {});
      } catch {
        errBox.style.display = '';
        errBox.replaceChildren(
          h('b', { style: { fontSize: '1.08rem' } }, isStory ? 'Создать историю' : 'Создать клип'),
          h('p', { class: 'small muted', style: { margin: 0, maxWidth: '300px' } }, 'Встроенная камера недоступна (часто при самоподписанном сертификате). Используйте системную камеру телефона — видео сразу откроется в редакторе!'),
          h('div.row',
            h('button.btn.primary', { type: 'button', onclick: () => camCaptureInput.click() }, icon('video', 16), ' Снять на камеру'),
            h('button.btn', { type: 'button', onclick: () => fileInput.click() }, icon('folder', 16), ' Галерея')));
      }
    };

    const recBtn = h('button.acs-rec-btn', { type: 'button', 'aria-label': 'Записать видео' }, h('span'));
    const toggleRec = () => {
      if (isRecording) {
        isRecording = false;
        recBtn.classList.remove('recording');
        if (recTick) { clearInterval(recTick); recTick = null; }
        if (recorder && recorder.state !== 'inactive') recorder.stop();
        return;
      }
      if (!stream) return ui.toast('Камера ещё не готова', 'warn');
      recChunks = [];
      const mime = pickRecorderMime();
      try {
        recorder = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
      } catch {
        recorder = new MediaRecorder(stream);
      }
      recorder.ondataavailable = e => { if (e.data && e.data.size > 0) recChunks.push(e.data); };
      recorder.onstop = () => {
        const blob = new Blob(recChunks, { type: mime || 'video/webm' });
        if (blob.size > 1000) {
          openEditorWithBlob(blob, 'clip-' + Date.now() + '.webm');
        }
      };
      recorder.start(250);
      isRecording = true;
      recStartAt = Date.now();
      recBtn.classList.add('recording');
      recTick = setInterval(() => {
        const sec = (Date.now() - recStartAt) / 1000;
        timerEl.textContent = `${fmtSec(sec)} / ${fmtSec(maxSec)}`;
        progFill.style.width = Math.min(100, (sec / maxSec) * 100) + '%';
        if (sec >= maxSec) toggleRec();
      }, 100);
    };
    recBtn.onclick = toggleRec;

    const topBar = h('div.acs-top',
      h('button.acs-circle', { type: 'button', onclick: close, title: 'Закрыть' }, icon('x', 20)),
      timerEl,
      h('div.acs-top-right',
        h('button.acs-circle', {
          type: 'button', title: 'Переключить фильтр',
          onclick: () => { filterIdx = (filterIdx + 1) % FILTERS.length; applyCamVisuals(); },
        }, icon('star', 19)),
        h('button.acs-circle', {
          type: 'button', title: 'Переключить камеру',
          onclick: () => { if (isRecording) return; facing = facing === 'user' ? 'environment' : 'user'; startCam(); },
        }, icon('rotate-ccw', 19))));

    const bottomBar = h('div.acs-cam-foot',
      h('button.acs-foot-btn', { type: 'button', onclick: () => { if (!isRecording) fileInput.click(); } },
        icon('folder', 22), h('span', 'Галерея')),
      recBtn,
      h('div.acs-foot-btn', { style: { opacity: .85 } }, filterNameEl));

    root.replaceChildren(
      fileInput,
      camCaptureInput,
      h('div.acs-prog', progFill),
      h('div.acs-stage', camVid, errBox),
      topBar,
      bottomBar);

    startCam();
  }

  // ─────────────────────────────── ЭКРАН 2: РЕДАКТОР КЛИПА / ИСТОРИИ
  function renderEditor() {
    const isImg = sourceKind === 'image';
    const mediaEl = isImg
      ? h('img.acs-video', { src: sourceURL, alt: '' })
      : h('video.acs-video', { src: sourceURL, autoplay: true, loop: true, playsinline: true, poster: BLANK_POSTER });
    if (!isImg) {
      mediaEl.setAttribute('playsinline', '');
      mediaEl.setAttribute('webkit-playsinline', '');
      mediaEl.muted = muteOutput;
    }
    mediaEl.style.filter = FILTERS[filterIdx].css;
    const vid = mediaEl;

    const stage = h('div.acs-stage');
    const ovlLayer = h('div.acs-ovl-layer');
    const playIcon = h('div.acs-play-mark', icon('play', 36));
    const drawer = h('div.acs-drawer', { style: { display: 'none' } });
    const muteBtn = !isImg ? h('button.acs-side-btn', {
      type: 'button',
      onclick: () => {
        muteOutput = !muteOutput;
        vid.muted = muteOutput;
        muteBtn.replaceChildren(icon(muteOutput ? 'volume-x' : 'volume', 20), h('span', muteOutput ? 'Без звука' : 'Звук'));
      },
    }, icon(muteOutput ? 'volume-x' : 'volume', 20), h('span', muteOutput ? 'Без звука' : 'Звук')) : null;

    if (!isImg) {
      vid.onloadedmetadata = () => {
        vidDuration = Math.min(vid.duration || 0, maxSec);
        if (!trimEnd || trimEnd > vidDuration) trimEnd = vidDuration;
        if (vid.duration > maxSec + 1.5) {
          ui.toast(`Видео длиннее ${maxSec} сек — обрежьте нужный фрагмент`, 'info');
        }
        vid.currentTime = trimStart;
        vid.play().catch(() => playIcon.classList.add('show'));
      };
      vid.ontimeupdate = () => {
        if (trimEnd > 0 && vid.currentTime >= trimEnd) {
          vid.currentTime = trimStart;
        }
      };
      vid.onclick = () => {
        selectedOverlayId = 0;
        drawOverlays();
        if (vid.paused) { vid.play().catch(() => {}); playIcon.classList.remove('show'); }
        else { vid.pause(); playIcon.classList.add('show'); }
      };
    } else {
      mediaEl.onclick = () => { selectedOverlayId = 0; drawOverlays(); };
    }

    const trashZone = h('div.acs-trash-zone', { style: { display: 'none' } }, icon('trash', 22), h('span', 'Перетащите сюда для удаления'));

    const updateSelectionClasses = () => {
      ovlLayer.querySelectorAll('.acs-ovl').forEach(node => {
        node.classList.toggle('selected', Number(node.dataset.id) === selectedOverlayId);
      });
    };

    const drawOverlays = () => {
      ovlLayer.replaceChildren(...overlays.map(o => {
        if (!o.scale) o.scale = 1;
        const isSel = o.id === selectedOverlayId;
        const st = TEXT_STYLES.find(s => s.id === o.style) || TEXT_STYLES[0];
        const baseFont = o.kind === 'emoji' ? '3.1rem' : '1.3rem';
        const el = h('div.acs-ovl' + (isSel ? '.selected' : '') + (o.kind === 'text' ? '.is-text' : ''), {
          'data-id': String(o.id),
          style: {
            left: (o.x * 100) + '%',
            top: (o.y * 100) + '%',
            fontSize: baseFont,
            transform: `translate(-50%,-50%) scale(${o.scale})`,
            color: o.kind === 'text' ? st.fg : undefined,
            background: o.kind === 'text' && st.bg ? st.bg : undefined,
          },
        }, o.text);

        // Мультитач: 1 палец = перемещение, 2 пальца = щипок (увеличение/уменьшение) + перемещение
        const pointers = new Map();
        let startCenter = null, startDist = 0, origX = o.x, origY = o.y, origScale = o.scale;

        const getMetrics = () => {
          const pts = [...pointers.values()];
          if (pts.length === 1) return { cx: pts[0].x, cy: pts[0].y, dist: 0 };
          const cx = (pts[0].x + pts[1].x) / 2;
          const cy = (pts[0].y + pts[1].y) / 2;
          const dist = Math.hypot(pts[1].x - pts[0].x, pts[1].y - pts[0].y);
          return { cx, cy, dist };
        };

        el.addEventListener('pointerdown', e => {
          e.stopPropagation();
          e.preventDefault();
          selectedOverlayId = o.id;
          updateSelectionClasses();
          el.setPointerCapture(e.pointerId);
          pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
          const m = getMetrics();
          startCenter = { x: m.cx, y: m.cy };
          startDist = m.dist;
          origX = o.x; origY = o.y; origScale = o.scale;
          trashZone.style.display = 'flex';
        });

        el.addEventListener('pointermove', e => {
          if (!pointers.has(e.pointerId)) return;
          e.preventDefault();
          pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
          const rect = ovlLayer.getBoundingClientRect();
          if (!rect.width || !rect.height) return;
          const m = getMetrics();
          o.x = Math.max(0.06, Math.min(0.94, origX + (m.cx - startCenter.x) / rect.width));
          o.y = Math.max(0.06, Math.min(0.94, origY + (m.cy - startCenter.y) / rect.height));
          if (pointers.size >= 2 && startDist > 12 && m.dist > 12) {
            o.scale = Math.max(0.45, Math.min(3.2, origScale * (m.dist / startDist)));
          }
          el.style.left = (o.x * 100) + '%';
          el.style.top = (o.y * 100) + '%';
          el.style.transform = `translate(-50%,-50%) scale(${o.scale})`;
          trashZone.classList.toggle('hot', o.y > 0.84);
        });

        const endPointer = e => {
          if (!pointers.has(e.pointerId)) return;
          pointers.delete(e.pointerId);
          if (pointers.size === 0) {
            trashZone.style.display = 'none';
            trashZone.classList.remove('hot');
            if (o.y > 0.84) {
              overlays = overlays.filter(x => x.id !== o.id);
              selectedOverlayId = 0;
              drawOverlays();
            }
          } else {
            // остался 1 палец — переинициализируем базу для плавного перехода
            const m = getMetrics();
            startCenter = { x: m.cx, y: m.cy };
            startDist = 0;
            origX = o.x; origY = o.y; origScale = o.scale;
          }
        };
        el.addEventListener('pointerup', endPointer);
        el.addEventListener('pointercancel', endPointer);
        // Поддержка колёсика мыши на случай теста с ПК
        el.addEventListener('wheel', e => {
          e.preventDefault();
          o.scale = Math.max(0.45, Math.min(3.2, (o.scale || 1) * (e.deltaY < 0 ? 1.08 : 0.92)));
          el.style.transform = `translate(-50%,-50%) scale(${o.scale})`;
        }, { passive: false });
        return el;
      }));
    };

    // Позволяем делать pinch-to-zoom двумя пальцами даже если второй палец лёг рядом со стикером
    const stagePointers = new Map();
    let stagePinchDist = 0, stagePinchScale = 1;
    stage.addEventListener('pointerdown', e => {
      if (!selectedOverlayId) return;
      stagePointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (stagePointers.size === 2) {
        const pts = [...stagePointers.values()];
        stagePinchDist = Math.hypot(pts[1].x - pts[0].x, pts[1].y - pts[0].y);
        const o = overlays.find(x => x.id === selectedOverlayId);
        stagePinchScale = o ? (o.scale || 1) : 1;
      }
    });
    stage.addEventListener('pointermove', e => {
      if (!stagePointers.has(e.pointerId)) return;
      stagePointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
      if (stagePointers.size === 2 && stagePinchDist > 12 && selectedOverlayId) {
        const pts = [...stagePointers.values()];
        const d = Math.hypot(pts[1].x - pts[0].x, pts[1].y - pts[0].y);
        const o = overlays.find(x => x.id === selectedOverlayId);
        if (o && d > 12) {
          o.scale = Math.max(0.45, Math.min(3.2, stagePinchScale * (d / stagePinchDist)));
          const node = ovlLayer.querySelector(`.acs-ovl[data-id="${o.id}"]`);
          if (node) node.style.transform = `translate(-50%,-50%) scale(${o.scale})`;
        }
      }
    });
    const clearStagePtr = e => { stagePointers.delete(e.pointerId); if (stagePointers.size < 2) stagePinchDist = 0; };
    stage.addEventListener('pointerup', clearStagePtr);
    stage.addEventListener('pointercancel', clearStagePtr);

    const openTool = tool => {
      activeTool = activeTool === tool ? '' : tool;
      if (!activeTool) { drawer.style.display = 'none'; return; }
      drawer.style.display = '';
      if (activeTool === 'stickers') {
        drawer.replaceChildren(
          h('div.acs-drawer-head', h('b', 'Стикеры и эмодзи'), h('button.acs-mini', { type: 'button', onclick: () => openTool('') }, 'Готово')),
          h('div.acs-sticker-grid', STICKERS.map(em => h('button.acs-sticker-btn', {
            type: 'button',
            onclick: () => {
              const id = nextOvlId++;
              overlays.push({ id, kind: 'emoji', text: em, x: 0.5, y: 0.45, scale: 1, style: 'white' });
              selectedOverlayId = id;
              drawOverlays();
            },
          }, em))));
      } else if (activeTool === 'text') {
        let styleId = 'dark';
        const inp = h('input.input', { placeholder: 'Введите надпись на видео…', maxlength: 60 });
        const styleBtns = TEXT_STYLES.map(st => {
          const b = h('button.acs-mini' + (st.id === styleId ? '.active' : ''), {
            type: 'button',
            onclick: () => {
              styleId = st.id;
              styleBtns.forEach(x => x.classList.remove('active'));
              b.classList.add('active');
            },
          }, st.label);
          return b;
        });
        const addText = () => {
          const t = inp.value.trim();
          if (!t) return;
          const id = nextOvlId++;
          overlays.push({ id, kind: 'text', text: t, x: 0.5, y: 0.35, scale: 1, style: styleId });
          selectedOverlayId = id;
          inp.value = '';
          drawOverlays();
          openTool('');
        };
        drawer.replaceChildren(
          h('div.acs-drawer-head', h('b', 'Текст на видео'), h('button.acs-mini', { type: 'button', onclick: () => openTool('') }, 'Закрыть')),
          h('div.row', ...styleBtns),
          h('div.row.nowrap', inp, h('button.btn.primary.sm', { type: 'button', onclick: addText }, 'Добавить')));
        setTimeout(() => inp.focus(), 40);
      } else if (activeTool === 'filters') {
        drawer.replaceChildren(
          h('div.acs-drawer-head', h('b', 'Фильтры'), h('button.acs-mini', { type: 'button', onclick: () => openTool('') }, 'Готово')),
          h('div.acs-filter-row', FILTERS.map((f, idx) => h('button.acs-filter-chip' + (idx === filterIdx ? '.active' : ''), {
            type: 'button',
            onclick: () => { filterIdx = idx; vid.style.filter = f.css; openTool('filters'); activeTool = 'filters'; openTool('filters'); },
          }, f.name))));
        // перерисуем активный чип без закрытия
        drawer.querySelectorAll('.acs-filter-chip').forEach((btn, idx) => {
          btn.onclick = () => {
            filterIdx = idx;
            vid.style.filter = FILTERS[idx].css;
            drawer.querySelectorAll('.acs-filter-chip').forEach((x, j) => x.classList.toggle('active', j === idx));
          };
        });
      } else if (activeTool === 'trim') {
        const dur = Math.max(1, vidDuration || vid.duration || 1);
        if (!trimEnd || trimEnd > dur) trimEnd = Math.min(dur, maxSec);
        const lbl = h('div.small.muted', `Фрагмент: ${fmtSec(trimStart)} — ${fmtSec(trimEnd)} (${Math.max(1, Math.round(trimEnd - trimStart))} сек.)`);
        const rStart = h('input', { type: 'range', min: 0, max: dur.toFixed(1), step: 0.5, value: trimStart.toFixed(1) });
        const rEnd = h('input', { type: 'range', min: 0.5, max: dur.toFixed(1), step: 0.5, value: trimEnd.toFixed(1) });
        const syncTrim = () => {
          trimStart = Math.min(+rStart.value, +rEnd.value - 0.5);
          trimEnd = Math.max(+rEnd.value, trimStart + 0.5);
          if (trimEnd - trimStart > maxSec) trimEnd = trimStart + maxSec;
          rStart.value = trimStart.toFixed(1);
          rEnd.value = trimEnd.toFixed(1);
          lbl.textContent = `Фрагмент: ${fmtSec(trimStart)} — ${fmtSec(trimEnd)} (${Math.max(1, Math.round(trimEnd - trimStart))} сек.)`;
          vid.currentTime = trimStart;
        };
        rStart.oninput = syncTrim;
        rEnd.oninput = syncTrim;
        drawer.replaceChildren(
          h('div.acs-drawer-head', h('b', 'Обрезка видео'), h('button.acs-mini', { type: 'button', onclick: () => openTool('') }, 'Готово')),
          lbl,
          h('div.acs-trim-row', h('span', 'Начало'), rStart),
          h('div.acs-trim-row', h('span', 'Конец'), rEnd));
      }
    };

    const topBar = h('div.acs-top',
      h('button.acs-circle', { type: 'button', title: 'Снять заново', onclick: () => { mode = 'camera'; render(); } }, icon('chevron-left', 22)),
      h('span.acs-pill', isStory ? 'Редактор истории' : 'Редактор клипа'),
      h('button.btn.primary.sm', {
        type: 'button',
        onclick: () => {
          if (!isImg) vid.pause();
          openPublishSheet();
        },
      }, 'Далее ', icon('chevron-right', 15)));

    const sideTools = h('div.acs-side-tools',
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('stickers') }, h('span', { style: { fontSize: '1.2rem' } }, '😀'), h('span', 'Стикеры')),
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('text') }, h('b', { style: { fontSize: '1.05rem' } }, 'Aa'), h('span', 'Текст')),
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('filters') }, icon('star', 20), h('span', 'Фильтр')),
      !isImg ? h('button.acs-side-btn', { type: 'button', onclick: () => openTool('trim') }, icon('edit', 20), h('span', 'Обрезка')) : null,
      muteBtn);

    stage.append(vid, ovlLayer, !isImg ? playIcon : null);
    root.replaceChildren(fileInput, stage, topBar, sideTools, trashZone, drawer);
    drawOverlays();
  }

  function drawOverlaysOnCtx(ctx, cw, ch) {
    for (const o of overlays) {
      const px = o.x * cw;
      const py = o.y * ch;
      const sc = o.scale || 1;
      ctx.save();
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      if (o.kind === 'emoji') {
        const sz = Math.round(cw * 0.14 * sc);
        ctx.font = `${sz}px "Apple Color Emoji","Segoe UI Emoji","Noto Color Emoji",sans-serif`;
        ctx.fillText(o.text, px, py);
      } else {
        const sz = Math.round(cw * 0.06 * sc);
        ctx.font = `700 ${sz}px system-ui,-apple-system,sans-serif`;
        const st = TEXT_STYLES.find(s => s.id === o.style) || TEXT_STYLES[0];
        const tw = ctx.measureText(o.text).width;
        if (st.bg) {
          const padX = sz * 0.45, padY = sz * 0.3;
          ctx.fillStyle = st.bg;
          ctx.beginPath();
          ctx.roundRect(px - tw / 2 - padX, py - sz / 2 - padY, tw + padX * 2, sz + padY * 2, sz * 0.3);
          ctx.fill();
        } else {
          ctx.shadowColor = 'rgba(0,0,0,0.8)';
          ctx.shadowBlur = 8;
        }
        ctx.fillStyle = st.fg;
        ctx.fillText(o.text, px, py);
      }
      ctx.restore();
    }
  }

  // ─────────────────────────────── ЗАПЕКАНИЕ ЭФФЕКТОВ В ВИДЕО / ФОТО
  async function bakeEditedVideo(onProgress) {
    if (sourceKind === 'image') {
      if (overlays.length === 0 && filterIdx === 0) return { blob: sourceBlob, name: sourceName };
      return new Promise((resolve, reject) => {
        const im = new Image();
        im.onload = () => {
          const scale = Math.min(1, 1080 / Math.max(im.width, im.height));
          const cw = Math.round(im.width * scale), ch = Math.round(im.height * scale);
          const canvas = document.createElement('canvas');
          canvas.width = cw; canvas.height = ch;
          const ctx = canvas.getContext('2d');
          ctx.filter = FILTERS[filterIdx].css;
          ctx.drawImage(im, 0, 0, cw, ch);
          ctx.filter = 'none';
          drawOverlaysOnCtx(ctx, cw, ch);
          onProgress && onProgress(100);
          canvas.toBlob(b => b ? resolve({ blob: b, name: 'story-' + Date.now() + '.jpg' }) : reject(new Error('Ошибка обработки фото')), 'image/jpeg', 0.9);
        };
        im.onerror = () => reject(new Error('Не удалось загрузить фото'));
        im.src = sourceURL;
      });
    }
    const dur = vidDuration || 0;
    const hasTrim = dur > 0 && (trimStart > 0.2 || (trimEnd > 0 && trimEnd < dur - 0.3));
    const hasEdits = overlays.length > 0 || filterIdx !== 0 || muteOutput || hasTrim;
    if (!hasEdits) return { blob: sourceBlob, name: sourceName };

    return new Promise((resolve, reject) => {
      const v = document.createElement('video');
      v.src = sourceURL;
      v.playsInline = true;
      v.muted = false;
      v.volume = muteOutput ? 0 : 1;

      v.onloadedmetadata = async () => {
        const vw = v.videoWidth || 720;
        const vh = v.videoHeight || 1280;
        const scale = Math.min(1, 720 / Math.max(vw, vh));
        const cw = Math.max(2, Math.round(vw * scale / 2) * 2);
        const ch = Math.max(2, Math.round(vh * scale / 2) * 2);

        const canvas = document.createElement('canvas');
        canvas.width = cw;
        canvas.height = ch;
        const ctx = canvas.getContext('2d');

        const startT = Math.max(0, trimStart || 0);
        const endT = Math.min(v.duration || maxSec, trimEnd > startT ? trimEnd : Math.min(v.duration || maxSec, startT + maxSec));
        const totalT = Math.max(0.5, endT - startT);

        const outStream = canvas.captureStream(30);
        let audioCtx = null;
        if (!muteOutput) {
          try {
            const AC = window.AudioContext || window.webkitAudioContext;
            audioCtx = new AC();
            const srcNode = audioCtx.createMediaElementSource(v);
            const dest = audioCtx.createMediaStreamDestination();
            srcNode.connect(dest);
            dest.stream.getAudioTracks().forEach(t => outStream.addTrack(t));
          } catch {}
        }

        const mime = pickRecorderMime();
        const chunks = [];
        const rec = new MediaRecorder(outStream, mime ? { mimeType: mime, videoBitsPerSecond: 2500000 } : undefined);
        rec.ondataavailable = e => { if (e.data && e.data.size > 0) chunks.push(e.data); };
        rec.onstop = () => {
          if (audioCtx) { try { audioCtx.close(); } catch {} }
          resolve({ blob: new Blob(chunks, { type: mime || 'video/webm' }), name: 'edited-' + Date.now() + '.webm' });
        };
        rec.onerror = e => reject(e.error || new Error('Ошибка рендера видео'));

        v.currentTime = startT;
        v.onseeked = () => {
          v.onseeked = null;
          rec.start(200);
          v.play().catch(() => {});

          const drawFrame = () => {
            if (v.paused || v.ended || v.currentTime >= endT) {
              v.pause();
              if (rec.state !== 'inactive') rec.stop();
              return;
            }
            ctx.save();
            ctx.filter = FILTERS[filterIdx].css;
            ctx.drawImage(v, 0, 0, cw, ch);
            ctx.restore();

            // Рисуем стикеры и текстовые плашки
            drawOverlaysOnCtx(ctx, cw, ch);

            onProgress && onProgress(Math.min(100, Math.round(((v.currentTime - startT) / totalT) * 100)));
            requestAnimationFrame(drawFrame);
          };
          requestAnimationFrame(drawFrame);
        };
      };
      v.onerror = () => reject(new Error('Не удалось обработать видео'));
    });
  }

  // ─────────────────────────────── ЭКРАН 3: ПУБЛИКАЦИЯ
  function openPublishSheet() {
    const titleIn = !isStory ? h('input.input', { placeholder: 'Название клипа', maxlength: 100 }) : null;
    const opisIn = h('textarea.input', { rows: 3, placeholder: isStory ? 'Подпись к истории (необязательно)' : 'Описание и #хештеги (необязательно)', maxlength: isStory ? 200 : 500 });
    const progFill = h('i');
    const progText = h('div.small.muted');
    const progWrap = h('div.stack-s', { style: { display: 'none' } }, h('div.upload-bar', progFill), progText);
    const errBox = h('div.form-err');
    const pubBtn = h('button.btn.primary', { type: 'button', style: { width: '100%' } }, icon('send', 16), isStory ? ' Опубликовать историю' : ' Опубликовать клип');

    const sheet = h('div.acs-pub-back',
      h('div.acs-pub-sheet',
        h('div.acs-drawer-head',
          h('b', isStory ? 'Публикация истории (24 часа)' : 'Публикация клипа'),
          h('button.acs-mini', { type: 'button', onclick: () => { if (!busyPublish) sheet.remove(); } }, 'Назад')),
        titleIn ? h('div.field', h('label', 'Название'), titleIn) : null,
        h('div.field', h('label', isStory ? 'Подпись' : 'Описание'), opisIn),
        progWrap, errBox, pubBtn));

    pubBtn.onclick = async () => {
      if (busyPublish) return;
      busyPublish = true;
      pubBtn.disabled = true;
      errBox.textContent = '';
      progWrap.style.display = '';
      progFill.style.width = '5%';
      progText.textContent = isStory ? 'Подготовка истории…' : 'Подготовка клипа…';

      try {
        const baked = await bakeEditedVideo(pct => {
          progFill.style.width = Math.round(pct * 0.5) + '%';
          progText.textContent = `Применяю эффекты: ${pct}%`;
        });
        if (baked.blob.size > maxMB * 1024 * 1024) {
          throw new Error(`Итоговый файл превышает ${maxMB} МБ`);
        }

        const fd = new FormData();
        fd.append('file', baked.blob, baked.name);
        if (isStory) {
          fd.append('caption', opisIn.value.trim());
        } else {
          fd.append('title', titleIn.value.trim());
          fd.append('opis', opisIn.value.trim());
        }

        await new Promise((res, rej) => {
          const xhr = new XMLHttpRequest();
          xhr.open('POST', isStory ? '/api/stories' : '/api/shorts', true);
          xhr.setRequestHeader('X-DCMS', '1');
          xhr.upload.onprogress = e => {
            if (e.lengthComputable) {
              const pct = Math.round((e.loaded / e.total) * 100);
              progFill.style.width = (50 + Math.round(pct * 0.5)) + '%';
              progText.textContent = pct < 100 ? `Загрузка на сервер: ${pct}%` : 'Обработка на сервере…';
            }
          };
          xhr.onload = () => {
            let d = {};
            try { d = JSON.parse(xhr.responseText || '{}'); } catch {}
            if (xhr.status >= 200 && xhr.status < 300) res(d);
            else rej(new Error(d.error || 'Ошибка загрузки'));
          };
          xhr.onerror = () => rej(new Error('Ошибка сети при загрузке'));
          xhr.send(fd);
        }).then(d => {
          ui.ok(isStory ? 'История опубликована на 24 часа' : 'Клип опубликован и отправлен в обработку');
          close();
          onDone && onDone(d.id);
        });
      } catch (e) {
        errBox.textContent = e.message || 'Не удалось опубликовать';
        progWrap.style.display = 'none';
      } finally {
        busyPublish = false;
        pubBtn.disabled = false;
      }
    };

    root.append(sheet);
    setTimeout(() => (titleIn || opisIn).focus(), 50);
  }

  function render() {
    if (mode === 'camera') renderCamera();
    else renderEditor();
  }
  render();
}

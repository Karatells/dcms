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

export function openAndroidClipStudio(maxSec = 60, maxMB = 100, onDone) {
  let mode = 'camera'; // camera | edit | publish
  let facing = 'user';
  let stream = null;
  let recorder = null;
  let recChunks = [];
  let recStartAt = 0;
  let recTick = null;
  let isRecording = false;

  let sourceBlob = null;
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

  const fileInput = h('input', { type: 'file', accept: 'video/*,.mp4,.mov,.webm,.mkv,.3gp', style: { display: 'none' } });
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
    if (blob.size > maxMB * 1024 * 1024) {
      ui.toast(`Видео слишком большое (максимум ${maxMB} МБ)`, 'error');
      return;
    }
    stopCamera();
    if (sourceURL) URL.revokeObjectURL(sourceURL);
    sourceBlob = blob;
    sourceName = name || 'clip.webm';
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
    const camVid = h('video.acs-video', { autoplay: true, muted: true, playsinline: true });
    camVid.setAttribute('playsinline', '');
    camVid.setAttribute('webkit-playsinline', '');
    const errBox = h('div.acs-cam-err', { style: { display: 'none' } });

    const applyCamVisuals = () => {
      camVid.style.filter = FILTERS[filterIdx].css;
      camVid.style.transform = facing === 'user' ? 'scaleX(-1)' : 'none';
      filterNameEl.textContent = FILTERS[filterIdx].name;
    };

    const startCam = async () => {
      stopCamera();
      errBox.style.display = 'none';
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        errBox.style.display = '';
        errBox.replaceChildren(h('p', 'Камера недоступна — выберите видео из галереи'),
          h('button.btn.primary', { type: 'button', onclick: () => fileInput.click() }, icon('upload', 16), ' Открыть галерею'));
        return;
      }
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: facing, width: { ideal: 720 }, height: { ideal: 1280 } },
          audio: true,
        });
        camVid.srcObject = stream;
        applyCamVisuals();
        await camVid.play().catch(() => {});
      } catch {
        errBox.style.display = '';
        errBox.replaceChildren(
          h('p', 'Нет доступа к камере или микрофону. Разрешите доступ или выберите готовое видео из галереи.'),
          h('div.row',
            h('button.btn', { type: 'button', onclick: startCam }, 'Повторить'),
            h('button.btn.primary', { type: 'button', onclick: () => fileInput.click() }, icon('upload', 16), ' Галерея')));
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
      h('div.acs-prog', progFill),
      h('div.acs-stage', camVid, errBox),
      topBar,
      bottomBar);

    startCam();
  }

  // ─────────────────────────────── ЭКРАН 2: РЕДАКТОР КЛИПА
  function renderEditor() {
    const vid = h('video.acs-video', { src: sourceURL, autoplay: true, loop: true, playsinline: true });
    vid.setAttribute('playsinline', '');
    vid.setAttribute('webkit-playsinline', '');
    vid.style.filter = FILTERS[filterIdx].css;
    vid.muted = muteOutput;

    const stage = h('div.acs-stage');
    const ovlLayer = h('div.acs-ovl-layer');
    const playIcon = h('div.acs-play-mark', icon('play', 36));
    const drawer = h('div.acs-drawer', { style: { display: 'none' } });
    const muteBtn = h('button.acs-side-btn', {
      type: 'button',
      onclick: () => {
        muteOutput = !muteOutput;
        vid.muted = muteOutput;
        muteBtn.replaceChildren(icon(muteOutput ? 'volume-x' : 'volume', 20), h('span', muteOutput ? 'Без звука' : 'Звук'));
      },
    }, icon(muteOutput ? 'volume-x' : 'volume', 20), h('span', muteOutput ? 'Без звука' : 'Звук'));

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

    const drawOverlays = () => {
      ovlLayer.replaceChildren(...overlays.map(o => {
        const isSel = o.id === selectedOverlayId;
        const st = TEXT_STYLES.find(s => s.id === o.style) || TEXT_STYLES[0];
        const fontSize = o.kind === 'emoji' ? (o.size === 1 ? '2.1rem' : o.size === 3 ? '4.1rem' : '3.1rem')
          : (o.size === 1 ? '1rem' : o.size === 3 ? '1.65rem' : '1.3rem');
        const el = h('div.acs-ovl' + (isSel ? '.selected' : '') + (o.kind === 'text' ? '.is-text' : ''), {
          style: {
            left: (o.x * 100) + '%',
            top: (o.y * 100) + '%',
            fontSize,
            color: o.kind === 'text' ? st.fg : undefined,
            background: o.kind === 'text' && st.bg ? st.bg : undefined,
          },
        }, o.text);

        let dragging = false, startX = 0, startY = 0, origX = o.x, origY = o.y;
        el.addEventListener('pointerdown', e => {
          e.stopPropagation();
          selectedOverlayId = o.id;
          dragging = true;
          startX = e.clientX; startY = e.clientY;
          origX = o.x; origY = o.y;
          el.setPointerCapture(e.pointerId);
          drawOverlays();
        });
        el.addEventListener('pointermove', e => {
          if (!dragging) return;
          const rect = ovlLayer.getBoundingClientRect();
          if (!rect.width || !rect.height) return;
          o.x = Math.max(0.08, Math.min(0.92, origX + (e.clientX - startX) / rect.width));
          o.y = Math.max(0.08, Math.min(0.92, origY + (e.clientY - startY) / rect.height));
          el.style.left = (o.x * 100) + '%';
          el.style.top = (o.y * 100) + '%';
        });
        const stopDrag = () => { dragging = false; };
        el.addEventListener('pointerup', stopDrag);
        el.addEventListener('pointercancel', stopDrag);
        return el;
      }));
      syncSelectionBar();
    };

    const selBar = h('div.acs-sel-bar', { style: { display: 'none' } });
    const syncSelectionBar = () => {
      const o = overlays.find(x => x.id === selectedOverlayId);
      if (!o) { selBar.style.display = 'none'; return; }
      selBar.style.display = 'flex';
      selBar.replaceChildren(
        h('button.acs-mini', { type: 'button', onclick: () => { o.size = Math.max(1, o.size - 1); drawOverlays(); } }, 'A−'),
        h('button.acs-mini', { type: 'button', onclick: () => { o.size = Math.min(3, o.size + 1); drawOverlays(); } }, 'A+'),
        h('button.acs-mini.danger', { type: 'button', onclick: () => { overlays = overlays.filter(x => x.id !== o.id); selectedOverlayId = 0; drawOverlays(); } }, icon('trash', 14), ' Удалить'));
    };

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
              overlays.push({ id, kind: 'emoji', text: em, x: 0.5, y: 0.45, size: 2, style: 'white' });
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
          overlays.push({ id, kind: 'text', text: t, x: 0.5, y: 0.35, size: 2, style: styleId });
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
      h('span.acs-pill', 'Редактор клипа'),
      h('button.btn.primary.sm', {
        type: 'button',
        onclick: () => {
          vid.pause();
          openPublishSheet();
        },
      }, 'Далее ', icon('chevron-right', 15)));

    const sideTools = h('div.acs-side-tools',
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('stickers') }, h('span', { style: { fontSize: '1.2rem' } }, '😀'), h('span', 'Стикеры')),
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('text') }, h('b', { style: { fontSize: '1.05rem' } }, 'Aa'), h('span', 'Текст')),
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('filters') }, icon('star', 20), h('span', 'Фильтр')),
      h('button.acs-side-btn', { type: 'button', onclick: () => openTool('trim') }, icon('edit', 20), h('span', 'Обрезка')),
      muteBtn);

    stage.append(vid, ovlLayer, playIcon);
    root.replaceChildren(fileInput, stage, topBar, sideTools, selBar, drawer);
    drawOverlays();
  }

  // ─────────────────────────────── ЗАПЕКАНИЕ ЭФФЕКТОВ В ВИДЕО (Canvas + MediaRecorder)
  async function bakeEditedVideo(onProgress) {
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
            for (const o of overlays) {
              const px = o.x * cw;
              const py = o.y * ch;
              ctx.save();
              ctx.textAlign = 'center';
              ctx.textBaseline = 'middle';
              if (o.kind === 'emoji') {
                const sz = Math.round(cw * (o.size === 1 ? 0.09 : o.size === 3 ? 0.19 : 0.14));
                ctx.font = `${sz}px "Apple Color Emoji","Segoe UI Emoji","Noto Color Emoji",sans-serif`;
                ctx.fillText(o.text, px, py);
              } else {
                const sz = Math.round(cw * (o.size === 1 ? 0.045 : o.size === 3 ? 0.08 : 0.06));
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
    const titleIn = h('input.input', { placeholder: 'Название клипа', maxlength: 100 });
    const opisIn = h('textarea.input', { rows: 3, placeholder: 'Описание и #хештеги (необязательно)', maxlength: 500 });
    const progFill = h('i');
    const progText = h('div.small.muted');
    const progWrap = h('div.stack-s', { style: { display: 'none' } }, h('div.upload-bar', progFill), progText);
    const errBox = h('div.form-err');
    const pubBtn = h('button.btn.primary', { type: 'button', style: { width: '100%' } }, icon('send', 16), ' Опубликовать клип');

    const sheet = h('div.acs-pub-back',
      h('div.acs-pub-sheet',
        h('div.acs-drawer-head',
          h('b', 'Публикация клипа'),
          h('button.acs-mini', { type: 'button', onclick: () => { if (!busyPublish) sheet.remove(); } }, 'Назад')),
        h('div.field', h('label', 'Название'), titleIn),
        h('div.field', h('label', 'Описание'), opisIn),
        progWrap, errBox, pubBtn));

    pubBtn.onclick = async () => {
      if (busyPublish) return;
      busyPublish = true;
      pubBtn.disabled = true;
      errBox.textContent = '';
      progWrap.style.display = '';
      progFill.style.width = '5%';
      progText.textContent = 'Подготовка клипа…';

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
        fd.append('title', titleIn.value.trim());
        fd.append('opis', opisIn.value.trim());

        await new Promise((res, rej) => {
          const xhr = new XMLHttpRequest();
          xhr.open('POST', '/api/shorts', true);
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
          ui.ok('Клип опубликован и отправлен в обработку');
          close();
          onDone && onDone(d.id);
        });
      } catch (e) {
        errBox.textContent = e.message || 'Не удалось опубликовать клип';
        progWrap.style.display = 'none';
      } finally {
        busyPublish = false;
        pubBtn.disabled = false;
      }
    };

    root.append(sheet);
    setTimeout(() => titleIn.focus(), 50);
  }

  function render() {
    if (mode === 'camera') renderCamera();
    else renderEditor();
  }
  render();
}

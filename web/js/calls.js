// Глобальный WebRTC-движок 1-на-1 аудио- и видеозвонков с сигналингом поверх SSE + REST.
import { h, api, ui, icon, state } from './core.js';

const BLANK_POSTER = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';

let active = null; // { id, peer, kind, dir:'out'|'in', pc, localStream, remoteStream, pendingIce:[], offer, muted, camOff, minimized, ringTimer, durTimer, startedAt, ringStop, root }

const fmtDur = sec => {
  sec = Math.max(0, Math.floor(sec || 0));
  const m = Math.floor(sec / 60), s = sec % 60;
  return `${m < 10 ? '0' : ''}${m}:${s < 10 ? '0' : ''}${s}`;
};

// Мягкий синтезированный рингтон / гудок через Web Audio API (без внешних аудиофайлов)
function startTone(incoming) {
  let ctx = null, iv = null;
  const playBeep = () => {
    try {
      const AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return;
      if (!ctx) ctx = new AC();
      const now = ctx.currentTime;
      const g = ctx.createGain();
      g.connect(ctx.destination);
      const o = ctx.createOscillator();
      o.type = 'sine';
      if (incoming) {
        o.frequency.setValueAtTime(523.25, now);
        o.frequency.setValueAtTime(659.25, now + 0.16);
        g.gain.setValueAtTime(0.001, now);
        g.gain.exponentialRampToValueAtTime(0.09, now + 0.04);
        g.gain.exponentialRampToValueAtTime(0.001, now + 0.42);
        o.connect(g); o.start(now); o.stop(now + 0.44);
      } else {
        o.frequency.setValueAtTime(425, now);
        g.gain.setValueAtTime(0.001, now);
        g.gain.exponentialRampToValueAtTime(0.05, now + 0.03);
        g.gain.exponentialRampToValueAtTime(0.001, now + 0.65);
        o.connect(g); o.start(now); o.stop(now + 0.68);
      }
    } catch {}
  };
  playBeep();
  iv = setInterval(playBeep, incoming ? 1800 : 2600);
  return () => {
    if (iv) clearInterval(iv);
    if (ctx) { try { ctx.close(); } catch {} }
  };
}

async function fetchIceServers() {
  try {
    const r = await api.get('/api/calls/ice', null, { silent: true });
    if (r && Array.isArray(r.ice_servers)) return r.ice_servers;
  } catch {}
  return [{ urls: 'stun:stun.l.google.com:19302' }];
}

async function getUserMediaSafe(wantVideo) {
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    throw new Error('Браузер или устройство не поддерживает WebRTC-звонки');
  }
  if (wantVideo) {
    try {
      return await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true },
        video: { facingMode: 'user', width: { ideal: 720 }, height: { ideal: 1280 } },
      });
    } catch {
      // fallback на аудио, если камера недоступна
      ui.toast('Камера недоступна — переключаюсь на аудиозвонок', 'info');
    }
  }
  return await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true }, video: false });
}

function cleanupCall(reasonText) {
  if (!active) return;
  const c = active;
  active = null;
  if (c.ringStop) { try { c.ringStop(); } catch {} }
  if (c.ringTimer) clearTimeout(c.ringTimer);
  if (c.durTimer) clearInterval(c.durTimer);
  if (c.pc) { try { c.pc.close(); } catch {} }
  if (c.localStream) c.localStream.getTracks().forEach(t => t.stop());
  if (c.root) c.root.remove();
  document.body.classList.remove('call-open');
  if (reasonText) ui.toast(reasonText, 'info');
}

function createPeerConnection(c, iceServers) {
  const pc = new RTCPeerConnection({ iceServers });
  c.pc = pc;
  c.remoteStream = new MediaStream();

  if (c.localStream) {
    c.localStream.getTracks().forEach(t => pc.addTrack(t, c.localStream));
  }

  pc.ontrack = e => {
    const st = (e.streams && e.streams[0]) ? e.streams[0] : c.remoteStream;
    if (!e.streams || !e.streams[0]) c.remoteStream.addTrack(e.track);
    c.remoteStream = st;
    if (c.remoteEl) {
      c.remoteEl.srcObject = st;
      c.remoteEl.play().catch(() => {});
    }
  };

  pc.onicecandidate = e => {
    if (e.candidate && c.id) {
      api.post('/api/calls/signal', { call_id: c.id, type: 'ice', candidate: e.candidate }, { silent: true }).catch(() => {});
    }
  };

  pc.onconnectionstatechange = () => {
    const st = pc.connectionState;
    if (!active || active !== c) return;
    if (st === 'connected') {
      if (c.ringStop) { c.ringStop(); c.ringStop = null; }
      if (c.ringTimer) { clearTimeout(c.ringTimer); c.ringTimer = null; }
      if (!c.startedAt) {
        c.startedAt = Date.now();
        c.durTimer = setInterval(() => {
          if (c.statusEl && c.startedAt) c.statusEl.textContent = fmtDur((Date.now() - c.startedAt) / 1000);
        }, 1000);
      }
      if (c.statusEl) c.statusEl.textContent = '00:00';
    } else if (st === 'failed' || st === 'disconnected' || st === 'closed') {
      if (st === 'failed') cleanupCall('Соединение прервано (проверьте сеть или TURN-сервер)');
    }
  };

  return pc;
}

function renderCallUI(c) {
  if (c.root) c.root.remove();
  const root = h('div.call-root' + (c.minimized ? '.mini' : ''));
  c.root = root;

  const isVideo = c.kind === 'video';
  const statusEl = h('div.call-status', c.dir === 'in' && !c.pc ? (isVideo ? 'Входящий видеозвонок…' : 'Входящий аудиозвонок…') : (c.startedAt ? fmtDur((Date.now() - c.startedAt) / 1000) : 'Вызов…'));
  c.statusEl = statusEl;

  const remoteEl = h(isVideo ? 'video.call-remote-vid' : 'audio', { autoplay: true, playsinline: true, poster: isVideo ? BLANK_POSTER : undefined });
  remoteEl.setAttribute('playsinline', '');
  remoteEl.setAttribute('webkit-playsinline', '');
  if (c.remoteStream && c.remoteStream.getTracks().length) {
    remoteEl.srcObject = c.remoteStream;
    remoteEl.play().catch(() => {});
  }
  c.remoteEl = remoteEl;

  const localVid = isVideo ? h('video.call-local-vid', { autoplay: true, muted: true, playsinline: true, poster: BLANK_POSTER }) : null;
  if (localVid) {
    localVid.setAttribute('playsinline', '');
    localVid.setAttribute('muted', '');
    if (c.localStream) { localVid.srcObject = c.localStream; localVid.play().catch(() => {}); }
  }

  const hangup = () => {
    const id = c.id;
    const wasIncomingPending = c.dir === 'in' && !c.pc;
    cleanupCall();
    if (id) api.post('/api/calls/signal', { call_id: id, type: wasIncomingPending ? 'reject' : 'hangup' }, { silent: true }).catch(() => {});
  };

  const toggleMute = btn => {
    c.muted = !c.muted;
    c.localStream?.getAudioTracks().forEach(t => (t.enabled = !c.muted));
    btn.classList.toggle('off', c.muted);
    btn.replaceChildren(icon(c.muted ? 'mic-off' : 'mic', 22));
  };

  const toggleCam = btn => {
    c.camOff = !c.camOff;
    c.localStream?.getVideoTracks().forEach(t => (t.enabled = !c.camOff));
    btn.classList.toggle('off', c.camOff);
    btn.replaceChildren(icon(c.camOff ? 'video-off' : 'video', 22));
    if (localVid) localVid.style.opacity = c.camOff ? '0.2' : '1';
  };

  const toggleMini = () => {
    c.minimized = !c.minimized;
    root.classList.toggle('mini', c.minimized);
    document.body.classList.toggle('call-open', !c.minimized);
  };

  let controls;
  if (c.dir === 'in' && !c.pc) {
    controls = h('div.call-controls',
      h('button.call-btn.danger', { type: 'button', title: 'Отклонить', onclick: hangup }, icon('phone-off', 24)),
      h('button.call-btn.accept', { type: 'button', title: 'Принять', onclick: () => acceptIncomingCall(c) }, icon(isVideo ? 'video' : 'phone', 24)));
  } else {
    const micBtn = h('button.call-btn' + (c.muted ? '.off' : ''), { type: 'button', title: 'Микрофон', onclick: () => toggleMute(micBtn) }, icon(c.muted ? 'mic-off' : 'mic', 22));
    const camBtn = isVideo ? h('button.call-btn' + (c.camOff ? '.off' : ''), { type: 'button', title: 'Камера', onclick: () => toggleCam(camBtn) }, icon(c.camOff ? 'video-off' : 'video', 22)) : null;
    const minBtn = h('button.call-btn', { type: 'button', title: 'Свернуть / развернуть', onclick: toggleMini }, icon('maximize', 20));
    controls = h('div.call-controls', micBtn, camBtn, minBtn, h('button.call-btn.danger', { type: 'button', title: 'Завершить', onclick: hangup }, icon('phone-off', 24)));
  }

  const peerBox = h('div.call-peer',
    h('div.call-ava-ring' + (!c.startedAt ? '.pulse' : ''), ui.avatar(c.peer, 88)),
    h('div.call-name', c.peer?.nick || 'Собеседник'),
    statusEl);

  const miniBar = h('div.call-mini-bar', { onclick: toggleMini },
    ui.avatar(c.peer, 34),
    h('div.grow', h('b', c.peer?.nick || 'Звонок'), h('div.small.muted', 'Нажмите, чтобы развернуть')),
    h('button.call-mini-end', { type: 'button', onclick: e => { e.stopPropagation(); hangup(); } }, icon('phone-off', 16)));

  root.append(remoteEl, localVid, peerBox, controls, miniBar);
  document.body.append(root);
  document.body.classList.toggle('call-open', !c.minimized);
}

// Исходящий звонок
export async function startCall(peer, kind = 'audio') {
  if (!state.user) return ui.toast('Войдите, чтобы звонить', 'warn');
  if (active) return ui.toast('Сначала завершите текущий звонок', 'warn');
  if (!peer || !peer.id || peer.id === state.user.id) return;

  const c = { id: '', peer, kind, dir: 'out', pc: null, localStream: null, remoteStream: null, pendingIce: [], muted: false, camOff: false, minimized: false };
  active = c;
  try {
    c.localStream = await getUserMediaSafe(kind === 'video');
    if (kind === 'video' && !c.localStream.getVideoTracks().length) c.kind = 'audio';
    const iceServers = await fetchIceServers();
    const pc = createPeerConnection(c, iceServers);
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);

    renderCallUI(c);
    c.ringStop = startTone(false);

    const r = await api.post('/api/calls/start', { to: peer.id, kind: c.kind, sdp: pc.localDescription }, { silent: true });
    c.id = r.call_id;
    c.ringTimer = setTimeout(() => {
      if (active === c && !c.startedAt) {
        api.post('/api/calls/signal', { call_id: c.id, type: 'hangup' }, { silent: true }).catch(() => {});
        cleanupCall('Собеседник не отвечает');
      }
    }, 45000);
  } catch (e) {
    cleanupCall();
    ui.toast(e.message || 'Не удалось начать звонок', 'error');
  }
}

async function acceptIncomingCall(c) {
  if (!active || active !== c || c.pc) return;
  if (c.ringStop) { c.ringStop(); c.ringStop = null; }
  try {
    c.localStream = await getUserMediaSafe(c.kind === 'video');
    if (c.kind === 'video' && !c.localStream.getVideoTracks().length) c.kind = 'audio';
    const iceServers = await fetchIceServers();
    const pc = createPeerConnection(c, iceServers);
    await pc.setRemoteDescription(new RTCSessionDescription(c.offer));
    for (const cand of c.pendingIce) {
      try { await pc.addIceCandidate(new RTCIceCandidate(cand)); } catch {}
    }
    c.pendingIce = [];
    const ans = await pc.createAnswer();
    await pc.setLocalDescription(ans);
    await api.post('/api/calls/signal', { call_id: c.id, type: 'answer', sdp: pc.localDescription }, { silent: true });
    renderCallUI(c);
    if (c.statusEl) c.statusEl.textContent = 'Соединение…';
  } catch (e) {
    api.post('/api/calls/signal', { call_id: c.id, type: 'reject' }, { silent: true }).catch(() => {});
    cleanupCall();
    ui.toast(e.message || 'Не удалось принять звонок', 'error');
  }
}

// Обработчик входящих SSE-сигналов call_signal
export async function handleCallSignal(sig) {
  if (!sig || !sig.type) return;

  if (sig.type === 'incoming') {
    if (active) {
      api.post('/api/calls/signal', { call_id: sig.call_id, type: 'busy' }, { silent: true }).catch(() => {});
      return;
    }
    const c = {
      id: sig.call_id,
      peer: sig.from,
      kind: sig.kind || 'audio',
      dir: 'in',
      offer: sig.sdp,
      pc: null,
      localStream: null,
      remoteStream: null,
      pendingIce: [],
      muted: false,
      camOff: false,
      minimized: false,
    };
    active = c;
    renderCallUI(c);
    c.ringStop = startTone(true);
    c.ringTimer = setTimeout(() => {
      if (active === c && !c.pc) cleanupCall('Пропущенный звонок от ' + (c.peer?.nick || 'пользователя'));
    }, 45000);
    return;
  }

  if (!active || active.id !== sig.call_id) return;
  const c = active;

  if (sig.type === 'answer' && c.pc && sig.sdp) {
    if (c.ringStop) { c.ringStop(); c.ringStop = null; }
    try {
      await c.pc.setRemoteDescription(new RTCSessionDescription(sig.sdp));
      for (const cand of c.pendingIce) {
        try { await c.pc.addIceCandidate(new RTCIceCandidate(cand)); } catch {}
      }
      c.pendingIce = [];
      if (c.statusEl) c.statusEl.textContent = 'Соединение…';
    } catch {}
  } else if (sig.type === 'ice' && sig.candidate) {
    if (c.pc && c.pc.remoteDescription) {
      try { await c.pc.addIceCandidate(new RTCIceCandidate(sig.candidate)); } catch {}
    } else {
      c.pendingIce.push(sig.candidate);
    }
  } else if (sig.type === 'reject') {
    cleanupCall('Собеседник отклонил звонок');
  } else if (sig.type === 'busy') {
    cleanupCall('Линия занята');
  } else if (sig.type === 'hangup') {
    cleanupCall('Звонок завершён');
  }
}

const { app, BrowserWindow, Menu, Tray, shell, ipcMain, nativeImage, session } = require('electron');
const path = require('path');
const fs = require('fs');

function loadBuildConfig() {
  try {
    return JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'config.json'), 'utf8'));
  } catch {
    return { serverUrl: 'http://localhost:8080', appName: 'DCMS Social', allowInsecureTls: false };
  }
}

const buildCfg = loadBuildConfig();
const statePath = () => path.join(app.getPath('userData'), 'state.json');

function loadState() {
  try {
    return JSON.parse(fs.readFileSync(statePath(), 'utf8'));
  } catch {
    return {};
  }
}

function saveState(patch) {
  try {
    const cur = loadState();
    fs.writeFileSync(statePath(), JSON.stringify({ ...cur, ...patch }, null, 2));
  } catch {}
}

let userState = {};
let serverUrl = process.env.DCMS_URL || buildCfg.serverUrl || 'http://localhost:8080';
let mainWin = null;
let tray = null;
let isQuitting = false;

function normalizeUrl(u) {
  let s = String(u || '').trim();
  if (!s) return serverUrl;
  if (!/^https?:\/\//i.test(s)) s = 'https://' + s;
  return s.replace(/\/+$/, '');
}

function getOrigin(u) {
  try { return new URL(u).origin; } catch { return ''; }
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
}

app.on('second-instance', () => {
  if (mainWin) {
    if (mainWin.isMinimized()) mainWin.restore();
    mainWin.show();
    mainWin.focus();
  }
});

if (buildCfg.allowInsecureTls) {
  app.on('certificate-error', (event, webContents, url, error, cert, callback) => {
    if (getOrigin(url) === getOrigin(serverUrl)) {
      event.preventDefault();
      callback(true);
      return;
    }
    callback(false);
  });
}

function createTray() {
  const iconPath = path.join(__dirname, '..', 'assets', 'icon.png');
  const img = nativeImage.createFromPath(iconPath).resize({ width: 22, height: 22 });
  tray = new Tray(img);
  tray.setToolTip(buildCfg.appName || 'DCMS Social');
  const menu = Menu.buildFromTemplate([
    { label: 'Открыть ' + (buildCfg.appName || 'DCMS'), click: () => { mainWin?.show(); mainWin?.focus(); } },
    { label: 'Обновить', click: () => mainWin?.webContents.reload() },
    { type: 'separator' },
    { label: 'Сменить сервер…', click: () => { mainWin?.show(); mainWin?.loadFile(path.join(__dirname, 'offline.html')); } },
    { type: 'separator' },
    { label: 'Выход', click: () => { isQuitting = true; app.quit(); } },
  ]);
  tray.setContextMenu(menu);
  tray.on('click', () => {
    if (!mainWin) return;
    if (mainWin.isVisible()) mainWin.focus();
    else mainWin.show();
  });
}

function createWindow() {
  userState = loadState();
  if (!process.env.DCMS_URL && userState.serverUrl) {
    serverUrl = normalizeUrl(userState.serverUrl);
  }
  const b = userState.bounds || { width: 1220, height: 820 };
  const iconPath = path.join(__dirname, '..', 'assets', 'icon.png');

  mainWin = new BrowserWindow({
    width: b.width || 1220,
    height: b.height || 820,
    x: b.x,
    y: b.y,
    minWidth: 380,
    minHeight: 540,
    title: buildCfg.appName || 'DCMS Social',
    icon: iconPath,
    backgroundColor: '#0f1219',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      spellcheck: true,
    },
  });

  // Разрешаем микрофон/камеру/уведомления только для нашего сервера
  session.defaultSession.setPermissionRequestHandler((wc, permission, callback) => {
    const allowed = ['media', 'audioCapture', 'notifications', 'fullscreen', 'clipboard-read'];
    if (allowed.includes(permission) && getOrigin(wc.getURL()) === getOrigin(serverUrl)) {
      return callback(true);
    }
    callback(false);
  });

  // Внешние ссылки открываем в системном браузере
  mainWin.webContents.setWindowOpenHandler(({ url }) => {
    if (getOrigin(url) === getOrigin(serverUrl)) {
      mainWin.loadURL(url);
    } else if (/^https?:\/\//i.test(url)) {
      shell.openExternal(url);
    }
    return { action: 'deny' };
  });

  mainWin.webContents.on('will-navigate', (event, url) => {
    if (url.startsWith('file://')) return;
    if (getOrigin(url) !== getOrigin(serverUrl) && /^https?:\/\//i.test(url)) {
      event.preventDefault();
      shell.openExternal(url);
    }
  });

  mainWin.webContents.on('did-fail-load', (event, errorCode, _desc, validatedURL, isMainFrame) => {
    if (!isMainFrame || errorCode === -3) return; // -3 = ERR_ABORTED
    if (validatedURL && !validatedURL.startsWith('file://')) {
      mainWin.loadFile(path.join(__dirname, 'offline.html'));
    }
  });

  // Контекстное меню
  mainWin.webContents.on('context-menu', (_e, params) => {
    const items = [];
    if (params.selectionText) items.push({ role: 'copy', label: 'Копировать' });
    if (params.isEditable) {
      items.push({ role: 'cut', label: 'Вырезать' }, { role: 'paste', label: 'Вставить' }, { role: 'selectAll', label: 'Выделить всё' });
    }
    if (params.linkURL) {
      items.push({ label: 'Открыть ссылку в браузере', click: () => shell.openExternal(params.linkURL) });
    }
    if (items.length) items.push({ type: 'separator' });
    items.push(
      { label: 'Назад', enabled: mainWin.webContents.canGoBack(), click: () => mainWin.webContents.goBack() },
      { label: 'Вперёд', enabled: mainWin.webContents.canGoForward(), click: () => mainWin.webContents.goForward() },
      { label: 'Обновить', click: () => mainWin.webContents.reload() },
    );
    Menu.buildFromTemplate(items).popup({ window: mainWin });
  });

  const saveBounds = () => {
    if (!mainWin || mainWin.isMinimized() || mainWin.isMaximized()) return;
    saveState({ bounds: mainWin.getBounds() });
  };
  mainWin.on('resize', saveBounds);
  mainWin.on('move', saveBounds);

  // Закрытие окна сворачивает в системный трей
  mainWin.on('close', (e) => {
    if (!isQuitting && tray) {
      e.preventDefault();
      mainWin.hide();
    }
  });

  mainWin.loadURL(serverUrl);
}

ipcMain.handle('dcms:getServerUrl', () => serverUrl);
ipcMain.handle('dcms:setServerUrl', (_e, url) => {
  serverUrl = normalizeUrl(url);
  saveState({ serverUrl });
  mainWin?.loadURL(serverUrl);
  return serverUrl;
});
ipcMain.handle('dcms:retry', () => {
  mainWin?.loadURL(serverUrl);
});
ipcMain.on('dcms:setBadge', (_e, count) => {
  if (typeof app.setBadgeCount === 'function') app.setBadgeCount(count > 0 ? count : 0);
  if (mainWin) {
    const base = buildCfg.appName || 'DCMS Social';
    mainWin.setTitle(count > 0 ? `(${count}) ${base}` : base);
  }
});

app.whenReady().then(() => {
  if (buildCfg.appName) app.setName(buildCfg.appName);
  createTray();
  createWindow();
});

app.on('before-quit', () => { isQuitting = true; });
app.on('activate', () => { if (mainWin) mainWin.show(); else createWindow(); });

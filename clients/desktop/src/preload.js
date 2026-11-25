const { contextBridge, ipcRenderer } = require('electron');
const pkg = require('../package.json');

contextBridge.exposeInMainWorld('dcmsNative', {
  platform: 'desktop',
  os: process.platform,
  arch: process.arch,
  version: pkg.version || '1.0.0',
  getServerUrl: () => ipcRenderer.invoke('dcms:getServerUrl'),
  setServerUrl: (url) => ipcRenderer.invoke('dcms:setServerUrl', url),
  retry: () => ipcRenderer.invoke('dcms:retry'),
  setBadge: (count) => ipcRenderer.send('dcms:setBadge', Number(count) || 0),
  downloadUpdate: (url, file, sha256) => ipcRenderer.invoke('dcms:downloadUpdate', { url, file, sha256 }),
  onUpdateProgress: (cb) => ipcRenderer.on('dcms:updateProgress', (_e, data) => cb && cb(data)),
});

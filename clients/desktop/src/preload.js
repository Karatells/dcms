const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('dcmsNative', {
  platform: 'desktop',
  os: process.platform,
  arch: process.arch,
  getServerUrl: () => ipcRenderer.invoke('dcms:getServerUrl'),
  setServerUrl: (url) => ipcRenderer.invoke('dcms:setServerUrl', url),
  retry: () => ipcRenderer.invoke('dcms:retry'),
  setBadge: (count) => ipcRenderer.send('dcms:setBadge', Number(count) || 0),
});

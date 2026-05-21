'use strict';

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('electronAPI', {
  onStatusUpdate: (cb) => ipcRenderer.on('status-update', (_event, msg) => cb(msg)),
  showOpenDialog: (options) => ipcRenderer.invoke('show-open-dialog', options),
  getDbPath:           ()       => ipcRenderer.invoke('get-db-path'),
  getLogLevel:         ()       => ipcRenderer.invoke('get-log-level'),
  selectDatabase:      (opts)   => ipcRenderer.invoke('select-db', opts),
  showSaveDialog:      (options)=> ipcRenderer.invoke('show-save-dialog', options),
  confirmContinueWithoutAI: ()   => ipcRenderer.invoke('confirm-continue-without-ai'),
  checkOllamaModel:    ()       => ipcRenderer.invoke('check-ollama-model'),
  pullOllamaModel:     ()       => ipcRenderer.invoke('pull-ollama-model'),
  startOllama:         ()       => ipcRenderer.invoke('start-ollama'),
  getAutoStartLocalAI: ()       => ipcRenderer.invoke('get-auto-start-local-ai'),
  setAutoStartLocalAI: (enabled)=> ipcRenderer.invoke('set-auto-start-local-ai', !!enabled),
  onOllamaPullProgress:(cb)     => ipcRenderer.on('ollama-pull-progress', (_e, msg) => cb(msg)),
  getProfiles:      ()     => ipcRenderer.invoke('get-profiles'),
  createProfile:    (opts) => ipcRenderer.invoke('create-profile', opts),
  updateProfile:    (opts) => ipcRenderer.invoke('update-profile', opts),
  getProfileDbPath: (id)   => ipcRenderer.invoke('get-profile-db-path', id),
  getAdminDataDir:  ()     => ipcRenderer.invoke('get-admin-data-dir'),
  suggestArchiveDbPath: (name) => ipcRenderer.invoke('suggest-archive-db-path', name),
});

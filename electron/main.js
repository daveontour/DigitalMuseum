'use strict';

const { app, BrowserWindow, Tray, Menu, nativeImage, ipcMain, dialog, globalShortcut } = require('electron');
const { spawn, execFile } = require('child_process');
const net = require('net');
const path = require('path');
const fs = require('fs');
// ---------------------------------------------------------------------------
// Global state
// ---------------------------------------------------------------------------

let mainWindow = null;
let loadingWindow = null;
let tray = null;
let goProcess = null;
let isQuitting = false;
let appPort = null;
let activeDotenv = null;
let ollamaProcess = null;
let activeProfileId = null;

// ---------------------------------------------------------------------------
// Resource path resolution
// ---------------------------------------------------------------------------
// Dev mode  (app.isPackaged === false): project root is one level above electron/
// Packaged: process.resourcesPath contains extraResources

function getResourcesDir() {
  return app.isPackaged ? process.resourcesPath : path.join(__dirname, '..');
}

function getPaths() {
  const res = getResourcesDir();
  const userData = app.getPath('userData');

  //userData = "C:/Users/dave_/OneDrive/Desktop/digitalmuseum"
  const goExe = path.join(res, 'bin', 'digitalmuseum.exe');
  return {
    // Install / project root (contains bin/, static/, templates/). The Go server
    // must run with this as cwd so relative paths like static/data/*.json in
    // cmd/server/main.go resolve correctly — same as cmd/launcher (cmd.Dir = root).
    appRoot:     res,
    goExe,
    templatesDir: path.join(res, 'templates'),
    staticDir:   path.join(res, 'static'),
    userData,
    defaultAdminSqlitePath: path.join(path.dirname(goExe), 'data', 'admin.sqlite'),
    dotEnvPath:  path.join(userData, '.env'),
    dotEnvDefaults: app.isPackaged
      ? path.join(process.resourcesPath, '.env.defaults')
      : path.join(__dirname, '.env.defaults'),
  };
}

/** Resolve ADMIN_SQLITE_PATH the same way as Go (relative paths vs goExe dir). */
function resolveAdminSqlitePath(paths, dotenv) {
  const raw = (dotenv && dotenv.ADMIN_SQLITE_PATH) ? String(dotenv.ADMIN_SQLITE_PATH).trim() : '';
  if (!raw) return paths.defaultAdminSqlitePath;
  if (path.isAbsolute(raw)) return path.normalize(raw);
  return path.normalize(path.join(path.dirname(paths.goExe), raw));
}

function getAdminDataDir(paths, dotenv) {
  return path.dirname(resolveAdminSqlitePath(paths, dotenv || {}));
}

function getAppLogFile(paths, dotenv) {
  return path.join(getAdminDataDir(paths, dotenv || {}), 'app.log');
}

/** Attach adminDataDir and appLogFile derived from resolved ADMIN_SQLITE_PATH. */
function applyResolvedDataPaths(paths, dotenv) {
  const adminDataDir = getAdminDataDir(paths, dotenv);
  paths.adminDataDir = adminDataDir;
  paths.appLogFile = path.join(adminDataDir, 'app.log');
  return paths;
}

/** Logs SQLite paths as configured, absolute, relative to userData/appRoot/cwd, and file presence. */
function logSqliteDatabasePaths(paths, mainPath, billingPath, ctxLabel) {
  const cwd = process.cwd();
  const abs = (p) => path.resolve(p);
  const rel = (baseDir, targetPath) => {
    try {
      return path.relative(path.resolve(baseDir), path.resolve(targetPath));
    } catch (e) {
      return `(relative error: ${e.message})`;
    }
  };
  const fileInfo = (label, p) => {
    const a = abs(p);
    let exists = false;
    let sizeBytes = 0;
    try {
      const st = fs.statSync(a);
      exists = true;
      sizeBytes = st.size;
    } catch (_) { /* not found */ }
    log(`[db:${ctxLabel}] ${label}: configured_path=${p}`);
    log(`[db:${ctxLabel}] ${label}: absolute=${a}`);
    log(`[db:${ctxLabel}] ${label}: relative_to_userData=${rel(paths.userData, a)} relative_to_appRoot=${rel(paths.appRoot, a)} relative_to_electron_cwd=${rel(cwd, a)}`);
    log(`[db:${ctxLabel}] ${label}: exists_on_disk=${exists} size_bytes=${exists ? sizeBytes : 0}${exists ? '' : ' (new or not yet created)'}`);
  };
  log(`[db:${ctxLabel}] --- sqlite path resolution ---`);
  log(`[db:${ctxLabel}] userData=${paths.userData}`);
  log(`[db:${ctxLabel}] appRoot_go_cwd=${paths.appRoot}`);
  log(`[db:${ctxLabel}] electron_process.cwd=${cwd}`);
  fileInfo('main', mainPath);
  fileInfo('billing', billingPath);
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

const APP_LOG_MAX_BYTES = 10 * 1024 * 1024;

let logStream = null;

/** If logFile exceeds maxBytes, drop oldest lines from the top and keep the tail. */
function trimLogFileToMaxSize(logFile, maxBytes = APP_LOG_MAX_BYTES) {
  try {
    if (!fs.existsSync(logFile)) return;
    const { size } = fs.statSync(logFile);
    if (size <= maxBytes) return;

    const fd = fs.openSync(logFile, 'r+');
    try {
      let dropThrough = size - maxBytes;
      const scan = Buffer.alloc(4096);
      let pos = dropThrough;
      while (pos < size) {
        const n = fs.readSync(fd, scan, 0, Math.min(scan.length, size - pos), pos);
        if (n <= 0) break;
        const idx = scan.subarray(0, n).indexOf('\n');
        if (idx >= 0) {
          dropThrough = pos + idx + 1;
          break;
        }
        pos += n;
      }

      const keepLen = size - dropThrough;
      const tail = Buffer.alloc(keepLen);
      fs.readSync(fd, tail, 0, keepLen, dropThrough);
      const marker = Buffer.from(
        `[${new Date().toISOString()}] --- log trimmed (${size} bytes -> ${keepLen} bytes retained) ---\n`,
        'utf8',
      );
      fs.ftruncateSync(fd, 0);
      fs.writeSync(fd, marker, 0, marker.length, 0);
      fs.writeSync(fd, tail, 0, keepLen, marker.length);
      fs.ftruncateSync(fd, marker.length + keepLen);
    } finally {
      fs.closeSync(fd);
    }
  } catch (e) {
    console.error(`Failed to trim log file ${logFile}: ${e.message}`);
  }
}

function setupLogging(logFile) {
  trimLogFileToMaxSize(logFile);
  try {
    logStream = fs.createWriteStream(logFile, { flags: 'a' });
  } catch (e) {
    // Fall back to console if log file can't be opened
  }
}

function log(msg) {
  const line = `[${new Date().toISOString()}] ${msg}`;
  console.log(line);
  if (logStream) logStream.write(line + '\n');
}

// ---------------------------------------------------------------------------
// Free port finding
// ---------------------------------------------------------------------------

function isPortFree(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => { server.close(); resolve(true); });
    server.listen(port, '127.0.0.1');
  });
}

async function findFreePort(preferred = 8080) {
  for (let p = preferred; p < preferred + 100; p++) {
    if (await isPortFree(p)) return p;
  }
  throw new Error('No free port found in range 8080–8179');
}

// ---------------------------------------------------------------------------
// HTTP readiness polling
// ---------------------------------------------------------------------------

async function waitForHealth(port, maxMs = 30000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://127.0.0.1:${port}/health`);
      if (res.ok) return;
    } catch (_) { /* not ready yet */ }
    await new Promise(r => setTimeout(r, 300));
  }
  throw new Error(`Go server did not become healthy within ${maxMs}ms`);
}

// ---------------------------------------------------------------------------
// .env file parser (no npm dependency)
// ---------------------------------------------------------------------------

function loadDotEnv(filePath) {
  const env = {};
  if (!fs.existsSync(filePath)) return env;
  const lines = fs.readFileSync(filePath, 'utf8').split('\n');
  for (const raw of lines) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq < 1) continue;
    const key = line.slice(0, eq).trim();
    let val = line.slice(eq + 1).trim();
    // Strip optional surrounding quotes
    if ((val.startsWith('"') && val.endsWith('"')) ||
        (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    env[key] = val;
  }
  return env;
}

function upsertEnvVar(content, key, value) {
  const re = new RegExp(`^${key}\\s*=.*`, 'm');
  return re.test(content)
    ? content.replace(re, `${key}=${value}`)
    : content.trimEnd() + `\n${key}=${value}\n`;
}

function parseBoolEnv(v) {
  const s = String(v || '').trim().toLowerCase();
  return s === '1' || s === 'true' || s === 'yes' || s === 'on';
}

// ---------------------------------------------------------------------------
// Ollama local AI helpers
// ---------------------------------------------------------------------------

function getOllamaExe() {
  return path.join(getResourcesDir(), 'bin', 'Ollama', 'ollama.exe');
}

async function waitForOllama(maxMs = 30000) {
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch('http://127.0.0.1:11434/');
      if (res.ok || res.status < 500) return;
    } catch (_) { /* not ready */ }
    await new Promise(r => setTimeout(r, 500));
  }
  throw new Error(`Ollama did not start within ${maxMs / 1000}s`);
}

async function ensureOllamaRunning() {
  const ollamaExe = getOllamaExe();
  if (!fs.existsSync(ollamaExe)) {
    return { ok: false, error: 'Ollama executable not found' };
  }
  try {
    const r = await fetch('http://127.0.0.1:11434/');
    if (r.ok || r.status < 500) {
      log('Ollama already running');
      return { ok: true };
    }
  } catch (_) { /* not running yet */ }
  const paths = getPaths();
  let logFd;
  const logFile = getAppLogFile(paths, activeDotenv || {});
  try { logFd = fs.openSync(logFile, 'a'); } catch (_) { /* ignore */ }
  ollamaProcess = spawn(ollamaExe, ['serve'], {
    windowsHide: true,
    stdio: ['ignore', logFd ?? 'ignore', logFd ?? 'ignore'],
    env: { ...process.env, OLLAMA_NUM_CTX: '32768' },
  });
  ollamaProcess.on('exit', (code) => log(`Ollama exited (code=${code})`));
  ollamaProcess.on('error', (err) => log(`Ollama error: ${err.message}`));
  try {
    await waitForOllama(30000);
    log('Ollama server ready');
    return { ok: true };
  } catch (err) {
    log(`start-ollama: ${err.message}`);
    return { ok: false, error: err.message };
  }
}

// ---------------------------------------------------------------------------
// Status (embedded PostgreSQL removed — app uses SQLite files under userData/data/)
// ---------------------------------------------------------------------------

function sendStatus(msg) {
  log(msg);
  if (loadingWindow && !loadingWindow.isDestroyed()) {
    loadingWindow.webContents.send('status-update', msg);
  }
}

async function killZombies() {
  return new Promise((resolve) => {
    execFile('taskkill', ['/f', '/im', 'digitalmuseum.exe', '/t'],
      { windowsHide: true }, () => resolve());
  });
}

/**
 * CGO-built digitalmuseum.exe often needs MinGW DLLs (libgcc, winpthread). Git Bash
 * may have those on PATH while the npm/Electron parent does not, which surfaces as
 * synchronous spawn UNKNOWN (-4094) before the process can start.
 */
function augmentPathWithMingwBin(env) {
  if (process.platform !== 'win32') return env;
  const prefixes = [];
  if (typeof env.MINGW_PREFIX === 'string' && env.MINGW_PREFIX.trim()) {
    const b = path.join(env.MINGW_PREFIX.trim(), 'bin');
    if (fs.existsSync(b)) prefixes.push(b);
  }
  for (const p of [
    'C:\\msys64\\mingw64\\bin',
    'C:\\msys64\\ucrt64\\bin',
    'C:\\msys64\\clang64\\bin',
    'C:\\msys64\\usr\\bin',
    'C:\\mingw64\\bin',
  ]) {
    if (fs.existsSync(p)) prefixes.push(p);
  }
  const seen = new Set();
  const unique = prefixes.filter((p) => !seen.has(p) && seen.add(p));
  if (unique.length === 0) return env;
  const pathKey = Object.prototype.hasOwnProperty.call(env, 'Path') ? 'Path' : 'PATH';
  const cur = String(env[pathKey] || '');
  const add = unique.join(path.delimiter);
  return { ...env, [pathKey]: `${add}${path.delimiter}${cur}` };
}

/** Best-effort: ensures digitalmuseum.exe is gone (covers shell-wrapped spawns on Windows). */
function forceKillDigitalMuseumWindows() {
  return new Promise((resolve) => {
    if (process.platform !== 'win32') return resolve();
    execFile(
      'taskkill',
      ['/f', '/im', 'digitalmuseum.exe', '/t'],
      { windowsHide: true },
      () => resolve(),
    );
  });
}

// ---------------------------------------------------------------------------
// Go server lifecycle
// ---------------------------------------------------------------------------

function startGoServer(port, paths, dotenv) {
  sendStatus('Starting application server...');
  log(`Starting Go server on port ${port}...`);

  const goExeResolved = path.resolve(paths.goExe);
  if (!fs.existsSync(goExeResolved)) {
    throw new Error(
      `Go server executable not found: ${goExeResolved} — run \`make build-exe\` (dev) or \`make build-exe-electron\` (installer) from the repo root.`,
    );
  }

  // Do not pass raw fds into stdio on Windows — libuv/CreateProcess often fails with
  // synchronous spawn errors ("spawn UNKNOWN", errno ~-4094). Pipe stdout/stderr into
  // the log when possible; if spawn still throws, retry with stdio fully ignored.
  let goLogStream;
  try {
    goLogStream = fs.createWriteStream(paths.appLogFile, { flags: 'a' });
  } catch (_) { /* ignore */ }

  const goStdioPiped = goLogStream
    ? /** @type {const} */ (['ignore', 'pipe', 'pipe'])
    : /** @type {const} */ (['ignore', 'ignore', 'ignore']);

  const rawEnv = {
    ...process.env,
    ...dotenv,
    HOST_PORT:             String(port),
    SQLITE_PATH:           '',
    ADMIN_SQLITE_PATH:   dotenv.ADMIN_SQLITE_PATH || paths.defaultAdminSqlitePath,
    TEMPLATES_DIR:         paths.templatesDir,
    ASSET_STATIC_DIR:      paths.staticDir,
    DEPLOYMENT_NATURE:     dotenv.DEPLOYMENT_NATURE  || 'local',
    SESSION_COOKIE_SECURE: 'false',
  };
  const env = augmentPathWithMingwBin(rawEnv);

  logSqliteDatabasePaths(paths, env.SQLITE_PATH, env.ADMIN_SQLITE_PATH, 'electron-spawn-env');

  const spawnBase = /** @type {const} */ ({
    cwd: paths.appRoot,
    env,
    windowsHide: true,
  });

  const ignore3 = /** @type {const} */ (['ignore', 'ignore', 'ignore']);
  let stdioUsesPipes = goStdioPiped[1] === 'pipe';

  /** @type {Error|undefined} */
  let lastSpawnErr;

  if (process.platform === 'win32') {
    const attempts = /** @type {const} */ ([
      ['spawn+pipe', () => spawn(goExeResolved, [], { ...spawnBase, stdio: goStdioPiped })],
      ['spawn+ignore', () => spawn(goExeResolved, [], { ...spawnBase, stdio: ignore3 })],
      ['spawn+detached', () => spawn(goExeResolved, [], { ...spawnBase, stdio: ignore3, detached: true })],
      ['execFile', () => execFile(goExeResolved, [], { ...spawnBase, stdio: ignore3 })],
      ['spawn+shell', () => spawn(goExeResolved, [], { ...spawnBase, stdio: ignore3, shell: true })],
    ]);
    let started = false;
    for (const [name, fn] of attempts) {
      try {
        goProcess = fn();
        log(`Go server process started (${name})`);
        started = true;
        stdioUsesPipes = name === 'spawn+pipe' && goStdioPiped[1] === 'pipe';
        if (!stdioUsesPipes) {
          try {
            goLogStream?.end();
          } catch (_) { /* ignore */ }
          goLogStream = null;
        }
        break;
      } catch (e) {
        lastSpawnErr = e;
        log(`Go ${name} failed: ${e.code || e.errno || ''} ${e.message}`);
      }
    }
    if (!started) {
      throw new Error(
        `${lastSpawnErr?.message || 'spawn failed'} — could not launch digitalmuseum.exe from Electron. `
        + `Open cmd.exe and run: "${goExeResolved}" `
        + `If Windows reports a missing DLL, ensure MinGW/MSYS64 is installed and on PATH (or run from Git Bash where \`make\` built the binary). `
        + `If the exe runs in cmd but not here, try excluding the repo from real-time antivirus (errno UNKNOWN / -4094).`,
      );
    }
  } else {
    try {
      goProcess = spawn(goExeResolved, [], { ...spawnBase, stdio: goStdioPiped });
    } catch (spawnErr) {
      log(`Go spawn failed (${spawnErr.code || spawnErr.errno || 'no-code'} ${spawnErr.message}); retrying with stdio ignored (server logs won't be captured)`);
      try {
        goProcess = spawn(goExeResolved, [], { ...spawnBase, stdio: ignore3 });
        stdioUsesPipes = false;
        try {
          goLogStream?.end();
        } catch (_) { /* ignore */ }
        goLogStream = null;
      } catch (spawnErr2) {
        throw new Error(
          `${spawnErr2.message}; first error: ${spawnErr.message}. Hint: rebuild with \`make build-exe\` (console subsystem), exclude the repo from real-time antivirus, and ensure GCC + SQLite CGO prerequisites from the Makefile are installed.`,
        );
      }
    }
  }

  if (stdioUsesPipes && goLogStream && goProcess.stdout && goProcess.stderr) {
    const opts = { end: false };
    goProcess.stdout.pipe(goLogStream, opts);
    goProcess.stderr.pipe(goLogStream, opts);
  }

  goProcess.on('exit', () => {
    try {
      goLogStream?.end();
    } catch (_) { /* ignore */ }
  });
  goProcess.on('exit', (code, signal) => {
    log(`Go server exited (code=${code}, signal=${signal})`);
  });

  goProcess.on('error', (err) => {
    log(`Go server spawn error: ${err.message}`);
  });
}

async function restartGoServer(logLevel) {
  if (goProcess && !goProcess.killed) {
    goProcess.kill('SIGTERM');
    await new Promise((resolve) => {
      const t = setTimeout(() => {
        if (goProcess && !goProcess.killed) goProcess.kill('SIGKILL');
        resolve();
      }, 5000);
      goProcess.once('exit', () => { clearTimeout(t); resolve(); });
    });
  }
  await forceKillDigitalMuseumWindows();
  const paths = applyResolvedDataPaths(getPaths(), activeDotenv || {});
  let envContent = fs.existsSync(paths.dotEnvPath)
    ? fs.readFileSync(paths.dotEnvPath, 'utf8') : '';

  envContent = upsertEnvVar(envContent, 'SQLITE_PATH', '');
  if (logLevel) {
    envContent = upsertEnvVar(envContent, 'LOG_LEVEL', logLevel);
  }
  fs.writeFileSync(paths.dotEnvPath, envContent, 'utf8');
  activeDotenv = { ...activeDotenv, SQLITE_PATH: '' };
  if (logLevel) activeDotenv = { ...activeDotenv, LOG_LEVEL: logLevel };
  startGoServer(appPort, paths, activeDotenv);
  await waitForHealth(appPort, 30000);
}

// ---------------------------------------------------------------------------
// Window management
// ---------------------------------------------------------------------------

// Shared custom title bar (Windows WCO requires frame: false + hidden style).
const TITLE_BAR_OVERLAY = {
  color: '#1a2540',
  symbolColor: '#e6edf3',
  height: 32,
};



function createLoadingWindow() {
  loadingWindow = new BrowserWindow({
    width: 480,
    height: 300,
    resizable: false,
    center: true,
    show: true,
    frame: false,
    backgroundColor: '#0d1117',
    titleBarStyle: 'hidden',
    titleBarOverlay: TITLE_BAR_OVERLAY,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      preload: path.join(__dirname, 'preload.js'),
    },
  });

  loadingWindow.loadFile(path.join(__dirname, 'loading.html'));
  loadingWindow.on('closed', () => { loadingWindow = null; });
}

function createMainWindow(port) {
  mainWindow = new BrowserWindow({
    width: 1680,
    height: 1000,
    show: false,
    title: 'Digital Museum',
    frame: false,
    backgroundColor: '#0d1117',
    titleBarStyle: 'hidden',
    titleBarOverlay: TITLE_BAR_OVERLAY,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      preload: path.join(__dirname, 'preload.js'),
    },
  });

  //mainWindow.loadURL(`http://localhost:${port}/`);

  mainWindow.loadURL(`http://localhost:${port}/`);

  mainWindow.once('ready-to-show', () => {
    mainWindow.show();
    if (loadingWindow && !loadingWindow.isDestroyed()) {
      loadingWindow.close();
    }
    log('Main window ready');
  });

  // Closing the window shuts down the backend and quits the app.
  mainWindow.on('close', (e) => {
    if (!isQuitting) {
      e.preventDefault();
      isQuitting = true;
      shutdown().finally(() => app.quit());
    }
  });

  mainWindow.on('closed', () => { mainWindow = null; });
}

function toggleMainWindowDevTools() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.toggleDevTools();
  }
}

/** Chromium-style shortcut; registered globally because Menu.setApplicationMenu(null) removes the default View menu. */
function registerDevToolsShortcut() {
  const ok = globalShortcut.register('CommandOrControl+Shift+I', toggleMainWindowDevTools);
  if (!ok) {
    log('Could not register Ctrl+Shift+I (Cmd+Opt+I on macOS) for Developer Tools — use the tray menu instead.');
  }
}

// ---------------------------------------------------------------------------
// System tray
// ---------------------------------------------------------------------------

function loadTrayNativeImage() {
  const icoPath = path.join(__dirname, 'build', 'icon.ico');
  const pngPath = path.join(__dirname, 'tray-icon.png');
  if (fs.existsSync(icoPath)) {
    const img = nativeImage.createFromPath(icoPath);
    if (!img.isEmpty()) return img;
    log(`Tray: could not decode ${icoPath}`);
  }
  if (fs.existsSync(pngPath)) {
    const img = nativeImage.createFromPath(pngPath);
    if (!img.isEmpty()) return img;
    log(`Tray: could not decode ${pngPath}`);
  }
  log('Tray: no icon.ico or tray-icon.png — using empty image');
  return nativeImage.createEmpty();
}

function setupTray(port) {
  const icon = loadTrayNativeImage();

  tray = new Tray(icon);
  tray.setToolTip('Digital Museum');

  const menu = Menu.buildFromTemplate([
    {
      label: 'Open Digital Museum',
      click: () => {
        if (mainWindow) {
          mainWindow.show();
          mainWindow.focus();
        } else {
          createMainWindow(port);
        }
      },
    },
    {
      label: 'Toggle Developer Tools',
      click: () => toggleMainWindowDevTools(),
    },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => {
        isQuitting = true;
        shutdown().finally(() => app.quit());
      },
    },
  ]);

  tray.setContextMenu(menu);
  tray.on('double-click', () => {
    if (mainWindow) { mainWindow.show(); mainWindow.focus(); }
  });
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

async function shutdown() {
  log('Shutting down...');
  const paths = getPaths();

  if (goProcess && !goProcess.killed) {
    goProcess.kill('SIGTERM');
    await new Promise((resolve) => {
      const t = setTimeout(() => {
        if (goProcess && !goProcess.killed) goProcess.kill('SIGKILL');
        resolve();
      }, 5000);
      goProcess.once('exit', () => { clearTimeout(t); resolve(); });
    });
  }
  await forceKillDigitalMuseumWindows();

  if (ollamaProcess && !ollamaProcess.killed) {
    ollamaProcess.kill('SIGTERM');
  }

  log('Shutdown complete');
}

// ---------------------------------------------------------------------------
// App lifecycle
// ---------------------------------------------------------------------------

// Single-instance guard
const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (mainWindow) { mainWindow.show(); mainWindow.focus(); }
  });
}

app.whenReady().then(async () => {
  Menu.setApplicationMenu(null);

  const paths = getPaths();

  const createdUserDotEnv = !fs.existsSync(paths.dotEnvPath) && fs.existsSync(paths.dotEnvDefaults);
  if (createdUserDotEnv) {
    fs.copyFileSync(paths.dotEnvDefaults, paths.dotEnvPath);
  }

  let dotenv = {
    ...loadDotEnv(paths.dotEnvDefaults),
    ...loadDotEnv(paths.dotEnvPath),
    ...(app.isPackaged ? {} : loadDotEnv(path.join(__dirname, '..', '.env'))),
  };
  dotenv = {
    ...dotenv,
    ADMIN_SQLITE_PATH: dotenv.ADMIN_SQLITE_PATH || paths.defaultAdminSqlitePath,
  };
  applyResolvedDataPaths(paths, dotenv);

  fs.mkdirSync(app.getPath('userData'), { recursive: true });
  fs.mkdirSync(paths.adminDataDir, { recursive: true });

  setupLogging(paths.appLogFile);
  log('Digital Museum starting...');
  if (createdUserDotEnv) {
    log(`Created ${paths.dotEnvPath} from defaults`);
  }

  createLoadingWindow();

  try {
    // ADMIN_SQLITE_PATH: honor .env when set; else <goExeDir>/data/admin.sqlite (Go resolves relative paths vs exe dir).
    // SQLITE_PATH is left empty when absent — Go handles nil pool on first run
    // and the user creates their first archive through the login page.
    logSqliteDatabasePaths(paths, dotenv.SQLITE_PATH, dotenv.ADMIN_SQLITE_PATH, 'electron-resolved');

    sendStatus('Cleaning up previous processes...');
    await killZombies();

    sendStatus('Finding available port...');
    //appPort = await findFreePort(8080);
    appPort = 8081;
    log(`Using port ${appPort}`);

    dotenv.GMAIL_REDIRECT_URL = `http://localhost:${appPort}/gmail/auth/callback`;

    activeDotenv = dotenv;
    startGoServer(appPort, paths, dotenv);

    sendStatus('Waiting for server to be ready...');
    await waitForHealth(appPort, 30000);
    log('Server is healthy');

    try {
      const res = await fetch(`http://127.0.0.1:${appPort}/api/resolved-main-sqlite-path`);
      if (res.ok) {
        const d = await res.json();
        const resolved = (d && d.sqlite_path) ? String(d.sqlite_path).trim() : '';
        if (resolved && activeDotenv && resolved !== (activeDotenv.SQLITE_PATH || '').trim()) {
          activeDotenv = { ...activeDotenv, SQLITE_PATH: resolved };
        }
      }
    } catch (_) {
      /* best-effort: keep dotenv SQLITE_PATH */
    }

    sendStatus('Ready!');
    createMainWindow(appPort);
    setupTray(appPort);
    registerDevToolsShortcut();
    ensureOllamaRunning().then((res) => {
      if (!res.ok) log(`Ollama auto-start failed: ${res.error || 'unknown'}`);
      else log('Ollama server available (auto-started when needed)');
    });

  } catch (err) {
    log(`Startup error: ${err.message}`);
    sendStatus(`Error: ${err.message}`);
    // Keep loading window open so user can see the error
  }
});

app.on('window-all-closed', () => {
  // On Windows/Linux, keep running in tray instead of quitting
  // (macOS convention would be different but this is a Windows-first app)
});

ipcMain.handle('get-app-info', () => ({ version: app.getVersion(), name:app.getName(),description: app.getName() }));

// ── File / directory picker (used by import dialogs) ─────────────────────────
ipcMain.handle('show-open-dialog', (_event, options) => dialog.showOpenDialog(options));
ipcMain.handle('show-save-dialog', (_event, options) => dialog.showSaveDialog(options));
ipcMain.handle('confirm-continue-without-ai', async () => {
  const parentWindow = mainWindow && !mainWindow.isDestroyed() ? mainWindow : null;
  const result = await dialog.showMessageBox(parentWindow, {
    type: 'warning',
    title: 'Local AI Setup Incomplete',
    message: 'Local AI setup is incomplete.',
    detail: 'Some features may not be available until Ollama is running and both required models are installed (gemma4 and embeddinggemma). Do you want to continue signing in?',
    buttons: ['Continue Sign In', 'Go to AI Setup'],
    defaultId: 1,
    cancelId: 1,
    noLink: true,
  });
  return { ok: true, proceed: result.response === 0 };
});

ipcMain.handle('get-db-path', () => (activeDotenv && activeDotenv.SQLITE_PATH) || null);

ipcMain.handle('get-log-level', () => (activeDotenv && activeDotenv.LOG_LEVEL) || 'warn');

ipcMain.handle('select-db', async (_event, opts) => {
  const logLevel = typeof opts === 'string' ? undefined : opts.logLevel;
  const profileId = (typeof opts === 'object' && opts.profileId) ? opts.profileId : null;
  try {
    if (profileId && appPort) {
      const patchRes = await goFetch(`/api/profiles/${encodeURIComponent(profileId)}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_default: true }),
      });
      if (!patchRes.ok) {
        const d = await patchRes.json().catch(() => ({}));
        log(`select-db set startup default warning: ${d.error || patchRes.status}`);
      }
    }
    await restartGoServer(logLevel);
    if (profileId) activeProfileId = profileId;
    return { ok: true };
  } catch (err) {
    log(`select-db error: ${err.message}`);
    return { ok: false, error: err.message };
  }
});

// ── Archive profile management ────────────────────────────────────────────────

function slugify(name) {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'archive';
}

function goFetch(urlPath, opts) {
  return fetch(`http://127.0.0.1:${appPort}${urlPath}`, opts);
}

ipcMain.handle('get-profiles', async () => {
  if (!appPort) return { profiles: [], activeId: null };
  try {
    const res = await goFetch('/profiles');
    if (!res.ok) return { profiles: [], activeId: null };
    const data = await res.json();
    const profiles = Array.isArray(data) ? data : (data.profiles || []);
    return { profiles, activeId: activeProfileId };
  } catch (_) { return { profiles: [], activeId: null }; }
});

ipcMain.handle('create-profile', async (_event, opts) => {
  const { name, dbPath: customPath } = opts || {};
  if (!name || !name.trim()) return { ok: false, error: 'Name is required' };

  const paths = applyResolvedDataPaths(getPaths(), activeDotenv || {});
  const archivesDir = paths.adminDataDir;
  if (!fs.existsSync(archivesDir)) fs.mkdirSync(archivesDir, { recursive: true });
  const dbPath = customPath || path.join(archivesDir, slugify(name.trim()) + '.sqlite');

  if (appPort) {
    try {
      const res = await goFetch('/api/profiles', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), db_path: dbPath }),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        const msg = (d && (d.detail || d.error)) || 'Server error registering profile';
        return { ok: false, error: msg };
      }
      const profile = await res.json();
      activeProfileId = profile.id;
      try {
        await goFetch(`/api/profiles/${encodeURIComponent(profile.id)}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ is_default: true }),
        });
      } catch (_) { /* best-effort */ }
    } catch (err) {
      return { ok: false, error: err.message };
    }
  }

  try {
    await restartGoServer(undefined);
    return { ok: true };
  } catch (err) {
    log(`create-profile restart error: ${err.message}`);
    return { ok: false, error: err.message };
  }
});

ipcMain.handle('update-profile', async (_event, opts) => {
  const { id, name, username } = opts || {};
  if (!id || !appPort) return { ok: false };
  try {
    const res = await goFetch(`/api/profiles/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, username }),
    });
    return res.ok ? { ok: true } : { ok: false };
  } catch (_) { return { ok: false }; }
});

ipcMain.handle('get-profile-db-path', async (_event, id) => {
  if (!id || !appPort) return { ok: false };
  try {
    const res = await goFetch(`/api/profiles/${id}/dbpath`);
    if (!res.ok) return { ok: false, error: 'Not found' };
    const d = await res.json();
    return { ok: true, dbPath: d.db_path };
  } catch (_) { return { ok: false, error: 'Network error' }; }
});

ipcMain.handle('get-admin-data-dir', () => {
  const paths = applyResolvedDataPaths(getPaths(), activeDotenv || {});
  return { ok: true, dir: paths.adminDataDir };
});

ipcMain.handle('suggest-archive-db-path', (_event, name) => {
  const paths = applyResolvedDataPaths(getPaths(), activeDotenv || {});
  const base = String(name || 'archive').trim() || 'archive';
  return { ok: true, dbPath: path.join(paths.adminDataDir, slugify(base) + '.sqlite') };
});

// ── Ollama local AI ───────────────────────────────────────────────────────────

ipcMain.handle('check-ollama-model', () => {
  const ollamaExe = getOllamaExe();
  if (!fs.existsSync(ollamaExe)) {
    return { ok: false, error: 'Ollama executable not found at ' + ollamaExe };
  }
  // Check manifest directories — avoids spawning the CLI which needs a running daemon.
  const libDir = path.join(
    require('os').homedir(),
    '.ollama', 'models', 'manifests', 'registry.ollama.ai', 'library',
  );
  const hasDir = (name) => {
    const d = path.join(libDir, name);
    return fs.existsSync(d) && fs.readdirSync(d).length > 0;
  };
  const hasGemma4         = hasDir('gemma4');
  const hasEmbeddingModel = hasDir('embeddinggemma');
  return { ok: true, hasModel: hasGemma4 && hasEmbeddingModel, hasGemma4, hasEmbeddingModel };
});

// Pull a single Ollama model and stream progress events.
// Resolves { ok: true } on success, { ok: false, error } on failure.
// Does NOT send a 'done' event — the caller sends it after all models finish.
function pullOllamaModel(ollamaExe, modelName, send) {
  return new Promise((resolve) => {
    const proc = spawn(ollamaExe, ['pull', modelName], { windowsHide: true });
    const fmtGB = (bytes) => (bytes / 1e9).toFixed(2) + ' GB';
    const parseLine = (line) => {
      const t = line.trim();
      if (!t) return;
      try {
        const obj = JSON.parse(t);
        if (obj.total && obj.completed) {
          send({ type: 'progress', model: modelName,
                 percent: Math.round(obj.completed / obj.total * 100),
                 downloaded: fmtGB(obj.completed),
                 total: fmtGB(obj.total),
                 status: obj.status || 'downloading' });
        } else if (obj.status) {
          send({ type: 'status', status: obj.status });
        }
      } catch (_) {
        send({ type: 'status', status: t });
      }
    };
    let errOut = '';
    proc.stdout && proc.stdout.on('data', (d) => d.toString().split('\n').forEach(parseLine));
    proc.stderr && proc.stderr.on('data', (d) => {
      errOut += d.toString();
      d.toString().split('\n').forEach(parseLine);
    });
    proc.on('close', (code) => {
      if (code === 0) resolve({ ok: true });
      else resolve({ ok: false, error: errOut || `exit ${code}` });
    });
    proc.on('error', (err) => resolve({ ok: false, error: err.message }));
  });
}

ipcMain.handle('pull-ollama-model', async () => {
  const ollamaExe = getOllamaExe();
  const send = (evt) => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send('ollama-pull-progress', evt);
    }
  };

  send({ type: 'status', status: 'Downloading chat model (gemma4:latest)…' });
  const r1 = await pullOllamaModel(ollamaExe, 'gemma4:latest', send);
  if (!r1.ok) return r1;

  send({ type: 'status', status: 'Downloading embedding model (embeddinggemma:latest)…' });
  const r2 = await pullOllamaModel(ollamaExe, 'embeddinggemma:latest', send);
  if (!r2.ok) return r2;

  send({ type: 'done', ok: true });
  return { ok: true };
});

ipcMain.handle('start-ollama', async () => {
  return ensureOllamaRunning();
});

ipcMain.handle('get-auto-start-local-ai', () => {
  const paths = getPaths();
  const env = activeDotenv || loadDotEnv(paths.dotEnvPath);
  return parseBoolEnv(env.AUTO_START_LOCAL_AI);
});

ipcMain.handle('set-auto-start-local-ai', (_event, enabled) => {
  const paths = getPaths();
  try {
    let envContent = fs.existsSync(paths.dotEnvPath)
      ? fs.readFileSync(paths.dotEnvPath, 'utf8') : '';
    envContent = upsertEnvVar(envContent, 'AUTO_START_LOCAL_AI', enabled ? 'true' : 'false');
    fs.writeFileSync(paths.dotEnvPath, envContent, 'utf8');
    activeDotenv = { ...(activeDotenv || {}), AUTO_START_LOCAL_AI: enabled ? 'true' : 'false' };
    return { ok: true };
  } catch (err) {
    log(`set-auto-start-local-ai error: ${err.message}`);
    return { ok: false, error: err.message };
  }
});

app.on('before-quit', () => {
  isQuitting = true;
});

app.on('will-quit', async (e) => {
  globalShortcut.unregisterAll();
  // Only intercept if Go process is still running to allow graceful shutdown
  if (goProcess && !goProcess.killed) {
    e.preventDefault();
    await shutdown();
    app.quit();
  }
});

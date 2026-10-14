import { spawn } from 'node:child_process';
import { access, mkdtemp, realpath, rm } from 'node:fs/promises';
import { constants } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { basename, join, resolve, sep } from 'node:path';
import { browserLaunchOptions } from './browser-launch.mjs';
import { WorkerError, delay } from './errors.mjs';

const EDGE = '/opt/microsoft/msedge/msedge';
const PREFIX = 'sub2api-vault-edge-';

async function availableLoopbackPort() {
  const server = createServer();
  await new Promise((resolveListen, rejectListen) => {
    server.once('error', rejectListen);
    server.listen(0, '127.0.0.1', resolveListen);
  });
  const port = server.address().port;
  await new Promise((resolveClose, rejectClose) => server.close(error => error ? rejectClose(error) : resolveClose()));
  return port;
}

async function removeOwnedProfile(profile) {
  const root = resolve(tmpdir());
  const target = resolve(profile);
  if (!target.startsWith(root + sep) || !basename(target).startsWith(PREFIX)) return;
  try {
    if (await realpath(target) !== target) return;
    await rm(target, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 });
  } catch { /* The browser may already have removed its profile. */ }
}

function browserEnvironment(source) {
  const keys = ['HOME', 'PATH', 'LANG', 'DISPLAY', 'XAUTHORITY', 'WAYLAND_DISPLAY', 'XDG_RUNTIME_DIR',
    'DBUS_SESSION_BUS_ADDRESS', 'TMPDIR'];
  return Object.fromEntries(keys.filter(key => source[key]).map(key => [key, source[key]]));
}

async function launchLinuxPrivateEdgeOnce(chromium, signal, initialURL) {
  if (process.platform !== 'linux') throw new WorkerError('BROWSER_EDGE_PLATFORM_UNSUPPORTED');
  try { await access(EDGE, constants.X_OK); }
  catch { throw new WorkerError('BROWSER_EDGE_NOT_FOUND'); }
  const { proxy } = browserLaunchOptions('linux', 'msedge');
  const profile = await mkdtemp(join(tmpdir(), PREFIX));
  const port = await availableLoopbackPort();
  const child = spawn(EDGE, [
    `--user-data-dir=${profile}`,
    '--remote-debugging-address=127.0.0.1', `--remote-debugging-port=${port}`,
    '--inprivate', '--no-first-run', '--no-default-browser-check',
    ...(proxy ? [`--proxy-server=${proxy.server}`] : []),
    initialURL,
  ], { env: browserEnvironment(process.env), stdio: 'ignore' });
  let browser, closed = false, spawnFailed = false;
  child.on('error', () => { spawnFailed = true; });
  const close = async () => {
    if (closed) return;
    closed = true;
    if (browser?.isConnected()) {
      try {
        const session = await browser.newBrowserCDPSession();
        await Promise.race([
          session.send('Browser.close').catch(() => {}),
          new Promise(resolveTimeout => setTimeout(resolveTimeout, 2000)),
        ]);
      } catch { /* The owned browser may already have exited. */ }
    }
    await browser?.close().catch(() => {});
    if (child.exitCode === null && child.signalCode === null) {
      child.kill('SIGTERM');
      await Promise.race([
        new Promise(resolveExit => child.once('exit', resolveExit)),
        new Promise(resolveTimeout => setTimeout(resolveTimeout, 1500)),
      ]);
      if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL');
    }
    await removeOwnedProfile(profile);
  };
  try {
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      if (signal?.aborted || spawnFailed || child.exitCode !== null || child.signalCode !== null) {
        throw new WorkerError('BROWSER_EDGE_CLOSED_ON_START');
      }
      try {
        const response = await fetch(`http://127.0.0.1:${port}/json/version`, { signal: AbortSignal.timeout(1000) });
        const version = await response.json();
        if (response.ok && typeof version.webSocketDebuggerUrl === 'string' && /^Edg\//.test(version.Browser)) {
          ready = true;
          break;
        }
      } catch { /* Edge has not opened its loopback endpoint yet. */ }
      await delay(150, signal);
    }
    if (!ready) throw new WorkerError('BROWSER_EDGE_START_TIMEOUT');
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`, { timeout: 15_000 });
    const context = browser.contexts()[0];
    if (!context) throw new WorkerError('BROWSER_EDGE_CONTEXT_UNAVAILABLE');
    const page = context.pages()[0] ?? await context.waitForEvent('page', { timeout: 10_000 });
    if (!page) throw new WorkerError('BROWSER_EDGE_PAGE_UNAVAILABLE');
    return { browser, context, page, close };
  } catch (error) {
    await close();
    if (error instanceof WorkerError) throw error;
    throw new WorkerError('BROWSER_EDGE_CDP_START_FAILED');
  }
}

export async function launchLinuxPrivateEdge(chromium, signal, initialURL = 'https://chatgpt.com/') {
  // Edge occasionally exits while its new private profile or first tab is
  // starting. Each attempt owns and removes only its own temporary profile.
  for (let attempt = 0; attempt < 3; attempt++) {
    if (signal?.aborted) throw new WorkerError('WORKER_STOPPED');
    try { return await launchLinuxPrivateEdgeOnce(chromium, signal, initialURL); }
    catch (error) {
      const retryable = ['BROWSER_EDGE_CLOSED_ON_START', 'BROWSER_EDGE_START_TIMEOUT',
        'BROWSER_EDGE_CDP_START_FAILED', 'BROWSER_EDGE_CONTEXT_UNAVAILABLE',
        'BROWSER_EDGE_PAGE_UNAVAILABLE'].includes(error?.code);
      if (!retryable || attempt === 2 || signal?.aborted) throw error;
      await delay(400 * (attempt + 1), signal);
    }
  }
  throw new WorkerError('BROWSER_EDGE_START_FAILED');
}

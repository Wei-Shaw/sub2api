import { WorkerError } from './errors.mjs';

export function browserLaunchOptions(platform = process.platform, configured = process.env.ACCOUNT_VAULT_BROWSER_CHANNEL,
  proxyAddress = process.env.ACCOUNT_VAULT_BROWSER_PROXY) {
  const channel = configured || (platform === 'win32' ? 'msedge' : 'chromium');
  if (channel !== 'msedge' && channel !== 'chromium') throw new WorkerError('BROWSER_CHANNEL_INVALID');
  let proxy;
  if (proxyAddress) {
    let url;
    try { url = new URL(proxyAddress); }
    catch { throw new WorkerError('BROWSER_PROXY_INVALID'); }
    if (!['http:', 'socks5:'].includes(url.protocol) ||
      !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) ||
      !url.port || url.username || url.password || (url.pathname && url.pathname !== '/') || url.search || url.hash) {
      throw new WorkerError('BROWSER_PROXY_INVALID');
    }
    proxy = { server: `${url.protocol}//${url.host}` };
  }
  return { headless: false, chromiumSandbox: true, ...(channel === 'msedge' ? { channel: 'msedge' } : {}),
    ...(proxy ? { proxy } : {}) };
}

export function browserLaunchFailure(error, options) {
  const message = typeof error?.message === 'string' ? error.message : '';
  if (options.channel !== 'msedge') return 'BROWSER_START_FAILED_RUN_INSTALL_BROWSER';
  if (/executable (?:does not|doesn.t) exist|not found|could not find/i.test(message)) return 'BROWSER_EDGE_NOT_FOUND';
  if (/timeout|timed out/i.test(message)) return 'BROWSER_EDGE_START_TIMEOUT';
  if (/target page, context or browser has been closed|browser has been closed|process exited/i.test(message)) return 'BROWSER_EDGE_CLOSED_ON_START';
  if (/spawn|eacces|access is denied/i.test(message)) return 'BROWSER_EDGE_SPAWN_FAILED';
  return 'BROWSER_EDGE_START_FAILED';
}

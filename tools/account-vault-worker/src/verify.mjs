// A fresh sign-in check for a completed vault account. This command never
// queues a rotation or calls an MFA mutation. Credentials arrive on stdin and
// stay in memory; output contains only verification flags.
import { launchPrivateEdge } from './edge-cdp.mjs';
import { browserLaunchFailure, browserLaunchOptions } from './browser-launch.mjs';
import { ChatGPTProvider } from './provider.mjs';
import { WorkerError, delay, safeCode } from './errors.mjs';
import { validateOrigin } from './security.mjs';

async function main() {
  delete process.env.DEBUG;
  delete process.env.PWDEBUG;
  const args = process.argv.slice(2);
  const closeAfterCheck = args.length === 5 && args[4] === '--close-after-check';
  if ((args.length !== 4 && !closeAfterCheck) || args[0] !== '--origin' || args[2] !== '--id') throw new WorkerError('VERIFY_ARGUMENT_INVALID');
  const origin = validateOrigin(args[1]);
  const id = Number(args[3]);
  if (!Number.isSafeInteger(id) || id < 1) throw new WorkerError('VERIFY_ARGUMENT_INVALID');
  const chunks = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    size += chunk.length;
    if (size > 65536) throw new WorkerError('VERIFY_INPUT_INVALID');
    chunks.push(chunk);
  }
  const bytes = Buffer.concat(chunks);
  let credentials;
  try { credentials = JSON.parse(bytes.toString('utf8')); }
  catch { throw new WorkerError('VERIFY_INPUT_INVALID'); }
  finally { bytes.fill(0); for (const chunk of chunks) chunk.fill(0); }
  if (typeof credentials?.email !== 'string' || typeof credentials?.password !== 'string') throw new WorkerError('VERIFY_INPUT_INVALID');
  let token = '';
  const controller = new AbortController();
  const stop = () => controller.abort();
  process.once('SIGINT', stop);
  process.once('SIGTERM', stop);
  let host, provider, accountPassword = '';
  async function request(path, body) {
    let response;
    try {
      response = await fetch(origin + path, {
        method: body === undefined ? 'GET' : 'POST', redirect: 'error', cache: 'no-store',
        headers: { Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: AbortSignal.any([controller.signal, AbortSignal.timeout(20000)]),
      });
    } catch { throw new WorkerError('VERIFY_BACKEND_UNAVAILABLE'); }
    if (!response.ok) throw new WorkerError('VERIFY_BACKEND_REJECTED');
    const data = await response.json().catch(() => null);
    if (data?.code !== 0) throw new WorkerError('VERIFY_BACKEND_REJECTED');
    return data.data;
  }
  try {
    const auth = await request('/api/v1/auth/login', credentials);
    credentials.password = ''; credentials = undefined;
    token = auth.access_token;
    if (typeof token !== 'string' || !token) throw new WorkerError('VERIFY_BACKEND_REJECTED');
    const list = await request('/api/v1/admin/account-vault?page=1&page_size=100');
    const account = list.items?.find(row => row.id === id);
    if (!account || account.rotation_state !== 'completed' || !account.rotation_completed_at) throw new WorkerError('VERIFY_ACCOUNT_NOT_COMPLETED');
    const jobs = await request('/api/v1/admin/account-vault/rotation/jobs/query', { ids: [id] });
    const job = jobs.jobs?.find(row => row.account_id === id);
    if (job?.status !== 'completed' || job.phase !== 'completed') throw new WorkerError('VERIFY_ACCOUNT_NOT_COMPLETED');
    const password = await request(`/api/v1/admin/account-vault/${id}/password`, {});
    accountPassword = password.password; password.password = '';
    if (typeof accountPassword !== 'string' || !accountPassword) throw new WorkerError('VERIFY_ACCOUNT_PASSWORD_MISSING');
    const { chromium } = await import('playwright');
    const privateEdge = process.platform === 'win32' && !process.env.ACCOUNT_VAULT_BROWSER_CHANNEL;
    if (privateEdge) {
      host = await launchPrivateEdge(chromium, controller.signal);
    } else {
      const launchOptions = browserLaunchOptions();
      let browser;
      try { browser = await chromium.launch(launchOptions); }
      catch (error) { throw new WorkerError(browserLaunchFailure(error, launchOptions)); }
      host = { browser, close: () => browser.close() };
      host.context = await browser.newContext({ acceptDownloads: false, serviceWorkers: 'block' });
      host.page = await host.context.newPage();
    }
    provider = new ChatGPTProvider({
      page: host.page, context: host.context, signal: controller.signal,
      manualLogin: false, externalNavigation: privateEdge, onUnexpectedClose: stop,
    });
    let otpSubmissions = 0, lastWait = '';
    console.log(`账号 #${id}：开始用当前账号库验证码重新登录；本次只复核，不换绑。`);
    await provider.login({
      email: account.email, password: accountPassword, checkLease: () => {},
      getOldCode: async () => {
        for (;;) {
          const result = await request('/api/v1/admin/account-vault/codes', { ids: [id] });
          const code = result.items?.find(row => row.id === id);
          if (!code || code.error || !/^\d{6}(?:\d{2})?$/.test(code.code)) throw new WorkerError('VERIFY_CURRENT_CODE_UNAVAILABLE');
          if (code.remaining >= 8) return { code: code.code };
          await delay((code.remaining + 1) * 1000, controller.signal);
        }
      },
      onLoginSubmit: kind => {
        if (kind === 'otp') { otpSubmissions++; console.log('已提交账号库当前种子生成的身份验证器验证码，等待官方结果。'); }
      },
      onManual: async reason => {
        if (reason !== lastWait) {
          lastWait = reason;
          console.log(reason === 'LOGIN_PROVIDER_REDIRECT' ? '官方登录中转页加载中，暂无需操作。' : reason === 'LOGIN_CLOUDFLARE_CHALLENGE' ? '请在复核窗口亲自完成人机验证，完成后自动继续。' : '请在复核窗口完成邮件码、Passkey 或页面要求的人工验证，完成后自动继续。');
        }
        await delay(1500, controller.signal);
        return false;
      },
    });
    accountPassword = '';
    const mfa = await provider.mfa();
    const remoteMfaEnabled = mfa.enabled && mfa.enabledV2 && mfa.ids.length === 1 && mfa.ids[0] === mfa.defaultId;
    const currentSeedVerified = otpSubmissions > 0 && remoteMfaEnabled;
    console.log(JSON.stringify({ account_id: id, identity_matched: true, current_seed_login_verified: currentSeedVerified, current_seed_otp_submitted: otpSubmissions > 0, remote_mfa_enabled: remoteMfaEnabled, checked_at: new Date().toISOString() }));
    if (!currentSeedVerified) throw new WorkerError('VERIFY_OTP_NOT_EXERCISED');
    console.log(closeAfterCheck ? '当前种子已通过全新登录验证，本条复核窗口将关闭。' : '当前种子已通过全新登录验证。窗口先保留供你查看，关闭窗口后复核结束。');
    token = '';
    if (closeAfterCheck) return;
    await host.page.waitForEvent('close', { timeout: 0 }).catch(() => {});
  } finally {
    token = ''; accountPassword = '';
    if (credentials) credentials.password = '';
    await provider?.close();
    await host?.close();
    process.removeListener('SIGINT', stop);
    process.removeListener('SIGTERM', stop);
  }
}

main().catch(error => { console.error(`复核停止：${safeCode(error, 'VERIFY_FAILED')}。`); process.exitCode = 1; });

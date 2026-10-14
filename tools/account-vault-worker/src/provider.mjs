import { WorkerError, assertAlive, delay } from './errors.mjs';
import { assertIdentity, normalizeMfa, parseEnrollment } from './security.mjs';
import { registerOAuthCallback } from './oauth-callback.mjs';

const OFFICIAL_ORIGIN = 'https://chatgpt.com';
const LOGIN_ORIGINS = new Set([OFFICIAL_ORIGIN, 'https://auth.openai.com']);
const CONTINUE = /^(?:Continue|Next|Log in|Sign in|继续|下一步|登录|登入)$/i;
const LOGIN = /^(?:Log in|Sign in|登录|登入)$/i;
const AUTHENTICATOR = /authenticator(?: app)?|authentication app|身份验证器|驗證器|身份验证应用|身份驗證應用/i;
const EXTRA_CHALLENGE = /check your (?:email|inbox)|email verification|verify your email|passkey|security key|检查(?:您|你)?的?邮箱|查看(?:您|你)?的?邮箱|邮件验证码|電子郵件驗證|通行密钥|安全密钥/i;
const EMAIL_INPUT_SELECTOR = 'input[type="email"], input[autocomplete="username"], input[name="email"], input[placeholder="电子邮件地址"], input[placeholder="Email address"]';

export const ordinaryClickDelay = (random = Math.random) => 1000 + Math.floor(Math.min(1, Math.max(0, random())) * 1000);

async function uniqueVisible(locator) {
  const visible = [];
  for (const element of await locator.all()) {
    if (await element.isVisible()) visible.push(element);
  }
  return visible.length === 1 ? visible[0] : null;
}

async function anyVisible(locator) {
  for (const element of await locator.all()) {
    if (await element.isVisible()) return true;
  }
  return false;
}

// The Windows host owns a fresh temporary Edge profile and its loopback CDP
// connection. This provider does not export storageState, HAR, traces, video,
// screenshots or response bodies.
export class ChatGPTProvider {
  #token = '';
  #context;
  #page;
  #origin;
  #allowedLoginOrigins;
  #signal;
  #closePromise;
  #closing = false;
  #browser;
  #closedListener;
  #manualLogin;
  #externalNavigation;
  #ownsContext;
  #lastLoginURL = '';
  #loginURLSeenAt = 0;
  #lastLoginStep = '';
  #loginStepSeenAt = 0;
  #callbackOrigin;
  #loginRedirectWaitMs;
  #loginHydrationWaitMs;
  #clickDelay;

  constructor({ page, context, signal, testOrigin, testCallbackOrigin, manualLogin = !testOrigin,
    externalNavigation = false, ownsContext = true, testLoginRedirectWaitMs,
    onUnexpectedClose = () => {} }) {
    this.#page = page;
    this.#clickDelay = testOrigin ? () => 0 : ordinaryClickDelay;
    this.#context = context;
    this.#signal = signal;
    this.#browser = context.browser();
    this.#manualLogin = manualLogin;
    this.#externalNavigation = externalNavigation;
    this.#ownsContext = ownsContext;
    this.#closedListener = () => {
      if (!this.#closing) onUnexpectedClose();
    };
    page.on('close', this.#closedListener);
    context.on('close', this.#closedListener);
    this.#browser?.on('disconnected', this.#closedListener);
    // Dependency injection is only used by local synthetic browser tests.
    this.#origin = testOrigin ?? OFFICIAL_ORIGIN;
    this.#allowedLoginOrigins = testOrigin ? new Set([testOrigin]) : LOGIN_ORIGINS;
    this.#callbackOrigin = testOrigin && testCallbackOrigin ? testCallbackOrigin : 'http://localhost:1455';
    this.#loginRedirectWaitMs = testOrigin ? (testLoginRedirectWaitMs ?? 20_000) : 20_000;
    this.#loginHydrationWaitMs = testOrigin ? 0 : 2000;
  }

  async #fetch(path, { method = 'GET', body, bearer = this.#token } = {}) {
    assertAlive(this.#signal);
    if (new URL(this.#page.url()).origin !== this.#origin || !path.startsWith('/') || path.startsWith('//')) {
      throw new WorkerError('UPSTREAM_ORIGIN_REQUIRED');
    }
    try {
      const result = await this.#page.evaluate(async ({ origin, path, method, body, bearer }) => {
        if (location.origin !== origin) return { failure: 'origin' };
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 20_000);
        try {
          const response = await fetch(path, {
            method, credentials: 'include', cache: 'no-store', redirect: 'error', signal: controller.signal,
            headers: { Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...(bearer ? { Authorization: `Bearer ${bearer}` } : {}) },
            body: body === undefined ? undefined : JSON.stringify(body),
          });
          const reader = response.body?.getReader();
          if (!reader) return { status: response.status, data: null };
          const chunks = []; let size = 0;
          for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            size += value.byteLength;
            if (size > 128 * 1024) { await reader.cancel(); return { failure: 'large' }; }
            chunks.push(value);
          }
          const raw = new Uint8Array(size); let offset = 0;
          for (const chunk of chunks) { raw.set(chunk, offset); offset += chunk.length; chunk.fill(0); }
          let data;
          try { data = JSON.parse(new TextDecoder().decode(raw)); } catch { data = null; }
          raw.fill(0);
          return { status: response.status, data };
        } catch {
          return { failure: 'request' };
        } finally { clearTimeout(timer); }
      }, { origin: this.#origin, path, method, body, bearer });
      assertAlive(this.#signal);
      if (result.failure) throw new WorkerError('UPSTREAM_RESPONSE_UNKNOWN', { unknown: method !== 'GET' });
      return result;
    } catch (error) {
      if (error instanceof WorkerError) throw error;
      throw new WorkerError('UPSTREAM_RESPONSE_UNKNOWN', { unknown: method !== 'GET' });
    }
  }

  async identity(expectedEmail, { allowUnauthenticated = false } = {}) {
    let result = await this.#fetch('/backend-api/me');
    const incomplete = result.status === 200 &&
      (typeof result.data?.id !== 'string' || !result.data.id ||
       typeof result.data?.email !== 'string' || !result.data.email.trim());
    if ([401, 403].includes(result.status) || incomplete) {
      this.#token = '';
      // This optional public-web session route was not present in the supplied
      // sanitized API fixture. Incompatibility pauses; it is never invented as
      // a guaranteed platform login method and never sent back to the backend.
      const session = await this.#fetch('/api/auth/session', { bearer: '' });
      if (session.status === 200 && typeof session.data?.accessToken === 'string' && session.data.accessToken.length <= 32_768) {
        this.#token = session.data.accessToken;
        result = await this.#fetch('/backend-api/me');
      }
    }
    if ([401, 403].includes(result.status) && allowUnauthenticated) return null;
    if (result.status !== 200) throw new WorkerError('UPSTREAM_AUTH_REQUIRED');
    if (typeof result.data?.id === 'string' && result.data.id &&
        (typeof result.data.email !== 'string' || !result.data.email.trim())) {
      const session = await this.#fetch('/api/auth/session', { bearer: '' });
      const user = session.status === 200 ? session.data?.user : null;
      if (user?.id === result.data.id && typeof user.email === 'string' && user.email) {
        if (typeof session.data.accessToken === 'string' && session.data.accessToken.length <= 32_768) this.#token = session.data.accessToken;
        return assertIdentity({ id: result.data.id, email: user.email }, expectedEmail);
      }
    }
    // The public landing page can return an incomplete shell identity before
    // login. Only a complete, valid email is evidence of a different account.
    // A strict identity check still runs before every MFA read/write.
    if (allowUnauthenticated && (typeof result.data?.id !== 'string' || !result.data.id ||
        typeof result.data?.email !== 'string' || !result.data.email.trim())) return null;
    return assertIdentity(result.data, expectedEmail);
  }

  async #post(path, body) {
    const result = await this.#fetch(path, { method: 'POST', body });
    if (result.status !== 200) {
      // Even a non-200 response is not treated as proof that an upstream write
      // did nothing. The durable intent is reconciled before any future write.
      throw new WorkerError([401, 403].includes(result.status) ? 'UPSTREAM_AUTH_REQUIRED' : 'UPSTREAM_WRITE_UNCONFIRMED', { unknown: true });
    }
    return result.data;
  }

  async mfa() {
    const result = await this.#fetch('/backend-api/accounts/mfa_info');
    if (result.status !== 200) throw new WorkerError('UPSTREAM_MFA_UNAVAILABLE');
    return normalizeMfa(result.data);
  }

  async disable(factorId) {
    await this.#post('/backend-api/accounts/mfa/user/disable_in_house', { factor_id: factorId });
  }

  async enroll(expectedEmail, oldFactorId) {
    const data = await this.#post('/backend-api/accounts/mfa/enroll', { factor_type: 'totp', source: 'settings' });
    return parseEnrollment(data, expectedEmail, oldFactorId);
  }

  async activate({ code, sessionId }) {
    if (!/^\d{6}(?:\d{2})?$/.test(code) || !sessionId) throw new WorkerError('ACTIVATION_INPUT_INVALID');
    const result = await this.#post('/backend-api/accounts/mfa/user/activate_enrollment', {
      code, factor_type: 'totp', session_id: sessionId, source: 'settings',
    });
    if (result?.success !== true) throw new WorkerError('ACTIVATION_UNCONFIRMED', { unknown: true });
  }

  async session(expectedIdentity) {
    const result = await this.#fetch('/api/auth/session', { bearer: '' });
    if (result.status !== 200 || typeof result.data?.accessToken !== 'string' || !result.data.accessToken
      || result.data.accessToken.length > 32768) throw new WorkerError('SESSION_UNAVAILABLE');
    const identity = assertIdentity(result.data.user, expectedIdentity.email);
    if (identity.id !== expectedIdentity.id) throw new WorkerError('ACCOUNT_IDENTITY_MISMATCH');
    // Some session endpoints omit the HTTP-only session cookie from JSON. If
    // present, include only that session cookie in the explicitly requested
    // encrypted session file, never any unrelated browser cookies.
    if (!result.data.sessionToken) {
      const cookies = await this.#context.cookies(this.#origin);
      for (const name of ['__Secure-next-auth.session-token', '__Secure-authjs.session-token', 'next-auth.session-token', 'authjs.session-token']) {
        const whole = cookies.find(cookie => cookie.name === name);
        const chunks = cookies.filter(cookie => cookie.name.startsWith(name + '.') && /^\d+$/.test(cookie.name.slice(name.length + 1)))
          .sort((a, b) => Number(a.name.slice(name.length + 1)) - Number(b.name.slice(name.length + 1)));
        const completeChunks = chunks.every((cookie, index) => Number(cookie.name.slice(name.length + 1)) === index);
        const token = whole?.value ?? (completeChunks ? chunks.map(cookie => cookie.value).join('') : '');
        if (token) { result.data.sessionToken = token; break; }
      }
    }
    return result.data;
  }

  async dismissKnownOnboarding() {
    try {
      const current = new URL(this.#page.url());
      if (current.origin !== this.#origin || current.pathname !== '/') return false;
      const introduction = await this.#page.getByText(/更相关、更个性化的回复|More relevant, more personalized responses/i).count();
      if (!introduction) return false;
      const acknowledge = await uniqueVisible(this.#page.getByRole('button', { name: /^(?:知道了|Got it)$/i }));
      if (!acknowledge) return false;
      await this.#clickOrdinary(acknowledge);
      return true;
    } catch {
      // This prompt is unrelated to MFA. Navigation or a changed page must
      // not turn an already verified rotation into a false failure.
      return false;
    }
  }

  async #selectExpectedAccount(email) {
    const current = new URL(this.#page.url());
    const authOrigin = this.#origin === OFFICIAL_ORIGIN ? 'https://auth.openai.com' : this.#origin;
    if (current.origin !== authOrigin || current.pathname !== '/choose-an-account') return false;
    const escaped = email.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const button = await uniqueVisible(this.#page.getByRole('button').filter({ hasText: new RegExp(escaped, 'i') }));
    if (!button) return false;
    // The click may succeed while the next auth page takes longer to load.
    // Do not treat Playwright's navigation wait as a failed login and close it.
    try { await this.#clickOrdinary(button); }
    catch { assertAlive(this.#signal); }
    await delay(250, this.#signal);
    return true;
  }

  async #clickOrdinary(button) {
    const url = this.#page.url();
    if (!await button.isVisible()) return false;
    if (!await button.isEnabled()) throw new WorkerError('LOGIN_MANUAL_REQUIRED');
    await delay(this.#clickDelay(), this.#signal);
    assertAlive(this.#signal);
    if (this.#page.url() !== url) return false;
    await button.click({ timeout: 3000, noWaitAfter: true });
    return true;
  }

  async authorizeSession(authURL, { email, password, expectedAccountId, getOldCode, checkLease, onManual }) {
    let authorize, redirect;
    try {
      authorize = new URL(authURL);
      redirect = new URL(authorize.searchParams.get('redirect_uri'));
    } catch { throw new WorkerError('SESSION_AUTHORIZATION_INVALID'); }
    const authOrigin = this.#origin === OFFICIAL_ORIGIN ? 'https://auth.openai.com' : this.#origin;
    const state = authorize.searchParams.get('state');
    if (authorize.origin !== authOrigin || authorize.pathname !== '/oauth/authorize' || authorize.username || authorize.password ||
        redirect.origin !== this.#callbackOrigin || redirect.pathname !== '/auth/callback' || redirect.search || redirect.hash ||
        redirect.username || redirect.password || !state || state.length > 256 ||
        authorize.searchParams.get('response_type') !== 'code' || authorize.searchParams.get('code_challenge_method') !== 'S256') {
      throw new WorkerError('SESSION_AUTHORIZATION_INVALID');
    }
    let callback, callbackError;
    // A different local app may own 1455. We still capture this page's exact
    // redirect before it fails; HTTP success at the callback is not required.
    const releaseCallback = await registerOAuthCallback(state, redirect.origin).catch(error => {
      if (error.code !== 'SESSION_CALLBACK_UNAVAILABLE') throw error;
      return () => {};
    });
    const handler = request => {
      const url = new URL(request.url());
      if (url.origin !== redirect.origin || url.pathname !== redirect.pathname) return;
      const code = url.searchParams.get('code');
      if (request.method() === 'GET' && request.isNavigationRequest() && request.frame() === this.#page.mainFrame() &&
          url.searchParams.getAll('state').length === 1 && url.searchParams.get('state') === state &&
          url.searchParams.getAll('code').length === 1 && code && code.length <= 8192 && !url.searchParams.has('error')) {
        callback = { code, state };
      } else callbackError = new WorkerError('SESSION_AUTHORIZATION_REJECTED');
    };
    this.#page.on('request', handler);
    try {
      await checkLease();
      try { await this.#page.goto(authorize.href, { waitUntil: 'commit', timeout: 30_000 }); }
      catch { if (!callback && !callbackError) await onManual('LOGIN_PROVIDER_REDIRECT'); }
      let lastURL = '', lastSubmit = 0, oldCode;
      for (;;) {
        assertAlive(this.#signal);
        await checkLease();
        if (callbackError) throw callbackError;
        if (callback) return callback;
        const current = new URL(this.#page.url());
        if (!this.#allowedLoginOrigins.has(current.origin)) { await onManual('LOGIN_OAUTH_CONSENT_REQUIRED'); continue; }
        const challenge = /^(?:just a moment|请稍候|请稍等)/i.test(await this.#page.title().catch(() => '')) ||
          await this.#page.locator('iframe[src*="captcha"], iframe[src*="challenges.cloudflare.com"], input[name="cf-turnstile-response"]').count();
        if (challenge) { await onManual('LOGIN_CLOUDFLARE_CHALLENGE'); continue; }
        if (await this.#selectExpectedAccount(email)) continue;
        // Provider challenges stay manual. A known Codex consent is submitted
        // only for the user's requested account and the captured workspace.
        if (await this.#page.getByRole('heading', { name: EXTRA_CHALLENGE }).count()) { await onManual('LOGIN_MANUAL_REQUIRED'); continue; }
        if (current.href !== lastURL) { lastURL = current.href; lastSubmit = 0; }
        if (lastSubmit && performance.now() - lastSubmit < 4000) { await delay(250, this.#signal); continue; }
        if (current.pathname === '/sign-in-with-chatgpt/codex/consent') {
          const visibleEmail = await uniqueVisible(this.#page.getByText(email, { exact: true }));
          const selected = await this.#page.locator('input[name="workspace_id"]:checked, input[type="hidden"][name="workspace_id"]').evaluateAll(inputs => inputs.map(input => input.value).filter(Boolean));
          const workspace = expectedAccountId || selected[0];
          if (visibleEmail && workspace && selected.length > 0 && selected.every(value => value === workspace)) {
            const button = await uniqueVisible(this.#page.getByRole('button', { name: /^(?:Continue|继续)$/i }));
            if (button) {
              try { if (!await this.#clickOrdinary(button)) continue; }
              catch { assertAlive(this.#signal); await onManual('LOGIN_OAUTH_CONSENT_REQUIRED'); }
              lastSubmit = performance.now(); continue;
            }
          }
          await onManual('LOGIN_OAUTH_CONSENT_REQUIRED');
          continue;
        }
        if (/^\/log-?in(?:\/|$)/i.test(current.pathname) || /^\/mfa-challenge(?:\/|$)/i.test(current.pathname)) {
          const emailInput = await uniqueVisible(this.#page.locator(EMAIL_INPUT_SELECTOR));
          const passwordInput = await uniqueVisible(this.#page.locator('input[type="password"]'));
          const otpInput = await uniqueVisible(this.#page.locator('input[autocomplete="one-time-code"]'));
          let input, value;
          if (passwordInput) { input = passwordInput; value = password; }
          else if (emailInput) { input = emailInput; value = email; }
          else if (otpInput && await this.#page.getByText(AUTHENTICATOR).count()) {
            if (!oldCode || oldCode.validUntilMono - performance.now() < 7000) oldCode = await getOldCode();
            input = otpInput; value = oldCode.code;
          }
          if (input && value) {
            const form = input.locator('xpath=ancestor::form[1]');
            const button = await uniqueVisible((await form.count() ? form : this.#page).getByRole('button', { name: CONTINUE }));
            if (button) {
              try { await input.fill(value, { timeout: 3000 }); if (!await this.#clickOrdinary(button)) continue; }
              catch { assertAlive(this.#signal); await onManual('LOGIN_MANUAL_REQUIRED'); }
              value = ''; lastSubmit = performance.now(); continue;
            }
          }
          await onManual('LOGIN_MANUAL_REQUIRED');
        } else await onManual('LOGIN_OAUTH_CONSENT_REQUIRED');
      }
    } finally { this.#page.off('request', handler); await releaseCallback(); }
  }

  async login({ email, password, getOldCode, onManual, checkLease, authorizationOnly = false, onLoginSubmit = () => {} }) {
    assertAlive(this.#signal);
    let navigationFailed = this.#externalNavigation;
    if (!this.#externalNavigation) {
      try { await this.#page.goto(this.#origin + '/', { waitUntil: 'commit', timeout: 30_000 }); }
      catch {
        // Keep the visible browser open so the operator can retry navigation.
        navigationFailed = true;
        await onManual('LOGIN_NAVIGATION_REQUIRED');
      }
    }
    const attempts = { entry: 0, email: 0, password: 0, otp: 0 };
    let oldLoginCode;
    let redirectSeenAt;
    for (;;) {
      assertAlive(this.#signal);
      await checkLease();
      let currentOrigin;
      let currentPath = '';
      let challengeURL = false;
      try {
        const currentURL = new URL(this.#page.url());
        currentOrigin = currentURL.origin;
        currentPath = currentURL.pathname;
        challengeURL = currentURL.searchParams.has('_cf_chl_rt_tk') || currentURL.pathname.includes('/cdn-cgi/challenge-platform/');
      } catch { currentOrigin = ''; }
      const challengeTitle = /^(?:just a moment|请稍候|请稍等)/i.test(await this.#page.title().catch(() => ''));
      const challengeFrame = await this.#page.locator('iframe[src*="captcha"], iframe[src*="challenges.cloudflare.com"], input[name="cf-turnstile-response"]').count();
      if (challengeURL || challengeTitle || challengeFrame) { await onManual('LOGIN_CLOUDFLARE_CHALLENGE'); continue; }
      if (await this.#selectExpectedAccount(email)) continue;
      if (navigationFailed && !this.#allowedLoginOrigins.has(currentOrigin)) {
        await onManual('LOGIN_NAVIGATION_REQUIRED');
        continue;
      }
      if (currentOrigin === this.#origin && currentPath === '/') await this.dismissKnownOnboarding();
      // This is only a UI readiness signal, not an identity assertion. The
      // server verifies the OAuth token email/subject/workspace before import.
      // Avoid the web Session and slow /me probes in an OAuth-only workflow.
      if (authorizationOnly && currentOrigin === this.#origin && currentPath === '/' &&
          await anyVisible(this.#page.locator('[data-testid="accounts-profile-button"], button[aria-label="打开个人资料菜单"], button[aria-label="Open profile menu"]'))) return { email };
      // The live OpenAI login form is a client application. Submitting it
      // before hydration can send a native HTML form request to a JSON route,
      // yielding HTTP 400. Let the operator finish this provider-owned step;
      // resume automatically only after a matching authenticated identity is
      // observable on chatgpt.com. Synthetic tests can exercise both modes.
      if (this.#manualLogin) {
        // Filling these exact provider login fields does not submit a form.
        // Never put vault credentials into another origin or an unrelated form.
        const officialHome = currentOrigin === OFFICIAL_ORIGIN && currentPath === '/';
        const officialAuth = currentOrigin === 'https://auth.openai.com' && /^\/log-?in(?:\/|$)/i.test(currentPath);
        const testLogin = this.#origin !== OFFICIAL_ORIGIN && currentOrigin === this.#origin &&
          (currentPath === '/' || /^\/log-?in(?:\/|$)/i.test(currentPath));
        if (officialHome || officialAuth || testLogin) {
          // The ChatGPT home page opens its email form in a dialog without
          // changing the URL. Match the field itself, not just auth.openai.com.
          const emailInput = await uniqueVisible(this.#page.locator(EMAIL_INPUT_SELECTOR));
          const passwordInput = await uniqueVisible(this.#page.locator('input[type="password"]'));
          const otpInput = await uniqueVisible(this.#page.locator('input[autocomplete="one-time-code"]'));
          try {
            if (emailInput && await emailInput.inputValue() !== email) await emailInput.fill(email, { timeout: 5000 });
            if (!officialHome && passwordInput && password && await passwordInput.inputValue() !== password) await passwordInput.fill(password, { timeout: 5000 });
            if (!officialHome && otpInput && await this.#page.getByText(AUTHENTICATOR).count()) {
              if (!oldLoginCode || oldLoginCode.validUntilMono - performance.now() < 7000) oldLoginCode = await getOldCode();
              if (await otpInput.inputValue() !== oldLoginCode.code) await otpInput.fill(oldLoginCode.code, { timeout: 5000 });
            }
          } catch (error) {
            if (error instanceof WorkerError) throw error;
            // Navigation or hydration can replace the form mid-fill. Keep the
            // browser open and let the operator click or complete the page.
          }
          // Opening the official sign-in dialog is safe even when submitting
          // its hydrated form still needs the operator. The home page can
          // expose equivalent login buttons in both header and sidebar.
          if ((officialHome || testLogin) && !emailInput && !passwordInput && !otpInput && attempts.entry < 3) {
            const entries = [];
            for (const candidate of await this.#page.getByRole('button', { name: LOGIN }).all()) {
              if (await candidate.isVisible()) entries.push(candidate);
            }
            const button = entries.length >= 1 && entries.length <= 3 ? entries[0] : null;
            if (button) {
              try {
                if (!await this.#clickOrdinary(button)) continue;
                attempts.entry++;
                await delay(250, this.#signal);
                continue;
              } catch {
                attempts.entry++;
                assertAlive(this.#signal);
              }
            }
          }
        }
        if (!authorizationOnly && currentOrigin === this.#origin && currentPath !== '/choose-an-account') {
          try {
            const me = await this.identity(email, { allowUnauthenticated: true });
            if (me) return me;
          } catch (error) {
            if (error instanceof WorkerError && (error.code === 'ACCOUNT_IDENTITY_MISMATCH' || error.code.startsWith('ACCOUNT_IDENTITY_RESPONSE_'))) throw error;
          }
        }
        await onManual('LOGIN_MANUAL_REQUIRED');
        continue;
      }
      // Bound the wait on ChatGPT's blank login redirect. OAuth authorization
      // has its own official login flow in the same browser; returning this
      // explicit outcome does not assert that the account is authenticated.
      // Challenges above always take precedence over this recovery.
      if (currentOrigin === this.#origin && currentPath === '/auth/login_with' &&
          (await this.#page.locator(`${EMAIL_INPUT_SELECTOR}, input[type="password"], input[autocomplete="one-time-code"]`).count()) === 0) {
        redirectSeenAt ??= performance.now();
        if (performance.now() - redirectSeenAt >= this.#loginRedirectWaitMs) {
          const blank = !(await this.#page.locator('body').innerText().catch(() => 'loading')).trim();
          if (authorizationOnly && blank) return { authorizationLoginRequired: true };
          await onManual('LOGIN_NAVIGATION_REQUIRED');
        } else await onManual('LOGIN_PROVIDER_REDIRECT');
        continue;
      }
      redirectSeenAt = undefined;
      // The signed-out home page can take a long time to answer identity API
      // calls. When a visible sign-in control is present, handle it first.
      const signedOutControls = currentOrigin === this.#origin &&
        (currentPath === '/choose-an-account' || (await this.#page.getByRole('button', { name: LOGIN }).count()) > 0 ||
         (await this.#page.locator(EMAIL_INPUT_SELECTOR).count()) > 0);
      if (!authorizationOnly && currentOrigin === this.#origin && !signedOutControls) {
        try {
          const me = await this.identity(email, { allowUnauthenticated: true });
          if (me) return me;
        } catch (error) {
          if (error instanceof WorkerError && (error.code === 'ACCOUNT_IDENTITY_MISMATCH' || error.code.startsWith('ACCOUNT_IDENTITY_RESPONSE_'))) throw error;
        }
      }
      const extra = await this.#page.getByRole('heading', { name: EXTRA_CHALLENGE }).count();
      if (!this.#allowedLoginOrigins.has(currentOrigin) || extra) {
        await onManual('LOGIN_MANUAL_REQUIRED');
        continue;
      }
      // Reset duplicate-submit tracking after navigation. Ordinary buttons
      // receive a single bounded delay once visible and enabled.
      const currentURL = this.#page.url();
      if (currentURL !== this.#lastLoginURL) {
        this.#lastLoginURL = currentURL;
        this.#loginURLSeenAt = performance.now();
      }
      const emailInput = await uniqueVisible(this.#page.locator(EMAIL_INPUT_SELECTOR));
      const passwordInput = await uniqueVisible(this.#page.locator('input[type="password"]'));
      const otpInput = await uniqueVisible(this.#page.locator('input[autocomplete="one-time-code"]'));
      const step = passwordInput ? 'password' : emailInput ? 'email' : otpInput ? 'otp' : 'entry';
      if (step !== this.#lastLoginStep) {
        this.#lastLoginStep = step;
        this.#loginStepSeenAt = performance.now();
      }
      let input, value, kind;
      if (passwordInput && attempts.password < 2 && password) {
        input = passwordInput; value = password; kind = 'password';
      } else if (emailInput && attempts.email < 2) {
        input = emailInput; value = email; kind = 'email';
      } else if (otpInput && attempts.otp < 2 && await this.#page.getByText(AUTHENTICATOR).count()) {
        const response = await getOldCode();
        if (!/^\d{6}(?:\d{2})?$/.test(response.code)) throw new WorkerError('OLD_OTP_INVALID');
        input = otpInput; value = response.code; kind = 'otp';
      }
      if (input) {
        // The live login form is hydrated after its fields first appear. A
        // premature click can submit native HTML to a JSON route (HTTP 400).
        // Wait for this step to settle before the usual 1–2 second click delay.
        const settleDelay = this.#loginHydrationWaitMs -
          (performance.now() - Math.max(this.#loginURLSeenAt, this.#loginStepSeenAt));
        if (settleDelay > 0) await delay(settleDelay, this.#signal);
        if (this.#page.url() !== currentURL) continue;
        await checkLease();
        assertAlive(this.#signal);
        const form = input.locator('xpath=ancestor::form[1]');
        const buttons = (await form.count() ? form : this.#page).getByRole('button', { name: CONTINUE });
        const button = await uniqueVisible(buttons);
        if (button) {
          try { await input.fill(value, { timeout: 3000 }); if (!await this.#clickOrdinary(button)) continue; }
          catch {
            // A login submit can navigate while Playwright is waiting for the
            // click to settle, or a challenge can disable the button. Neither
            // should end the task and close the browser the operator is using.
            value = '';
            attempts[kind]++;
            await onManual('LOGIN_MANUAL_REQUIRED');
            continue;
          }
          value = ''; attempts[kind]++;
          onLoginSubmit(kind);
          await delay(250, this.#signal);
          continue;
        }
      }
      if (!emailInput && !passwordInput && !otpInput && attempts.entry < 3) {
        // ChatGPT currently renders equivalent Log in buttons in both its
        // header and sidebar. Either opens the same official sign-in dialog.
        const entries = [];
        for (const candidate of await this.#page.getByRole('button', { name: LOGIN }).all()) {
          if (await candidate.isVisible()) entries.push(candidate);
        }
        const button = entries.length >= 1 && entries.length <= 3 ? entries[0] : null;
        if (button) {
          try { if (!await this.#clickOrdinary(button)) continue; }
          catch {
            attempts.entry++;
            await onManual('LOGIN_MANUAL_REQUIRED');
            continue;
          }
          attempts.entry++;
          await delay(250, this.#signal);
          continue;
        }
      }
      await onManual('LOGIN_MANUAL_REQUIRED');
    }
  }

  async close() {
    this.#token = '';
    this.#closing = true;
    this.#page.off('close', this.#closedListener);
    this.#context.off('close', this.#closedListener);
    this.#browser?.off('disconnected', this.#closedListener);
    // Abort listeners and the job's finally block may close simultaneously.
    // They must all wait for the same completion before another account opens.
    this.#closePromise ??= (this.#ownsContext ? this.#context.close() : this.#page.close()).catch(() => {});
    await this.#closePromise;
  }
}

export async function createProvider(browser, signal, onUnexpectedClose, options = {}) {
  const context = options.context ?? await browser.newContext({ acceptDownloads: false, serviceWorkers: 'block' });
  const page = options.page ?? await context.newPage();
  page.setDefaultTimeout(5000);
  return new ChatGPTProvider({ context, page, signal, onUnexpectedClose,
    externalNavigation: options.externalNavigation, ownsContext: !options.context,
    manualLogin: options.manualLogin });
}

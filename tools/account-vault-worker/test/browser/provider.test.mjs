import test from 'node:test';
import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { ChatGPTProvider } from '../../src/provider.mjs';
import { isNewFactorConfirmed } from '../../src/security.mjs';
import { WorkerError } from '../../src/errors.mjs';
import { fixtureServer } from './fixture.mjs';
import { createServer } from 'node:net';

const portProbe = createServer();
await new Promise(resolve => portProbe.listen(0, '127.0.0.1', resolve));
const callbackOrigin = `http://localhost:${portProbe.address().port}`;
await new Promise(resolve => portProbe.close(resolve));

async function browserFixture(t, options = {}) {
  const server = await fixtureServer(options);
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.VAULT_TEST_BROWSER ? { executablePath: process.env.VAULT_TEST_BROWSER } : {}),
    // Only this synthetic test harness supports a root/container browser.
    args: process.env.VAULT_TEST_BROWSER ? ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu'] : [],
  });
  const context = await browser.newContext({ acceptDownloads: false, serviceWorkers: 'block' });
  await context.route('**/*', route => new URL(route.request().url()).origin === server.origin ? route.continue() : route.abort());
  const page = await context.newPage();
  const controller = new AbortController();
  const provider = new ChatGPTProvider({ page, context, signal: controller.signal, testOrigin: server.origin, testCallbackOrigin: callbackOrigin, testLoginRedirectWaitMs: options.testLoginRedirectWaitMs, manualLogin: options.manualLogin, onUnexpectedClose: () => controller.abort() });
  controller.signal.addEventListener('abort', () => { void provider.close(); }, { once: true });
  t.after(async () => { await provider.close(); await browser.close(); await server.close(); });
  return { ...server, context, page, provider, controller };
}

const loginOptions = {
  email: 'account@example.test', password: 'PUBLIC-SYNTHETIC-PASSWORD',
  getOldCode: async () => ({ code: '123456' }), checkLease: () => {},
  onManual: async () => { throw new Error('synthetic login unexpectedly required manual input'); },
};

function syntheticAuthURL(origin, state = 'synthetic-state') {
  const url = new URL('/oauth/authorize', origin);
  url.search = new URLSearchParams({ redirect_uri: callbackOrigin + '/auth/callback', state, response_type: 'code', code_challenge_method: 'S256' }).toString();
  return url.href;
}

test('OAuth callback is captured only in this page with the expected state, including a 302 redirect', async t => {
  const fixture = await browserFixture(t);
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  await fixture.page.route(fixture.origin + '/oauth/authorize**', route => route.fulfill({ status: 302,
    headers: { location: callbackOrigin + '/auth/callback?code=synthetic-code&state=synthetic-state' }, body: '' }));
  const result = await fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), loginOptions);
  assert.deepEqual(result, { code: 'synthetic-code', state: 'synthetic-state' });
  assert.equal(fixture.counts.disable, 0);
  assert.equal(fixture.counts.enroll, 0);
  assert.equal(fixture.counts.activate, 0);
});

test('parallel OAuth windows retain separate state and callbacks on the loopback listener', async t => {
  const first = await browserFixture(t);
  const second = await browserFixture(t);
  for (const [fixture, state, code] of [[first, 'first-state', 'first-code'], [second, 'second-state', 'second-code']]) {
    await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
    await fixture.page.route(fixture.origin + '/oauth/authorize**', route => route.fulfill({ status: 302,
      headers: { location: `${callbackOrigin}/auth/callback?code=${code}&state=${state}` }, body: '' }));
  }
  const results = await Promise.all([
    first.provider.authorizeSession(syntheticAuthURL(first.origin, 'first-state'), loginOptions),
    second.provider.authorizeSession(syntheticAuthURL(second.origin, 'second-state'), loginOptions),
  ]);
  assert.deepEqual(results, [{ code: 'first-code', state: 'first-state' }, { code: 'second-code', state: 'second-state' }]);
});

test('account chooser clicks the intended email even when the accessible label omits it', async t => {
  const fixture = await browserFixture(t, { accountChooser: true });
  await fixture.context.addCookies([{ name: 'fixture_session', value: 'active', url: fixture.origin }]);
  let waits = 0;
  const me = await fixture.provider.login({ ...loginOptions, onManual: async () => {
    if (++waits > 8) throw new Error('Synthetic chooser did not progress: ' + new URL(fixture.page.url()).pathname);
    await new Promise(resolve => setTimeout(resolve, 100));
  } });
  assert.equal(me.email, 'account@example.test');
  assert.equal(new URL(fixture.page.url()).pathname, '/');
});

test('OAuth callback rejects state substitution and an external redirect URI', async t => {
  const fixture = await browserFixture(t);
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  await fixture.page.route(fixture.origin + '/oauth/authorize**', route => route.fulfill({ status: 302,
    headers: { location: callbackOrigin + '/auth/callback?code=synthetic-code&state=wrong-state' }, body: '' }));
  await assert.rejects(() => fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), loginOptions), { code: 'SESSION_AUTHORIZATION_REJECTED' });
  const external = new URL(syntheticAuthURL(fixture.origin));
  external.searchParams.set('redirect_uri', 'https://external.example.test/auth/callback');
  await assert.rejects(() => fixture.provider.authorizeSession(external.href, loginOptions), { code: 'SESSION_AUTHORIZATION_INVALID' });
});

test('known Codex consent automatically continues only for the matching captured workspace', async t => {
  const fixture = await browserFixture(t, { oauthConsent: true });
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  const result = await fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), { ...loginOptions, expectedAccountId: 'fixture-workspace' });
  assert.equal(result.code, 'synthetic-code');
  assert.equal(fixture.counts.authorization, 1);
});

test('known Codex consent does not submit a different selected workspace', async t => {
  const fixture = await browserFixture(t, { oauthConsent: true, consentWorkspace: 'wrong-workspace' });
  await assert.rejects(() => fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), {
    ...loginOptions, expectedAccountId: 'fixture-workspace', onManual: async reason => {
      assert.equal(reason, 'LOGIN_OAUTH_CONSENT_REQUIRED'); throw new WorkerError('SYNTHETIC_MANUAL_REQUIRED');
    },
  }), { code: 'SYNTHETIC_MANUAL_REQUIRED' });
  assert.equal(fixture.counts.authorization, 0);
});

test('OAuth-only login never requests Session and automatically authorizes the matching personal account', async t => {
  const fixture = await browserFixture(t, { oauthConsent: true, onboardingDialog: true });
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  const identity = await fixture.provider.login({ ...loginOptions, authorizationOnly: true });
  assert.equal(identity.email, loginOptions.email);
  assert.equal(fixture.counts.session, 0);
  assert.equal(await fixture.page.getByRole('dialog').count(), 0);
  const result = await fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), loginOptions);
  assert.equal(result.code, 'synthetic-code');
  assert.equal(fixture.counts.authorization, 1);
  assert.equal(fixture.counts.session, 0);
  assert.equal(fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

test('OAuth captures callback code even when another local app owns the callback port', async t => {
  const blocker = createServer(socket => socket.destroy());
  await new Promise(resolve => blocker.listen(Number(new URL(callbackOrigin).port), '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => blocker.close(resolve)));
  const fixture = await browserFixture(t);
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  await fixture.page.route(fixture.origin + '/oauth/authorize**', route => route.fulfill({ status: 302,
    headers: { location: callbackOrigin + '/auth/callback?code=synthetic-code&state=synthetic-state' }, body: '' }));
  const result = await fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), loginOptions);
  assert.deepEqual(result, { code: 'synthetic-code', state: 'synthetic-state' });
});

test('parallel account contexts isolate cookies and closing one leaves the other usable', async t => {
  const first = await browserFixture(t);
  const context = await first.context.browser().newContext({ acceptDownloads: false, serviceWorkers: 'block' });
  await context.route('**/*', route => new URL(route.request().url()).origin === first.origin ? route.continue() : route.abort());
  const page = await context.newPage();
  const controller = new AbortController();
  const second = new ChatGPTProvider({ page, context, signal: controller.signal, testOrigin: first.origin, onUnexpectedClose: () => controller.abort() });
  t.after(() => second.close());
  await first.context.addCookies([{ name: 'fixture_session', value: 'active', url: first.origin }]);
  assert.deepEqual(await context.cookies(), []);
  await first.provider.close();
  assert.equal(page.isClosed(), false);
  assert.equal(controller.signal.aborted, false);
  const identity = await second.login(loginOptions);
  assert.equal(identity.email, loginOptions.email);
  await second.close();
  assert.equal(page.isClosed(), true);
});

test('signed-in browser fetches Session and only its session cookie without any MFA changes', async t => {
  const fixture = await browserFixture(t, { requireBearer: true });
  const identity = await fixture.provider.login(loginOptions);
  await fixture.context.addCookies([
    { name: 'next-auth.session-token.0', value: 'PUBLIC-SYNTHETIC-', url: fixture.origin, httpOnly: true },
    { name: 'next-auth.session-token.1', value: 'SESSION', url: fixture.origin, httpOnly: true },
    { name: 'unrelated_cookie', value: 'UNRELATED-CANARY', url: fixture.origin },
  ]);
  const result = await fixture.provider.session(identity);
  assert.equal(result.user.email, loginOptions.email);
  assert.equal(result.accessToken, 'PUBLIC-SYNTHETIC-BEARER-TOKEN');
  assert.equal(result.sessionToken, 'PUBLIC-SYNTHETIC-SESSION');
  assert.ok(!JSON.stringify(result).includes('UNRELATED-CANARY'));
  assert.equal(fixture.counts.disable, 0);
  assert.equal(fixture.counts.enroll, 0);
  assert.equal(fixture.counts.activate, 0);
});

test('real Chromium submits only the synthetic email/password/authenticator forms and MFA API shapes', async t => {
  const fixture = await browserFixture(t, { requireBearer: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.deepEqual((await fixture.provider.mfa()).ids, ['fixture-old']);
  await fixture.provider.disable('fixture-old');
  assert.deepEqual((await fixture.provider.mfa()).ids, []);
  const enrollment = await fixture.provider.enroll('account@example.test', 'fixture-old');
  assert.equal(enrollment.factorId, 'fixture-new');
  await fixture.provider.activate({ code: '234567', sessionId: enrollment.sessionId });
  assert.equal(isNewFactorConfirmed(await fixture.provider.mfa(), 'fixture-old', 'fixture-new'), true);
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp, fixture.counts.disable, fixture.counts.enroll, fixture.counts.activate], [1, 1, 1, 1, 1, 1]);
  assert.ok(fixture.counts.session > 0);
  await fixture.provider.close();
  assert.equal(fixture.context.pages().length, 0);
});

test('a pre-login shell identity without email continues to the real login form', async t => {
  const fixture = await browserFixture(t, { anonymousMe200: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
});

test('session email is accepted only when its user ID matches the backend identity', async t => {
  const fixture = await browserFixture(t, { meWithoutEmail: true, requireBearer: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.ok(fixture.counts.session > 0);
  assert.deepEqual((await fixture.provider.mfa()).ids, ['fixture-old']);
});

test('a signed session token resolves an incomplete browser identity before MFA access', async t => {
  const fixture = await browserFixture(t, { meEmailWithBearerOnly: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.ok(fixture.counts.session > 0);
  assert.deepEqual((await fixture.provider.mfa()).ids, ['fixture-old']);
});

test('the known first-login introduction closes before account verification', async t => {
  const fixture = await browserFixture(t, { onboardingDialog: true });
  await fixture.context.addCookies([{ name: 'fixture_session', value: 'active', url: fixture.origin }]);
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(await fixture.page.getByRole('dialog').count(), 0);
});

test('an introduction appearing after login can be dismissed before browser cleanup', async t => {
  const fixture = await browserFixture(t);
  await fixture.provider.login(loginOptions);
  await fixture.page.evaluate(() => {
    const dialog = document.createElement('div');
    dialog.setAttribute('role', 'dialog');
    dialog.innerHTML = '<h2>更相关、更个性化的回复</h2><button onclick="this.closest(\'[role=dialog]\').remove()">知道了</button>';
    document.body.append(dialog);
  });
  assert.equal(await fixture.provider.dismissKnownOnboarding(), true);
  assert.equal(await fixture.page.getByRole('dialog').count(), 0);
});

test('equivalent header and sidebar login buttons do not stall sign-in', async t => {
  const fixture = await browserFixture(t, { dualLoginButtons: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
});

test('the ChatGPT home email dialog advances automatically', async t => {
  const fixture = await browserFixture(t, { loginModal: true });
  const submitted = [];
  const me = await fixture.provider.login({ ...loginOptions, onLoginSubmit: kind => submitted.push(kind) });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
  assert.deepEqual(submitted, ['email', 'password', 'otp']);
});

test('a login redirect back to signed-out home retries the entry and email steps', async t => {
  const fixture = await browserFixture(t, { dualLoginButtons: true, bounceToHomeAfterEmailOnce: true });
  const me = await fixture.provider.login(loginOptions);
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [2, 1, 1]);
});

test('a blank official sign-in redirect reports loading and then continues', async t => {
  const fixture = await browserFixture(t, { intermediateRedirect: true });
  const reasons = [];
  const me = await fixture.provider.login({ ...loginOptions, onManual: async reason => {
    reasons.push(reason);
    await new Promise(resolve => setTimeout(resolve, 100));
    return false;
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.ok(reasons.includes('LOGIN_PROVIDER_REDIRECT'));
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
});

test('a stalled blank redirect hands off to OAuth in the same browser without claiming login success', async t => {
  const fixture = await browserFixture(t, { stalledRedirect: true, oauthConsent: true, testLoginRedirectWaitMs: 200 });
  const reasons = [];
  const options = { ...loginOptions, authorizationOnly: true, onManual: async reason => {
    reasons.push(reason);
    await new Promise(resolve => setTimeout(resolve, 50));
  } };
  const result = await fixture.provider.login(options);
  assert.deepEqual(result, { authorizationLoginRequired: true });
  assert.ok(reasons.includes('LOGIN_PROVIDER_REDIRECT'));
  assert.equal(fixture.page.isClosed(), false);
  assert.equal(fixture.counts.password + fixture.counts.otp + fixture.counts.session, 0);
  await fixture.context.route(callbackOrigin + '/auth/callback**', route => route.continue());
  const oauth = await fixture.provider.authorizeSession(syntheticAuthURL(fixture.origin), options);
  assert.deepEqual(oauth, { code: 'synthetic-code', state: 'synthetic-state' });
  assert.equal(fixture.counts.authorization, 1);
  assert.equal(fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

test('two visible account menus count as OAuth login readiness without reading Session', async t => {
  const fixture = await browserFixture(t);
  await fixture.context.addCookies([{ name: 'fixture_session', value: 'active', url: fixture.origin }]);
  await fixture.page.goto(fixture.origin + '/');
  await fixture.page.evaluate(() => {
    for (let index = 0; index < 2; index++) {
      const button = document.createElement('button');
      button.dataset.testid = 'accounts-profile-button';
      button.textContent = 'Fixture profile';
      document.body.append(button);
    }
  });
  const provider = new ChatGPTProvider({ page: fixture.page, context: fixture.context,
    signal: fixture.controller.signal, testOrigin: fixture.origin, externalNavigation: true,
  });
  assert.deepEqual(await provider.login({ ...loginOptions, authorizationOnly: true }), { email: 'account@example.test' });
  assert.equal(fixture.counts.session, 0);
  assert.equal(fixture.counts.authorization, 0);
});

test('a challenge on the blank redirect stays manual rather than triggering OAuth recovery', async t => {
  const fixture = await browserFixture(t, { stalledRedirect: true, redirectChallenge: true, testLoginRedirectWaitMs: 200 });
  await assert.rejects(fixture.provider.login({ ...loginOptions, authorizationOnly: true,
    onManual: async reason => {
      if (reason === 'LOGIN_PROVIDER_REDIRECT') {
        await new Promise(resolve => setTimeout(resolve, 50));
        return;
      }
      throw new WorkerError(reason);
    },
  }), error => error.code === 'LOGIN_CLOUDFLARE_CHALLENGE');
  assert.equal(fixture.page.isClosed(), false);
  assert.equal(fixture.counts.authorization + fixture.counts.password + fixture.counts.otp, 0);
});

test('a failed automatic login click keeps the browser open for manual completion', async t => {
  const fixture = await browserFixture(t, { disabledPasswordButton: true });
  let manual = 0;
  const me = await fixture.provider.login({ ...loginOptions, onManual: async () => {
    manual++;
    assert.equal(fixture.page.isClosed(), false);
    await fixture.page.getByRole('button', { name: 'Continue' }).evaluate(button => { button.disabled = false; });
    await fixture.page.getByRole('button', { name: 'Continue' }).click();
    await fixture.page.waitForURL('**/login/otp');
    await fixture.page.locator('input[autocomplete="one-time-code"]').fill('123456');
    await fixture.page.getByRole('button', { name: 'Continue' }).click();
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(manual, 1);
  assert.equal(fixture.counts.password, 1);
});

test('manual login opens the sign-in page and fills known credentials while the operator submits forms', async t => {
  const fixture = await browserFixture(t, { manualLogin: true, requireBearer: true });
  let manual = 0;
  const me = await fixture.provider.login({ ...loginOptions,
    onManual: async () => {
      manual++;
      if (fixture.page.url().endsWith('/login')) {
        assert.equal(await fixture.page.locator('input[type="email"]').inputValue(), 'account@example.test');
        await fixture.page.getByRole('button', { name: 'Continue' }).click();
      } else if (fixture.page.url().endsWith('/login/password')) {
        assert.equal(await fixture.page.locator('input[type="password"]').inputValue(), 'PUBLIC-SYNTHETIC-PASSWORD');
        await fixture.page.getByRole('button', { name: 'Continue' }).click();
      } else if (fixture.page.url().endsWith('/login/otp')) {
        assert.equal(await fixture.page.locator('input[autocomplete="one-time-code"]').inputValue(), '123456');
        await fixture.page.getByRole('button', { name: 'Continue' }).click();
      } else {
        throw new Error('unexpected manual login page');
      }
    },
  });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(manual, 3);
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
});

test('Linux-style manual login clicks either visible home login button and fills the dialog', async t => {
  const fixture = await browserFixture(t, { manualLogin: true, homeLoginDialog: true, dualLoginButtons: true });
  let manual = 0;
  const me = await fixture.provider.login({ ...loginOptions, onManual: async () => {
    manual++;
    if (fixture.page.url() === fixture.origin + '/') {
      assert.equal(await fixture.page.getByRole('dialog').getByPlaceholder('电子邮件地址').inputValue(), 'account@example.test');
      await fixture.page.getByRole('dialog').getByRole('button', { name: '继续' }).click();
    } else if (fixture.page.url().endsWith('/login/password')) {
      assert.equal(await fixture.page.locator('input[type="password"]').inputValue(), 'PUBLIC-SYNTHETIC-PASSWORD');
      await fixture.page.getByRole('button', { name: 'Continue' }).click();
    } else if (fixture.page.url().endsWith('/login/otp')) {
      assert.equal(await fixture.page.locator('input[autocomplete="one-time-code"]').inputValue(), '123456');
      await fixture.page.getByRole('button', { name: 'Continue' }).click();
    } else throw new Error('unexpected login page');
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(manual, 3);
  assert.deepEqual([fixture.counts.email, fixture.counts.password, fixture.counts.otp], [1, 1, 1]);
});

test('ChatGPT home login dialog gets its email filled before the operator clicks Continue', async t => {
  const fixture = await browserFixture(t, { manualLogin: true, loginModal: true });
  let manual = 0;
  const me = await fixture.provider.login({ ...loginOptions, onManual: async () => {
    manual++;
    if (fixture.page.url() === fixture.origin + '/') {
      assert.equal(await fixture.page.getByRole('dialog').getByPlaceholder('电子邮件地址').inputValue(), 'account@example.test');
      await fixture.page.getByRole('dialog').getByRole('button', { name: '继续' }).click();
    } else if (fixture.page.url().endsWith('/login/password')) {
      assert.equal(await fixture.page.locator('input[type="password"]').inputValue(), 'PUBLIC-SYNTHETIC-PASSWORD');
      await fixture.page.getByRole('button', { name: 'Continue' }).click();
    } else if (fixture.page.url().endsWith('/login/otp')) {
      assert.equal(await fixture.page.locator('input[autocomplete="one-time-code"]').inputValue(), '123456');
      await fixture.page.getByRole('button', { name: 'Continue' }).click();
    } else {
      throw new Error('unexpected login page');
    }
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(manual, 3);
});

test('external Edge page waits for its own navigation and closes only that page', async t => {
  const fixture = await browserFixture(t, { manualLogin: true, loginModal: true });
  const page = await fixture.context.newPage();
  const external = new ChatGPTProvider({
    page, context: fixture.context, signal: fixture.controller.signal,
    testOrigin: fixture.origin, manualLogin: true, externalNavigation: true, ownsContext: false,
  });
  let manual = 0;
  const me = await external.login({ ...loginOptions, onManual: async () => {
    manual++;
    if (page.url() === 'about:blank') await page.goto(fixture.origin + '/');
    else if (page.url() === fixture.origin + '/') await page.getByRole('dialog').getByRole('button', { name: '继续' }).click();
    else if (page.url().endsWith('/login/password')) await page.getByRole('button', { name: 'Continue' }).click();
    else if (page.url().endsWith('/login/otp')) await page.getByRole('button', { name: 'Continue' }).click();
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.ok(manual >= 3);
  await external.close();
  assert.equal(page.isClosed(), true);
  assert.equal(fixture.page.isClosed(), false);
  assert.equal(fixture.context.browser().isConnected(), true);
});

test('failed first navigation leaves the browser open for operator navigation', async t => {
  const fixture = await browserFixture(t);
  let aborted = false; let manual = 0;
  await fixture.context.route(fixture.origin + '/', route => {
    if (!aborted) { aborted = true; return route.abort('failed'); }
    return route.continue();
  });
  const me = await fixture.provider.login({ ...loginOptions, onManual: async reason => {
    manual++;
    assert.equal(reason, 'LOGIN_NAVIGATION_REQUIRED');
    assert.equal(fixture.page.isClosed(), false);
    await fixture.page.waitForURL(url => url.protocol === 'chrome-error:', { timeout: 5000 });
    await fixture.page.goto(fixture.origin + '/login');
  } });
  assert.deepEqual(me, { id: 'fixture-subject', email: 'account@example.test' });
  assert.equal(manual, 1);
});

test('official browser challenge is reported without attempting password or MFA writes', async t => {
  const fixture = await browserFixture(t, { cloudflareChallenge: true });
  let reason = '';
  await assert.rejects(fixture.provider.login({ ...loginOptions, onManual: async value => {
    reason = value;
    throw new WorkerError(value);
  } }), error => error.code === 'LOGIN_CLOUDFLARE_CHALLENGE');
  assert.equal(reason, 'LOGIN_CLOUDFLARE_CHALLENGE');
  assert.equal(fixture.counts.password + fixture.counts.otp + fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

test('email verification is handed to the operator without reading an authenticator code', async t => {
  const fixture = await browserFixture(t, { emailChallenge: true });
  let codeReads = 0; let manual = 0;
  await fixture.provider.login({
    ...loginOptions,
    getOldCode: async () => { codeReads++; return { code: '123456' }; },
    onManual: async () => {
      manual++;
      await fixture.page.locator('input[autocomplete="one-time-code"]').fill('PUBLIC-EMAIL-FIXTURE-CODE');
      await fixture.page.getByRole('button', { name: 'Continue', exact: true }).click();
    },
  });
  assert.equal(manual, 1);
  assert.equal(codeReads, 0);
  assert.equal(fixture.counts.otp, 1);
});

test('an authenticated wrong email is rejected before MFA mutation', async t => {
  const fixture = await browserFixture(t, { wrongIdentity: true });
  await fixture.context.addCookies([{ name: 'fixture_session', value: 'active', url: fixture.origin }]);
  await assert.rejects(fixture.provider.login(loginOptions), error => error.code === 'ACCOUNT_IDENTITY_MISMATCH');
  assert.equal(fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

test('lease cancellation closes the owned context and interrupts manual UI waiting', async t => {
  const fixture = await browserFixture(t, { emailChallenge: true });
  const running = fixture.provider.login({
    ...loginOptions,
    onManual: () => new Promise((resolve, reject) => {
      fixture.controller.signal.addEventListener('abort', () => reject(new WorkerError('WORKER_STOPPED')), { once: true });
      fixture.controller.abort();
    }),
  });
  await assert.rejects(running, error => error.code === 'WORKER_STOPPED');
  await fixture.provider.close();
  assert.equal(fixture.context.pages().length, 0);
  assert.equal(fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

test('a provider redirect is refused and never resends the enrollment request', async t => {
  const fixture = await browserFixture(t, { redirectEnroll: true });
  await fixture.context.addCookies([{ name: 'fixture_session', value: 'active', url: fixture.origin }]);
  await fixture.provider.login(loginOptions);
  await fixture.provider.disable('fixture-old');
  await assert.rejects(fixture.provider.enroll('account@example.test', 'fixture-old'), error => error.code === 'UPSTREAM_RESPONSE_UNKNOWN' && error.unknown);
  assert.equal(fixture.counts.enroll, 1);
  assert.equal(fixture.counts.redirected, 0);
});

test('closing the visible task page aborts its manual wait and releases the owned context', async t => {
  const fixture = await browserFixture(t, { emailChallenge: true });
  const running = fixture.provider.login({
    ...loginOptions,
    onManual: () => new Promise((resolve, reject) => {
      fixture.controller.signal.addEventListener('abort', () => reject(new WorkerError('BROWSER_CLOSED')), { once: true });
      void fixture.page.close();
    }),
  });
  await assert.rejects(running, error => error.code === 'BROWSER_CLOSED');
  assert.equal(fixture.controller.signal.aborted, true);
  await fixture.provider.close();
  assert.equal(fixture.context.pages().length, 0);
  assert.equal(fixture.counts.disable + fixture.counts.enroll + fixture.counts.activate, 0);
});

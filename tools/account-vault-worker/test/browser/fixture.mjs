import { createServer } from 'node:http';
import { once } from 'node:events';

// Entirely synthetic, public fixtures. No HAR, production account, credential,
// screenshot, profile export or real provider network request is involved.
export async function fixtureServer({ oauthConsent = false, consentWorkspace = 'fixture-workspace', accountChooser = false, emailChallenge = false, requireBearer = false, wrongIdentity = false, redirectEnroll = false, anonymousMe200 = false, cloudflareChallenge = false, meWithoutEmail = false, meEmailWithBearerOnly = false, disabledPasswordButton = false, loginModal = false, homeLoginDialog = false, onboardingDialog = false, dualLoginButtons = false, bounceToHomeAfterEmailOnce = false, intermediateRedirect = false, stalledRedirect = false, redirectChallenge = false } = {}) {
  const counts = { email: 0, password: 0, otp: 0, disable: 0, enroll: 0, activate: 0, redirected: 0, session: 0, authorization: 0 };
  let state = 'old';
  let authorization;
  const json = (response, value, status = 200) => { response.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' }); response.end(JSON.stringify(value)); };
  const html = (response, content) => { response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' }); response.end(`<!doctype html><html><body>${content}</body></html>`); };
  const redirect = (response, location, authenticated = false) => { response.writeHead(303, { Location: location, ...(authenticated ? { 'Set-Cookie': 'fixture_session=active; HttpOnly; SameSite=Lax; Path=/' } : {}) }); response.end(); };
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, 'http://fixture.invalid');
    let body = '';
    for await (const chunk of request) body += chunk;
    const signedIn = request.headers.cookie?.includes('fixture_session=active');
    if (request.method === 'GET' && url.pathname === '/oauth/authorize' && oauthConsent) {
      authorization = { state: url.searchParams.get('state'), redirect: url.searchParams.get('redirect_uri') };
      return redirect(response, '/sign-in-with-chatgpt/codex/consent');
    }
    if (request.method === 'GET' && url.pathname === '/sign-in-with-chatgpt/codex/consent' && oauthConsent) return html(response,
      `<span>account@example.test</span><form action="/oauth/complete"><input type="radio" name="workspace_id" value="${consentWorkspace}" checked><input type="hidden" name="workspace_id" value="${consentWorkspace}"><button>继续</button></form>`);
    if (request.method === 'GET' && url.pathname === '/oauth/complete' && oauthConsent && authorization) {
      counts.authorization++;
      const callback = new URL(authorization.redirect);
      callback.search = new URLSearchParams({ code: 'synthetic-code', state: authorization.state }).toString();
      return redirect(response, callback.href);
    }
    if (request.method === 'GET' && url.pathname === '/' && accountChooser && !request.headers.cookie?.includes('fixture_selected=yes')) return redirect(response, '/choose-an-account');
    if (request.method === 'GET' && url.pathname === '/choose-an-account' && accountChooser) return html(response, '<button aria-label="选择账户" onclick="document.cookie=\'fixture_selected=yes; Path=/\'; location.href=\'/\'">Chloe Jones<span>account@example.test</span></button><button aria-label="选择账户">other@example.test</button>');
    if (request.method === 'GET' && url.pathname === '/') return html(response, signedIn ? `<h1>Fixture account</h1><button aria-label="打开个人资料菜单">Profile</button>${onboardingDialog ? '<div role="dialog"><h2>更相关、更个性化的回复</h2><button onclick="this.closest(\'[role=dialog]\').remove()">知道了</button></div>' : ''}` : loginModal ? '<div role="dialog"><h2>登录或注册</h2><form method="post" action="/login/email"><input type="text" placeholder="电子邮件地址" name="email"><button>继续</button></form></div>' : homeLoginDialog ? `<button onclick="document.querySelector('[role=dialog]').hidden=false">登录</button>${dualLoginButtons ? '<button onclick="document.querySelector(\'[role=dialog]\').hidden=false">登录</button>' : ''}<div role="dialog" hidden><h2>登录或注册</h2><form method="post" action="/login/email"><input type="text" placeholder="电子邮件地址" name="email"><button>继续</button></form></div>` : `<button onclick="location.href='/login'">Log in</button>${dualLoginButtons ? '<button onclick="location.href=\'/login\'">Log in</button>' : ''}`);
    if (request.method === 'GET' && url.pathname === '/login') return html(response, '<form method="post" action="/login/email"><label>Email<input type="email" name="email"></label><button>Continue</button></form>');
    if (request.method === 'POST' && url.pathname === '/login/email') {
      counts.email++;
      return redirect(response, bounceToHomeAfterEmailOnce && counts.email === 1 ? '/' : intermediateRedirect || stalledRedirect ? '/auth/login_with' : '/login/password');
    }
    if (request.method === 'GET' && url.pathname === '/auth/login_with') return html(response, redirectChallenge ? '<title>Just a moment...</title>' : stalledRedirect ? '' : '<script>setTimeout(() => { location.href = "/login/password" }, 2500)</script>');
    if (request.method === 'GET' && url.pathname === '/login/password') return html(response, cloudflareChallenge ? '<title>Just a moment...</title><h1>Checking your browser</h1>' : `<form method="post" action="/login/password"><label>Password<input type="password" name="password"></label><button ${disabledPasswordButton ? 'disabled' : ''}>Continue</button></form>`);
    if (request.method === 'POST' && url.pathname === '/login/password') { counts.password++; return redirect(response, '/login/otp'); }
    if (request.method === 'GET' && url.pathname === '/login/otp') return html(response, `<h1>${emailChallenge ? 'Check your email' : 'Authenticator app'}</h1><form method="post" action="/login/otp"><label>Code<input autocomplete="one-time-code" name="code"></label><button>Continue</button></form>`);
    if (request.method === 'POST' && url.pathname === '/login/otp') { counts.otp++; return redirect(response, '/', true); }
    if (url.pathname === '/api/auth/session') { counts.session++; return json(response, signedIn ? { ...(requireBearer || meEmailWithBearerOnly ? { accessToken: 'PUBLIC-SYNTHETIC-BEARER-TOKEN' } : {}), user: { id: 'fixture-subject', email: 'account@example.test' } } : {}); }
    if (url.pathname === '/backend-api/me') {
      if (!signedIn && anonymousMe200) return json(response, { id: 'anonymous-shell', email: null });
      if (!signedIn || (requireBearer && request.headers.authorization !== 'Bearer PUBLIC-SYNTHETIC-BEARER-TOKEN')) return json(response, {}, 401);
      return json(response, { id: 'fixture-subject', email: meWithoutEmail || (meEmailWithBearerOnly && !request.headers.authorization) ? null : wrongIdentity ? 'other@example.test' : 'account@example.test' });
    }
    if (url.pathname.startsWith('/backend-api/') && (!signedIn || (requireBearer && request.headers.authorization !== 'Bearer PUBLIC-SYNTHETIC-BEARER-TOKEN'))) return json(response, {}, 401);
    if (url.pathname === '/backend-api/accounts/mfa_info') {
      const id = state === 'old' ? 'fixture-old' : state === 'new' ? 'fixture-new' : '';
      return json(response, { mfa_enabled: !!id, mfa_enabled_v2: !!id, native_default_factor_id: id || null, factors: { totp: id ? [{ id, factor_type: 'totp', is_recovery: false }] : [] } });
    }
    if (url.pathname === '/backend-api/accounts/mfa/user/disable_in_house' && request.method === 'POST') {
      counts.disable++;
      if (JSON.parse(body).factor_id !== 'fixture-old') return json(response, {}, 400);
      state = 'off'; return json(response, {});
    }
    if (url.pathname === '/backend-api/accounts/mfa/enroll' && request.method === 'POST') {
      counts.enroll++;
      if (redirectEnroll) { response.writeHead(302, { Location: '/redirected' }); response.end(); return; }
      const data = JSON.parse(body);
      if (state !== 'off' || data.factor_type !== 'totp' || data.source !== 'settings') return json(response, {}, 400);
      return json(response, { secret: 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ', email: null, session_id: 'fixture-enrollment-session', factor: { id: 'fixture-new', factor_type: 'totp' } });
    }
    if (url.pathname === '/backend-api/accounts/mfa/user/activate_enrollment' && request.method === 'POST') {
      counts.activate++;
      const data = JSON.parse(body);
      if (data.code !== '234567' || data.session_id !== 'fixture-enrollment-session' || data.factor_type !== 'totp' || data.source !== 'settings') return json(response, {}, 400);
      state = 'new'; return json(response, { success: true });
    }
    if (url.pathname === '/redirected') counts.redirected++;
    return json(response, {}, 404);
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  return { origin: `http://127.0.0.1:${server.address().port}`, counts, close: async () => { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); } };
}

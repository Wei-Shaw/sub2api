// ==UserScript==
// @name         Sub2API panel connector (__STATION_MATCH__)
// @namespace    sub2api-fork-local
// @version      1.0.0
// @description  Import this station's signed-in panel token after a local admin starts a single-use connection.
// @match        __STATION_MATCH__
// @connect      __LOCAL_HOST__
// @grant        GM_xmlhttpRequest
// @run-at       document-idle
// @noframes
// ==/UserScript==

(async () => {
  'use strict';
  const config = __CONNECTOR_CONFIG__;
  const marker = /^#sub2api-connect=([a-f0-9]{48})$/.exec(location.hash);
  if (window.top !== window.self || location.origin !== config.site || !marker) return;
  let ticket = marker[1];
  history.replaceState(history.state, '', location.pathname + location.search);
  function notify(text) {
    const box = document.getElementById('sub2api-connector-status') || document.createElement('div');
    box.id = 'sub2api-connector-status'; box.textContent = text;
    box.style.cssText = 'position:fixed;top:16px;right:16px;max-width:360px;padding:16px;background:#fff;color:#111;border:1px solid #ccc;border-radius:8px;z-index:2147483647;box-shadow:0 4px 20px #0003;font:14px sans-serif';
    document.body.appendChild(box);
  }
  function request(action, data) {
    return new Promise((resolve, reject) => {
      GM_xmlhttpRequest({
        method: 'POST', url: config.receiver + '/api/v1/upstream-pricing/browser-import/connector-' + action,
        headers: { 'Content-Type': 'application/json', 'X-Sub2API-Connector-Origin': config.receiver },
        data: JSON.stringify({ ticket, ...data }), anonymous: true, redirect: 'error', timeout: 60000,
        onload(response) {
          try {
            // Never forward credentials through a redirect or show raw diagnostics.
            if (new URL(response.finalUrl).origin !== config.receiver) throw new Error();
            const value = JSON.parse(response.responseText);
            if (response.status !== 200 || value.code !== 0) throw new Error();
            resolve(value.data);
          } catch { reject(new Error('本机连接验证失败，请重新发起。')); }
        },
        onerror: () => reject(new Error('无法连接本机 Sub2API，请检查脚本的本机访问权限。')),
        ontimeout: () => reject(new Error('连接超时，请重新发起。')),
      });
    });
  }
  const json = key => { try { return JSON.parse(localStorage.getItem(key) || 'null'); } catch { return null; } };
  let token = '';
  try {
    const info = await request('info', {});
    if (new URL(info.base_url).origin !== location.origin) throw new Error('站点与本机配置不一致。');
    // Read only documented/known panel storage keys on this exact station.
    const user = info.provider === 'sub2api' ? json('auth_user') : json('user');
    token = info.provider === 'sub2api' ? localStorage.getItem('auth_token') || '' : user?.access_token || '';
    if (typeof token !== 'string' || !token || token.length > 16384 || /[\r\n]/.test(token)) throw new Error('未找到可读取的面板登录令牌，请先登录；此站点若仅在内存保存令牌，请使用手动导入。');
    const userID = info.user_id || String(user?.id || '');
    notify('正在向本机验证面板读取权限…');
    await request('complete', { token, user_id: userID });
    notify('已自动导入。请回到 Sub2API 选择分组、预览并应用，此页面可以关闭。');
  } catch (error) {
    try { await request('fail', {}); } catch { /* A consumed or expired ticket is already closed. */ }
    notify(error.message || '导入失败，请重新发起。');
  }
  finally { token = ''; ticket = ''; }
})();

// This module has no browser side effects. Never log a session document.
export const SESSION_NAMES = ['hatch_sess', 'hatch_gw', 'hatch_vml', 'hatch_native_auth_device']

export function safeOrigin(value) {
  let url
  try { url = new URL(value) } catch { throw new Error('unsupported-origin') }
  if (url.username || url.password || (url.protocol !== 'https:' &&
      !(url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)))) throw new Error('unsupported-origin')
  return url.origin
}

export function validateRequest(request, tabURL, now = Date.now()) {
  const origin = safeOrigin(tabURL)
  if (!request || request.version !== '1' || request.origin !== origin ||
      !/^[a-f0-9]{64}$/.test(request.nonce || '') || !Number.isSafeInteger(request.ownerId) || request.ownerId <= 0 ||
      !Number.isSafeInteger(request.accountId) || request.accountId < 0 ||
      !Number.isSafeInteger(request.expiresAt) || request.expiresAt <= now || request.expiresAt > now + 120_000 ||
      typeof request.ownerLabel !== 'string' || !request.ownerLabel.trim() || request.ownerLabel.length > 320 || /[\x00-\x1f\x7f]/.test(request.ownerLabel)) throw new Error('invalid-request')
  return { ...request, origin }
}

export function sameRequest(a, b) {
  return ['version', 'origin', 'nonce', 'ownerId', 'accountId', 'ownerLabel', 'expiresAt'].every(key => a[key] === b[key])
}

export function sessionFromCookies(all, now = Date.now()) {
  const cookies = {}, expires = {}
  for (const cookie of all) {
    if (cookie.domain.replace(/^\./, '').toLowerCase() !== 'muse.ai' || cookie.path !== '/' || !SESSION_NAMES.includes(cookie.name)) continue
    if (cookies[cookie.name] && cookies[cookie.name] !== cookie.value) throw new Error('ambiguous-session')
    if (!cookie.value || typeof cookie.value !== 'string' || cookie.value.length > 16 * 1024 || /[\x00-\x20;\x7f]/.test(cookie.value)) throw new Error('invalid-session')
    if (cookie.expirationDate !== undefined && (!Number.isFinite(cookie.expirationDate) || cookie.expirationDate * 1000 <= now)) throw new Error('expired-session')
    cookies[cookie.name] = cookie.value
    if (cookie.expirationDate !== undefined) expires[cookie.name] = Math.floor(cookie.expirationDate)
  }
  if (SESSION_NAMES.some(name => name !== 'hatch_vml' && !cookies[name])) throw new Error('sign-in-required')
  return { cookies, expires }
}

// These functions are serialized into the user-approved tab's isolated world.
export function readReceiver() {
  if (window.top !== window) return null
  const markers = document.querySelectorAll('[data-sub2api-muse-connect="1"]')
  if (markers.length !== 1) return null
  const e = markers[0]
  return { version: '1', nonce: e.dataset.nonce, origin: e.dataset.origin, ownerId: Number(e.dataset.ownerId),
    ownerLabel: e.dataset.ownerLabel, accountId: Number(e.dataset.accountId), expiresAt: Number(e.dataset.expiresAt) }
}

export function deliverSession(request, session) {
  if (window.top !== window || window.location.origin !== request.origin || Date.now() >= request.expiresAt) return false
  const markers = document.querySelectorAll('[data-sub2api-muse-connect="1"]')
  if (markers.length !== 1) return false
  const e = markers[0]
  if (e.dataset.nonce !== request.nonce || e.dataset.origin !== request.origin || Number(e.dataset.ownerId) !== request.ownerId ||
      Number(e.dataset.accountId) !== request.accountId || Number(e.dataset.expiresAt) !== request.expiresAt || e.dataset.ownerLabel !== request.ownerLabel) return false
  window.postMessage({ type: 'sub2api:muse:session:v1', nonce: request.nonce, ownerId: request.ownerId,
    accountId: request.accountId, session }, request.origin)
  return true
}

export async function readPending(api) {
  const [tab] = await api.tabs.query({ active: true, currentWindow: true })
  if (!tab || !Number.isInteger(tab.id) || !tab.url) throw new Error('open-sub2api')
  safeOrigin(tab.url)
  const results = await api.scripting.executeScript({ target: { tabId: tab.id, frameIds: [0] }, func: readReceiver })
  const request = validateRequest(results[0]?.result, tab.url)
  return { tabId: tab.id, request }
}

export async function connectSession(api, approved) {
  // Check the exact approved destination again before reading any Muse cookie.
  const tab = await api.tabs.get(approved.tabId)
  const results = await api.scripting.executeScript({ target: { tabId: approved.tabId, frameIds: [0] }, func: readReceiver })
  const current = validateRequest(results[0]?.result, tab.url)
  if (!sameRequest(current, approved.request)) throw new Error('request-changed')
  const session = sessionFromCookies(await api.cookies.getAll({ url: 'https://muse.ai/' }))
  const delivered = await api.scripting.executeScript({ target: { tabId: approved.tabId, frameIds: [0] }, func: deliverSession, args: [current, session] })
  if (delivered[0]?.result !== true) throw new Error('request-changed')
}

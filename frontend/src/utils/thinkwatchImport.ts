import type { GroupPlatform } from '@/types'

/**
 * Import links for ThinkWatch Lite, a desktop AI gateway
 * (https://github.com/ThinkWatchProject/ThinkWatch-Lite).
 *
 * `thinkwatch://import?service=sub2api&url=<site>&key=<key>` opens the app's
 * "New upstream" dialog prefilled; nothing is saved until the user confirms
 * there. With `service=sub2api` the app reads the key's quota, rate-limit
 * windows and usage from this site's `GET /v1/usage`.
 */
export interface ThinkWatchImportLinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  apiKey: string
}

/**
 * The app appends the API paths itself (`/v1/messages`, `/v1/responses`,
 * `/v1/usage`, ...), so the link carries the site root: no trailing slash and
 * no trailing `/v1`.
 */
export function normalizeThinkWatchBaseUrl(baseUrl: string): string {
  return baseUrl.trim().replace(/\/+$/, '').replace(/\/v1$/, '').replace(/\/+$/, '')
}

export function buildThinkWatchImportLink(input: ThinkWatchImportLinkInput): string {
  const baseUrl = normalizeThinkWatchBaseUrl(input.baseUrl)
  const params: [string, string][] = [['service', 'sub2api']]

  if (input.platform === 'antigravity') {
    // Antigravity groups are served under /antigravity (Messages API and /v1/usage).
    params.push(['url', `${baseUrl}/antigravity`], ['protocol', 'anthropic'])
  } else {
    // Other groups: the app forwards each client's own API format unchanged.
    params.push(['url', baseUrl])
  }
  params.push(['key', input.apiKey])

  const query = params.map(([name, value]) => `${name}=${encodeURIComponent(value)}`).join('&')
  return `thinkwatch://import?${query}`
}

/**
 * Opens the link in the current tab, like the CC Switch import. A hidden
 * iframe is not an option: the default CSP (`frame-src 'self' ...`) blocks
 * custom schemes in frames.
 *
 * `onNotOpened` runs when the page never lost focus within `delayMs`: no
 * browser prompt appeared and no app came to the front, so the app is most
 * likely not installed.
 */
export function openThinkWatchImportLink(
  link: string,
  onNotOpened: () => void,
  delayMs = 1500
): void {
  let lostFocus = false
  const markLostFocus = () => {
    lostFocus = true
  }
  window.addEventListener('blur', markLostFocus)

  try {
    window.open(link, '_self')
  } catch {
    window.removeEventListener('blur', markLostFocus)
    onNotOpened()
    return
  }

  window.setTimeout(() => {
    window.removeEventListener('blur', markLostFocus)
    if (!lostFocus && document.hasFocus()) {
      onNotOpened()
    }
  }, delayMs)
}

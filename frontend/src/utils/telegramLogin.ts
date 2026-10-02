import { buildApiUrl } from '@/api/url'

const TELEGRAM_OAUTH_ORIGIN = 'https://oauth.telegram.org'
// Posted by TelegramCallbackView when Telegram redirects the popup to return_to instead of messaging us.
export const TELEGRAM_POPUP_RESULT_EVENT = 'sub2api_telegram_auth_result'

type TelegramAuthResult = Record<string, unknown>

/**
 * Opens the Telegram login popup. Must run synchronously inside the click handler,
 * before any await, or popup blockers will stop it. Returns null when blocked.
 */
export function openTelegramPopup(): Window | null {
  const width = 550
  const height = 470
  const left = Math.max(0, Math.round((window.screen.width - width) / 2))
  const top = Math.max(0, Math.round((window.screen.height - height) / 2))
  return window.open(
    'about:blank',
    'telegram_oauth',
    `width=${width},height=${height},left=${left},top=${top},status=0,location=0,menubar=0,toolbar=0`
  )
}

/** Sends a #tgAuthResult value (base64 JSON) to the backend, which redirects to /auth/telegram/callback. */
export function postTelegramAuthResult(encoded: string): void {
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = buildApiUrl('/auth/oauth/telegram/callback')
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = 'tg_auth_result'
  input.value = encoded
  form.appendChild(input)
  document.body.appendChild(form)
  form.submit()
}

// Same fallback telegram-widget.js uses when the popup closes before its postMessage arrives.
async function fetchTelegramAuthUser(botId: string): Promise<TelegramAuthResult | null> {
  try {
    const res = await fetch(`${TELEGRAM_OAUTH_ORIGIN}/auth/get`, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-Requested-With': 'XMLHttpRequest'
      },
      body: `bot_id=${encodeURIComponent(botId)}`
    })
    const data = await res.json()
    return data?.user && typeof data.user === 'object' ? data.user : null
  } catch {
    return null
  }
}

function encodeTelegramAuthResult(result: TelegramAuthResult): string {
  const bytes = new TextEncoder().encode(JSON.stringify(result))
  let binary = ''
  bytes.forEach((b) => {
    binary += String.fromCharCode(b)
  })
  return window.btoa(binary)
}

// Resolves with the base64 auth result (the #tgAuthResult format), or null if cancelled.
function waitForTelegramResult(popup: Window, botId: string): Promise<string | null> {
  return new Promise((resolve) => {
    let done = false
    const finish = (result: TelegramAuthResult | string | null) => {
      if (done) return
      done = true
      window.removeEventListener('message', onMessage)
      window.clearInterval(timer)
      resolve(result && typeof result === 'object' ? encodeTelegramAuthResult(result) : result || null)
    }
    function onMessage(event: MessageEvent) {
      if (event.origin === window.location.origin && event.data?.event === TELEGRAM_POPUP_RESULT_EVENT) {
        finish(typeof event.data.result === 'string' ? event.data.result : null)
        return
      }
      if (event.origin !== TELEGRAM_OAUTH_ORIGIN) return
      let data: { event?: string; result?: unknown } | null = null
      try {
        data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data
      } catch {
        return
      }
      if (data?.event !== 'auth_result') return
      finish(data.result && typeof data.result === 'object' ? (data.result as TelegramAuthResult) : null)
      try {
        popup.close()
      } catch {
        // ignore
      }
    }
    window.addEventListener('message', onMessage)
    const timer = window.setInterval(() => {
      if (!popup.closed) return
      window.clearInterval(timer)
      // Give a postMessage sent right before close() time to land first.
      window.setTimeout(() => {
        void (botId ? fetchTelegramAuthUser(botId) : Promise.resolve(null)).then(finish)
      }, 500)
    }, 300)
  })
}

/**
 * Points the popup at a start URL (our backend start endpoint or Telegram's authorize URL),
 * waits for Telegram's postMessage result and hands it to the backend. Cancelling just closes the popup.
 */
export async function runTelegramPopup(popup: Window, startUrl: string): Promise<void> {
  popup.location.href = startUrl
  const botId = new URL(startUrl, window.location.href).searchParams.get('bot_id') || ''
  const encoded = await waitForTelegramResult(popup, botId)
  if (encoded) postTelegramAuthResult(encoded)
}

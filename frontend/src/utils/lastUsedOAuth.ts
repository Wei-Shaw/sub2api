/**
 * "Last time used" OAuth hint.
 *
 * Purely browser-local (per device / per browser): the login page is
 * unauthenticated, so the server cannot tell us which provider this visitor
 * used last. We record the provider the user actually completed a login with
 * and surface it on the login page as a subtle badge, similar to Google's
 * account chooser "Last used" marker.
 *
 * Flow:
 *  - On click, before redirecting to the provider, the button calls
 *    {@link setPendingOAuthProvider}.
 *  - On a successful OAuth callback (auth store `setToken`), we call
 *    {@link promotePendingOAuthProvider} to persist the pending provider as the
 *    last-used one.
 *  - On a password / 2FA / register success (auth store `setAuthFromResponse`),
 *    we call {@link clearLastUsedOAuthProvider} so the badge never shows stale
 *    info: the last login was not an OAuth provider.
 *
 * The value expires after 90 days so a shared/public browser does not leak a
 * previous visitor's provider forever.
 */

export type OAuthProviderId = 'github' | 'google' | 'linuxdo' | 'dingtalk' | 'wechat' | 'oidc'

const VALID_PROVIDERS: readonly OAuthProviderId[] = [
  'github',
  'google',
  'linuxdo',
  'dingtalk',
  'wechat',
  'oidc'
]

const PENDING_KEY = 'sub2api_pending_oauth_provider'
const LAST_USED_KEY = 'sub2api_last_used_oauth'
const MAX_AGE_MS = 90 * 24 * 60 * 60 * 1000 // 90 days

interface LastUsedRecord {
  provider: OAuthProviderId
  ts: number
}

function isValidProvider(value: unknown): value is OAuthProviderId {
  return typeof value === 'string' && (VALID_PROVIDERS as readonly string[]).includes(value)
}

/** Record which provider the user is about to authenticate with (call on click). */
export function setPendingOAuthProvider(provider: OAuthProviderId): void {
  try {
    window.sessionStorage.setItem(PENDING_KEY, provider)
  } catch {
    // storage unavailable (private mode / disabled) — degrade silently
  }
}

/**
 * Promote a pending provider to the persisted "last used" slot.
 * Call on a successful OAuth login. No-op when there is no pending provider
 * (e.g. an email-verification link), so it never clobbers an existing value.
 */
export function promotePendingOAuthProvider(): void {
  try {
    const pending = window.sessionStorage.getItem(PENDING_KEY)
    if (!isValidProvider(pending)) return
    window.sessionStorage.removeItem(PENDING_KEY)
    const record: LastUsedRecord = { provider: pending, ts: Date.now() }
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify(record))
  } catch {
    // storage unavailable — degrade silently
  }
}

/**
 * Forget the last-used provider. Call on a password / 2FA / register success so
 * the badge reflects the true last login method.
 */
export function clearLastUsedOAuthProvider(): void {
  try {
    window.localStorage.removeItem(LAST_USED_KEY)
    window.sessionStorage.removeItem(PENDING_KEY)
  } catch {
    // storage unavailable — degrade silently
  }
}

/**
 * Read the last-used provider for display, or null when there is none or it has
 * expired. Expired / malformed records are pruned as a side effect.
 */
export function getLastUsedOAuthProvider(): OAuthProviderId | null {
  let raw: string | null = null
  try {
    raw = window.localStorage.getItem(LAST_USED_KEY)
  } catch {
    return null
  }
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw) as Partial<LastUsedRecord>
    if (
      isValidProvider(parsed.provider) &&
      typeof parsed.ts === 'number' &&
      Date.now() - parsed.ts <= MAX_AGE_MS
    ) {
      return parsed.provider
    }
  } catch {
    // malformed JSON — fall through and prune below
  }

  // expired, malformed, or invalid: prune and report nothing
  try {
    window.localStorage.removeItem(LAST_USED_KEY)
  } catch {
    // ignore
  }
  return null
}

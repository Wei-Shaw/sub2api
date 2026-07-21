import { onScopeDispose, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { KimiTokenInfo } from '@/api/admin/kimi'

export const KIMI_DEFAULT_BASE_URL = 'https://api.kimi.com/coding/v1'
export const KIMI_DEFAULT_POLL_INTERVAL_SECONDS = 5

// Business reasons returned by the backend poll endpoint
// (see backend/internal/service/kimi_oauth_service.go).
export const KIMI_OAUTH_REASON_PENDING = 'KIMI_OAUTH_AUTHORIZATION_PENDING'
export const KIMI_OAUTH_REASON_SLOW_DOWN = 'KIMI_OAUTH_SLOW_DOWN'
export const KIMI_OAUTH_REASON_EXPIRED = 'KIMI_OAUTH_DEVICE_CODE_EXPIRED'
export const KIMI_OAUTH_REASON_ACCESS_DENIED = 'KIMI_OAUTH_ACCESS_DENIED'

export type KimiDeviceFlowStatus =
  | 'idle'
  | 'starting'
  | 'awaiting_user'
  | 'success'
  | 'expired'
  | 'denied'
  | 'error'

function parsePositiveInt(value: unknown): number | null {
  const n = Number(value)
  if (!Number.isFinite(n) || n <= 0) return null
  return Math.floor(n)
}

/**
 * Build account credentials from a Kimi token payload.
 * Mirrors the backend BuildAccountCredentials shape so the refresher can
 * rotate tokens uniformly regardless of which create path was used.
 */
export function buildKimiCredentials(tokenInfo: KimiTokenInfo): Record<string, unknown> {
  let expiresAt: string | number | undefined = tokenInfo.expires_at
  if (typeof expiresAt === 'number' && Number.isFinite(expiresAt) && expiresAt > 0) {
    expiresAt = new Date(expiresAt * 1000).toISOString()
  }

  const credentials: Record<string, unknown> = {
    access_token: tokenInfo.access_token,
    refresh_token: tokenInfo.refresh_token,
    token_type: tokenInfo.token_type,
    expires_at: expiresAt,
    client_id: tokenInfo.client_id,
    scope: tokenInfo.scope,
    device_id: tokenInfo.device_id,
    base_url: KIMI_DEFAULT_BASE_URL
  }
  return Object.fromEntries(
    Object.entries(credentials).filter(([, value]) => value !== undefined && value !== '')
  )
}

/**
 * Build account extra info from a Kimi token payload.
 * Kimi device flow does not expose profile data, so this is currently empty;
 * kept for parity with the other OAuth composables.
 */
export function buildKimiExtraInfo(_tokenInfo: KimiTokenInfo): Record<string, unknown> {
  return {}
}

interface UseKimiOAuthOptions {
  onAuthorized?: (tokenInfo: KimiTokenInfo) => void
}

export function useKimiOAuth(options: UseKimiOAuthOptions = {}) {
  const appStore = useAppStore()
  const { t } = useI18n()

  const sessionId = ref('')
  const userCode = ref('')
  const verificationUri = ref('')
  const verificationUriComplete = ref('')
  const pollInterval = ref(KIMI_DEFAULT_POLL_INTERVAL_SECONDS)
  const expiresIn = ref(0)
  const status = ref<KimiDeviceFlowStatus>('idle')
  const loading = ref(false)
  const polling = ref(false)
  const error = ref('')

  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let expiresAtMs = 0
  let currentProxyId: number | null | undefined = null

  const clearPollTimer = () => {
    if (pollTimer !== null) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
  }

  const stopPolling = () => {
    clearPollTimer()
    polling.value = false
  }

  const resetState = () => {
    stopPolling()
    sessionId.value = ''
    userCode.value = ''
    verificationUri.value = ''
    verificationUriComplete.value = ''
    pollInterval.value = KIMI_DEFAULT_POLL_INTERVAL_SECONDS
    expiresIn.value = 0
    status.value = 'idle'
    loading.value = false
    error.value = ''
    expiresAtMs = 0
  }

  onScopeDispose(() => {
    stopPolling()
  })

  const scheduleNextPoll = (delaySeconds: number) => {
    clearPollTimer()
    const delay = delaySeconds > 0 ? delaySeconds : KIMI_DEFAULT_POLL_INTERVAL_SECONDS
    polling.value = true
    pollTimer = setTimeout(() => {
      pollTimer = null
      void pollDeviceToken()
    }, delay * 1000)
  }

  const startDeviceAuth = async (proxyId: number | null | undefined): Promise<boolean> => {
    stopPolling()
    sessionId.value = ''
    userCode.value = ''
    verificationUri.value = ''
    verificationUriComplete.value = ''
    error.value = ''
    loading.value = true
    status.value = 'starting'
    currentProxyId = proxyId

    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId

      const result = await adminAPI.kimi.startDeviceAuth(payload)
      sessionId.value = result.session_id
      userCode.value = result.user_code
      verificationUri.value = result.verification_uri
      verificationUriComplete.value = result.verification_uri_complete || result.verification_uri
      pollInterval.value =
        parsePositiveInt(result.interval) ?? KIMI_DEFAULT_POLL_INTERVAL_SECONDS
      expiresIn.value = result.expires_in || 0
      expiresAtMs = expiresIn.value > 0 ? Date.now() + expiresIn.value * 1000 : 0
      status.value = 'awaiting_user'
      scheduleNextPoll(pollInterval.value)
      return true
    } catch (err: any) {
      status.value = 'error'
      error.value = err?.message || t('admin.accounts.oauth.kimi.failedToStart')
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Execute a single device-token poll. Normally driven by the internal timer
   * started by startDeviceAuth; exposed for manual retries and tests.
   */
  const pollDeviceToken = async (): Promise<KimiTokenInfo | null> => {
    if (!sessionId.value || status.value !== 'awaiting_user') return null

    // Client-side expiry guard: the device code is only valid for expires_in seconds.
    if (expiresAtMs > 0 && Date.now() >= expiresAtMs) {
      stopPolling()
      status.value = 'expired'
      error.value = t('admin.accounts.oauth.kimi.expired')
      return null
    }

    try {
      const payload: Record<string, unknown> = { session_id: sessionId.value }
      if (currentProxyId) payload.proxy_id = currentProxyId

      const tokenInfo = await adminAPI.kimi.pollDeviceToken(
        payload as { session_id: string; proxy_id?: number }
      )
      stopPolling()
      status.value = 'success'
      error.value = ''
      options.onAuthorized?.(tokenInfo)
      return tokenInfo
    } catch (err: any) {
      const reason = err?.reason
      switch (reason) {
        case KIMI_OAUTH_REASON_PENDING: {
          const next = parsePositiveInt(err?.metadata?.interval) ?? pollInterval.value
          pollInterval.value = next
          scheduleNextPoll(next)
          return null
        }
        case KIMI_OAUTH_REASON_SLOW_DOWN: {
          const suggested =
            parsePositiveInt(err?.metadata?.suggested_interval) ??
            pollInterval.value + KIMI_DEFAULT_POLL_INTERVAL_SECONDS
          pollInterval.value = suggested
          scheduleNextPoll(suggested)
          return null
        }
        case KIMI_OAUTH_REASON_EXPIRED: {
          stopPolling()
          status.value = 'expired'
          error.value = t('admin.accounts.oauth.kimi.expired')
          return null
        }
        case KIMI_OAUTH_REASON_ACCESS_DENIED: {
          stopPolling()
          status.value = 'denied'
          error.value = t('admin.accounts.oauth.kimi.denied')
          return null
        }
        default: {
          stopPolling()
          status.value = 'error'
          error.value = err?.message || t('admin.accounts.oauth.kimi.pollFailed')
          return null
        }
      }
    }
  }

  const buildCredentials = (tokenInfo: KimiTokenInfo): Record<string, unknown> =>
    buildKimiCredentials(tokenInfo)

  const buildExtraInfo = (tokenInfo: KimiTokenInfo): Record<string, unknown> =>
    buildKimiExtraInfo(tokenInfo)

  return {
    sessionId,
    userCode,
    verificationUri,
    verificationUriComplete,
    pollInterval,
    expiresIn,
    status,
    loading,
    polling,
    error,
    resetState,
    stopPolling,
    startDeviceAuth,
    pollDeviceToken,
    buildCredentials,
    buildExtraInfo
  }
}

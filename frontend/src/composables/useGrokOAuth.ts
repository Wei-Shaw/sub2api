import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { GrokTokenInfo } from '@/api/admin/grok'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

const DEFAULT_POLL_INTERVAL_MS = 5_000
const MAX_POLL_INTERVAL_MS = 30_000

export function useGrokOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const flow = ref('')
  const userCode = ref('')
  const pollIntervalSec = ref(5)
  const deviceExpiresAt = ref<number | null>(null)
  const deviceStatus = ref('')
  const authorizedToken = ref<GrokTokenInfo | null>(null)
  const loading = ref(false)
  const polling = ref(false)
  const error = ref('')

  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let pollGeneration = 0

  const clearPollTimer = () => {
    if (pollTimer != null) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
  }

  const stopPolling = () => {
    pollGeneration += 1
    clearPollTimer()
    polling.value = false
  }

  const resetState = () => {
    stopPolling()
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    flow.value = ''
    userCode.value = ''
    pollIntervalSec.value = 5
    deviceExpiresAt.value = null
    deviceStatus.value = ''
    authorizedToken.value = null
    loading.value = false
    error.value = ''
  }

  const schedulePoll = (session: string, proxyId: number | null | undefined, delayMs: number, generation: number) => {
    clearPollTimer()
    pollTimer = setTimeout(() => {
      void pollOnce(session, proxyId, generation)
    }, Math.max(1_000, delayMs))
  }

  const pollOnce = async (
    session: string,
    proxyId: number | null | undefined,
    generation: number
  ): Promise<GrokTokenInfo | null> => {
    if (generation !== pollGeneration) return null
    if (!session) return null

    try {
      const payload: Record<string, unknown> = { session_id: session }
      if (proxyId) payload.proxy_id = proxyId
      const result = await adminAPI.grok.pollDeviceLogin(payload as any)
      if (generation !== pollGeneration) return null

      deviceStatus.value = result.status || ''
      if (result.user_code) userCode.value = result.user_code
      if (result.expires_at) deviceExpiresAt.value = result.expires_at
      if (result.interval && result.interval > 0) {
        pollIntervalSec.value = result.interval
      }

      if (result.status === 'authorized' && result.token) {
        authorizedToken.value = result.token
        polling.value = false
        return result.token
      }

      if (result.status === 'denied' || result.status === 'expired') {
        polling.value = false
        error.value =
          result.error ||
          (result.status === 'expired'
            ? t('admin.accounts.oauth.grok.deviceCodeExpired')
            : t('admin.accounts.oauth.grok.deviceAuthorizationDenied'))
        appStore.showError(error.value)
        return null
      }

      if (result.status === 'error') {
        // Transient upstream error: keep polling with a modest backoff.
        const delay = Math.min(
          MAX_POLL_INTERVAL_MS,
          Math.max(DEFAULT_POLL_INTERVAL_MS, (pollIntervalSec.value || 5) * 1000 + 2_000)
        )
        schedulePoll(session, proxyId, delay, generation)
        return null
      }

      const intervalSec = Math.max(1, pollIntervalSec.value || 5)
      const delay =
        result.status === 'slow_down'
          ? Math.min(MAX_POLL_INTERVAL_MS, (intervalSec + 5) * 1000)
          : intervalSec * 1000
      schedulePoll(session, proxyId, delay, generation)
      return null
    } catch (err: any) {
      if (generation !== pollGeneration) return null
      // Network blips should not kill the whole device session.
      const delay = Math.min(MAX_POLL_INTERVAL_MS, (pollIntervalSec.value || 5) * 1000 + 2_000)
      schedulePoll(session, proxyId, delay, generation)
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.grok.errors',
        t('admin.accounts.oauth.grok.failedToPollDevice')
      )
      return null
    }
  }

  const startDevicePolling = (proxyId?: number | null) => {
    if (!sessionId.value) return
    stopPolling()
    const generation = pollGeneration
    polling.value = true
    deviceStatus.value = 'pending'
    schedulePoll(sessionId.value, proxyId, 1_500, generation)
  }

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    flow.value = ''
    userCode.value = ''
    pollIntervalSec.value = 5
    deviceExpiresAt.value = null
    deviceStatus.value = ''
    authorizedToken.value = null
    error.value = ''
    stopPolling()

    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId

      const response = await adminAPI.grok.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      flow.value = response.flow || 'device'
      userCode.value = response.user_code || ''
      pollIntervalSec.value = response.interval && response.interval > 0 ? response.interval : 5
      deviceExpiresAt.value = response.expires_at ?? null
      startDevicePolling(proxyId)
      return true
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.grok.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeAuthCode = async (params: {
    code?: string
    sessionId: string
    state?: string
    proxyId?: number | null
  }): Promise<GrokTokenInfo | null> => {
    if (authorizedToken.value) {
      return authorizedToken.value
    }
    if (!params.sessionId) {
      error.value = t('admin.accounts.oauth.grok.missingExchangeParams')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const payload: Record<string, unknown> = {
        session_id: params.sessionId
      }
      if (params.state) payload.state = params.state
      if (params.code) payload.code = params.code
      if (params.proxyId) payload.proxy_id = params.proxyId

      const token = await adminAPI.grok.exchangeCode(payload as any)
      authorizedToken.value = token
      stopPolling()
      return token
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.grok.errors',
        t('admin.accounts.oauth.grok.failedToExchangeCode')
      )
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<GrokTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.grok.pleaseEnterRefreshToken')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      return await adminAPI.grok.refreshGrokToken(refreshToken.trim(), proxyId)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.grok.errors',
        t('admin.accounts.oauth.grok.failedToValidateRT')
      )
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      token_type: tokenInfo.token_type,
      expires_at: tokenInfo.expires_at,
      client_id: tokenInfo.client_id,
      scope: tokenInfo.scope,
      email: tokenInfo.email,
      sub: tokenInfo.sub,
      team_id: tokenInfo.team_id,
      subscription_tier: tokenInfo.subscription_tier,
      entitlement_status: tokenInfo.entitlement_status,
      base_url: 'https://cli-chat-proxy.grok.com/v1'
    }
    if (tokenInfo.refresh_token) credentials.refresh_token = tokenInfo.refresh_token
    if (tokenInfo.id_token) credentials.id_token = tokenInfo.id_token
    return Object.fromEntries(Object.entries(credentials).filter(([, value]) => value !== undefined && value !== ''))
  }

  const buildExtraInfo = (tokenInfo: GrokTokenInfo): Record<string, unknown> => {
    const extra: Record<string, unknown> = {}
    if (tokenInfo.email) extra.email = tokenInfo.email
    if (tokenInfo.subscription_tier) extra.subscription_tier = tokenInfo.subscription_tier
    if (tokenInfo.entitlement_status) extra.entitlement_status = tokenInfo.entitlement_status
    return extra
  }

  return {
    authUrl,
    sessionId,
    state,
    flow,
    userCode,
    pollIntervalSec,
    deviceExpiresAt,
    deviceStatus,
    authorizedToken,
    loading,
    polling,
    error,
    resetState,
    generateAuthUrl,
    startDevicePolling,
    stopPolling,
    exchangeAuthCode,
    validateRefreshToken,
    buildCredentials,
    buildExtraInfo
  }
}

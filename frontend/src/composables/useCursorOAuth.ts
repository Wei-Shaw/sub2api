import { getCurrentScope, onScopeDispose, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { CursorTokenInfo } from '@/api/admin/cursor'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

const POLL_INTERVAL_MS = 2000
const POLL_TIMEOUT_MS = 5 * 60 * 1000

export function useCursorOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const loading = ref(false)
  const error = ref('')
  let generation = 0
  let pollController: AbortController | null = null

  const resetState = () => {
    generation++
    pollController?.abort()
    pollController = null
    authUrl.value = ''
    sessionId.value = ''
    loading.value = false
    error.value = ''
  }

  if (getCurrentScope()) onScopeDispose(resetState)

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    resetState()
    const attempt = generation
    loading.value = true
    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId
      const response = await adminAPI.cursor.generateAuthUrl(payload)
      if (attempt !== generation) return false
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      return true
    } catch (err: unknown) {
      if (attempt !== generation) return false
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.cursor.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      if (attempt === generation) loading.value = false
    }
  }

  const pollOnce = async (proxyId?: number | null, signal?: AbortSignal): Promise<CursorTokenInfo | null> => {
    if (!sessionId.value) {
      error.value = t('admin.accounts.oauth.cursor.missingExchangeParams')
      return null
    }
    const payload: { session_id: string; proxy_id?: number } = { session_id: sessionId.value }
    if (proxyId) payload.proxy_id = proxyId
    return adminAPI.cursor.poll(payload, signal)
  }

  const pollUntilReady = async (proxyId?: number | null): Promise<CursorTokenInfo | null> => {
    pollController?.abort()
    const controller = new AbortController()
    pollController = controller
    const attempt = ++generation
    const started = Date.now()
    loading.value = true
    error.value = ''
    try {
      while (!controller.signal.aborted && Date.now() - started < POLL_TIMEOUT_MS) {
        const info = await pollOnce(proxyId, controller.signal)
        if (attempt !== generation || controller.signal.aborted) return null
        if (info && !info.pending && info.access_token) return info
        await new Promise<void>((resolve) => {
          const finish = () => {
            clearTimeout(timer)
            controller.signal.removeEventListener('abort', finish)
            resolve()
          }
          const timer = setTimeout(finish, POLL_INTERVAL_MS)
          controller.signal.addEventListener('abort', finish, { once: true })
          if (controller.signal.aborted) finish()
        })
      }
      if (attempt !== generation || controller.signal.aborted) return null
      error.value = t('admin.accounts.oauth.cursor.pollTimeout')
      appStore.showError(error.value)
      return null
    } catch (err: unknown) {
      if (attempt !== generation || controller.signal.aborted) return null
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.cursor.errors',
        t('admin.accounts.oauth.cursor.failedToPoll')
      )
      appStore.showError(error.value)
      return null
    } finally {
      if (attempt === generation) {
        pollController = null
        loading.value = false
      }
    }
  }

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<CursorTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.cursor.pleaseEnterRefreshToken')
      return null
    }
    const attempt = generation
    loading.value = true
    error.value = ''
    try {
      const tokenInfo = await adminAPI.cursor.refreshCursorToken(refreshToken.trim(), proxyId)
      return attempt === generation ? tokenInfo : null
    } catch (err: unknown) {
      if (attempt !== generation) return null
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.cursor.errors',
        t('admin.accounts.oauth.cursor.failedToValidateRT')
      )
      appStore.showError(error.value)
      return null
    } finally {
      if (attempt === generation) loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: CursorTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      expires_at: tokenInfo.expires_at,
      user_id: tokenInfo.user_id,
      email: tokenInfo.email
    }
    return Object.fromEntries(
      Object.entries(credentials).filter(([, value]) => value !== undefined && value !== '')
    )
  }

  const buildExtraInfo = (tokenInfo: CursorTokenInfo): Record<string, unknown> => {
    const extra: Record<string, unknown> = {}
    if (tokenInfo.email) extra.email = tokenInfo.email
    if (tokenInfo.user_id) extra.user_id = tokenInfo.user_id
    return extra
  }

  return {
    authUrl,
    sessionId,
    loading,
    error,
    resetState,
    generateAuthUrl,
    pollOnce,
    pollUntilReady,
    validateRefreshToken,
    buildCredentials,
    buildExtraInfo
  }
}

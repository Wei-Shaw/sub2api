import { getCurrentScope, onScopeDispose, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { DevinTokenInfo } from '@/api/admin/devin'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

export function useDevinOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')
  let generation = 0

  const resetState = () => {
    generation++
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
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
      const response = await adminAPI.devin.generateAuthUrl(payload)
      if (attempt !== generation) return false
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: unknown) {
      if (attempt !== generation) return false
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.devin.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      if (attempt === generation) loading.value = false
    }
  }

  const exchangeAuthCode = async (params: {
    code: string
    sessionId: string
    state?: string
    proxyId?: number | null
  }): Promise<DevinTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId) {
      error.value = t('admin.accounts.oauth.devin.missingExchangeParams')
      return null
    }
    const attempt = generation
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {
        session_id: params.sessionId,
        code,
        state: params.state || state.value
      }
      if (params.proxyId) payload.proxy_id = params.proxyId
      const tokenInfo = await adminAPI.devin.exchangeCode(payload as {
        session_id: string
        code: string
        state?: string
        proxy_id?: number
      })
      return attempt === generation ? tokenInfo : null
    } catch (err: unknown) {
      if (attempt !== generation) return null
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.devin.errors',
        t('admin.accounts.oauth.devin.failedToExchangeCode')
      )
      appStore.showError(error.value)
      return null
    } finally {
      if (attempt === generation) loading.value = false
    }
  }

  const importSessionToken = async (sessionToken: string): Promise<DevinTokenInfo | null> => {
    if (!sessionToken.trim()) {
      error.value = t('admin.accounts.oauth.devin.pleaseEnterSessionToken')
      return null
    }
    const attempt = generation
    loading.value = true
    error.value = ''
    try {
      const tokenInfo = await adminAPI.devin.importToken(sessionToken.trim())
      return attempt === generation ? tokenInfo : null
    } catch (err: unknown) {
      if (attempt !== generation) return null
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.devin.errors',
        t('admin.accounts.oauth.devin.failedToImportToken')
      )
      appStore.showError(error.value)
      return null
    } finally {
      if (attempt === generation) loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: DevinTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token || tokenInfo.access_token,
      expires_at: tokenInfo.expires_at,
      api_endpoint: tokenInfo.api_endpoint,
      enterprise_url: tokenInfo.enterprise_url
    }
    return Object.fromEntries(
      Object.entries(credentials).filter(([, value]) => value !== undefined && value !== '')
    )
  }

  const buildExtraInfo = (_tokenInfo: DevinTokenInfo): Record<string, unknown> => {
    return {}
  }

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    importSessionToken,
    buildCredentials,
    buildExtraInfo
  }
}

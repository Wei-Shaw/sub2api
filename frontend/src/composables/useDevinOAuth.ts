import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { DevinExchangeCodeRequest, DevinTokenInfo } from '@/api/admin/devin'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

/**
 * Devin (Cognition) PKCE 登录：
 * 生成授权链接 → 浏览器授权 → 粘贴授权码（或直接粘贴 devin-session-token$…）。
 */
export function useDevinOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    loading.value = false
    error.value = ''
  }

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {}
      if (proxyId) payload.proxy_id = proxyId

      const response = await adminAPI.devin.generateAuthUrl(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.devin.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  // 粘贴 PKCE 授权码需要 session_id/state；直接粘贴 devin-session-token$…
  // 形态的 api key 时后端跳过交换，可省略会话字段。
  const exchangeAuthCode = async (params: {
    code: string
    sessionId?: string
    state?: string
    proxyId?: number | null
  }): Promise<DevinTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code) {
      error.value = t('admin.accounts.oauth.devin.missingExchangeParams')
      return null
    }
    const isTokenPaste = code.startsWith('devin-session-token$')
    if (!isTokenPaste && (!params.sessionId || !params.state)) {
      error.value = t('admin.accounts.oauth.devin.missingExchangeParams')
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const payload: DevinExchangeCodeRequest = { code }
      if (params.sessionId) payload.session_id = params.sessionId
      if (params.state) payload.state = params.state
      if (params.proxyId) payload.proxy_id = params.proxyId

      return await adminAPI.devin.exchangeCode(payload)
    } catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.devin.errors',
        t('admin.accounts.oauth.devin.failedToExchangeCode')
      )
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  // 建号凭据：access_token 是 Connect api key；api_server_url/client_version
  // 为空时后端回落默认值。
  const buildCredentials = (tokenInfo: DevinTokenInfo): Record<string, unknown> => {
    const credentials: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      api_server_url: tokenInfo.api_server_url,
      client_version: tokenInfo.client_version
    }
    return Object.fromEntries(
      Object.entries(credentials).filter(
        ([, value]) => value !== undefined && value !== ''
      )
    )
  }

  const buildExtraInfo = (tokenInfo: DevinTokenInfo): Record<string, unknown> => {
    const extra: Record<string, unknown> = {}
    if (tokenInfo.email) extra.email = tokenInfo.email
    if (tokenInfo.org_id) extra.org_id = tokenInfo.org_id
    if (tokenInfo.plan_name) extra.plan_name = tokenInfo.plan_name
    if (tokenInfo.account_display_name) extra.account_display_name = tokenInfo.account_display_name
    return extra
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
    buildCredentials,
    buildExtraInfo
  }
}

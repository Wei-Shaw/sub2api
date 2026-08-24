import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { KiroTokenInfo } from '@/api/admin/kiro'

export function useKiroOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')
  // External IdP(Microsoft Entra ID)两阶段登录进度：
  // 'portal' = 阶段1，等待用户粘贴 Kiro 门户回调 descriptor；
  // 'idp'    = 阶段2，已拿到 Entra 授权 URL，等待用户粘贴 M365 登录后的 code 回调。
  const externalIdpStage = ref<'portal' | 'idp'>('portal')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    loading.value = false
    error.value = ''
    externalIdpStage.value = 'portal'
  }

  const generateAuthUrl = async (
    proxyId: number | null | undefined,
    provider: 'Google' | 'Github' | 'ExternalIdp' = 'Google'
  ): Promise<boolean> => {
    loading.value = true
    error.value = ''
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    externalIdpStage.value = 'portal'

    try {
      const response = await adminAPI.kiro.generateAuthUrl({
        proxy_id: proxyId || undefined,
        provider
      })
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const generateIDCAuthUrl = async (
    params: { proxyId?: number | null; startUrl?: string; region?: string }
  ): Promise<boolean> => {
    loading.value = true
    error.value = ''
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''

    try {
      const response = await adminAPI.kiro.generateIDCAuthUrl({
        proxy_id: params.proxyId || undefined,
        start_url: params.startUrl,
        region: params.region
      })
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeAuthCode = async (params: {
    code: string
    sessionId: string
    state: string
    callbackPath?: string
    loginOption?: string
    proxyId?: number | null
  }): Promise<KiroTokenInfo | null> => {
    loading.value = true
    error.value = ''
    try {
      const response = await adminAPI.kiro.exchangeCode({
        session_id: params.sessionId,
        state: params.state,
        code: params.code.trim(),
        callback_path: params.callbackPath,
        login_option: params.loginOption,
        proxy_id: params.proxyId || undefined
      })
      if (response.auth_url && response.session_id && response.state && !response.access_token) {
        // External IdP 第一阶段完成：后端返回了 Entra 授权 URL，进入第二阶段。
        authUrl.value = response.auth_url
        sessionId.value = response.session_id
        state.value = response.state
        externalIdpStage.value = 'idp'
        return null
      }
      return response
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = async (payload: {
    refreshToken: string
    authMethod?: string
    provider?: string
    clientId?: string
    clientSecret?: string
    startUrl?: string
    region?: string
    profileArn?: string
    tokenEndpoint?: string
    issuerUrl?: string
    scopes?: string
    proxyId?: number | null
  }): Promise<KiroTokenInfo | null> => {
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.refreshToken({
        refresh_token: payload.refreshToken.trim(),
        auth_method: payload.authMethod,
        provider: payload.provider,
        client_id: payload.clientId,
        client_secret: payload.clientSecret,
        start_url: payload.startUrl,
        region: payload.region,
        profile_arn: payload.profileArn,
        token_endpoint: payload.tokenEndpoint,
        issuer_url: payload.issuerUrl,
        scopes: payload.scopes,
        proxy_id: payload.proxyId || undefined
      })
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      return null
    } finally {
      loading.value = false
    }
  }

  const importToken = async (
    tokenJSON: string,
    deviceRegistrationJSON?: string
  ): Promise<KiroTokenInfo | null> => {
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.importToken({
        token_json: tokenJSON,
        device_registration_json: deviceRegistrationJSON
      })
    } catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const buildCredentials = (tokenInfo: KiroTokenInfo): Record<string, unknown> => ({
    access_token: tokenInfo.access_token,
    refresh_token: tokenInfo.refresh_token,
    profile_arn: tokenInfo.profile_arn,
    expires_at: tokenInfo.expires_at,
    auth_method: tokenInfo.auth_method,
    provider: tokenInfo.provider,
    client_id: tokenInfo.client_id,
    client_secret: tokenInfo.client_secret,
    client_id_hash: tokenInfo.client_id_hash,
    email: tokenInfo.email,
    start_url: tokenInfo.start_url,
    region: tokenInfo.region,
    token_endpoint: tokenInfo.token_endpoint,
    issuer_url: tokenInfo.issuer_url,
    scopes: tokenInfo.scopes
  })

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    externalIdpStage,
    resetState,
    generateAuthUrl,
    generateIDCAuthUrl,
    exchangeAuthCode,
    validateRefreshToken,
    importToken,
    buildCredentials
  }
}

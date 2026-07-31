import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { apiClient } from '@/api/client'
import {
  type OAuthComposableOptions,
  type OAuthSurface,
  effectiveProxyId,
  oauthPrefix
} from '@/utils/oauthSurface'

export type AddMethod = 'oauth' | 'setup-token'
export type AuthInputMethod = 'manual' | 'cookie' | 'refresh_token' | 'mobile_refresh_token' | 'session_token' | 'access_token' | 'codex_session' | 'agent_identity' | 'codex_pat' | 'sso_cookie'

export interface OAuthState {
  authUrl: string
  authCode: string
  sessionId: string
  sessionKey: string
  loading: boolean
  error: string
}

export interface TokenInfo {
  org_uuid?: string
  account_uuid?: string
  email_address?: string
  [key: string]: unknown
}

export function useAccountOAuth(opts?: OAuthComposableOptions) {
  const appStore = useAppStore()
  const surface: OAuthSurface = opts?.surface ?? 'admin'
  const prefix = oauthPrefix(surface, 'accounts')

  // State
  const authUrl = ref('')
  const authCode = ref('')
  const sessionId = ref('')
  const sessionKey = ref('')
  const loading = ref(false)
  const error = ref('')

  // Reset state
  const resetState = () => {
    authUrl.value = ''
    authCode.value = ''
    sessionId.value = ''
    sessionKey.value = ''
    loading.value = false
    error.value = ''
  }

  const postOAuth = async <T>(
    endpoint: string,
    body: Record<string, unknown>
  ): Promise<T> => {
    if (surface === 'user') {
      const { data } = await apiClient.post<T>(endpoint, body)
      return data
    }
    // admin: reuse accounts helpers that take full endpoint path
    if (endpoint.includes('exchange') || endpoint.includes('cookie-auth')) {
      return (await adminAPI.accounts.exchangeCode(endpoint, body as any)) as T
    }
    return (await adminAPI.accounts.generateAuthUrl(endpoint, body as any)) as T
  }

  // Generate auth URL
  const generateAuthUrl = async (
    addMethod: AddMethod,
    proxyId?: number | null
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    error.value = ''

    try {
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      const proxyConfig = effectiveProxy ? { proxy_id: effectiveProxy } : {}
      const endpoint =
        addMethod === 'oauth'
          ? `${prefix}/generate-auth-url`
          : `${prefix}/generate-setup-token-url`

      const response = await postOAuth<{ auth_url: string; session_id: string }>(
        endpoint,
        proxyConfig
      )
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to generate auth URL'
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  // Exchange auth code for tokens
  const exchangeAuthCode = async (
    addMethod: AddMethod,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!authCode.value.trim() || !sessionId.value) {
      error.value = 'Missing auth code or session ID'
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      const proxyConfig = effectiveProxy ? { proxy_id: effectiveProxy } : {}
      const endpoint =
        addMethod === 'oauth'
          ? `${prefix}/exchange-code`
          : `${prefix}/exchange-setup-token-code`

      const tokenInfo = await postOAuth<TokenInfo>(endpoint, {
        session_id: sessionId.value,
        code: authCode.value.trim(),
        ...proxyConfig
      })

      return tokenInfo
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to exchange auth code'
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  // Cookie-based authentication
  const cookieAuth = async (
    addMethod: AddMethod,
    sessionKeyValue: string,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!sessionKeyValue.trim()) {
      error.value = 'Please enter sessionKey'
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const effectiveProxy = effectiveProxyId(surface, proxyId)
      const proxyConfig = effectiveProxy ? { proxy_id: effectiveProxy } : {}
      const endpoint =
        addMethod === 'oauth'
          ? `${prefix}/cookie-auth`
          : `${prefix}/setup-token-cookie-auth`

      const tokenInfo = await postOAuth<TokenInfo>(endpoint, {
        session_id: '',
        code: sessionKeyValue.trim(),
        ...proxyConfig
      })

      return tokenInfo
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Cookie authorization failed'
      return null
    } finally {
      loading.value = false
    }
  }

  // Parse multiple session keys
  const parseSessionKeys = (input: string): string[] => {
    return input
      .split('\n')
      .map((k) => k.trim())
      .filter((k) => k)
  }

  // Build extra info from token response
  const buildExtraInfo = (tokenInfo: TokenInfo): Record<string, string> | undefined => {
    const extra: Record<string, string> = {}
    if (tokenInfo.org_uuid) {
      extra.org_uuid = tokenInfo.org_uuid
    }
    if (tokenInfo.account_uuid) {
      extra.account_uuid = tokenInfo.account_uuid
    }
    if (tokenInfo.email_address) {
      extra.email_address = tokenInfo.email_address
    }
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  return {
    // State
    authUrl,
    authCode,
    sessionId,
    sessionKey,
    loading,
    error,
    // Methods
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    cookieAuth,
    parseSessionKeys,
    buildExtraInfo
  }
}

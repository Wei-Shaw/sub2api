import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'

export type AddMethod = 'oauth' | 'setup-token'
export type AuthInputMethod = 'manual' | 'cookie' | 'refresh_token' | 'mobile_refresh_token' | 'session_token' | 'access_token' | 'codex_session'

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

export function useAccountOAuth() {
  const appStore = useAppStore()

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
      const proxyConfig = proxyId ? { proxy_id: proxyId } : {}
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/generate-auth-url'
          : '/admin/accounts/generate-setup-token-url'

      const response = await adminAPI.accounts.generateAuthUrl(endpoint, proxyConfig)
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
      const proxyConfig = proxyId ? { proxy_id: proxyId } : {}
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/exchange-code'
          : '/admin/accounts/exchange-setup-token-code'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: sessionId.value,
        code: authCode.value.trim(),
        ...proxyConfig
      })

      return tokenInfo as TokenInfo
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
      const proxyConfig = proxyId ? { proxy_id: proxyId } : {}
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/cookie-auth'
          : '/admin/accounts/setup-token-cookie-auth'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: '',
        code: sessionKeyValue.trim(),
        ...proxyConfig
      })

      return tokenInfo as TokenInfo
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

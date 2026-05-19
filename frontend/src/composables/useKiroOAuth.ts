/**
 * Kiro OAuth composable
 *
 * Three auth flows:
 *  - validateSocialRefreshToken: paste a refresh token from the Kiro
 *    desktop app and validate it against Kiro's social refresh endpoint.
 *  - startIdCLogin / completeIdCLogin: PKCE auth-code flow against an
 *    AWS Identity Center startUrl. The admin opens the returned auth_url,
 *    completes login in their browser, then pastes the redirected
 *    `http://127.0.0.1/oauth/callback?code=...&state=...` URL back into
 *    completeIdCLogin together with the session_id.
 *  - startBuilderIDLogin / pollBuilderIDLogin: AWS Builder ID device-code
 *    flow. The UI shows the user_code + verification_uri and polls until
 *    status === 'completed'.
 */

import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  KiroBuilderIDLoginResult,
  KiroBuilderIDPollResult,
  KiroIdCAuthURLResult,
  KiroTokenInfo
} from '@/api/admin/kiro'

export function useKiroOAuth() {
  const appStore = useAppStore()

  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    loading.value = false
    error.value = ''
  }

  const errorOf = (err: any, fallback: string): string =>
    err?.response?.data?.detail || err?.response?.data?.message || fallback

  /** Validate a pasted Social refresh token. */
  const validateSocialRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<KiroTokenInfo | null> => {
    const trimmed = refreshToken.trim()
    if (!trimmed) {
      error.value = 'Please enter a Kiro refresh token'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.validateSocialRefreshToken({
        refresh_token: trimmed,
        proxy_id: proxyId ?? undefined
      })
    } catch (err: any) {
      error.value = errorOf(err, 'Failed to validate Kiro refresh token')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  /** Begin an IdC PKCE auth-code flow. Returns the authorize URL + session id. */
  const startIdCLogin = async (
    startUrl: string,
    region?: string,
    proxyId?: number | null
  ): Promise<KiroIdCAuthURLResult | null> => {
    const trimmed = startUrl.trim()
    if (!trimmed) {
      error.value = 'Please enter the AWS Identity Center start URL'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.startIdCLogin({
        start_url: trimmed,
        region: region?.trim() || undefined,
        proxy_id: proxyId ?? undefined
      })
    } catch (err: any) {
      error.value = errorOf(err, 'Failed to start IdC login')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  /** Complete an IdC PKCE flow with the pasted redirected callback URL. */
  const completeIdCLogin = async (
    sessionId: string,
    callbackUrl: string
  ): Promise<KiroTokenInfo | null> => {
    if (!sessionId.trim() || !callbackUrl.trim()) {
      error.value = 'session_id and callback_url are required'
      return null
    }
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.completeIdCLogin({
        session_id: sessionId.trim(),
        callback_url: callbackUrl.trim()
      })
    } catch (err: any) {
      error.value = errorOf(err, 'Failed to complete IdC login')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  /** Begin a Builder ID device-code flow. */
  const startBuilderIDLogin = async (
    region?: string,
    proxyId?: number | null
  ): Promise<KiroBuilderIDLoginResult | null> => {
    loading.value = true
    error.value = ''
    try {
      return await adminAPI.kiro.startBuilderIDLogin({
        region: region?.trim() || undefined,
        proxy_id: proxyId ?? undefined
      })
    } catch (err: any) {
      error.value = errorOf(err, 'Failed to start Builder ID login')
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  /** Poll a Builder ID session once. */
  const pollBuilderIDLogin = async (
    sessionId: string
  ): Promise<KiroBuilderIDPollResult | null> => {
    if (!sessionId.trim()) {
      error.value = 'session_id is required'
      return null
    }
    error.value = ''
    try {
      return await adminAPI.kiro.pollBuilderIDLogin({ session_id: sessionId.trim() })
    } catch (err: any) {
      error.value = errorOf(err, 'Builder ID polling failed')
      // Don't auto-toast on every failed poll attempt — the UI shows the
      // error inline.
      return null
    }
  }

  /**
   * Translate a KiroTokenInfo into the credentials JSON shape persisted
   * on the account row. Matches the structure the backend
   * KiroOAuthService.BuildAccountCredentials writes during refresh.
   */
  const buildCredentials = (tokenInfo: KiroTokenInfo): Record<string, unknown> => {
    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      expires_at: tokenInfo.expires_at,
      auth_method: tokenInfo.auth_method ?? 'social'
    }
    if (tokenInfo.client_id) creds.client_id = tokenInfo.client_id
    if (tokenInfo.client_secret) creds.client_secret = tokenInfo.client_secret
    if (tokenInfo.region) creds.region = tokenInfo.region
    if (tokenInfo.start_url) creds.start_url = tokenInfo.start_url
    return creds
  }

  /** Suggested account name from a KiroTokenInfo. */
  const suggestAccountName = (tokenInfo: KiroTokenInfo): string => {
    if (tokenInfo.email && tokenInfo.email.includes('@')) return tokenInfo.email
    if (tokenInfo.user_id) return `kiro-${tokenInfo.user_id}`
    if (tokenInfo.auth_method === 'idc' && tokenInfo.start_url) {
      try {
        const u = new URL(tokenInfo.start_url)
        return `kiro-idc-${u.hostname}`
      } catch {
        // fall through
      }
    }
    return 'Kiro account'
  }

  return {
    loading,
    error,
    resetState,
    validateSocialRefreshToken,
    startIdCLogin,
    completeIdCLogin,
    startBuilderIDLogin,
    pollBuilderIDLogin,
    buildCredentials,
    suggestAccountName
  }
}

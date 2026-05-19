/**
 * Kiro OAuth composable
 *
 * Phase 2 supports the Social refresh-token paste flow:
 *  - validateSocialRefreshToken: admin pastes a refresh token captured
 *    from the Kiro desktop app, we exchange it for fresh tokens + email.
 *
 * Phase 3 will extend this composable with IdC (PKCE auth-code) and
 * Builder ID (device-code) flows.
 */

import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { KiroTokenInfo } from '@/api/admin/kiro'

export function useKiroOAuth() {
  const appStore = useAppStore()

  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    loading.value = false
    error.value = ''
  }

  /**
   * Validate a pasted Social refresh token. Returns the freshly-rotated
   * tokens + best-effort email/user_id, or null on failure.
   */
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
      const tokenInfo = await adminAPI.kiro.validateSocialRefreshToken({
        refresh_token: trimmed,
        proxy_id: proxyId ?? undefined
      })
      return tokenInfo
    } catch (err: any) {
      error.value =
        err.response?.data?.detail ||
        err.response?.data?.message ||
        'Failed to validate Kiro refresh token'
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Translate a KiroTokenInfo into the credentials JSON shape persisted
   * on the account row. Matches the structure the backend
   * KiroOAuthService.BuildAccountCredentials writes during refresh, so
   * the very-first refresh after account create sees a familiar shape.
   */
  const buildCredentials = (tokenInfo: KiroTokenInfo): Record<string, unknown> => {
    return {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      expires_at: tokenInfo.expires_at,
      auth_method: tokenInfo.auth_method ?? 'social'
    }
  }

  /**
   * Suggested account name from a KiroTokenInfo: prefer email, fall back
   * to user_id, then to a generic placeholder.
   */
  const suggestAccountName = (tokenInfo: KiroTokenInfo): string => {
    if (tokenInfo.email && tokenInfo.email.includes('@')) return tokenInfo.email
    if (tokenInfo.user_id) return `kiro-${tokenInfo.user_id}`
    return 'Kiro account'
  }

  return {
    loading,
    error,
    resetState,
    validateSocialRefreshToken,
    buildCredentials,
    suggestAccountName
  }
}

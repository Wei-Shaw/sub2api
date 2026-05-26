import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { extractApiErrorMessage } from '@sub2api/plugin-sdk'
import { getClient } from '../api/client'

export type AddMethod = 'oauth' | 'setup-token'

export interface TokenInfo {
  org_uuid?: string
  account_uuid?: string
  email_address?: string
  [key: string]: unknown
}

export function useAccountOAuth() {
  const { t } = useI18n()

  const authUrl = ref('')
  const authCode = ref('')
  const sessionId = ref('')
  const sessionKey = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    authCode.value = ''
    sessionId.value = ''
    sessionKey.value = ''
    loading.value = false
    error.value = ''
  }

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
      const endpoint = addMethod === 'oauth'
        ? '/admin/accounts/generate-auth-url'
        : '/admin/accounts/generate-setup-token-url'
      const { data } = await getClient().post<{ auth_url: string; session_id: string }>(endpoint, proxyConfig)
      authUrl.value = data.auth_url
      sessionId.value = data.session_id
      return true
    } catch (err: unknown) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.failedToGenerateUrl'))
      return false
    } finally {
      loading.value = false
    }
  }

  const parseSessionKeys = (input: string): string[] => {
    return input.split('\n').map(k => k.trim()).filter(k => k)
  }

  const buildExtraInfo = (tokenInfo: TokenInfo): Record<string, string> | undefined => {
    const extra: Record<string, string> = {}
    if (tokenInfo.org_uuid) extra.org_uuid = tokenInfo.org_uuid
    if (tokenInfo.account_uuid) extra.account_uuid = tokenInfo.account_uuid
    if (tokenInfo.email_address) extra.email_address = tokenInfo.email_address
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  return {
    authUrl, authCode, sessionId, sessionKey, loading, error,
    resetState, generateAuthUrl, parseSessionKeys, buildExtraInfo,
  }
}

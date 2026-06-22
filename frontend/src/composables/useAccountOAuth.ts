import { ref REDACTED from 'vue'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'

export type AddMethod = 'oauth' | 'setup-token'
export type AuthInputMethod = 'manual' | 'cookie' | 'refresh_token' | 'mobile_refresh_token' | 'session_token' | 'access_token' | 'codex_session' | 'codex_pat'

export interface OAuthState {
  authUrl: string
  authCode: string
  sessionId: string
  sessionKey: string
  loading: boolean
  error: string
REDACTED

export interface TokenInfo {
  org_uuid?: string
  account_uuid?: string
  email_address?: string
  [key: string]: unknown
REDACTED

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
  REDACTED

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
      const proxyConfig = proxyId ? { proxy_id: proxyId REDACTED : {REDACTED
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/generate-auth-url'
          : '/admin/accounts/generate-setup-token-url'

      const response = await adminAPI.accounts.generateAuthUrl(endpoint, proxyConfig)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      return true
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to generate auth URL'
      appStore.showError(error.value)
      return false
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Exchange auth code for tokens
  const exchangeAuthCode = async (
    addMethod: AddMethod,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!authCode.value.trim() || !sessionId.value) {
      error.value = 'Missing auth code or session ID'
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      const proxyConfig = proxyId ? { proxy_id: proxyId REDACTED : {REDACTED
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/exchange-code'
          : '/admin/accounts/exchange-setup-token-code'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: sessionId.value,
        code: authCode.value.trim(),
        ...proxyConfig
      REDACTED)

      return tokenInfo as TokenInfo
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to exchange auth code'
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Cookie-based authentication
  const cookieAuth = async (
    addMethod: AddMethod,
    sessionKeyValue: string,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!sessionKeyValue.trim()) {
      error.value = 'Please enter sessionKey'
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      const proxyConfig = proxyId ? { proxy_id: proxyId REDACTED : {REDACTED
      const endpoint =
        addMethod === 'oauth'
          ? '/admin/accounts/cookie-auth'
          : '/admin/accounts/setup-token-cookie-auth'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: '',
        code: sessionKeyValue.trim(),
        ...proxyConfig
      REDACTED)

      return tokenInfo as TokenInfo
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || 'Cookie authorization failed'
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Parse multiple session keys
  const parseSessionKeys = (input: string): string[] => {
    return input
      .split('\n')
      .map((k) => k.trim())
      .filter((k) => k)
  REDACTED

  // Build extra info from token response
  const buildExtraInfo = (tokenInfo: TokenInfo): Record<string, string> | undefined => {
    const extra: Record<string, string> = {REDACTED
    if (tokenInfo.org_uuid) {
      extra.org_uuid = tokenInfo.org_uuid
    REDACTED
    if (tokenInfo.account_uuid) {
      extra.account_uuid = tokenInfo.account_uuid
    REDACTED
    if (tokenInfo.email_address) {
      extra.email_address = tokenInfo.email_address
    REDACTED
    return Object.keys(extra).length > 0 ? extra : undefined
  REDACTED

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
  REDACTED
REDACTED

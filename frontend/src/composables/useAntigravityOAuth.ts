import { ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { AntigravityTokenInfo REDACTED from '@/api/admin/antigravity'

export function useAntigravityOAuth() {
  const appStore = useAppStore()
  const { t REDACTED = useI18n()

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
  REDACTED

  const generateAuthUrl = async (proxyId: number | null | undefined): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {REDACTED
      if (proxyId) payload.proxy_id = proxyId

      const response = await adminAPI.antigravity.generateAuthUrl(payload as any)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    REDACTED catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToGenerateUrl')
      appStore.showError(error.value)
      return false
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  const exchangeAuthCode = async (params: {
    code: string
    sessionId: string
    state: string
    proxyId?: number | null
  REDACTED): Promise<AntigravityTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.antigravity.missingExchangeParams')
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      const payload: Record<string, unknown> = {
        session_id: params.sessionId,
        state: params.state,
        code
      REDACTED
      if (params.proxyId) payload.proxy_id = params.proxyId

      const tokenInfo = await adminAPI.antigravity.exchangeCode(payload as any)
      return tokenInfo as AntigravityTokenInfo
    REDACTED catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToExchangeCode')
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
  ): Promise<AntigravityTokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      const tokenInfo = await adminAPI.antigravity.refreshAntigravityToken(
        refreshToken.trim(),
        proxyId
      )
      return tokenInfo as AntigravityTokenInfo
    REDACTED catch (err: any) {
      error.value =
        err.response?.data?.detail || t('admin.accounts.oauth.antigravity.failedToValidateRT')
      // Don't show global error toast for batch validation to avoid spamming
      // appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  const buildCredentials = (
    tokenInfo: AntigravityTokenInfo,
    fallbackRefreshToken?: string
  ): Record<string, unknown> => {
    let expiresAt: string | undefined
    if (typeof tokenInfo.expires_at === 'number' && Number.isFinite(tokenInfo.expires_at)) {
      expiresAt = Math.floor(tokenInfo.expires_at).toString()
    REDACTED else if (typeof tokenInfo.expires_at === 'string' && tokenInfo.expires_at.trim()) {
      expiresAt = tokenInfo.expires_at.trim()
    REDACTED
    const refreshToken = tokenInfo.refresh_token?.trim()
      ? tokenInfo.refresh_token
      : fallbackRefreshToken

    return {
      access_token: tokenInfo.access_token,
      refresh_token: refreshToken,
      token_type: tokenInfo.token_type,
      expires_at: expiresAt,
      project_id: tokenInfo.project_id,
      email: tokenInfo.email
    REDACTED
  REDACTED

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    validateRefreshToken,
    buildCredentials
  REDACTED
REDACTED

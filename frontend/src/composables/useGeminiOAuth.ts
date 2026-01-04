import { ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { GeminiOAuthCapabilities REDACTED from '@/api/admin/gemini'

export interface GeminiTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  scope?: string
  expires_at?: number | string
  project_id?: string
  oauth_type?: string
  tier_id?: string
  extra?: Record<string, unknown>
  [key: string]: unknown
REDACTED

export function useGeminiOAuth() {
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

  const generateAuthUrl = async (
    proxyId: number | null | undefined,
    projectId?: string | null,
    oauthType?: string,
    tierId?: string
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {REDACTED
      if (proxyId) payload.proxy_id = proxyId
      const trimmedProjectID = projectId?.trim()
      if (trimmedProjectID) payload.project_id = trimmedProjectID
      if (oauthType) payload.oauth_type = oauthType
      const trimmedTierID = tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      const response = await adminAPI.gemini.generateAuthUrl(payload as any)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      state.value = response.state
      return true
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || t('admin.accounts.oauth.gemini.failedToGenerateUrl')
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
    oauthType?: string
    tierId?: string
  REDACTED): Promise<GeminiTokenInfo | null> => {
    const code = params.code?.trim()
    if (!code || !params.sessionId || !params.state) {
      error.value = t('admin.accounts.oauth.gemini.missingExchangeParams')
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
      if (params.oauthType) payload.oauth_type = params.oauthType
      const trimmedTierID = params.tierId?.trim()
      if (trimmedTierID) payload.tier_id = trimmedTierID

      const tokenInfo = await adminAPI.gemini.exchangeCode(payload as any)
      return tokenInfo as GeminiTokenInfo
    REDACTED catch (err: any) {
      // Check for specific missing project_id error
      const errorMessage = err.message || err.response?.data?.message || ''
      if (errorMessage.includes('missing project_id')) {
        error.value = t('admin.accounts.oauth.gemini.missingProjectId')
      REDACTED else {
        error.value = errorMessage || t('admin.accounts.oauth.gemini.failedToExchangeCode')
      REDACTED
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  const buildCredentials = (tokenInfo: GeminiTokenInfo): Record<string, unknown> => {
    let expiresAt: string | undefined
    if (typeof tokenInfo.expires_at === 'number' && Number.isFinite(tokenInfo.expires_at)) {
      expiresAt = Math.floor(tokenInfo.expires_at).toString()
    REDACTED else if (typeof tokenInfo.expires_at === 'string' && tokenInfo.expires_at.trim()) {
      expiresAt = tokenInfo.expires_at.trim()
    REDACTED

    return {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      token_type: tokenInfo.token_type,
      expires_at: expiresAt,
      scope: tokenInfo.scope,
      project_id: tokenInfo.project_id,
      oauth_type: tokenInfo.oauth_type,
      tier_id: tokenInfo.tier_id
    REDACTED
  REDACTED

  const buildExtraInfo = (tokenInfo: GeminiTokenInfo): Record<string, unknown> | undefined => {
    if (!tokenInfo.extra || typeof tokenInfo.extra !== 'object') return undefined
    return tokenInfo.extra
  REDACTED

  const getCapabilities = async (): Promise<GeminiOAuthCapabilities | null> => {
    try {
      return await adminAPI.gemini.getCapabilities()
    REDACTED catch (err: any) {
      // Capabilities are optional for older servers; don't block the UI.
      return null
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
    buildCredentials,
    buildExtraInfo,
    getCapabilities
  REDACTED
REDACTED

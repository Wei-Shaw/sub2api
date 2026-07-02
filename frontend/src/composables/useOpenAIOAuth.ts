import { ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import { extractApiErrorMessage, extractI18nErrorMessage REDACTED from '@/utils/apiError'

export interface OpenAITokenInfo {
  access_token?: string
  refresh_token?: string
  client_id?: string
  id_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number
  scope?: string
  email?: string
  name?: string
  plan_type?: string
  subscription_expires_at?: string
  privacy_mode?: string
  // OpenAI specific IDs (extracted from ID Token)
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  organization_id?: string
  [key: string]: unknown
REDACTED

export type OpenAIOAuthPlatform = 'openai'

export function useOpenAIOAuth() {
  const appStore = useAppStore()
  const { t REDACTED = useI18n()
  const endpointPrefix = '/admin/openai'

  // State
  const authUrl = ref('')
  const sessionId = ref('')
  const oauthState = ref('')
  const loading = ref(false)
  const error = ref('')

  // Reset state
  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    oauthState.value = ''
    loading.value = false
    error.value = ''
  REDACTED

  // Generate auth URL for OpenAI OAuth
  const generateAuthUrl = async (
    proxyId?: number | null,
    redirectUri?: string
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    oauthState.value = ''
    error.value = ''

    try {
      const payload: Record<string, unknown> = {REDACTED
      if (proxyId) {
        payload.proxy_id = proxyId
      REDACTED
      if (redirectUri) {
        payload.redirect_uri = redirectUri
      REDACTED

      const response = await adminAPI.accounts.generateAuthUrl(
        `${endpointPrefixREDACTED/generate-auth-url`,
        payload
      )
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      try {
        const parsed = new URL(response.auth_url)
        oauthState.value = parsed.searchParams.get('state') || ''
      REDACTED catch {
        oauthState.value = ''
      REDACTED
      return true
    REDACTED catch (err: any) {
      error.value = extractApiErrorMessage(err, t('admin.accounts.oauth.openai.failedToGenerateUrl'))
      appStore.showError(error.value)
      return false
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Exchange auth code for tokens
  const exchangeAuthCode = async (
    code: string,
    currentSessionId: string,
    state: string,
    proxyId?: number | null
  ): Promise<OpenAITokenInfo | null> => {
    if (!code.trim() || !currentSessionId || !state.trim()) {
      error.value = 'Missing auth code, session ID, or state'
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      const payload: { session_id: string; code: string; state: string; proxy_id?: number REDACTED = {
        session_id: currentSessionId,
        code: code.trim(),
        state: state.trim()
      REDACTED
      if (proxyId) {
        payload.proxy_id = proxyId
      REDACTED

      const tokenInfo = await adminAPI.accounts.exchangeCode(`${endpointPrefixREDACTED/exchange-code`, payload)
      return tokenInfo as OpenAITokenInfo
    REDACTED catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.openai.errors',
        t('admin.accounts.oauth.openai.failedToExchangeCode')
      )
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Validate refresh token and get full token info
  // clientId: 指定 OAuth client_id（用于第三方渠道获取的 RT，如 app_LlGpXReQgckcGGUo2JrYvtJK）
  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null,
    clientId?: string
  ): Promise<OpenAITokenInfo | null> => {
    if (!refreshToken.trim()) {
      error.value = 'Missing refresh token'
      return null
    REDACTED

    loading.value = true
    error.value = ''

    try {
      // Use dedicated refresh-token endpoint
      const tokenInfo = await adminAPI.accounts.refreshOpenAIToken(
        refreshToken.trim(),
        proxyId,
        `${endpointPrefixREDACTED/refresh-token`,
        clientId
      )
      return tokenInfo as OpenAITokenInfo
    REDACTED catch (err: any) {
      error.value = extractI18nErrorMessage(
        err,
        t,
        'admin.accounts.oauth.openai.errors',
        t('admin.accounts.oauth.openai.failedToValidateRT')
      )
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Build credentials for OpenAI OAuth account (aligned with backend BuildAccountCredentials)
  const buildCredentials = (tokenInfo: OpenAITokenInfo): Record<string, unknown> => {
    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      expires_at: tokenInfo.expires_at
    REDACTED

    // 仅在返回了新的 refresh_token 时才写入，防止用空值覆盖已有令牌
    if (tokenInfo.refresh_token) {
      creds.refresh_token = tokenInfo.refresh_token
    REDACTED
    if (tokenInfo.id_token) {
      creds.id_token = tokenInfo.id_token
    REDACTED
    if (tokenInfo.email) {
      creds.email = tokenInfo.email
    REDACTED
    if (tokenInfo.chatgpt_account_id) {
      creds.chatgpt_account_id = tokenInfo.chatgpt_account_id
    REDACTED
    if (tokenInfo.chatgpt_user_id) {
      creds.chatgpt_user_id = tokenInfo.chatgpt_user_id
    REDACTED
    if (tokenInfo.organization_id) {
      creds.organization_id = tokenInfo.organization_id
    REDACTED
    if (tokenInfo.plan_type) {
      creds.plan_type = tokenInfo.plan_type
    REDACTED
    if (tokenInfo.subscription_expires_at) {
      creds.subscription_expires_at = tokenInfo.subscription_expires_at
    REDACTED
    if (tokenInfo.client_id) {
      creds.client_id = tokenInfo.client_id
    REDACTED

    return creds
  REDACTED

  // Build extra info from token response
  const buildExtraInfo = (tokenInfo: OpenAITokenInfo): Record<string, string> | undefined => {
    const extra: Record<string, string> = {REDACTED
    if (tokenInfo.email) {
      extra.email = tokenInfo.email
    REDACTED
    if (tokenInfo.name) {
      extra.name = tokenInfo.name
    REDACTED
    if (tokenInfo.privacy_mode) {
      extra.privacy_mode = tokenInfo.privacy_mode
    REDACTED
    return Object.keys(extra).length > 0 ? extra : undefined
  REDACTED

  return {
    // State
    authUrl,
    sessionId,
    oauthState,
    loading,
    error,
    // Methods
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    validateRefreshToken,
    buildCredentials,
    buildExtraInfo
  REDACTED
REDACTED

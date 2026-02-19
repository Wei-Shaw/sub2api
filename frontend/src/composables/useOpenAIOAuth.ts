import { ref REDACTED from 'vue'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'

export interface OpenAITokenInfo {
  access_token?: string
  refresh_token?: string
  id_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number
  scope?: string
  email?: string
  name?: string
  // OpenAI specific IDs (extracted from ID Token)
  chatgpt_account_id?: string
  chatgpt_user_id?: string
  organization_id?: string
  [key: string]: unknown
REDACTED

export type OpenAIOAuthPlatform = 'openai' | 'sora'

interface UseOpenAIOAuthOptions {
  platform?: OpenAIOAuthPlatform
REDACTED

export function useOpenAIOAuth(options?: UseOpenAIOAuthOptions) {
  const appStore = useAppStore()
  const oauthPlatform = options?.platform ?? 'openai'
  const endpointPrefix = oauthPlatform === 'sora' ? '/admin/sora' : '/admin/openai'

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
      error.value = err.response?.data?.detail || 'Failed to generate OpenAI auth URL'
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
      error.value = err.response?.data?.detail || 'Failed to exchange OpenAI auth code'
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Validate refresh token and get full token info
  const validateRefreshToken = async (
    refreshToken: string,
    proxyId?: number | null
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
        `${endpointPrefixREDACTED/refresh-token`
      )
      return tokenInfo as OpenAITokenInfo
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to validate refresh token'
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Validate Sora session token and get access token
  const validateSessionToken = async (
    sessionToken: string,
    proxyId?: number | null
  ): Promise<OpenAITokenInfo | null> => {
    if (!sessionToken.trim()) {
      error.value = 'Missing session token'
      return null
    REDACTED
    loading.value = true
    error.value = ''
    try {
      const tokenInfo = await adminAPI.accounts.validateSoraSessionToken(
        sessionToken.trim(),
        proxyId,
        `${endpointPrefixREDACTED/st2at`
      )
      return tokenInfo as OpenAITokenInfo
    REDACTED catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to validate session token'
      appStore.showError(error.value)
      return null
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  // Build credentials for OpenAI OAuth account
  const buildCredentials = (tokenInfo: OpenAITokenInfo): Record<string, unknown> => {
    const creds: Record<string, unknown> = {
      access_token: tokenInfo.access_token,
      refresh_token: tokenInfo.refresh_token,
      token_type: tokenInfo.token_type,
      expires_in: tokenInfo.expires_in,
      expires_at: tokenInfo.expires_at,
      scope: tokenInfo.scope
    REDACTED

    // Include OpenAI specific IDs (required for forwarding)
    if (tokenInfo.chatgpt_account_id) {
      creds.chatgpt_account_id = tokenInfo.chatgpt_account_id
    REDACTED
    if (tokenInfo.chatgpt_user_id) {
      creds.chatgpt_user_id = tokenInfo.chatgpt_user_id
    REDACTED
    if (tokenInfo.organization_id) {
      creds.organization_id = tokenInfo.organization_id
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
    validateSessionToken,
    buildCredentials,
    buildExtraInfo
  REDACTED
REDACTED

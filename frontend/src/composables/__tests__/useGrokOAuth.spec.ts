import { describe, expect, it, vi REDACTED from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  REDACTED)
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const messages: Record<string, string> = {
        'admin.accounts.oauth.grok.failedToExchangeCode': 'Grok 授权码兑换失败',
        'admin.accounts.oauth.grok.errors.GROK_OAUTH_INVALID_STATE':
          'Grok OAuth state 与当前会话不匹配。请粘贴同一次生成的授权链接返回的回调 URL。'
      REDACTED
      return messages[key] ?? key
    REDACTED
  REDACTED)
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshGrokToken: vi.fn()
    REDACTED
  REDACTED
REDACTED))

import { useGrokOAuth REDACTED from '@/composables/useGrokOAuth'
import { adminAPI REDACTED from '@/api/admin'

describe('useGrokOAuth.exchangeAuthCode', () => {
  it('shows a state mismatch recovery hint from structured backend errors', async () => {
    vi.mocked(adminAPI.grok.exchangeCode).mockRejectedValueOnce({
      status: 400,
      reason: 'GROK_OAUTH_INVALID_STATE',
      message: 'invalid oauth state'
    REDACTED)
    const oauth = useGrokOAuth()

    const tokenInfo = await oauth.exchangeAuthCode({
      code: 'code',
      sessionId: 'session-id',
      state: 'wrong-state'
    REDACTED)

    expect(tokenInfo).toBeNull()
    expect(oauth.error.value).toBe(
      'Grok OAuth state 与当前会话不匹配。请粘贴同一次生成的授权链接返回的回调 URL。'
    )
  REDACTED)
REDACTED)

describe('useGrokOAuth.buildCredentials', () => {
  it('persists the Grok CLI subscription proxy for OAuth inference', () => {
    const oauth = useGrokOAuth()

    const credentials = oauth.buildCredentials({
      access_token: 'access-token',
      token_type: 'Bearer',
      expires_at: 1_900_000_000,
      client_id: 'client-id',
      scope: 'openid grok-cli:access',
      email: 'grok@example.com'
    REDACTED)

    expect(credentials.base_url).toBe('https://cli-chat-proxy.grok.com/v1')
  REDACTED)
REDACTED)

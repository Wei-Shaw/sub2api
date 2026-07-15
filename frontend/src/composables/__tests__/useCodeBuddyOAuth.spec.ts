import { describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const messages: Record<string, string> = {
        'admin.accounts.oauth.codebuddy.failedToExchangeState': 'CodeBuddy state 兑换失败',
        'admin.accounts.oauth.codebuddy.errors.CODEBUDDY_OAUTH_INVALID_STATE':
          'CodeBuddy OAuth state 与当前会话不匹配。请使用同一授权链接产生的回调链接。'
      }
      return messages[key] ?? key
    }
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    codebuddy: {
      generateAuthUrl: vi.fn(),
      exchangeState: vi.fn(),
      refreshCodeBuddyToken: vi.fn()
    }
  }
}))

import { useCodeBuddyOAuth } from '@/composables/useCodeBuddyOAuth'
import { adminAPI } from '@/api/admin'

describe('useCodeBuddyOAuth.exchangeState', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a state mismatch recovery hint from structured backend errors', async () => {
    vi.mocked(adminAPI.codebuddy.exchangeState).mockRejectedValueOnce({
      status: 400,
      reason: 'CODEBUDDY_OAUTH_INVALID_STATE',
      message: 'invalid oauth state'
    })
    const oauth = useCodeBuddyOAuth()

    const tokenInfo = await oauth.exchangeState({
      state: 'wrong-state',
      sessionId: 'session-id'
    })

    expect(tokenInfo).toBeNull()
    expect(oauth.error.value).toBe(
      'CodeBuddy OAuth state 与当前会话不匹配。请使用同一授权链接产生的回调链接。'
    )
  })

  it('returns null when state is empty', async () => {
    const oauth = useCodeBuddyOAuth()
    const tokenInfo = await oauth.exchangeState({ state: '   ' })
    expect(tokenInfo).toBeNull()
    expect(adminAPI.codebuddy.exchangeState).not.toHaveBeenCalled()
  })

  it('builds credentials from token info', async () => {
    vi.mocked(adminAPI.codebuddy.exchangeState).mockResolvedValueOnce({
      access_token: 'at',
      refresh_token: 'rt',
      domain: 'copilot.tencent.com',
      scope: 'all',
      enabled_models: ['auto', 'model-a']
    })
    const oauth = useCodeBuddyOAuth()
    const tokenInfo = await oauth.exchangeState({ state: 's', sessionId: 'sid' })
    expect(tokenInfo).not.toBeNull()

    const credentials = oauth.buildCredentials(tokenInfo!)
    expect(credentials.access_token).toBe('at')
    expect(credentials.refresh_token).toBe('rt')
    expect((credentials.models as string[]).sort()).toEqual(['auto', 'model-a'])
  })
})

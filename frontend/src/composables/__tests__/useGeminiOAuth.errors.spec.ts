import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGeminiOAuth } from '../useGeminiOAuth'

const mocks = vi.hoisted(() => ({ generateAuthUrl: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { gemini: { generateAuthUrl: mocks.generateAuthUrl } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => vi.resetAllMocks())

describe('Gemini authorization URL errors', () => {
  it.each([
    [{ status: 400, message: 'Selected proxy is unavailable' }, 'Selected proxy is unavailable'],
    [{ response: { data: { detail: 'OAuth is not configured' } } }, 'OAuth is not configured'],
    [{}, 'admin.accounts.oauth.gemini.failedToGenerateUrl'],
    [null, 'admin.accounts.oauth.gemini.failedToGenerateUrl']
  ])('preserves useful API feedback for %j', async (error, expected) => {
    mocks.generateAuthUrl.mockRejectedValue(error)
    const oauth = useGeminiOAuth()
    expect(await oauth.generateAuthUrl(7)).toBe(false)
    expect(oauth.error.value).toBe(expected)
    expect(mocks.showError).toHaveBeenCalledWith(expected)
    expect(oauth.loading.value).toBe(false)
    expect(oauth.authUrl.value).toBe('')
  })

  it('can retry successfully after an error', async () => {
    mocks.generateAuthUrl.mockRejectedValueOnce({ message: 'Temporary failure' }).mockResolvedValueOnce({
      auth_url: 'https://example.com/auth', session_id: 'session', state: 'state'
    })
    const oauth = useGeminiOAuth()
    await oauth.generateAuthUrl(7)
    expect(await oauth.generateAuthUrl(7, ' project ')).toBe(true)
    expect(oauth.error.value).toBe('')
    expect(oauth.authUrl.value).toBe('https://example.com/auth')
    expect(oauth.sessionId.value).toBe('session')
    expect(oauth.state.value).toBe('state')
    expect(mocks.generateAuthUrl).toHaveBeenLastCalledWith({ proxy_id: 7, project_id: 'project' })
  })
})

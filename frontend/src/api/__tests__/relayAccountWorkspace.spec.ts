import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('../client', () => ({ apiClient: client }))
import { relayAccountWorkspace } from '../relayAccountWorkspace'

beforeEach(() => {
  vi.clearAllMocks()
  client.get.mockResolvedValue({ data: {} })
  client.post.mockResolvedValue({ data: {} })
  client.put.mockResolvedValue({ data: {} })
})

describe('relay account workspace', () => {
  it('maps the existing OAuth flow to user endpoints without a global interceptor', async () => {
    const api = relayAccountWorkspace.api
    await api.accounts.generateAuthUrl('/admin/accounts/generate-auth-url', {})
    await api.accounts.exchangeCode('/admin/accounts/exchange-code', { session_id: 'own-session', code: 'code' })
    await api.accounts.refreshOpenAIToken('refresh-token', 9, '/admin/openai/refresh-token', 'client-id')
    await api.gemini.generateAuthUrl({})
    await api.antigravity.generateAuthUrl({})
    await api.grok.generateAuthUrl({})
    expect(client.post.mock.calls.map(call => call[0])).toEqual([
      '/relay/accounts/generate-auth-url', '/relay/accounts/exchange-code', '/relay/openai/refresh-token',
      '/relay/gemini/oauth/auth-url', '/relay/antigravity/oauth/auth-url', '/relay/grok/oauth/auth-url'
    ])
    expect(client.post.mock.calls[2][1]).toEqual({ refresh_token: 'refresh-token', client_id: 'client-id' })
    expect(() => api.accounts.generateAuthUrl('https://evil.example.com', {})).toThrow()
  })

  it('routes account editing, model synchronization and reauthorization to owned account endpoints', async () => {
    const api = relayAccountWorkspace.api.accounts
    await api.update(7, { credentials: { access_token: 'new' } })
    await api.syncUpstreamModels(7)
    await api.clearError(7)
    await api.getAvailableModels(7)
    await api.getStats(7)
    await api.refreshCredentials(7)
    expect(client.put).toHaveBeenCalledWith('/relay/accounts/7', { credentials: { access_token: 'new' } })
    expect(client.post.mock.calls.map(call => call[0])).toEqual([
      '/relay/accounts/7/models/sync-upstream', '/relay/accounts/7/clear-error', '/relay/accounts/7/refresh'
    ])
    expect(client.get.mock.calls.map(call => call[0])).toEqual(['/relay/accounts/7/models', '/relay/accounts/7/stats'])
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GrokModelDiagnostics } from '@/types'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import { getById, syncUpstreamModels } from '@/api/admin/accounts'

const diagnostics: GrokModelDiagnostics = {
  state: 'fresh',
  endpoint: 'https://cli-chat-proxy.grok.com/v1/models',
  fetched_at: '2026-10-25T11:00:00.000Z',
  expires_at: '2026-10-25T17:00:00.000Z',
  models: ['grok-4.7'],
  last_attempt: {
    started_at: '2026-10-25T11:00:00.000Z',
    finished_at: '2026-10-25T11:00:01.000Z',
    status: 'failed',
    error_code: 'upstream_unavailable',
    scope: {
      account_id: 25,
      account_type: 'oauth',
      endpoint: 'https://cli-chat-proxy.grok.com/v1/models',
      fingerprint: 'abc'
    }
  },
  mappings: [
    { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }
  ]
}

describe('admin account grok model diagnostics API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('keeps optional diagnostics from the existing account detail response', async () => {
    const account = { id: 25, platform: 'grok', grok_model_diagnostics: diagnostics }
    get.mockResolvedValueOnce({ data: account })

    await expect(getById(25)).resolves.toEqual(account)
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/admin/accounts/25')
    expect(get.mock.calls[0]).toHaveLength(1)
  })

  it('preserves discovery source and diagnostics on the existing sync URL', async () => {
    const result = {
      models: ['grok-4.7'],
      discovery_source: 'upstream' as const,
      grok_model_diagnostics: diagnostics,
      warnings: [{ code: 'upstream_model_metadata_incomplete', message: 'incomplete' }]
    }
    post.mockResolvedValueOnce({ data: result })

    await expect(syncUpstreamModels(25)).resolves.toEqual(result)
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith('/admin/accounts/25/models/sync-upstream')
    expect(post.mock.calls[0]).toHaveLength(1)
  })

  it('leaves omitted diagnostics and discovery source absent for older responses', async () => {
    get.mockResolvedValueOnce({ data: { id: 3, platform: 'openai' } })
    post.mockResolvedValueOnce({ data: { models: ['gpt-6'] } })

    const account = await getById(3)
    const synced = await syncUpstreamModels(3)

    expect(account).not.toHaveProperty('grok_model_diagnostics')
    expect(synced).not.toHaveProperty('discovery_source')
    expect(synced).not.toHaveProperty('grok_model_diagnostics')
    expect(post).toHaveBeenLastCalledWith('/admin/accounts/3/models/sync-upstream')
  })
})

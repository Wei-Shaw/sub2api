import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post },
}))

import {
  createFromSSO,
  getGrokSSOImportTimeout,
  getProbeAllStatus,
  startProbeAll
} from '@/api/admin/grok'

describe('admin Grok SSO import API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    post.mockResolvedValue({ data: { created: [], failed: [] } })
  })

  it.each([
    [1, 180_000],
    [3, 180_000],
    [4, 270_000],
    [7, 360_000],
  ])('uses a timeout sized for %i keys', async (keyCount, expectedTimeout) => {
    expect(getGrokSSOImportTimeout(keyCount)).toBe(expectedTimeout)

    await createFromSSO({
      sso_tokens: Array.from({ length: keyCount }, (_, index) => `sso-${index + 1}`),
    })

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/sso-to-oauth',
      expect.objectContaining({ sso_tokens: expect.any(Array) }),
      { timeout: expectedTimeout },
    )
  })
})

describe('admin Grok probe-all API', () => {
  const status = {
    run_id: 'probe-1',
    running: true,
    total: 10,
    completed: 4,
    succeeded: 3,
    failed: 1,
    started_at: '2026-07-22T12:00:00Z',
    finished_at: null,
    status_counts: { '200': 3, '402': 1 }
  }

  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    get.mockResolvedValue({ data: status })
    post.mockResolvedValue({ data: status })
  })

  it('starts the shared low-concurrency probe job', async () => {
    await expect(startProbeAll()).resolves.toEqual(status)
    expect(post).toHaveBeenCalledWith('/admin/grok/accounts/probe-all')
  })

  it('loads the current probe job status', async () => {
    await expect(getProbeAllStatus()).resolves.toEqual(status)
    expect(get).toHaveBeenCalledWith('/admin/grok/accounts/probe-all/status')
  })
})

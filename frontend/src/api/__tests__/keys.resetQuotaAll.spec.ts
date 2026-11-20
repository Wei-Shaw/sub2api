import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetQuotaAll } from '../keys'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

describe('API key bulk quota reset', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('posts to the bulk reset endpoint without touching rate limit usage by default', async () => {
    const payload = {
      matched: 2,
      reset: 2,
      skipped: 1,
      failed: 0,
      reset_key_ids: [1, 2],
      failed_key_ids: []
    }
    post.mockResolvedValue({ data: payload })

    const result = await resetQuotaAll()

    expect(post).toHaveBeenCalledWith('/keys/reset-quota', { include_rate_limit_usage: false })
    expect(result).toEqual(payload)
  })

  it('asks the backend to reset rate limit usage as well when requested', async () => {
    post.mockResolvedValue({
      data: { matched: 1, reset: 1, skipped: 0, failed: 0, reset_key_ids: [5], failed_key_ids: [] }
    })

    await resetQuotaAll(true)

    expect(post).toHaveBeenCalledWith('/keys/reset-quota', { include_rate_limit_usage: true })
  })
})

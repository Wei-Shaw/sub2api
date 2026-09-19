import { beforeEach, describe, expect, it, vi } from 'vitest'
import { create } from '../keys'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

describe('API key creation expiry', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { id: 1 } })
  })

  it('sends the exact timestamp instead of legacy days and preserves other options', async () => {
    await create('test', 2, undefined, ['192.0.2.1'], [], 10, 7,
      { rate_limit_5h: 5 }, '2030-09-19T05:00:00.000Z')

    expect(post).toHaveBeenCalledOnce()
    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'test', group_id: 2, ip_whitelist: ['192.0.2.1'], quota: 10,
      rate_limit_5h: 5, expires_at: '2030-09-19T05:00:00.000Z',
    })
  })

  it('preserves legacy days when no exact expiry is supplied', async () => {
    await create('test', undefined, undefined, undefined, undefined, undefined, 7)
    expect(post).toHaveBeenCalledOnce()
    expect(post).toHaveBeenCalledWith('/keys', { name: 'test', expires_in_days: 7 })
  })

  it.each([undefined, null])('omits expiry when it is %s', async (expiresAt) => {
    await create('test', undefined, undefined, undefined, undefined, undefined,
      undefined, undefined, expiresAt)
    expect(post).toHaveBeenCalledOnce()
    expect(post).toHaveBeenCalledWith('/keys', { name: 'test' })
  })
})

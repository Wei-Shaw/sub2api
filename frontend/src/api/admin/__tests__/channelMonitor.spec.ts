import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiClientPost = vi.fn()

vi.mock('../../client', () => ({
  apiClient: {
    post: (...args: unknown[]) => apiClientPost(...args),
  },
}))

describe('admin channel monitor api', () => {
  beforeEach(() => {
    apiClientPost.mockReset()
  })

  it('uses an extended timeout for manual monitor runs', async () => {
    apiClientPost.mockResolvedValue({ data: { results: [] } })

    const { runNow, CHANNEL_MONITOR_RUN_TIMEOUT_MS } = await import('../channelMonitor')
    await runNow(42)

    expect(apiClientPost).toHaveBeenCalledWith(
      '/admin/channel-monitors/42/run',
      undefined,
      { timeout: CHANNEL_MONITOR_RUN_TIMEOUT_MS }
    )
    expect(CHANNEL_MONITOR_RUN_TIMEOUT_MS).toBe(180000)
  })
})

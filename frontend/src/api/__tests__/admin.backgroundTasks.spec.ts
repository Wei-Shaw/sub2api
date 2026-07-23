import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post }
}))

const request = {
  expected_expires_at: '2030-01-01T02:00:00Z',
  lead_time_minutes: 60 as const,
}

describe('admin background task API', () => {
  beforeEach(() => {
    vi.resetModules()
    sessionStorage.clear()
    post.mockReset()
    post.mockResolvedValue({ data: { task: { id: 7 }, created: true } })
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue('11111111-1111-4111-8111-111111111111')
  })

  it('sends the durable task creation key in the idempotency header', async () => {
    const { createOpenAIQuotaReset } = await import('@/api/admin/backgroundTasks')
    await createOpenAIQuotaReset(42, request)

    expect(post).toHaveBeenCalledWith('/admin/openai/accounts/42/quota-reset-tasks', request, {
      headers: {
        'Idempotency-Key': 'openai-quota-reset-task-42-11111111-1111-4111-8111-111111111111'
      }
    })
    expect(sessionStorage.length).toBe(0)
  })

  it('reuses the same creation key after an ambiguous failure and page reload', async () => {
    post.mockRejectedValueOnce(new Error('response lost'))
    const firstModule = await import('@/api/admin/backgroundTasks')
    await expect(firstModule.createOpenAIQuotaReset(42, request)).rejects.toThrow('response lost')
    const firstHeaders = post.mock.calls[0][2].headers

    vi.resetModules()
    post.mockResolvedValueOnce({ data: { task: { id: 7 }, created: false } })
    const reloadedModule = await import('@/api/admin/backgroundTasks')
    await reloadedModule.createOpenAIQuotaReset(42, request)

    expect(post).toHaveBeenCalledTimes(2)
    expect(post.mock.calls[1][2].headers).toEqual(firstHeaders)
    expect(sessionStorage.length).toBe(0)
  })
})

import { expect, it, vi } from 'vitest'
const { get } = vi.hoisted(() => ({ get: vi.fn().mockResolvedValue({ data: { items: [] } }) }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))
import { listPromptRecords } from '../api'

it('forwards the cancellation signal to the HTTP client', async () => {
  const controller = new AbortController()
  await listPromptRecords({ pagination: 'cursor' }, controller.signal)
  expect(get).toHaveBeenCalledWith('/admin/prompt-records', {
    params: { pagination: 'cursor' }, signal: controller.signal,
  })
})

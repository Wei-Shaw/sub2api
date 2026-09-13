import { expect, it, vi } from 'vitest'
const { get, deleteRequest } = vi.hoisted(() => ({
	get: vi.fn().mockResolvedValue({ data: { items: [] } }),
	deleteRequest: vi.fn().mockResolvedValue({ data: { deleted: 9 } }),
}))
vi.mock('@/api/client', () => ({ apiClient: { get, delete: deleteRequest } }))
import { deleteAllPromptRecords, listPromptRecords } from '../api'

it('forwards the cancellation signal to the HTTP client', async () => {
  const controller = new AbortController()
  await listPromptRecords({ pagination: 'cursor' }, controller.signal)
  expect(get).toHaveBeenCalledWith('/admin/prompt-records', {
    params: { pagination: 'cursor' }, signal: controller.signal,
  })
})

it('deletes the complete prompt record table through the dedicated endpoint', async () => {
	await expect(deleteAllPromptRecords()).resolves.toEqual({ deleted: 9 })
	expect(deleteRequest).toHaveBeenCalledWith('/admin/prompt-records/all')
})

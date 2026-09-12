import { expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))
import { accountsAPI } from '@/api/admin/accounts'

it('reads persisted account window history with the selected range and cancellation signal', async () => {
  const response = { windows: { '5h': [] } }
  get.mockResolvedValue({ data: response })
  const controller = new AbortController()
  await expect(accountsAPI.getWindowHistory(7, 90, controller.signal)).resolves.toEqual(response)
  expect(get).toHaveBeenCalledWith('/admin/accounts/7/window-history', { params: { days: 90 }, signal: controller.signal })
})

import { AxiosHeaders } from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import * as funding from '../byteplusPrepaid'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() } }))
const reply = <T>(data: T) => ({ data, status: 200, statusText: 'OK', headers: {}, config: { headers: new AxiosHeaders() } })
beforeEach(() => vi.resetAllMocks())

describe('prepaid funding API', () => {
  it('keeps decimal balances as strings in the authenticated admin request', async () => {
    const input = { pack_id: 'fixture-pack', model: 'dreamina-seedance-2-5-260628', remaining_tokens: '1234567.015625', expires_at: '2030-01-01T00:00:00Z', confirmed: true }
    vi.mocked(apiClient.post).mockResolvedValue(reply({ pool: null }))
    await funding.update(11, 'grant', input)
    expect(apiClient.post).toHaveBeenCalledWith('/admin/accounts/11/byteplus-prepaid/grant', input)
    expect(typeof input.remaining_tokens).toBe('string')
  })
  it('reconciles a reservation without sending client-supplied usage or terminal status', async () => {
    vi.mocked(apiClient.post).mockResolvedValue(reply({ pool: null }))
    await funding.reconcile(11, 'fixture/reservation')
    expect(apiClient.post).toHaveBeenCalledWith('/admin/accounts/11/byteplus-prepaid/reservations/fixture%2Freservation/reconcile')
  })
  it('loads status and the shared-pool catalog without inferring a provider balance endpoint', async () => {
    vi.mocked(apiClient.get).mockResolvedValue(reply([]))
    await funding.getStatus(11)
    await funding.listPools(11)
    expect(apiClient.get).toHaveBeenNthCalledWith(1, '/admin/accounts/11/byteplus-prepaid')
    expect(apiClient.get).toHaveBeenNthCalledWith(2, '/admin/accounts/11/byteplus-prepaid/pools')
  })
})

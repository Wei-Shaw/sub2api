import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import web3DepositsAPI from '@/api/admin/web3Deposits'

describe('admin Web3 deposits API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('queries deposits with filters', () => {
    const params = { page: 2, page_size: 20, status: 'manual_review' }
    web3DepositsAPI.list(params)
    expect(get).toHaveBeenCalledWith('/admin/web3-deposits', { params })
  })

  it('sends immutable review actions without chain facts', () => {
    web3DepositsAPI.approve(7)
    web3DepositsAPI.ignore(8, 'unsupported source')
    web3DepositsAPI.retry(9)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/web3-deposits/7/approve')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/web3-deposits/8/ignore', { reason: 'unsupported source' })
    expect(post).toHaveBeenNthCalledWith(3, '/admin/web3-deposits/9/retry')
  })

  it('sends bounded block ranges as strings', () => {
    web3DepositsAPI.rescan('conflux_espace', 'usdt0', '100', '120')
    expect(post).toHaveBeenCalledWith('/admin/web3-deposits/rescan', {
      network_key: 'conflux_espace',
      asset_key: 'usdt0',
      from_block: '100',
      to_block: '120',
    })
  })
})

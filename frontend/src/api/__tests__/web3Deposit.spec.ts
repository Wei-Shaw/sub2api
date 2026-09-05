import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import { web3DepositAPI } from '@/api/web3Deposit'

describe('Web3 deposit balance API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('loads authenticated Web3 balances', () => {
    web3DepositAPI.listBalances()
    expect(get).toHaveBeenCalledWith('/payment/web3/balances')
  })

  it('transfers with an explicit idempotency key', () => {
    web3DepositAPI.transferBalance('usdt', '5.00000000', 'request-123')
    expect(post).toHaveBeenCalledWith('/payment/web3/transfers', {
      asset_key: 'usdt',
      amount: '5.00000000',
    }, {
      headers: { 'Idempotency-Key': 'request-123' },
    })
  })
})

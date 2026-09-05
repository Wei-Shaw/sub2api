import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'
import type {
  Web3DepositAddress,
  Web3BalanceTransfer,
  Web3DepositConfig,
  Web3DepositRecord,
  Web3UserBalance,
} from '@/types/web3Deposit'

export const web3DepositAPI = {
  getConfig() {
    return apiClient.get<Web3DepositConfig>('/payment/web3/config')
  },

  getOrCreateAddress() {
    return apiClient.post<Web3DepositAddress>('/payment/web3/address')
  },

  listBalances() {
    return apiClient.get<Web3UserBalance[]>('/payment/web3/balances')
  },

  transferBalance(assetKey: string, amount: string, idempotencyKey: string) {
    return apiClient.post<Web3BalanceTransfer>('/payment/web3/transfers', {
      asset_key: assetKey,
      amount,
    }, {
      headers: { 'Idempotency-Key': idempotencyKey },
    })
  },

  listDeposits(params?: { page?: number; page_size?: number; network_key?: string; asset_key?: string }) {
    return apiClient.get<BasePaginationResponse<Web3DepositRecord>>('/payment/web3/deposits', { params })
  },

  getDeposit(id: number) {
    return apiClient.get<Web3DepositRecord>(`/payment/web3/deposits/${id}`)
  },
}

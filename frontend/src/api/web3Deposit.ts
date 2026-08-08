import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'
import type {
  Web3DepositAddress,
  Web3DepositConfig,
  Web3DepositRecord,
} from '@/types/web3Deposit'

export const web3DepositAPI = {
  getConfig() {
    return apiClient.get<Web3DepositConfig>('/payment/web3/config')
  },

  getAddress() {
    return apiClient.get<Web3DepositAddress>('/payment/web3/address')
  },

  getOrCreateAddress() {
    return apiClient.post<Web3DepositAddress>('/payment/web3/address')
  },

  listDeposits(params?: { page?: number; page_size?: number }) {
    return apiClient.get<BasePaginationResponse<Web3DepositRecord>>('/payment/web3/deposits', { params })
  },

  getDeposit(id: number) {
    return apiClient.get<Web3DepositRecord>(`/payment/web3/deposits/${id}`)
  },
}

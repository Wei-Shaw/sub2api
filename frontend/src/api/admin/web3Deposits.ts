import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'

export interface AdminWeb3Deposit {
  id: number; user_id: number; chain_id: number; token_contract: string; tx_hash: string; log_index: number; block_number: number; from_address: string; to_address: string; token_amount: string; credited_amount?: string; status: string; review_reason?: string; failure_reason?: string; detected_at: string; finalized_at?: string; credited_at?: string
}
export interface Web3DepositRuntime { state: string; leader: boolean; last_error: string; latest_block: string; scanned_block: string; lag_blocks: string }

const web3DepositsAPI = {
  list(params?: Record<string, unknown>) { return apiClient.get<BasePaginationResponse<AdminWeb3Deposit>>('/admin/web3-deposits', { params }) },
  get(id: number) { return apiClient.get<AdminWeb3Deposit>(`/admin/web3-deposits/${id}`) },
  stats() { return apiClient.get<Record<string, number>>('/admin/web3-deposits/stats') },
  runtime() { return apiClient.get<Web3DepositRuntime>('/admin/web3-deposits/runtime') },
  approve(id: number) { return apiClient.post(`/admin/web3-deposits/${id}/approve`) },
  ignore(id: number, reason: string) { return apiClient.post(`/admin/web3-deposits/${id}/ignore`, { reason }) },
  retry(id: number) { return apiClient.post(`/admin/web3-deposits/${id}/retry`) },
  rescan(fromBlock: string, toBlock: string) { return apiClient.post('/admin/web3-deposits/rescan', { from_block: fromBlock, to_block: toBlock }) },
}
export default web3DepositsAPI

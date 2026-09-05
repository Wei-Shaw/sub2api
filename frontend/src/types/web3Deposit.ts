export interface Web3DepositAssetConfig {
  key: string
  balance_asset_key: string
  display_name: string
  contract_address: string
  decimals: number
  minimum_deposit: string
  automatic_credit_limit: string
  fee_rate: string
  credit_finality: string
}

export interface Web3UserBalance {
  asset_key: string
  available_amount: string
  total_deposited: string
  total_transferred: string
  updated_at: string
}

export interface Web3BalanceTransfer {
  id: number
  asset_key: string
  amount: string
  web3_balance_before: string
  web3_balance_after: string
  user_balance_before: string
  user_balance_after: string
  already_done: boolean
}

export interface Web3DepositNetworkConfig {
  key: string
  display_name: string
  chain_id: string
  assets: Web3DepositAssetConfig[]
}

export interface Web3DepositConfig {
  enabled: boolean
  unavailable_reason?: 'feature_disabled' | 'user_entry_disabled' | 'runtime_unhealthy'
  networks: Web3DepositNetworkConfig[]
}

export interface Web3DepositAddress {
  assigned?: boolean
  address?: string
  networks: string[]
}

export type Web3DepositStatus =
  | 'confirming'
  | 'credited'
  | 'below_minimum'
  | 'under_review'
  | 'failed'

export interface Web3DepositRecord {
  id: number
  chain_id: string
  token_contract: string
  tx_hash: string
  log_index: string
  block_number: string
  from_address: string
  to_address: string
  token_amount: string
  credited_amount?: string
  status: Web3DepositStatus
  detected_at: string
  finalized_at?: string
  credited_at?: string
}

export interface Web3DepositAssetConfig {
  key: string
  display_name: string
  contract_address: string
  decimals: number
  minimum_deposit: string
  automatic_credit_limit: string
}

export interface Web3DepositNetworkConfig {
  key: string
  display_name: string
  chain_id: string
  assets: Web3DepositAssetConfig[]
}

export interface Web3DepositConfig {
  enabled: boolean
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

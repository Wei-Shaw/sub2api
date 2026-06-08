export interface SimpleAccount {
  id: number
  name: string
}

export interface ModelRoutingRule {
  pattern: string
  accounts: SimpleAccount[]
}

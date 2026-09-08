interface EthereumRequestArguments {
  method: string
  params?: unknown[] | Record<string, unknown>
}

interface EthereumProvider {
  request<T = unknown>(args: EthereumRequestArguments): Promise<T>
  on?(event: 'accountsChanged' | 'chainChanged', listener: (...args: unknown[]) => void): void
  removeListener?(event: 'accountsChanged' | 'chainChanged', listener: (...args: unknown[]) => void): void
}

interface Window {
  ethereum?: EthereumProvider
}

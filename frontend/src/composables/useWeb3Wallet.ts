import { computed, onBeforeUnmount, ref } from 'vue'

function normalizeChainID(chainID: string): string {
  try {
    const value = BigInt(chainID)
    if (value <= 0n) {
      throw new Error('invalid chain ID')
    }
    return value.toString(10)
  } catch {
    throw new Error('WEB3_CHAIN_ID_INVALID')
  }
}

function utf8ToHex(value: string): string {
  const bytes = new TextEncoder().encode(value)
  return `0x${Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')}`
}

export function useWeb3Wallet(onContextChanged?: () => void) {
  const address = ref('')
  const chainID = ref('')
  const connecting = ref(false)
  const signing = ref(false)
  let listenersAttached = false

  const provider = computed(() => (typeof window === 'undefined' ? undefined : window.ethereum))
  const available = computed(() => Boolean(provider.value))
  const connected = computed(() => Boolean(address.value && chainID.value))

  const handleAccountsChanged = (...args: unknown[]) => {
    const accounts = Array.isArray(args[0]) ? args[0] : []
    address.value = typeof accounts[0] === 'string' ? accounts[0] : ''
    onContextChanged?.()
  }

  const handleChainChanged = (...args: unknown[]) => {
    const nextChainID = args[0]
    chainID.value = typeof nextChainID === 'string' ? normalizeChainID(nextChainID) : ''
    onContextChanged?.()
  }

  function attachListeners(): void {
    if (listenersAttached || !provider.value?.on) {
      return
    }
    provider.value.on('accountsChanged', handleAccountsChanged)
    provider.value.on('chainChanged', handleChainChanged)
    listenersAttached = true
  }

  async function connect(): Promise<{ address: string; chainID: string }> {
    if (!provider.value) {
      throw new Error('WEB3_WALLET_NOT_FOUND')
    }
    connecting.value = true
    try {
      const accounts = await provider.value.request<string[]>({ method: 'eth_requestAccounts' })
      const activeAddress = accounts[0]
      if (!activeAddress) {
        throw new Error('WEB3_WALLET_ACCOUNT_MISSING')
      }
      const rawChainID = await provider.value.request<string>({ method: 'eth_chainId' })
      address.value = activeAddress
      chainID.value = normalizeChainID(rawChainID)
      attachListeners()
      return { address: address.value, chainID: chainID.value }
    } finally {
      connecting.value = false
    }
  }

  async function signMessage(message: string): Promise<string> {
    if (!provider.value || !address.value) {
      throw new Error('WEB3_WALLET_NOT_CONNECTED')
    }
    signing.value = true
    try {
      return await provider.value.request<string>({
        method: 'personal_sign',
        params: [utf8ToHex(message), address.value],
      })
    } finally {
      signing.value = false
    }
  }

  onBeforeUnmount(() => {
    if (!listenersAttached || !provider.value?.removeListener) {
      return
    }
    provider.value.removeListener('accountsChanged', handleAccountsChanged)
    provider.value.removeListener('chainChanged', handleChainChanged)
  })

  return {
    address,
    chainID,
    available,
    connected,
    connecting,
    signing,
    connect,
    signMessage,
  }
}

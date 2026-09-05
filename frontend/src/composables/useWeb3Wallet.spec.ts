import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { useWeb3Wallet } from './useWeb3Wallet'

describe('useWeb3Wallet', () => {
  afterEach(() => {
    delete window.ethereum
  })

  it('connects, normalizes the chain ID, and signs UTF-8 messages', async () => {
    const request = vi.fn(async ({ method, params }: EthereumRequestArguments) => {
      if (method === 'eth_requestAccounts') return ['0x52908400098527886E0F7030069857D2E4169EE7']
      if (method === 'eth_chainId') return '0x2105'
      if (method === 'personal_sign') {
        expect(params).toEqual(['0x6869', '0x52908400098527886E0F7030069857D2E4169EE7'])
        return '0xsigned'
      }
      throw new Error(`unexpected method: ${method}`)
    })
    window.ethereum = { request }

    const Component = defineComponent({
      setup(_, { expose }) {
        const wallet = useWeb3Wallet()
        expose(wallet)
        return () => null
      },
    })
    const wrapper = mount(Component)
    const wallet = wrapper.vm as unknown as {
      address: string
      chainID: string
      connect: () => Promise<unknown>
      signMessage: (message: string) => Promise<string>
    }

    await wallet.connect()
    expect(wallet.address).toBe('0x52908400098527886E0F7030069857D2E4169EE7')
    expect(wallet.chainID).toBe('8453')
    await expect(wallet.signMessage('hi')).resolves.toBe('0xsigned')
  })

  it('reports an unavailable injected wallet', async () => {
    const Component = defineComponent({
      setup(_, { expose }) {
        const wallet = useWeb3Wallet()
        expose(wallet)
        return () => null
      },
    })
    const wrapper = mount(Component)
    const wallet = wrapper.vm as unknown as {
      available: boolean
      connect: () => Promise<unknown>
    }

    expect(wallet.available).toBe(false)
    await expect(wallet.connect()).rejects.toThrow('WEB3_WALLET_NOT_FOUND')
  })
})

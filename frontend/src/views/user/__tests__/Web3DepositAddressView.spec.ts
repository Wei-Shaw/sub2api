import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

import Web3DepositAddressView from '../Web3DepositAddressView.vue'

const { getConfig, getOrCreateAddress, listBalances, transferBalance, showError, showSuccess, refreshUser, toDataURL, push, replace, routeQuery } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  getOrCreateAddress: vi.fn(),
  listBalances: vi.fn(),
  transferBalance: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  refreshUser: vi.fn(),
  toDataURL: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  routeQuery: {} as Record<string, string>,
}))

vi.mock('@/api/web3Deposit', () => ({
  web3DepositAPI: { getConfig, getOrCreateAddress, listBalances, transferBalance },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query: routeQuery }),
    useRouter: () => ({ push, replace }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('qrcode', () => ({
  default: { toDataURL },
}))

const enabledConfig = {
  enabled: true,
  networks: [{
    key: 'conflux_espace_mainnet',
    display_name: 'Conflux eSpace Mainnet',
    chain_id: '1030',
    assets: [{
      key: 'usdt0',
      balance_asset_key: 'usdt',
      display_name: 'USDT0',
      contract_address: '0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff',
      decimals: 6,
      minimum_deposit: '1.000000',
      automatic_credit_limit: '10000.000000',
      fee_rate: '0',
      credit_finality: 'finalized',
    }],
  }, {
    key: 'conflux_espace_testnet',
    display_name: 'Conflux eSpace Testnet',
    chain_id: '71',
    assets: [{
      key: 'usdt0',
      balance_asset_key: 'usdt',
      display_name: 'USDT0',
      contract_address: '0x2222222222222222222222222222222222222222',
      decimals: 6,
      minimum_deposit: '1.000000',
      automatic_credit_limit: '10000.000000',
      fee_rate: '0',
      credit_finality: 'finalized',
    }],
  }],
}

function mountView() {
  return shallowMount(Web3DepositAddressView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
      },
    },
  })
}

describe('Web3DepositAddressView', () => {
  beforeEach(() => {
    getConfig.mockReset()
    getOrCreateAddress.mockReset()
    listBalances.mockReset().mockResolvedValue({ data: [] })
    transferBalance.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    refreshUser.mockReset()
    push.mockReset()
    replace.mockReset()
    delete routeQuery.network
    delete routeQuery.asset
    toDataURL.mockReset().mockResolvedValue('data:image/png;base64,qr')
  })

  it('automatically gets or creates the deposit address when the feature is enabled', async () => {
    getConfig.mockResolvedValue({ data: enabledConfig })
    getOrCreateAddress.mockResolvedValue({
      data: {
        assigned: true,
        address: '0x1111111111111111111111111111111111111111',
        networks: ['conflux_espace_mainnet'],
      },
    })

    const wrapper = mountView()
    await flushPromises()

    expect(getOrCreateAddress).toHaveBeenCalledOnce()
    expect(toDataURL).toHaveBeenCalledWith('0x1111111111111111111111111111111111111111', {
      width: 320,
      margin: 2,
    })
    expect(wrapper.text()).toContain('0x1111111111111111111111111111111111111111')
  })

  it('does not allocate an address when the feature is unavailable', async () => {
    getConfig.mockResolvedValue({
      data: { enabled: false, unavailable_reason: 'feature_disabled', networks: [] },
    })

    mountView()
    await flushPromises()

    expect(getOrCreateAddress).not.toHaveBeenCalled()
  })

  it('shows the aggregated Web3 balance independently of the selected network', async () => {
    getConfig.mockResolvedValue({ data: enabledConfig })
    getOrCreateAddress.mockResolvedValue({ data: { assigned: true, address: '0x1111111111111111111111111111111111111111', networks: enabledConfig.networks.map(item => item.key) } })
    listBalances.mockResolvedValue({ data: [{
      asset_key: 'usdt',
      available_amount: '12.34000000',
      total_deposited: '15.00000000',
      total_transferred: '2.66000000',
      updated_at: '2026-08-10T00:00:00Z',
    }] })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('12.3400 USDT')
    expect(wrapper.text()).not.toContain('12.34000000 USDT')
    await wrapper.find('select').setValue('conflux_espace_testnet')
    expect(wrapper.text()).toContain('12.3400 USDT')
  })

  it('shows the contract configured for the selected network', async () => {
    getConfig.mockResolvedValue({ data: enabledConfig })
    getOrCreateAddress.mockResolvedValue({
      data: { assigned: true, address: '0x1111111111111111111111111111111111111111', networks: enabledConfig.networks.map(item => item.key) },
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff')
    await wrapper.find('select').setValue('conflux_espace_testnet')

    expect(wrapper.text()).toContain('0x2222222222222222222222222222222222222222')
    expect(wrapper.text()).toContain('web3Deposit.chainId')
    expect(replace).toHaveBeenLastCalledWith({ query: { network: 'conflux_espace_testnet', asset: 'usdt0' } })
  })

  it('reuses the transfer idempotency key after an uncertain response and resets it when the request changes', async () => {
    getConfig.mockResolvedValue({ data: enabledConfig })
    getOrCreateAddress.mockResolvedValue({
      data: { assigned: true, address: '0x1111111111111111111111111111111111111111', networks: enabledConfig.networks.map(item => item.key) },
    })
    listBalances.mockResolvedValue({ data: [{
      asset_key: 'usdt',
      available_amount: '12.00000000',
      total_deposited: '12.00000000',
      total_transferred: '0.00000000',
      updated_at: '2026-08-10T00:00:00Z',
    }] })
    transferBalance.mockRejectedValueOnce(new Error('response lost')).mockRejectedValueOnce(new Error('response lost again')).mockResolvedValue({ data: {} })
    const wrapper = mountView()
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      transferAmount: string
      submitTransfer: () => Promise<void>
    }

    vm.transferAmount = '5'
    await wrapper.vm.$nextTick()
    await vm.submitTransfer()
    await vm.submitTransfer()

    const firstKey = transferBalance.mock.calls[0][2]
    expect(firstKey).toEqual(expect.any(String))
    expect(transferBalance.mock.calls[1][2]).toBe(firstKey)

    vm.transferAmount = '6'
    await wrapper.vm.$nextTick()
    await vm.submitTransfer()

    expect(transferBalance.mock.calls[2][2]).not.toBe(firstKey)
  })
})

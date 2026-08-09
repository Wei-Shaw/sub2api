import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

import Web3DepositAddressView from '../Web3DepositAddressView.vue'

const { getConfig, getOrCreateAddress, showError, toDataURL } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  getOrCreateAddress: vi.fn(),
  showError: vi.fn(),
  toDataURL: vi.fn(),
}))

vi.mock('@/api/web3Deposit', () => ({
  web3DepositAPI: { getConfig, getOrCreateAddress },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return { ...actual, useRouter: () => ({ push: vi.fn() }) }
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
      display_name: 'USDT0',
      contract_address: '0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff',
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
        Icon: true,
      },
    },
  })
}

describe('Web3DepositAddressView', () => {
  beforeEach(() => {
    getConfig.mockReset()
    getOrCreateAddress.mockReset()
    showError.mockReset()
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
})

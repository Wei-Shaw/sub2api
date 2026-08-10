import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

import Web3DepositHistoryView from '../Web3DepositHistoryView.vue'

const { getConfig, listDeposits, getDeposit, showError, routeQuery, replace } = vi.hoisted(() => ({
  getConfig: vi.fn(),
  listDeposits: vi.fn(),
  getDeposit: vi.fn(),
  showError: vi.fn(),
  routeQuery: {} as Record<string, string>,
  replace: vi.fn(),
}))

vi.mock('@/api/web3Deposit', () => ({
  web3DepositAPI: { getConfig, listDeposits, getDeposit },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query: routeQuery }),
    useRouter: () => ({ push: vi.fn(), replace }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const config = {
  enabled: true,
  networks: [
    {
      key: 'mainnet', display_name: 'Mainnet', chain_id: '1030',
      assets: [{ key: 'usdt0', balance_asset_key: 'usdt', display_name: 'USDT0', contract_address: '0x1111111111111111111111111111111111111111', decimals: 6, minimum_deposit: '1.000000', automatic_credit_limit: '10000.000000', fee_rate: '0', credit_finality: 'finalized' }],
    },
    {
      key: 'testnet', display_name: 'Testnet', chain_id: '71',
      assets: [{ key: 'usdt0', balance_asset_key: 'usdt', display_name: 'USDT0', contract_address: '0x2222222222222222222222222222222222222222', decimals: 6, minimum_deposit: '1.000000', automatic_credit_limit: '10000.000000', fee_rate: '0', credit_finality: 'finalized' }],
    },
  ],
}

describe('Web3DepositHistoryView', () => {
  beforeEach(() => {
    getConfig.mockReset().mockResolvedValue({ data: config })
    listDeposits.mockReset().mockResolvedValue({ data: { items: [], total: 0 } })
    getDeposit.mockReset()
    showError.mockReset()
    replace.mockReset()
    delete routeQuery.network
    delete routeQuery.asset
  })

  it('loads and switches history using the selected network and asset', async () => {
    routeQuery.network = 'mainnet'
    routeQuery.asset = 'usdt0'
    const wrapper = shallowMount(Web3DepositHistoryView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: true,
          Pagination: true,
        },
      },
    })
    await flushPromises()

    expect(listDeposits).toHaveBeenLastCalledWith(expect.objectContaining({ network_key: 'mainnet', asset_key: 'usdt0' }))

    await wrapper.find('select').setValue('testnet')
    await flushPromises()

    expect(listDeposits).toHaveBeenLastCalledWith(expect.objectContaining({ network_key: 'testnet', asset_key: 'usdt0' }))
    expect(replace).toHaveBeenLastCalledWith({ query: { network: 'testnet', asset: 'usdt0' } })
  })
})

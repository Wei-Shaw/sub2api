import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import AccountStatsModal from '../AccountStatsModal.vue'

const { getStats } = vi.hoisted(() => ({
  getStats: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getStats
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'admin.accounts.usageStatistics': '使用统计',
    'admin.accounts.last30DaysUsage': '近30天使用统计',
    'usage.accountMultiplier': '账号倍率'
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] || key
    })
  }
})

function mountModal(rateMultiplier?: number) {
  return mount(AccountStatsModal, {
    props: {
      show: false,
      account: {
        id: 7,
        name: '测试账号',
        status: 'active',
        rate_multiplier: rateMultiplier
      }
    } as any,
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>'
        },
        LoadingSpinner: true,
        ModelDistributionChart: true,
        EndpointDistributionChart: true,
        Icon: true,
        Line: true
      }
    }
  })
}

describe('AccountStatsModal', () => {
  beforeEach(() => {
    getStats.mockReset()
    getStats.mockReturnValue(new Promise(() => {}))
  })

  it('在加载统计时也会在头部显示当前账号倍率', async () => {
    const wrapper = mountModal(1.5)

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getStats).toHaveBeenCalledWith(7, 30)
    expect(wrapper.text()).toContain('账号倍率')
    expect(wrapper.text()).toContain('1.50x')
  })

  it('倍率缺失时回退显示 1.00x', async () => {
    const wrapper = mountModal(undefined)

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('1.00x')
  })
})

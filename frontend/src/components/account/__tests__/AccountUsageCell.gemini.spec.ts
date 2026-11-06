import { getUsage, makeAccount } from './accountUsageCellTestFixtures'
import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'

describe('AccountUsageCell Gemini quota', () => {
  it('Vertex 账号会在 Gemini 用量窗口里展示 today stats 徽章', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 4001,
		      platform: 'gemini',
		      type: 'service_account',
          credentials: {
            tier_id: 'vertex',
            project_id: 'vertex-proj',
            client_email: 'svc@vertex-proj.iam.gserviceaccount.com',
            location: 'global'
          },
		      extra: {}
		    }),
		    todayStats: {
		      requests: 0,
		      tokens: 0,
		      cost: 0,
		      standard_cost: 0,
		      user_cost: 0
		    }
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.text()).toContain('0 req')
		expect(wrapper.text()).toContain('0')
		expect(wrapper.text()).toContain('A $0.00')
		expect(wrapper.text()).toContain('U $0.00')
		expect(wrapper.text()).toContain('vertex')
		expect(wrapper.text()).not.toContain('admin.accounts.gemini.quotaPolicy.title')
		expect(wrapper.text()).not.toContain('admin.accounts.gemini.rateLimit.unlimited')
		expect(getUsage).not.toHaveBeenCalled()
  })

  it('Vertex 账号不设上限时不显示配额进度条', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 4011,
          platform: 'gemini',
          type: 'service_account',
          credentials: { tier_id: 'vertex', project_id: 'vertex-proj', location: 'global' },
          extra: {}
        }),
        todayStats: {
          requests: 3,
          tokens: 1200,
          cost: 0.42,
          standard_cost: 0.42,
          user_cost: 0.5
        }
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('A $0.42')
    expect(wrapper.findAll('usage-progress-bar-stub')).toHaveLength(0)
    expect(getUsage).not.toHaveBeenCalled()
  })

  it('Vertex 账号设置了配额上限时显示配额进度条', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 4012,
          platform: 'gemini',
          type: 'service_account',
          credentials: { tier_id: 'vertex', project_id: 'vertex-proj', location: 'global' },
          extra: { quota_daily_start: new Date().toISOString() },
          quota_used: 123.456,
          quota_limit: 500,
          quota_daily_limit: 50,
          quota_daily_used: 12.5
        }),
        todayStats: null,
        todayStatsLoading: false
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    const bars = wrapper.findAll('usage-progress-bar-stub')
    expect(bars.map((bar) => bar.attributes('label'))).toEqual(['1d', 'total'])
    expect(getUsage).not.toHaveBeenCalled()
  })

  it('Gemini 第三方上游 API Key 按普通 Key 账号展示今日统计与额度，不显示 Gemini 徽章', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 4002,
          platform: 'gemini',
          type: 'apikey',
          credentials: {
            base_url: 'https://relay.example.com/gemini',
            api_key: 'sk-relay',
            tier_id: 'aistudio_free'
          },
          extra: {},
          quota_daily_limit: 10,
          quota_daily_used: 2.5
        }),
        todayStats: {
          requests: 123,
          tokens: 2300000,
          cost: 1.79,
          standard_cost: 1.79,
          user_cost: 0
        }
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).not.toContain('api key')
    expect(text).not.toContain('ai studio')
    expect(text).not.toContain('free')
    expect(text).not.toContain('relay.example.com')
    expect(wrapper.find('account-quota-info-stub').exists()).toBe(false)
    expect(text).toContain('123 req')
    expect(text).toContain('A $1.79')
    expect(text).toContain('U $0.00')
    const bars = wrapper.findAll('usage-progress-bar-stub')
    expect(bars.map((bar) => bar.attributes('label'))).toEqual(['1d'])
    expect(text).not.toContain('admin.accounts.gemini.quotaPolicy.title')
    expect(text).not.toContain('admin.accounts.gemini.quotaPolicy.simulatedNote')
    expect(text).not.toContain('admin.accounts.gemini.rateLimit.unlimited')
    expect(getUsage).not.toHaveBeenCalled()
  })

  it('Gemini 官方 API Key 保留 AI Studio 等级与模拟配额', async () => {
    getUsage.mockResolvedValue({
      gemini_pro_daily: {
        utilization: 20,
        resets_at: '2026-10-11T07:00:00Z',
        remaining_seconds: 3600
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 4003,
          platform: 'gemini',
          type: 'apikey',
          credentials: {
            base_url: 'https://generativelanguage.googleapis.com',
            api_key: 'AIza-official',
            tier_id: 'aistudio_paid'
          },
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('ai studio paid')
    expect(text).toContain('admin.accounts.gemini.quotaPolicy.title')
    expect(text).toContain('admin.accounts.gemini.quotaPolicy.simulatedNote')
    expect(getUsage).toHaveBeenCalledWith(4003)
  })

})

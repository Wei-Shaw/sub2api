import { getUsage, makeAccount, makeOllamaUsage, makeOpenCodeGoUsage, cnUsageCellStubs } from './accountUsageCellTestFixtures'
import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account } from '@/types'

describe('AccountUsageCell provider and key cells', () => {
  it('renders eligible Ollama Cloud state and forwards query updates', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9001,
          platform: 'openai',
          type: 'apikey',
          ollama_cloud_usage: {
            account_id: 9001,
            eligible: true,
            configured: true,
            auto_refresh_enabled: true,
            encryption_key_configured: true,
            snapshot: {
              status: 'ok',
              last_attempt_at: '2026-07-23T00:00:00Z',
              next_refresh_at: '2026-07-23T01:00:00Z',
              data: {
                five_hour: { used_percent: 12 },
                seven_day: { used_percent: 34 }
              }
            }
          }
        })
      },
      global: {
        stubs: {
          OllamaCloudUsageCell: {
            props: ['account'],
            emits: ['updated'],
            template: '<button data-test="embedded-ollama" @click="$emit(\'updated\', { ...account.ollama_cloud_usage, auto_refresh_enabled: false })">{{ account.ollama_cloud_usage.snapshot.data.five_hour.used_percent }}</button>'
          },
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    expect(wrapper.get('[data-test="embedded-ollama"]').text()).toBe('12')
    expect(getUsage).not.toHaveBeenCalled()

    await wrapper.get('[data-test="embedded-ollama"]').trigger('click')

    const updatedAccount = wrapper.emitted<Account[]>('account-updated')?.[0]?.[0]
    expect(updatedAccount?.id).toBe(9001)
    expect(updatedAccount?.ollama_cloud_usage?.auto_refresh_enabled).toBe(false)
  })

  it.each(['kimi', 'zhipu', 'deepseek', 'minimax'] as const)(
    '%s apikey 账号 Ollama Cloud eligible 时渲染 Ollama 用量单元格并跳过 CN 子单元格',
    async (platform) => {
      const wrapper = mount(AccountUsageCell, {
        props: {
          account: makeAccount({
            id: 9002,
            platform,
            type: 'apikey',
            credentials: { account_mode: 'coding' },
            ollama_cloud_usage: makeOllamaUsage(9002)
          })
        },
        global: {
          stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
        }
      })

      await flushPromises()

      expect(wrapper.find('[data-test="embedded-ollama"]').exists()).toBe(true)
      expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(false)
      expect(wrapper.find('[data-test="cn-balance-cell"]').exists()).toBe(false)
      expect(wrapper.find('div[title="admin.accounts.cnProviders.noBalanceEndpoint"]').exists()).toBe(false)
      expect(getUsage).not.toHaveBeenCalled()
    }
  )

  it('CN 平台 Ollama Cloud eligible 账号的用量更新经 account-updated 透传', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9003,
          platform: 'kimi',
          type: 'apikey',
          credentials: { account_mode: 'coding' },
          ollama_cloud_usage: makeOllamaUsage(9003)
        })
      },
      global: {
        stubs: {
          ...cnUsageCellStubs,
          OllamaCloudUsageCell: {
            props: ['account'],
            emits: ['updated'],
            template: '<button data-test="embedded-ollama" @click="$emit(\'updated\', { ...account.ollama_cloud_usage, auto_refresh_enabled: false })" />'
          },
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await wrapper.get('[data-test="embedded-ollama"]').trigger('click')

    const updatedAccount = wrapper.emitted<Account[]>('account-updated')?.[0]?.[0]
    expect(updatedAccount?.id).toBe(9003)
    expect(updatedAccount?.ollama_cloud_usage?.auto_refresh_enabled).toBe(false)
  })

  it.each([
    { name: '无 ollama_cloud_usage', usage: undefined },
    { name: 'eligible=false', usage: makeOllamaUsage(9004, { eligible: false }) }
  ])('普通 kimi apikey 账号（$name）仍渲染 CN 子单元格', async ({ usage }) => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9004,
          platform: 'kimi',
          type: 'apikey',
          credentials: { account_mode: 'coding' },
          ollama_cloud_usage: usage
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="cn-balance-cell"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="embedded-ollama"]').exists()).toBe(false)
  })

  it.each(['opencode_go', 'kimi', 'zhipu', 'deepseek', 'minimax'] as const)(
    '%s 平台 OpenCode Go eligible 时只渲染 OpenCode 用量单元格并跳过 CN 子单元格',
    async (platform) => {
      const wrapper = mount(AccountUsageCell, {
        props: {
          account: makeAccount({
            id: 9100,
            platform,
            type: 'apikey',
            credentials: { account_mode: platform === 'opencode_go' ? 'go' : 'coding' },
            opencode_go_usage: makeOpenCodeGoUsage(9100)
          })
        },
        global: {
          stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
        }
      })

      await flushPromises()

      // 同一账号只渲染一次 OpenCode 用量单元格
      expect(wrapper.findAll('[data-test="opencode-go-cell"]')).toHaveLength(1)
      expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(false)
      expect(wrapper.find('[data-test="cn-balance-cell"]').exists()).toBe(false)
      expect(wrapper.find('[data-test="embedded-ollama"]').exists()).toBe(false)
      expect(wrapper.find('div[title="admin.accounts.cnProviders.noBalanceEndpoint"]').exists()).toBe(false)
      expect(getUsage).not.toHaveBeenCalled()
    }
  )

  it.each([
    { name: 'opencode_go（go 模式）', platform: 'opencode_go' as const, mode: 'go' },
    { name: 'kimi（coding 模式）', platform: 'kimi' as const, mode: 'coding' }
  ])('OpenCode Go 不合格时（$name）仍渲染 CN 子单元格', async ({ platform, mode }) => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9101,
          platform,
          type: 'apikey',
          credentials: { account_mode: mode },
          opencode_go_usage: makeOpenCodeGoUsage(9101, { eligible: false })
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-test="opencode-go-cell"]')).toHaveLength(0)
  })

  it('openai apikey 挂载 OpenCode Go 时在非 CN 分支渲染一次且不渲染占位符', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9102,
          platform: 'openai',
          type: 'apikey',
          opencode_go_usage: makeOpenCodeGoUsage(9102)
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.findAll('[data-test="opencode-go-cell"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(false)
    // 用量单元格已渲染时不再叠加 `-` 占位符
    expect(wrapper.text()).not.toContain('-')
  })

  it('Command Code 账号渲染额度与积分余额单元格', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9103,
          platform: 'command_code',
          type: 'apikey',
          credentials: { api_key: 'user_test_key', account_mode: 'payg' }
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="cn-balance-cell"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('-')
  })

  it('自定义中转的 Command Code 账号没有可查的用量接口，显示占位符', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9105,
          platform: 'command_code',
          type: 'apikey',
          credentials: { api_key: 'user_test_key', account_mode: 'payg', base_url: 'https://relay.example.com/v1' }
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.find('div[title="admin.accounts.cnProviders.noBalanceEndpoint"]').exists()).toBe(true)
  })

  it('Cline 账号渲染 ClinePass 窗口与积分余额单元格', async () => {
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 9104,
          platform: 'cline',
          type: 'apikey',
          credentials: { api_key: 'sk-cline' }
        })
      },
      global: {
        stubs: { ...cnUsageCellStubs, UsageProgressBar: true, AccountQuotaInfo: true }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="cn-quota-cell"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="cn-balance-cell"]').exists()).toBe(true)
    // 子单元格可见时不显示 `-` 占位符（按量账号与订阅账号用同一套单元格）。
    expect(wrapper.text()).not.toContain('-')
  })

  it('Key 账号会展示 today stats 徽章并带 A/U 提示', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3001,
		      platform: 'anthropic',
		      type: 'apikey'
		    }),
		    todayStats: {
		      requests: 1_000_000,
		      tokens: 1_000_000_000,
		      cost: 12.345,
		      standard_cost: 12.345,
		      user_cost: 6.789
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

		expect(wrapper.text()).toContain('1.0M req')
		expect(wrapper.text()).toContain('1.0B')
		expect(wrapper.text()).toContain('A $12.35')
		expect(wrapper.text()).toContain('U $6.79')

		const badges = wrapper.findAll('span[title]')
		expect(badges.some(node => node.attributes('title') === 'usage.accountBilled')).toBe(true)
		expect(badges.some(node => node.attributes('title') === 'usage.userBilled')).toBe(true)
  })

  it('Key 账号在 today stats loading 时显示骨架屏', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3002,
		      platform: 'anthropic',
		      type: 'apikey'
		    }),
		    todayStats: null,
		    todayStatsLoading: true
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.findAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('Key 账号在无 today stats 且无配额时显示兜底短横线', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3003,
		      platform: 'anthropic',
		      type: 'apikey',
		      quota_limit: 0,
		      quota_daily_limit: 0,
		      quota_weekly_limit: 0
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

		expect(wrapper.text().trim()).toBe('-')
  })

})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminGroup } from '@/types'

const { getQuotaSummaryMock } = vi.hoisted(() => ({
  getQuotaSummaryMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      getQuotaSummary: getQuotaSummaryMock
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key === 'admin.groups.quotaSummaryTitle' && params?.name) {
          return `admin.groups.quotaSummaryTitle:${String(params.name)}`
        }
        return key
      }
    })
  }
})

import GroupQuotaSummaryModal from '../GroupQuotaSummaryModal.vue'

const openAIGroup: AdminGroup = {
  id: 1,
  name: 'OpenAI Group',
  description: '',
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  simulate_claude_max_enabled: false,
  sort_order: 0
}

const anthropicGroup: AdminGroup = {
  ...openAIGroup,
  id: 2,
  name: 'Anthropic Group',
  platform: 'anthropic'
}

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show"><h1>{{ title }}</h1><slot /><slot name="footer" /></div>'
}

function mountModal(group: AdminGroup | null) {
  return mount(GroupQuotaSummaryModal, {
    props: {
      show: true,
      group
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        PlatformIcon: true
      }
    }
  })
}

describe('GroupQuotaSummaryModal', () => {
  beforeEach(() => {
    getQuotaSummaryMock.mockReset()
    getQuotaSummaryMock.mockResolvedValue({
      group_id: 1,
      platform: 'openai',
      supported: true,
      tabs: [
        {
          window: '5h',
          bucket_counts: [
            { used_percent: 100, account_count: 1 },
            { used_percent: 90, account_count: 2 }
          ],
          matched_account_count: 3,
          skipped_account_count: 1
        },
        {
          window: '7d',
          bucket_counts: [
            { used_percent: 70, account_count: 1 }
          ],
          matched_account_count: 1,
          skipped_account_count: 0
        }
      ]
    })
  })

  it('默认展示 5H tab 数据并请求分组额度摘要', async () => {
    const wrapper = mountModal(openAIGroup)
    await flushPromises()

    expect(getQuotaSummaryMock).toHaveBeenCalledTimes(1)
    expect(getQuotaSummaryMock).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('admin.groups.quotaSummaryTitle:OpenAI Group')
    expect(wrapper.text()).toContain('admin.groups.quotaSummaryWindows.fiveHour')
    expect(wrapper.get('[data-testid="quota-bucket-row-100"]').text()).toContain('100%')
    expect(wrapper.get('[data-testid="quota-bucket-row-100"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="quota-bucket-row-90"]').text()).toContain('90%')
    expect(wrapper.get('[data-testid="quota-bucket-row-90"]').text()).toContain('2')
    expect(wrapper.get('[data-testid="quota-bucket-row-80"]').text()).toContain('80%')
    expect(wrapper.get('[data-testid="quota-bucket-row-80"]').text()).toContain('0')
    expect(wrapper.get('[data-testid="quota-bucket-row-0"]').text()).toContain('0%')
    expect(wrapper.text()).toContain('admin.groups.quotaSummaryMatchedAccounts')
    expect(wrapper.text()).toContain('admin.groups.quotaSummarySkippedAccounts')
  })

  it('切换到 7D tab 后展示对应数据', async () => {
    const wrapper = mountModal(openAIGroup)
    await flushPromises()

    const sevenDayTab = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.groups.quotaSummaryWindows.sevenDay')
    )

    expect(sevenDayTab).toBeTruthy()
    await sevenDayTab!.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-bucket-row-70"]').text()).toContain('70%')
    expect(wrapper.get('[data-testid="quota-bucket-row-70"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="quota-bucket-row-60"]').text()).toContain('60%')
    expect(wrapper.get('[data-testid="quota-bucket-row-60"]').text()).toContain('0')
  })

  it('非 OpenAI 分组显示暂未适配', async () => {
    getQuotaSummaryMock.mockResolvedValue({
      group_id: 2,
      platform: 'anthropic',
      supported: false,
      tabs: []
    })

    const wrapper = mountModal(anthropicGroup)
    await flushPromises()

    expect(getQuotaSummaryMock).toHaveBeenCalledWith(2)
    expect(wrapper.text()).toContain('admin.groups.quotaSummaryUnsupported')
    expect(wrapper.text()).toContain('admin.groups.quotaSummaryOnlyOpenAI')
  })

  it('空数据时展示 100% 到 0% 的全档位 0 值', async () => {
    getQuotaSummaryMock.mockResolvedValue({
      group_id: 1,
      platform: 'openai',
      supported: true,
      tabs: [
        {
          window: '5h',
          bucket_counts: [],
          matched_account_count: 0,
          skipped_account_count: 0
        },
        {
          window: '7d',
          bucket_counts: [],
          matched_account_count: 0,
          skipped_account_count: 0
        }
      ]
    })

    const wrapper = mountModal(openAIGroup)
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-bucket-row-100"]').text()).toContain('100%')
    expect(wrapper.get('[data-testid="quota-bucket-row-100"]').text()).toContain('0')
    expect(wrapper.get('[data-testid="quota-bucket-row-0"]').text()).toContain('0%')
    expect(wrapper.get('[data-testid="quota-bucket-row-0"]').text()).toContain('0')
  })
})

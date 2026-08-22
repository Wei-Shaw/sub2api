import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupsStatusContent from '../GroupsStatusContent.vue'
import type { GroupsStatusResponse } from '@/api/groupsStatus'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const response: GroupsStatusResponse = {
  groups: [
    {
      id: 1,
      name: 'Healthy Claude',
      description: 'Primary public pool',
      platform: 'anthropic',
      subscription_type: 'standard',
      rate_multiplier: 0.8,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      account_count: 4,
      available_account_count: 4,
      rate_limited_account_count: 0,
      status: 'active',
      availability: 'available',
      available: true
    },
    {
      id: 2,
      name: 'OpenAI Mixed',
      description: 'Some accounts are cooling down',
      platform: 'openai',
      subscription_type: 'subscription',
      rate_multiplier: 1.2,
      peak_rate_enabled: true,
      peak_start: '09:00',
      peak_end: '12:00',
      peak_rate_multiplier: 1.5,
      account_count: 5,
      available_account_count: 2,
      rate_limited_account_count: 2,
      status: 'active',
      availability: 'degraded',
      available: true
    },
    {
      id: 3,
      name: 'Gemini Cooling',
      description: 'All accounts are temporarily limited',
      platform: 'gemini',
      subscription_type: 'standard',
      rate_multiplier: 1,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      account_count: 2,
      available_account_count: 0,
      rate_limited_account_count: 2,
      status: 'active',
      availability: 'rate_limited',
      available: false
    },
    {
      id: 4,
      name: 'OpenAI Offline',
      description: 'Disabled public group',
      platform: 'openai',
      subscription_type: 'standard',
      rate_multiplier: 2,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      account_count: 1,
      available_account_count: 0,
      rate_limited_account_count: 0,
      status: 'disabled',
      availability: 'unavailable',
      available: false
    }
  ],
  summary: {
    group_count: 4,
    available_group_count: 2,
    account_count: 12,
    available_account_count: 6,
    rate_limited_account_count: 4
  }
}

function mountContent(props: Record<string, unknown> = {}) {
  return mount(GroupsStatusContent, {
    props: {
      response,
      loading: false,
      error: false,
      lastUpdatedAt: new Date('2026-08-09T12:00:00Z'),
      ...props
    },
    global: {
      plugins: [createPinia()],
      stubs: {
        Icon: { props: ['name'], template: '<i :data-icon="name" />' },
        PlatformIcon: { props: ['platform'], template: '<i :data-platform-icon="platform" />' },
        GroupBadge: {
          props: ['name', 'rateMultiplier'],
          template: '<span data-group-badge>{{ name }}:{{ rateMultiplier }}x</span>'
        }
      }
    }
  })
}

describe('GroupsStatusContent required states and fields', () => {
  it('renders every required group metric on desktop and mobile surfaces', () => {
    const wrapper = mountContent()

    expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid^="mobile-group-"]')).toHaveLength(4)
    expect(wrapper.get('[data-testid="group-row-1"]').text()).toContain('Healthy Claude')
    expect(wrapper.get('[data-testid="group-row-1"]').text()).toContain('0.8x')
    expect(wrapper.get('[data-testid="group-row-1"]').text()).toContain('4')
    expect(wrapper.get('[data-testid="group-row-2"]').text()).toContain('1.2x')
    expect(wrapper.get('[data-testid="group-row-2"]').text()).toContain('2')
    expect(wrapper.get('[data-testid="group-row-3"]').text()).toContain('groupsStatus.status.rate_limited')
    expect(wrapper.get('[data-testid="group-row-4"]').text()).toContain('groupsStatus.status.unavailable')
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')
    expect(wrapper.get('[role="progressbar"]').attributes('aria-label')).toBe('groupsStatus.summary.availabilityRate')
  })

  it('covers all four availability classifications', () => {
    const wrapper = mountContent()
    for (const state of ['available', 'degraded', 'rate_limited', 'unavailable']) {
      expect(wrapper.text()).toContain(`groupsStatus.status.${state}`)
    }
  })

  it('shows loading, error with retry, empty, and filtered-empty states', async () => {
    const loading = mountContent({ loading: true, response: null })
    expect(loading.get('[data-testid="groups-status-loading"]').attributes('role')).toBe('status')
    expect(loading.get('[data-testid="groups-status-loading"]').attributes('aria-label')).toBe('groupsStatus.loading')

    const error = mountContent({ error: true, response: null })
    expect(error.get('[data-testid="groups-status-error"]').attributes('role')).toBe('alert')
    await error.get('button').trigger('click')
    expect(error.emitted('retry')).toHaveLength(1)

    const empty = mountContent({ response: { groups: [], summary: { ...response.summary, group_count: 0 } } })
    expect(empty.get('[data-testid="groups-status-empty"]')).toBeTruthy()

    const noResults = mountContent()
    await noResults.get('[data-testid="group-search"]').setValue('does-not-exist')
    expect(noResults.get('[data-testid="groups-status-no-results"]')).toBeTruthy()
  })

  it('renders a true all-zero summary with 0% overall availability', () => {
    const wrapper = mountContent({
      response: {
        groups: [],
        summary: {
          group_count: 0,
          available_group_count: 0,
          account_count: 0,
          available_account_count: 0,
          rate_limited_account_count: 0
        }
      }
    })

    const progressbar = wrapper.get('[role="progressbar"]')
    expect(progressbar.attributes('aria-valuenow')).toBe('0')
    expect(progressbar.get('div').attributes('style')).toContain('width: 0%')
    expect(wrapper.get('[data-testid="groups-status-empty"]')).toBeTruthy()
  })
})

describe('GroupsStatusContent channel and status selection', () => {
  it('filters by channel and can restore all channels', async () => {
    const wrapper = mountContent()

    expect(wrapper.get('[data-testid="channel-filter-all"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="channel-filter-openai"]').attributes('aria-pressed')).toBe('false')

    await wrapper.get('[data-testid="channel-filter-openai"]').trigger('click')
    expect(wrapper.get('[data-testid="channel-filter-all"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.get('[data-testid="channel-filter-openai"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="group-row-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="group-row-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="group-row-4"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="group-row-3"]').exists()).toBe(false)

    await wrapper.get('[data-testid="channel-filter-all"]').trigger('click')
    expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(4)
  })

  it('filters every status case and resets combined filters', async () => {
    const wrapper = mountContent()

    for (const [state, id] of [
      ['available', 1],
      ['degraded', 2],
      ['rate_limited', 3],
      ['unavailable', 4]
    ] as const) {
      await wrapper.get(`[data-testid="status-filter-${state}"]`).trigger('click')
      expect(wrapper.get('[data-testid="status-filter-all"]').attributes('aria-pressed')).toBe('false')
      expect(wrapper.get(`[data-testid="status-filter-${state}"]`).attributes('aria-pressed')).toBe('true')
      expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(1)
      expect(wrapper.find(`[data-testid="group-row-${id}"]`).exists()).toBe(true)
    }

    await wrapper.get('[data-testid="group-search"]').setValue('OpenAI')
    await wrapper.get('[data-testid="reset-filters"]').trigger('click')
    expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(4)
    expect((wrapper.get('[data-testid="group-search"]').element as HTMLInputElement).value).toBe('')
  })

  it('searches group name, description, and channel text', async () => {
    const wrapper = mountContent()

    await wrapper.get('[data-testid="group-search"]').setValue('cooling down')
    expect(wrapper.find('[data-testid="group-row-2"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(1)

    await wrapper.get('[data-testid="group-search"]').setValue('gemini')
    expect(wrapper.find('[data-testid="group-row-3"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="group-row-"]')).toHaveLength(1)
  })

  it('emits refresh from the toolbar control', async () => {
    const wrapper = mountContent()
    await wrapper.get('[data-testid="refresh-status"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})

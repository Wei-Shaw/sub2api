import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getModelMarketplace, showError, authStore, routerPush } = vi.hoisted(() => ({
  getModelMarketplace: vi.fn(),
  showError: vi.fn(),
  authStore: {
    isAuthenticated: false,
  },
  routerPush: vi.fn(),
}))

vi.mock('@/api/modelMarketplace', () => ({
  default: {
    getModelMarketplace,
  },
  getModelMarketplace,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
  }),
  useAuthStore: () => authStore,
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'Load failed',
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      push: routerPush,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'modelMarketplace.empty': 'No models',
    'modelMarketplace.searchPlaceholder': 'Search models',
    'modelMarketplace.columns.model': 'Model',
    'modelMarketplace.columns.platform': 'Platform',
    'modelMarketplace.columns.access': 'Access',
    'modelMarketplace.columns.channels': 'Channels',
    'modelMarketplace.columns.groups': 'Groups',
    'modelMarketplace.columns.pricing': 'Pricing',
    'modelMarketplace.filters.allPlatforms': 'All platforms',
    'modelMarketplace.filters.allAccess': 'All access',
    'modelMarketplace.filters.allBillingModes': 'All billing modes',
    'modelMarketplace.access.available': 'Available',
    'modelMarketplace.access.subscribed': 'Subscribed',
    'modelMarketplace.access.purchasable': 'Purchasable',
    'modelMarketplace.billingMode.token': 'Token',
    'modelMarketplace.billingMode.perRequest': 'Per request',
    'modelMarketplace.billingMode.image': 'Image',
    'modelMarketplace.channelsCount': 'channels',
    'modelMarketplace.buyPlan': 'Buy {name}',
    'modelMarketplace.noPricing': 'No pricing',
    'common.refresh': 'Refresh',
    'common.error': 'Error',
  }

  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string> | string) => {
        const template = messages[key] ?? key
        if (!params || typeof params === 'string') return template
        return template.replace(/\{(\w+)\}/g, (_, token) => params[token] ?? `{${token}}`)
      },
    }),
  }
})

import ModelMarketplaceView from '../ModelMarketplaceView.vue'

const AppLayoutStub = {
  template: '<section data-testid="app-layout"><slot /></section>',
}

const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /></div>',
}

const GroupBadgeStub = defineComponent({
  props: {
    name: {
      type: String,
      required: true,
    },
  },
  setup(props) {
    return () => h('span', props.name)
  },
})

const SupportedModelChipStub = defineComponent({
  props: {
    model: {
      type: Object,
      required: true,
    },
  },
  setup(props) {
    return () => h('span', String((props.model as { name?: string }).name ?? ''))
  },
})

const marketplaceResponse = {
  auth: {
    authenticated: false,
  },
  models: [
    {
      name: 'gpt-4o-mini',
      platform: 'openai',
      pricing: {
        billing_mode: 'token',
        input_price: 1,
        output_price: 2,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [],
      },
      channels: [{ name: 'Main', description: '' }],
      groups: [
        {
          id: 1,
          name: 'Starter',
          platform: 'openai',
          subscription_type: 'monthly',
          rate_multiplier: 1,
          user_rate_multiplier: null,
          is_exclusive: false,
          access_state: 'available',
          active_subscription: null,
          plans: [],
        },
      ],
      access_state: 'available',
    },
    {
      name: 'claude-3-5-sonnet',
      platform: 'anthropic',
      pricing: {
        billing_mode: 'per_request',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: 0.25,
        intervals: [],
      },
      channels: [{ name: 'Premium', description: '' }],
      groups: [
        {
          id: 2,
          name: 'Pro',
          platform: 'anthropic',
          subscription_type: 'monthly',
          rate_multiplier: 1,
          user_rate_multiplier: null,
          is_exclusive: false,
          access_state: 'purchasable',
          active_subscription: null,
          plans: [
            {
              id: 8,
              group_id: 2,
              name: 'Pro Plan',
              description: '',
              price: 9.9,
              original_price: null,
              validity_days: 30,
              validity_unit: 'day',
              features: [],
              product_name: 'pro-plan',
              sort_order: 1,
            },
          ],
        },
      ],
      access_state: 'purchasable',
    },
  ],
}

function mountView() {
  return mount(ModelMarketplaceView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        Icon: true,
        PlatformIcon: true,
        GroupBadge: GroupBadgeStub,
        SupportedModelChip: SupportedModelChipStub,
      },
    },
  })
}

function setViewportWidth(width: number) {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    writable: true,
    value: width,
  })
  window.dispatchEvent(new Event('resize'))
}

describe('ModelMarketplaceView', () => {
  beforeEach(() => {
    getModelMarketplace.mockReset()
    showError.mockReset()
    routerPush.mockReset()
    authStore.isAuthenticated = false
    setViewportWidth(1280)
    getModelMarketplace.mockResolvedValue(marketplaceResponse)
  })

  it('renders the empty state when the marketplace has no models', async () => {
    getModelMarketplace.mockResolvedValueOnce({
      ...marketplaceResponse,
      models: [],
    })

    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.text()).toContain('No models')
  })

  it('shows an error toast and falls back to the empty state when loading fails', async () => {
    getModelMarketplace.mockRejectedValueOnce(new Error('boom'))

    const wrapper = mountView()

    await flushPromises()

    expect(showError).toHaveBeenCalledWith('Load failed')
    expect(wrapper.text()).toContain('No models')
  })

  it('retries after a failed load when refresh is clicked and recovers the content', async () => {
    getModelMarketplace.mockRejectedValueOnce(new Error('boom'))
    getModelMarketplace.mockResolvedValueOnce(marketplaceResponse)

    const wrapper = mountView()

    await flushPromises()

    expect(showError).toHaveBeenCalledWith('Load failed')
    expect(wrapper.text()).toContain('No models')

    await wrapper.get('[data-testid="marketplace-refresh-button"]').trigger('click')
    await flushPromises()

    expect(getModelMarketplace).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('gpt-4o-mini')
    expect(wrapper.text()).toContain('claude-3-5-sonnet')
  })

  it('renders marketplace models and filters them by search text', async () => {
    const wrapper = mountView()

    await flushPromises()

    expect(getModelMarketplace).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('gpt-4o-mini')
    expect(wrapper.text()).toContain('claude-3-5-sonnet')

    await wrapper.get('input[type="text"]').setValue('claude')
    await flushPromises()

    expect(wrapper.text()).toContain('claude-3-5-sonnet')
    expect(wrapper.text()).not.toContain('gpt-4o-mini')
  })

  it('applies platform, access state, and billing mode filters together', async () => {
    const wrapper = mountView()

    await flushPromises()

    const selects = wrapper.findAll('select')
    await selects[0]!.setValue('anthropic')
    await selects[1]!.setValue('purchasable')
    await selects[2]!.setValue('per_request')
    await flushPromises()

    expect(wrapper.text()).toContain('claude-3-5-sonnet')
    expect(wrapper.text()).not.toContain('gpt-4o-mini')
  })

  it('navigates to the purchase page when a plan button is clicked', async () => {
    const wrapper = mountView()

    await flushPromises()

    await wrapper.get('[data-testid="marketplace-plan-button-8"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({
      path: '/purchase',
      query: { plan: '8' },
    })
  })

  it('renders mobile cards and keeps the mobile purchase button usable on a narrow viewport', async () => {
    setViewportWidth(375)
    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.find('[data-testid="marketplace-desktop-table"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="marketplace-mobile-cards"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="marketplace-mobile-card"]')).toHaveLength(2)

    await wrapper.get('[data-testid="marketplace-plan-button-8"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({
      path: '/purchase',
      query: { plan: '8' },
    })
  })

  it('switches between desktop and mobile layouts as the viewport changes after mount', async () => {
    const wrapper = mountView()

    await flushPromises()

    expect(wrapper.find('[data-testid="marketplace-desktop-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="marketplace-mobile-cards"]').exists()).toBe(false)

    setViewportWidth(375)
    await nextTick()

    expect(wrapper.find('[data-testid="marketplace-desktop-table"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="marketplace-mobile-cards"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="marketplace-mobile-card"]')).toHaveLength(2)

    setViewportWidth(1280)
    await nextTick()

    expect(wrapper.find('[data-testid="marketplace-desktop-table"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="marketplace-mobile-cards"]').exists()).toBe(false)
  })

  it('uses the plain page shell for guests and AppLayout for signed-in users', async () => {
    authStore.isAuthenticated = false
    const guestWrapper = mountView()

    await flushPromises()

    expect(guestWrapper.find('[data-testid="app-layout"]').exists()).toBe(false)

    authStore.isAuthenticated = true
    const userWrapper = mountView()

    await flushPromises()

    expect(userWrapper.find('[data-testid="app-layout"]').exists()).toBe(true)
  })
})

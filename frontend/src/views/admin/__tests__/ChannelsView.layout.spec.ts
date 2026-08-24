import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { channelsList, getModelDefaultPricing, groupsGetAll, settingsGetWebSearchEmulationConfig, syncPricingModels } = vi.hoisted(() => ({
  channelsList: vi.fn(),
  getModelDefaultPricing: vi.fn(),
  groupsGetAll: vi.fn(),
  settingsGetWebSearchEmulationConfig: vi.fn(),
  syncPricingModels: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: vi.fn(),
    },
    channels: {
      create: vi.fn(),
      getModelDefaultPricing,
      list: channelsList,
      remove: vi.fn(),
      syncPricingModels,
      update: vi.fn(),
    },
    groups: {
      getAll: groupsGetAll,
    },
    settings: {
      getWebSearchEmulationConfig: settingsGetWebSearchEmulationConfig,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (_key: string, paramsOrFallback?: Record<string, unknown> | string, fallback?: string) => {
        if (typeof paramsOrFallback === 'string') return paramsOrFallback
        return fallback ?? _key
      },
    }),
  }
})

import ChannelsView from '../ChannelsView.vue'

const SlotStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const TablePageLayoutStub = defineComponent({
  setup(_, { slots }) {
    return () => h('main', [slots.filters?.(), slots.table?.(), slots.pagination?.()])
  },
})

const DataTableStub = defineComponent({
  props: {
    data: {
      type: Array,
      default: () => [],
    },
  },
  setup(props, { slots }) {
    return () => h('div', {}, props.data.length === 0 ? slots.empty?.() : [])
  },
})

const BaseDialogStub = defineComponent({
  props: {
    show: Boolean,
    title: String,
  },
  setup(props, { slots }) {
    return () => (props.show ? h('section', { class: 'base-dialog-stub' }, [slots.default?.(), slots.footer?.()]) : null)
  },
})

const PricingEntryCardStub = defineComponent({
  props: {
    entry: {
      type: Object,
      required: true,
    },
    platform: String,
  },
  emits: ['update', 'remove'],
  setup(props) {
    return () => {
      const entry = props.entry as { models?: string[], input_price?: number | string | null }
      return h('div', { class: 'pricing-entry-card-stub', 'data-platform': props.platform }, [
        h('span', { class: 'pricing-models' }, entry.models?.join(',') ?? ''),
        h('span', { class: 'pricing-input-price' }, String(entry.input_price ?? '')),
      ])
    }
  },
})

const SelectStub = defineComponent({
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: '',
    },
    options: {
      type: Array,
      default: () => [],
    },
    placeholder: String,
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    type Option = { value: string | number | boolean, label: string }
    return () =>
      h('select', {
        value: props.modelValue ?? '',
        onChange: (event: Event) => {
          const value = (event.target as HTMLSelectElement).value
          emit('update:modelValue', value)
          emit('change', value, null)
        },
      }, (props.options as Option[]).map((option) => h('option', { value: option.value }, option.label)))
  },
})

describe('ChannelsView 弹框布局', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    channelsList.mockResolvedValue({ items: [], total: 0 })
    getModelDefaultPricing.mockResolvedValue({ found: false })
    groupsGetAll.mockResolvedValue([])
    settingsGetWebSearchEmulationConfig.mockResolvedValue({ enabled: false, providers: [] })
  })

  it('创建渠道表单为焦点 ring 保留横向安全区', async () => {
    const wrapper = mount(ChannelsView, {
      global: {
        stubs: {
          AppLayout: SlotStub,
          BaseDialog: BaseDialogStub,
          ConfirmDialog: true,
          DataTable: DataTableStub,
          EmptyState: SlotStub,
          Icon: true,
          Pagination: true,
          PlatformIcon: true,
          PricingEntryCard: true,
          Select: SelectStub,
          TablePageLayout: TablePageLayoutStub,
          Toggle: true,
        },
      },
    })

    await flushPromises()

    const createButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Create Channel'))
    expect(createButton).toBeTruthy()

    await createButton!.trigger('click')
    await flushPromises()

    const form = wrapper.get('form#channel-form')
    expect(form.classes()).toContain('channel-dialog-form')
    expect(form.classes()).toContain('px-0.5')

    const toggleControl = wrapper.get('.channel-dialog-toggle-control')
    expect(toggleControl.classes()).toContain('flex-shrink-0')
    expect(toggleControl.classes()).toContain('p-1')
  })

  it('Kiro 填充默认模型无论计费基准如何都按请求模型分组并查默认价格', async () => {
    groupsGetAll.mockResolvedValue([
      { id: 10, name: 'kiro free', platform: 'kiro', rate_multiplier: 1, account_count: 0 },
    ])
    getModelDefaultPricing.mockImplementation(async (model: string) => {
      if (model === 'gpt-5.6-sol') {
        return { found: true, input_price: 0.000005, output_price: 0.00003, cache_write_price: 0.00000625, cache_read_price: 0.0000005 }
      }
      if (model === 'gpt-5.6-terra') {
        return { found: true, input_price: 0.000002, output_price: 0.000012, cache_write_price: 0.0000025, cache_read_price: 0.0000002 }
      }
      if (model === 'gpt-5.6-luna') {
        return { found: true, input_price: 0.0000002, output_price: 0.0000012, cache_write_price: 0.00000025, cache_read_price: 0.00000002 }
      }
      if (model === 'codex-auto-review') {
        return { found: true, input_price: 0.0000002, output_price: 0.0000012, cache_write_price: 0.00000025, cache_read_price: 0.00000002 }
      }
      if (model === 'claude-opus-4-8') {
        return { found: true, input_price: 0.000001, output_price: 0.000005, cache_write_price: 0.00000125, cache_read_price: 0.0000001 }
      }
      if (model === 'claude-opus-4-8-thinking') {
        return { found: true, input_price: 0.000002, output_price: 0.000006, cache_write_price: 0.00000225, cache_read_price: 0.0000002 }
      }
      return { found: false }
    })

    const wrapper = mount(ChannelsView, {
      global: {
        stubs: {
          AppLayout: SlotStub,
          BaseDialog: BaseDialogStub,
          ConfirmDialog: true,
          DataTable: DataTableStub,
          EmptyState: SlotStub,
          Icon: true,
          Pagination: true,
          PlatformIcon: true,
          PricingEntryCard: PricingEntryCardStub,
          Select: SelectStub,
          TablePageLayout: TablePageLayoutStub,
          Toggle: true,
        },
      },
    })

    await flushPromises()

    const createButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Create Channel'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')
    await flushPromises()

    const billingModelSourceSelect = wrapper
      .findAll('select')
      .find((select) => (select.element as HTMLSelectElement).value === 'channel_mapped')
    expect(billingModelSourceSelect).toBeTruthy()
    await billingModelSourceSelect!.setValue('upstream')
    await flushPromises()

    const kiroPlatformLabel = wrapper
      .findAll('label')
      .find((label) => label.text().includes('kiro'))
    expect(kiroPlatformLabel).toBeTruthy()
    await kiroPlatformLabel!.find('input[type="checkbox"]').setValue(true)
    await flushPromises()

    const kiroTab = wrapper
      .findAll('button')
      .find((button) => button.text().includes('kiro'))
    expect(kiroTab).toBeTruthy()
    await kiroTab!.trigger('click')
    await flushPromises()

    const fillButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('填充默认模型'))
    expect(fillButton).toBeTruthy()
    await fillButton!.trigger('click')
    await flushPromises()

    expect(getModelDefaultPricing).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(getModelDefaultPricing).toHaveBeenCalledWith('gpt-5.6-terra')
    expect(getModelDefaultPricing).toHaveBeenCalledWith('gpt-5.6-luna')
    expect(getModelDefaultPricing).toHaveBeenCalledWith('codex-auto-review')
    expect(getModelDefaultPricing).toHaveBeenCalledWith('claude-opus-4-8')
    expect(getModelDefaultPricing).toHaveBeenCalledWith('claude-opus-4-8-thinking')
    expect(getModelDefaultPricing).not.toHaveBeenCalledWith('gpt-5.6')
    expect(getModelDefaultPricing).not.toHaveBeenCalledWith('claude-opus-4.8')
    expect(syncPricingModels).not.toHaveBeenCalled()

    const pricingCards = wrapper.findAll('.pricing-entry-card-stub')
    expect(pricingCards[0].find('.pricing-models').text()).toBe('gpt-5.6-sol')
    expect(pricingCards[0].find('.pricing-input-price').text()).toBe('5')
    expect(pricingCards[1].find('.pricing-models').text()).toBe('gpt-5.6-terra')
    expect(pricingCards[1].find('.pricing-input-price').text()).toBe('2')
    expect(pricingCards[2].find('.pricing-models').text()).toBe('gpt-5.6-luna')
    expect(pricingCards[2].find('.pricing-input-price').text()).toBe('0.2')
    expect(pricingCards[3].find('.pricing-models').text()).toBe('codex-auto-review')
    expect(pricingCards[3].find('.pricing-input-price').text()).toBe('0.2')
    expect(pricingCards[4].find('.pricing-models').text()).toBe('claude-opus-4-8')
    expect(pricingCards[4].find('.pricing-input-price').text()).toBe('1')
    expect(pricingCards[5].find('.pricing-models').text()).toBe('claude-opus-4-8-thinking')
    expect(pricingCards[5].find('.pricing-input-price').text()).toBe('2')
  })
})

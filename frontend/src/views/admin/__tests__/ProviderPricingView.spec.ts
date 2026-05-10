import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const { listOverrides, updateOverrides, showSuccess, showError } = vi.hoisted(() => ({
  listOverrides: vi.fn(),
  updateOverrides: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    providerPricing: {
      listOverrides,
      updateOverrides,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showSuccess,
    showError,
  }),
}))

import ProviderPricingView from '../ProviderPricingView.vue'

describe('admin ProviderPricingView', () => {
  beforeEach(() => {
    listOverrides.mockReset()
    updateOverrides.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    listOverrides.mockResolvedValue([
      {
        id: 'ovr-1',
        group_name: '公开组',
        model_name: 'gpt-5.4',
        input_price: 12.5,
        output_price: 25.5,
        cache_input_price: null,
        cache_create_price: null,
        cache_create_price_1h: null,
        enabled: true,
        note: '推荐价',
        sort_order: 0,
      },
    ])
    updateOverrides.mockImplementation(async (items) => items)
  })

  it('loads and saves provider price overrides', async () => {
    const wrapper = mount(ProviderPricingView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        },
      },
    })

    await flushPromises()
    await nextTick()

    expect(listOverrides).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('公开价格导出配置')
    const inputs = wrapper.findAll('input')
    const textValues = inputs.map((input) => String(input.element.value))
    expect(textValues).toContain('公开组')
    expect(textValues).toContain('gpt-5.4')

    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateOverrides).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('公开价格导出配置已保存')
  })

  it('ignores fully blank rows during save', async () => {
    const wrapper = mount(ProviderPricingView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        },
      },
    })

    await flushPromises()
    await nextTick()

    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text() === '新增')?.trigger('click')
    await flushPromises()

    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateOverrides).toHaveBeenCalledTimes(1)
    const payload = updateOverrides.mock.calls[0]?.[0]
    expect(payload).toHaveLength(1)
    expect(payload[0].model_name).toBe('gpt-5.4')
    expect(wrapper.text()).toContain('忽略了 1 条空白项')
  })

  it('shows inline validation when a partially filled row is invalid', async () => {
    listOverrides.mockResolvedValue([])
    const wrapper = mount(ProviderPricingView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        },
      },
    })

    await flushPromises()
    await nextTick()

    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text() === '新增')?.trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('input')
    await inputs[1]?.setValue('default')
    await inputs[3]?.setValue('0.2')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateOverrides).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalled()
    expect(wrapper.text()).toContain('模型名必填')
  })
})

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ApiKeyPlatformLimitsEditor from '../ApiKeyPlatformLimitsEditor.vue'
import type { ApiKeyPlatformLimits, ApiKeyPlatformUsage } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `<select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
    <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
  </select>`
}

const mountEditor = (modelValue: ApiKeyPlatformLimits, usages?: ApiKeyPlatformUsage[]) =>
  mount(ApiKeyPlatformLimitsEditor, {
    props: { modelValue, usages },
    global: { stubs: { Select: SelectStub } }
  })

const lastModel = (wrapper: ReturnType<typeof mountEditor>): ApiKeyPlatformLimits => {
  const emitted = wrapper.emitted('update:modelValue')
  expect(emitted).toBeTruthy()
  return emitted![emitted!.length - 1][0] as ApiKeyPlatformLimits
}

describe('ApiKeyPlatformLimitsEditor', () => {
  it('renders one row per configured source and nothing when empty', () => {
    expect(mountEditor({}).findAll('[data-test="platform-limit-row"]')).toHaveLength(0)
    expect(mountEditor({}).find('[data-test="platform-limits-empty"]').exists()).toBe(true)

    const wrapper = mountEditor({ openai: { quota: 10 }, antigravity: { rate_limit_5h: 1 } })
    expect(wrapper.findAll('[data-test="platform-limit-row"]')).toHaveLength(2)
  })

  it('adds a source that is not configured yet', async () => {
    const wrapper = mountEditor({ anthropic: { quota: 1 } })
    await wrapper.find('[data-test="platform-limit-add"]').trigger('click')
    const model = lastModel(wrapper)
    expect(Object.keys(model)).toContain('anthropic')
    expect(Object.keys(model)).toHaveLength(2)
  })

  it('drops a field when it is cleared or non-positive', async () => {
    const wrapper = mountEditor({ openai: { quota: 10, rate_limit_1d: 5 } })
    await wrapper.find('[data-test="platform-limit-quota-0"]').setValue('')
    expect(lastModel(wrapper).openai).toEqual({ rate_limit_1d: 5 })

    const other = mountEditor({ openai: { quota: 10 } })
    await other.find('[data-test="platform-limit-rate_limit_5h-0"]').setValue('2.5')
    expect(lastModel(other).openai).toEqual({ quota: 10, rate_limit_5h: 2.5 })
  })

  it('removes a source', async () => {
    const wrapper = mountEditor({ openai: { quota: 10 }, gemini: { quota: 2 } })
    await wrapper.find('[data-test="platform-limit-remove-0"]').trigger('click')
    expect(lastModel(wrapper)).toEqual({ gemini: { quota: 2 } })
  })

  it('switching a row to another source keeps its limits and position', async () => {
    const wrapper = mountEditor({ openai: { quota: 10 }, gemini: { quota: 2 } })
    await wrapper.find('[data-test="platform-limit-select-0"]').setValue('grok')
    const model = lastModel(wrapper)
    expect(Object.keys(model)).toEqual(['grok', 'gemini'])
    expect(model.grok).toEqual({ quota: 10 })
  })

  it('never offers a source that is already configured', () => {
    const wrapper = mountEditor({ openai: { quota: 10 }, gemini: { quota: 2 } })
    const firstRowOptions = wrapper
      .find('[data-test="platform-limit-select-0"]')
      .findAll('option')
      .map((option) => option.attributes('value'))
    expect(firstRowOptions).toContain('openai')
    expect(firstRowOptions).not.toContain('gemini')
  })

  it('shows usage bars only for windows that have a limit', () => {
    const usages: ApiKeyPlatformUsage[] = [
      { platform: 'openai', quota_used: 3, usage_5h: 1, usage_1d: 2, usage_7d: 4 }
    ]
    const wrapper = mountEditor({ openai: { quota: 10, rate_limit_1d: 5 } }, usages)
    const usageBlock = wrapper.find('[data-test="platform-limit-usage"]')
    expect(usageBlock.exists()).toBe(true)
    // quota + rate_limit_1d are configured; 5h/7d are not, so they get no bar.
    expect(usageBlock.text()).toContain('$3.0000 / $10.00')
    expect(usageBlock.text()).toContain('$2.0000 / $5.00')
    expect(usageBlock.text()).not.toContain('$1.0000')
    expect(usageBlock.text()).not.toContain('$4.0000')
  })

  it('shows no usage block in create mode', () => {
    const wrapper = mountEditor({ openai: { quota: 10 } })
    expect(wrapper.find('[data-test="platform-limit-usage"]').exists()).toBe(false)
  })
})

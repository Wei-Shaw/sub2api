import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountTableFilters from '../AccountTableFilters.vue'
import {
  ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER,
  ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER
} from '../accountStatusFilter'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (params) return `${key}:${JSON.stringify(params)}`
      return key
    }
  })
}))

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  methods: {
    onChange(event: Event) {
      const value = (event.target as HTMLSelectElement).value
      this.$emit('update:modelValue', value)
      this.$emit('change', value)
    }
  },
  template: '<select :value="modelValue || \'\'" @change="onChange"><option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option></select>'
}

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show" data-test="dialog"><div data-test="dialog-title">{{ title }}</div><slot /><slot name="footer" /></div>'
}

const mountFilters = (filters: Record<string, unknown> = {}) => mount(AccountTableFilters, {
  props: {
    searchQuery: '',
    filters: {
      platform: '',
      type: '',
      status: '',
      privacy_mode: '',
      group: '',
      ...filters
    },
    groups: []
  },
  global: {
    stubs: {
      Select: SelectStub,
      SearchInput: { template: '<input />' },
      BaseDialog: BaseDialogStub,
      Teleport: true
    }
  }
})

describe('AccountTableFilters', () => {
  it('确认指定额度弹窗后发出编码状态值并触发刷新', async () => {
    const wrapper = mountFilters()
    const statusSelect = wrapper.findAll('select')[2]

    await statusSelect.setValue(ACCOUNT_STATUS_FILTER_QUOTA_USED_RANGE_PICKER)
    expect(wrapper.find('[data-test="dialog-title"]').text()).toBe('admin.accounts.status.openAIQuotaUsedRange')

    const inputs = wrapper.findAll('input[type="number"]')
    await inputs[0].setValue('42')
    await inputs[1].setValue('42')
    await wrapper.findAll('button').at(-1)!.trigger('click')

    expect(wrapper.emitted('update:filters')?.at(-1)?.[0]).toMatchObject({
      status: 'openai_quota_used_range:7d:42:42'
    })
    expect(wrapper.emitted('change')).toHaveLength(1)
  })

  it('确认额度已满弹窗后发出编码状态值并触发刷新', async () => {
    const wrapper = mountFilters()
    const statusSelect = wrapper.findAll('select')[2]

    await statusSelect.setValue(ACCOUNT_STATUS_FILTER_QUOTA_FULL_PICKER)
    expect(wrapper.find('[data-test="dialog-title"]').text()).toBe('admin.accounts.status.openAIQuotaFull')

    const radios = wrapper.findAll('input[type="radio"]')
    await radios[0].setValue()
    await wrapper.findAll('button').at(-1)!.trigger('click')

    expect(wrapper.emitted('update:filters')?.at(-1)?.[0]).toMatchObject({
      status: 'openai_quota_full:5h'
    })
    expect(wrapper.emitted('change')).toHaveLength(1)
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BulkEditGroupModal from '../BulkEditGroupModal.vue'
import type { AdminGroup } from '@/types'

const { bulkUpdate, showSuccess, showError } = vi.hoisted(() => ({
  bulkUpdate: vi.fn(), showSuccess: vi.fn(), showError: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { bulkUpdate } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key} ${JSON.stringify(params ?? {})}` })
}))

type SelectedGroup = Pick<AdminGroup, 'id' | 'name' | 'subscription_type'>
const selectedGroups: SelectedGroup[] = [
  { id: 1, name: 'First', subscription_type: 'subscription' },
  { id: 2, name: 'Second', subscription_type: 'subscription' }
]
const mountModal = (groups = selectedGroups) => mount(BulkEditGroupModal, {
  props: { show: true, selectedGroups: groups },
  global: {
    stubs: {
      BaseDialog: {
        name: 'BaseDialog',
        props: ['show', 'closeOnEscape', 'showCloseButton'],
        emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: {
        props: ['modelValue', 'options', 'disabled'],
        emits: ['update:modelValue'],
        template: `<select :disabled="disabled" :value="String(modelValue)" @change="$emit('update:modelValue', options.find(option => String(option.value) === $event.target.value).value)">
          <option v-for="option in options" :key="String(option.value)" :value="String(option.value)">{{ option.label }}</option>
        </select>`
      }
    }
  }
})

describe('BulkEditGroupModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    bulkUpdate.mockResolvedValue({ succeededIds: [1, 2], failures: [] })
  })

  it('requires a field choice and updates only the selected rate multiplier', async () => {
    const wrapper = mountModal()
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
    await wrapper.get('[data-test="enable-rate"]').setValue(true)
    await wrapper.get('[data-test="rate-input"]').setValue('0.25')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { rate_multiplier: 0.25 })
    expect(wrapper.emitted('updated')).toEqual([[[1, 2]]])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it.each(['', '0', '-0.1', 'not-a-number'])('rejects invalid multipliers: %s', async (value) => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rate"]').setValue(true)
    await wrapper.get('[data-test="rate-input"]').setValue(value)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
  })

  it('preserves explicit false, empty descriptions, and zero RPM', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-exclusive"]').setValue(true)
    await wrapper.get('[data-test="exclusive-input"]').setValue('false')
    await wrapper.get('[data-test="enable-description"]').setValue(true)
    await wrapper.get('[data-test="enable-rpm"]').setValue(true)
    await wrapper.get('[data-test="rpm-input"]').setValue('0')
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { is_exclusive: false, description: '', rpm_limit: 0 })
  })

  it.each(['', '-1', '1.5', '9007199254740992'])('rejects invalid RPM: %s', async (value) => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rpm"]').setValue(true)
    await wrapper.get('[data-test="rpm-input"]').setValue(value)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
  })

  it('distinguishes zero quota from unlimited while preserving unchecked limits', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-daily_limit_usd"]').setValue(true)
    await wrapper.get('[data-test="daily_limit_usd-input"]').setValue('0')
    await wrapper.get('[data-test="enable-weekly_limit_usd"]').setValue(true)
    await wrapper.get('[data-test="unlimited-weekly_limit_usd"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { daily_limit_usd: 0, weekly_limit_usd: null })
  })

  it.each(['', '-10'])('requires a valid subscription quota or explicit unlimited: %s', async (value) => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-monthly_limit_usd"]').setValue(true)
    await wrapper.get('[data-test="monthly_limit_usd-input"]').setValue(value)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
  })

  it('disables subscription quotas for a mixed selection while allowing common fields', async () => {
    const wrapper = mountModal([
      selectedGroups[0], { id: 2, name: 'Standard', subscription_type: 'standard' }
    ])
    expect(wrapper.get('[data-test="enable-daily_limit_usd"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.groups.bulkEdit.subscriptionOnly')
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('[data-test="status-input"]').setValue('inactive')
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { status: 'inactive' })
  })

  it('discards a value when its field is unchecked again', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rate"]').setValue(true)
    await wrapper.get('[data-test="rate-input"]').setValue('0.5')
    await wrapper.get('[data-test="enable-rate"]').setValue(false)
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { status: 'active' })
  })

  it('shows backend errors and retries only the failed groups from the original selection', async () => {
    bulkUpdate.mockResolvedValueOnce({
      succeededIds: [1], failures: [{ id: 2, error: { message: 'Permission denied' } }]
    }).mockResolvedValueOnce({ succeededIds: [2], failures: [] })
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rate"]').setValue(true)
    await wrapper.get('[data-test="rate-input"]').setValue('2')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('#2 Second: Permission denied')
    expect(wrapper.emitted('updated')).toEqual([[[1]]])
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(showSuccess).not.toHaveBeenCalled()
    await wrapper.setProps({ selectedGroups: [{ id: 3, name: 'New group', subscription_type: 'standard' }] })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkUpdate).toHaveBeenLastCalledWith([2], { rate_multiplier: 2 })
    expect(wrapper.emitted('updated')).toEqual([[[1]], [[2]]])
  })

  it('blocks duplicate submissions and closing while requests are in flight', async () => {
    let finish!: (result: { succeededIds: number[]; failures: [] }) => void
    bulkUpdate.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    wrapper.findComponent({ name: 'BaseDialog' }).vm.$emit('close')
    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    finish({ succeededIds: [1, 2], failures: [] })
    await flushPromises()
  })

  it('resets field choices when reopened for another selection', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rate"]').setValue(true)
    await wrapper.get('[data-test="rate-input"]').setValue('2')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, selectedGroups: [{ id: 3, name: 'Third', subscription_type: 'standard' }] })
    expect(wrapper.find('[data-test="rate-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([3], { status: 'active' })
  })
})

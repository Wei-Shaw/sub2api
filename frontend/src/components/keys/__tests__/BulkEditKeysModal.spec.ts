import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BulkEditKeysModal from '../BulkEditKeysModal.vue'
import type { Group } from '@/types'

const { bulkUpdate, showSuccess, showError } = vi.hoisted(() => ({
  bulkUpdate: vi.fn(), showSuccess: vi.fn(), showError: vi.fn()
}))
vi.mock('@/api', () => ({ keysAPI: { bulkUpdate } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key} ${JSON.stringify(params ?? {})}` })
}))

const mountModal = () => mount(BulkEditKeysModal, {
  props: {
    show: true,
    selectedKeys: [{ id: 1, name: 'First' }, { id: 2, name: 'Second' }],
    groups: [{ id: 7, name: 'Available group' }] as Group[]
  },
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
        template: `<select :disabled="disabled" :value="modelValue" @change="$emit('update:modelValue', $event.target.value === '7' ? 7 : $event.target.value)">
          <option value=""></option>
          <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>`
      }
    }
  }
})

describe('BulkEditKeysModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    bulkUpdate.mockResolvedValue({ succeededIds: [1, 2], failures: [] })
  })

  it('requires an explicit field choice and sends only that field', async () => {
    const wrapper = mountModal()
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()

    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('[data-test="status-input"]').setValue('inactive')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { status: 'inactive' })
    expect(wrapper.emitted('updated')).toEqual([[[1, 2]]])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('preserves other fields when changing one window limit, including zero', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-rate_limit_1d"]').setValue(true)
    await wrapper.get('[data-test="rate_limit_1d-input"]').setValue('0')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { rate_limit_1d: 0 })
  })

  it.each(['', '-1', 'not-a-number'])('rejects an invalid selected quota: %s', async (value) => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-quota"]').setValue(true)
    await wrapper.get('[data-test="quota-input"]').setValue(value)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
  })

  it('requires an available group when changing group', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-group"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
    await wrapper.get('[data-test="group-input"]').setValue('7')
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { group_id: 7 })
  })

  it('clears only an explicitly selected IP list and expiration', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-ip_whitelist"]').setValue(true)
    await wrapper.get('[data-test="enable-expiration"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).not.toHaveBeenCalled()
    await wrapper.get('[data-test="never-expires"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { ip_whitelist: [], expires_at: '' })
  })

  it('normalizes IP lines and converts the chosen local date to ISO', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-ip_blacklist"]').setValue(true)
    await wrapper.get('[data-test="ip_blacklist-input"]').setValue(' 192.0.2.1 \n\n 198.51.100.0/24\n')
    await wrapper.get('[data-test="enable-expiration"]').setValue(true)
    await wrapper.get('[data-test="expiration-input"]').setValue('2030-01-02T03:04')
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      ip_blacklist: ['192.0.2.1', '198.51.100.0/24'],
      expires_at: new Date('2030-01-02T03:04').toISOString()
    })
  })

  it('reports individual failures and retries only failed keys', async () => {
    bulkUpdate.mockResolvedValueOnce({
      succeededIds: [1],
      failures: [{ id: 2, error: { status: 403, message: 'Group access denied' } }]
    }).mockResolvedValueOnce({ succeededIds: [2], failures: [] })
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-quota"]').setValue(true)
    await wrapper.get('[data-test="quota-input"]').setValue('25.50')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('#2 Second: Group access denied')
    expect(wrapper.emitted('updated')).toEqual([[[1]]])
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(showSuccess).not.toHaveBeenCalled()
    await wrapper.setProps({ selectedKeys: [{ id: 2, name: 'Second' }, { id: 3, name: 'New selection' }] })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(bulkUpdate).toHaveBeenLastCalledWith([2], { quota: 25.5 })
    expect(wrapper.emitted('updated')).toEqual([[[1]], [[2]]])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('prevents duplicate submissions and closing during an update', async () => {
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

  it('resets all field choices when reopened for a new selection', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-test="enable-quota"]').setValue(true)
    await wrapper.get('[data-test="quota-input"]').setValue('12')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, selectedKeys: [{ id: 3, name: 'Third' }] })
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="quota-input"]').exists()).toBe(false)
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(bulkUpdate).toHaveBeenCalledWith([3], { status: 'active' })
  })
})

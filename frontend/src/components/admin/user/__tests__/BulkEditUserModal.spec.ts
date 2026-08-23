import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import BulkEditUserModal from '../BulkEditUserModal.vue'

const { batchUpdateLimits, showSuccess, showError } = vi.hoisted(() => ({
  batchUpdateLimits: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      batchUpdateLimits
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = () => mount(BulkEditUserModal, {
  props: {
    show: true,
    selectedIds: [4, 7]
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      }
    }
  }
})

describe('BulkEditUserModal', () => {
  beforeEach(() => {
    batchUpdateLimits.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    batchUpdateLimits.mockResolvedValue({ affected: 2 })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('disables submission until at least one enabled field has a value', async () => {
    const wrapper = mountModal()

    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="concurrency-input"]').setValue('5')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('disables submission when more than 500 users are selected', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ selectedIds: Array.from({ length: 501 }, (_, index) => index + 1) })
    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('5')

    expect(wrapper.text()).toContain('admin.users.bulkLimits.selectionLimit')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
  })

  it('submits only the enabled RPM field and preserves zero as unlimited', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-rpm-limit"]').trigger('click')
    await wrapper.get('[data-test="rpm-limit-input"]').setValue('0')
    expect(wrapper.text()).toContain('admin.users.bulkLimits.unlimited')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      rpm_limit: 0
    })
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.rpmUnlimitedValue')
    )
    expect(wrapper.emitted('success')).toEqual([[2]])
  })

  it('omits disabled fields from the request', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('9')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      concurrency: 9
    })
  })

  it('does not call the API when overwrite confirmation is cancelled', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('9')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).not.toHaveBeenCalled()
  })
})

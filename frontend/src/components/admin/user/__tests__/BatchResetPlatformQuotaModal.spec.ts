import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const apiMocks = vi.hoisted(() => ({
  batchResetPlatformQuotaWindows: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      batchResetPlatformQuotaWindows: apiMocks.batchResetPlatformQuotaWindows,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    props: ['show', 'title', 'width'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>',
  },
}))

import BatchResetPlatformQuotaModal from '../BatchResetPlatformQuotaModal.vue'

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.batchResetPlatformQuotaWindows.mockResolvedValue({ affected: 2 })
})

describe('BatchResetPlatformQuotaModal', () => {
  it('提交显式用户、平台和窗口集合', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(BatchResetPlatformQuotaModal, {
      props: { show: true, selectedIds: [11, 12] },
    })
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[1].setValue(true) // openai
    await checkboxes[5].setValue(true) // five_hour
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(apiMocks.batchResetPlatformQuotaWindows).toHaveBeenCalledWith({
      user_ids: [11, 12],
      platforms: ['openai'],
      windows: ['five_hour'],
    })
    expect(wrapper.emitted('success')).toEqual([[2]])
    expect(wrapper.emitted('close')).toHaveLength(1)
    confirmSpy.mockRestore()
  })

  it('超过 500 个用户时禁止提交', () => {
    const wrapper = mount(BatchResetPlatformQuotaModal, {
      props: { show: true, selectedIds: Array.from({ length: 501 }, (_, index) => index + 1) },
    })
    expect(wrapper.get('[data-test="submit-batch-quota-reset"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.users.bulkQuotaReset.selectionLimit')
  })
})

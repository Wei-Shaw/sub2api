import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ResetAllQuotaDialog from '../ResetAllQuotaDialog.vue'

const { resetAllQuotaPreview, resetAllQuota, resetAllQuotaRetry } = vi.hoisted(() => ({
  resetAllQuotaPreview: vi.fn(),
  resetAllQuota: vi.fn(),
  resetAllQuotaRetry: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { resetAllQuotaPreview, resetAllQuota, resetAllQuotaRetry },
  },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, named?: Record<string, unknown>) =>
      named ? `${key}:${Object.values(named).join(',')}` : key }),
  }
})

function mountDialog() {
  return mount(ResetAllQuotaDialog, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show', 'title'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
      },
    },
  })
}

describe('ResetAllQuotaDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetAllQuotaPreview.mockResolvedValue(2)
    resetAllQuota.mockResolvedValue({
      processed: 2, remaining: 0, success_count: 2, failed_count: 0, failures: [],
    })
    resetAllQuotaRetry.mockResolvedValue({
      processed: 1, remaining: 0, success_count: 1, failed_count: 0, failures: [],
    })
  })

  it('默认不勾选任何窗口，需显式选择', () => {
    const wrapper = mountDialog()
    const boxes = wrapper.findAll('input[type="checkbox"]')
    expect(boxes).toHaveLength(3)
    for (const box of boxes) {
      expect((box.element as HTMLInputElement).checked).toBe(false)
    }
  })

  it('勾选窗口后先预览受影响数量，再二次确认才执行', async () => {
    const wrapper = mountDialog()
    const boxes = wrapper.findAll('input[type="checkbox"]')
    await boxes[0].setValue(true)

    // 第一步：预览
    await wrapper.findAll('button').find((b) => b.text() === 'common.next')!.trigger('click')
    await flushPromises()

    expect(resetAllQuotaPreview).toHaveBeenCalledWith({ daily: true, weekly: false, monthly: false })
    expect(resetAllQuota).not.toHaveBeenCalled()

    // 第二步：确认后才真正执行
    const confirmBtn = wrapper.findAll('button').find((b) => b.text() === 'admin.subscriptions.resetAll.confirm')
    expect(confirmBtn).toBeDefined()
    await confirmBtn!.trigger('click')
    await flushPromises()

    expect(resetAllQuota).toHaveBeenCalledWith({ daily: true, weekly: false, monthly: false, limit: 100 })
  })

  it('失败项展示明细并可重试', async () => {
    // 第一批：有 1 条失败、仍有剩余 → 组件会继续下一批
    resetAllQuota.mockResolvedValueOnce({
      processed: 2, remaining: 1, success_count: 1, failed_count: 1,
      failures: [{ subscription_id: 42, error: 'boom' }],
    })
    // 第二批：无剩余 → 循环结束
    resetAllQuota.mockResolvedValueOnce({
      processed: 1, remaining: 0, success_count: 1, failed_count: 0,
      failures: [],
    })
    const wrapper = mountDialog()
    await wrapper.findAll('input[type="checkbox"]')[0].setValue(true)
    await wrapper.findAll('button').find((b) => b.text() === 'common.next')!.trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((b) => b.text() === 'admin.subscriptions.resetAll.confirm')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('#42')
    expect(wrapper.text()).toContain('boom')

    const retryBtn = wrapper.findAll('button').find((b) => b.text() === 'admin.subscriptions.resetAll.retryFailures')
    expect(retryBtn).toBeDefined()
    await retryBtn!.trigger('click')
    await flushPromises()

    expect(resetAllQuotaRetry).toHaveBeenCalledWith([42], { daily: true, weekly: false, monthly: false })
  })

  it('未勾选任何窗口时不能进入下一步', () => {
    const wrapper = mountDialog()
    const nextBtn = wrapper.findAll('button').find((b) => b.text() === 'common.next')
    expect(nextBtn).toBeDefined()
    expect(nextBtn!.attributes('disabled')).toBeDefined()
  })
})

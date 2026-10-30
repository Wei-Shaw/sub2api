import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserErrorDetailModal from '../UserErrorDetailModal.vue'

const { getDetail } = vi.hoisted(() => ({ getDetail: vi.fn() }))
vi.mock('@/api/usage', () => ({ getMyErrorDetail: getDetail }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { getDetail.mockReset(); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => vi.restoreAllMocks())
function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (error: Error) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const detail = (model: string) => ({ model, status_code: 500, category: 'upstream', created_at: '2026-09-20' })
async function open() {
  const wrapper = mount(UserErrorDetailModal, {
    props: { show: false, errorId: 1 },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('user error detail requests', () => {
  it('keeps the newer detail when an earlier request finishes last', async () => {
    const old = deferred(); const current = deferred()
    getDetail.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await open()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, errorId: 2 })
    current.resolve(detail('current-model')); await flushPromises()
    old.resolve(detail('old-model')); await flushPromises()
    expect(wrapper.text()).toContain('current-model')
    expect(wrapper.text()).not.toContain('old-model')
  })

  it('keeps loading until the current request settles', async () => {
    const old = deferred(); const current = deferred()
    getDetail.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await open()
    await wrapper.setProps({ errorId: 2 })
    old.resolve(detail('old-model')); await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
    current.resolve(detail('current-model')); await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
    expect(wrapper.text()).toContain('current-model')
  })

  it('ignores an obsolete failure, including when reopening the same record', async () => {
    const old = deferred()
    getDetail.mockReturnValueOnce(old.promise).mockResolvedValueOnce(detail('current-model'))
    const wrapper = await open()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true }); await flushPromises()
    old.reject(new Error('obsolete')); await flushPromises()
    expect(wrapper.text()).toContain('current-model')
    expect(wrapper.text()).not.toContain('usage.errors.detail.loadFailed')
  })

  it('still reports a failure for the current record', async () => {
    getDetail.mockRejectedValueOnce(new Error('current failure'))
    const wrapper = await open(); await flushPromises()
    expect(wrapper.text()).toContain('usage.errors.detail.loadFailed')
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
  })
})

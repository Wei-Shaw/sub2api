import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QualityTestView from '../QualityTestView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  getAvailableModels: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { list: mocks.list, getAvailableModels: mocks.getAvailableModels } }
}))
vi.mock('@/api/client', () => ({ buildApiUrl: (path: string) => path }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

const accountPage = (page: number, pageSize: number, total: number) => {
  const start = (page - 1) * pageSize
  const count = Math.max(0, Math.min(pageSize, total - start))
  return { items: Array.from({ length: count }, (_, i) => ({ id: start + i + 1, name: `acc-${start + i + 1}` })), total }
}

function mountView() {
  return mount(QualityTestView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Select: true, Icon: true } } })
}

describe('QualityTestView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads every account page instead of only the first 200', async () => {
    mocks.list.mockImplementation((page: number, pageSize: number) => Promise.resolve(accountPage(page, pageSize, 1500)))
    const wrapper = mountView()
    await flushPromises()

    expect(mocks.list.mock.calls.map(([page, pageSize]) => [page, pageSize])).toEqual([[1, 1000], [2, 1000]])
    const accounts = (wrapper.vm as any).accounts as { id: number }[]
    expect(accounts).toHaveLength(1500)
    expect(accounts.at(-1)?.id).toBe(1500)
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it('reports account and model load failures', async () => {
    mocks.list.mockRejectedValueOnce({ message: 'accounts down' })
    const wrapper = mountView()
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('accounts down')

    mocks.getAvailableModels.mockRejectedValueOnce(new Error('network'))
    const slot = (wrapper.vm as any).slots[0]
    slot.accountId = 7
    await (wrapper.vm as any).loadModels(slot)
    expect(mocks.showError).toHaveBeenLastCalledWith('network')

    mocks.getAvailableModels.mockRejectedValueOnce({})
    await (wrapper.vm as any).loadModels(slot)
    expect(mocks.showError).toHaveBeenLastCalledWith('admin.qualityTest.loadModelsFailed')
  })
})

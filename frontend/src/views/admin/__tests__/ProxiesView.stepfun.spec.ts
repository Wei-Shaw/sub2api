import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'

import ProxiesView from '../ProxiesView.vue'

const { listProxies, getAllWithCount, checkProxyQuality } = vi.hoisted(() => ({
  listProxies: vi.fn(),
  getAllWithCount: vi.fn(),
  checkProxyQuality: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    proxies: {
      list: listProxies,
      getAllWithCount,
      checkProxyQuality,
    },
  },
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))

const mountView = () =>
  shallowMount(ProxiesView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="table" /></div>' },
        DataTable: {
          props: [
            'columns',
            'data',
            'loading',
            'serverSideSort',
            'defaultSortKey',
            'defaultSortOrder',
          ],
          emits: ['sort'],
          template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
        },
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      },
    },
  })

let wrapper: ReturnType<typeof mountView>

beforeEach(() => {
  vi.clearAllMocks()
  listProxies.mockResolvedValue({
    items: [{ id: 9, name: 'proxy', protocol: 'http', host: 'proxy.example', port: 8080, status: 'active' }],
    total: 1,
    pages: 1,
  })
  getAllWithCount.mockResolvedValue([])
  checkProxyQuality.mockResolvedValue({
    proxy_id: 9,
    score: 90,
    grade: 'A',
    summary: 'ok',
    base_latency_ms: 10,
    passed_count: 2,
    warn_count: 0,
    failed_count: 0,
    challenge_count: 0,
    checked_at: 1,
    items: [
      { target: 'base_connectivity', status: 'pass' },
      { target: 'stepfun', status: 'pass' },
    ],
  })
})

afterEach(() => wrapper?.unmount())

describe('ProxiesView StepFun quality target', () => {
  it('shows the StepFun display name in the quality report', async () => {
    wrapper = mountView()
    await flushPromises()

    const qualityButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.proxies.qualityCheck'),
    )
    expect(qualityButton).toBeDefined()
    await qualityButton!.trigger('click')
    await flushPromises()

    expect(checkProxyQuality).toHaveBeenCalledWith(9)
    expect(wrapper.text()).toContain('StepFun')
  })
})

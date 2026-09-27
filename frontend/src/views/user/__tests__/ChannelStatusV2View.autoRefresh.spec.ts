import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import ChannelStatusV2View from '../ChannelStatusV2View.vue'

const { api } = vi.hoisted(() => ({
  api: {
    getDimensions: vi.fn(),
    getSnapshot: vi.fn(),
    getMatrix: vi.fn(),
    getModels: vi.fn(),
    getErrors: vi.fn(),
    getUsers: vi.fn(),
  },
}))

vi.mock('@/api/channelMonitorV2', () => api)
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAdmin: false }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), cachedPublicSettings: {} }) }))
vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorThroughputHidden: () => false,
  isChannelMonitorUserRankingHidden: () => false,
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key, te: () => false, locale: ref('en') }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
}

let wrapper: ReturnType<typeof shallowMount> | undefined

beforeEach(() => {
  vi.useFakeTimers()
  api.getDimensions.mockResolvedValue({ platforms: [], groups: [], models: [] })
  api.getSnapshot.mockResolvedValue({ config: { refresh_interval_seconds: 60 } })
  api.getMatrix.mockResolvedValue({ items: [] })
  api.getModels.mockResolvedValue({ items: [] })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  delete (document as { hidden?: boolean }).hidden
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe('ChannelStatusV2View auto refresh', () => {
  it('does not poll while the tab is hidden and resumes when shown', async () => {
    wrapper = shallowMount(ChannelStatusV2View)
    await flushPromises()
    expect(api.getSnapshot).toHaveBeenCalledTimes(1)

    setHidden(true)
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
    expect(api.getSnapshot).toHaveBeenCalledTimes(1)
    expect(api.getDimensions).toHaveBeenCalledTimes(1)

    setHidden(false)
    await vi.advanceTimersByTimeAsync(60 * 1000)
    expect(api.getSnapshot).toHaveBeenCalledTimes(2)
  })
})

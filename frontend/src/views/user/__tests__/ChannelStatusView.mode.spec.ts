import { describe, expect, it, vi, beforeEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const isV1 = vi.fn(() => false)
const loads = { v1: 0, v2: 0 }

vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorV1Mode: () => isV1(),
}))

const view = (name: string, testid: string) => ({
  __esModule: true,
  default: defineComponent({ name, setup: () => () => h('div', { 'data-testid': testid }) }),
})

// Fresh module per test with re-registered view mocks (doMock drops the cached mock), so the
// load counts are per test and prove V1/V2 are not static imports in any test order.
async function mountView() {
  const { default: ChannelStatusView } = await import('../ChannelStatusView.vue')
  const wrapper = mount(ChannelStatusView)
  await flushPromises()
  return wrapper
}

describe('ChannelStatusView mode switch', () => {
  beforeEach(() => {
    isV1.mockReset()
    vi.resetModules()
    loads.v1 = 0
    loads.v2 = 0
    vi.doMock('../ChannelStatusV1View.vue', () => (loads.v1++, view('ChannelStatusV1View', 'v1')))
    vi.doMock('../ChannelStatusV2View.vue', () => (loads.v2++, view('ChannelStatusV2View', 'v2')))
  })

  it('renders V2 when not in v1 mode and never loads V1', async () => {
    isV1.mockReturnValue(false)
    const wrapper = await mountView()
    expect(wrapper.find('[data-testid="v2"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="v1"]').exists()).toBe(false)
    expect(loads).toEqual({ v1: 0, v2: 1 })
  })

  it('renders V1 when in v1 mode and never loads V2', async () => {
    isV1.mockReturnValue(true)
    const wrapper = await mountView()
    expect(wrapper.find('[data-testid="v1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="v2"]').exists()).toBe(false)
    expect(loads).toEqual({ v1: 1, v2: 0 })
  })
})

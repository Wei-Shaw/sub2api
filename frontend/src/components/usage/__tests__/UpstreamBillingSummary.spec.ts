import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UpstreamBillingSummary from '../UpstreamBillingSummary.vue'

const { report } = vi.hoisted(() => ({ report: vi.fn() }))
vi.mock('@/api/upstreamBilling', () => ({ upstreamBillingAPI: { summary: report } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

beforeEach(() => {
  report.mockReset()
  report.mockResolvedValue({ is_admin: false, totals: [{ currency: 'USD', allocated_cost: '1.00000001' }, { currency: 'CNY', allocated_cost: '3.42' }] })
})

function render() {
  return mount(UpstreamBillingSummary, {
    props: { month: '2026-09' },
    global: { stubs: { RouterLink: { name: 'RouterLink', props: ['to'], template: '<a><slot /></a>' } } }
  })
}

describe('UpstreamBillingSummary', () => {
  it('keeps currencies separate and only reloads when the month changes', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('USD 1.00000001')
    expect(wrapper.text()).toContain('CNY 3.42')
    expect(wrapper.text()).toContain('upstreamBilling.summaryUserScope')
    expect(wrapper.text()).not.toContain('upstreamBilling.summaryAdminScope')
    expect(wrapper.findComponent({ name: 'RouterLink' }).props('to')).toEqual({ path: '/upstream-billing', query: { month: '2026-09' } })
    await wrapper.setProps({ month: '2026-09' })
    expect(report).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ month: '2026-08' })
    await flushPromises()
    expect(report).toHaveBeenCalledTimes(2)
    expect((report.mock.calls[0][1] as AbortSignal).aborted).toBe(true)
    wrapper.unmount()
  })

  it('labels administrator totals as site-wide rather than page-filtered costs', async () => {
    report.mockResolvedValueOnce({ is_admin: true, totals: [] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('upstreamBilling.summaryAdminScope')
    expect(wrapper.text()).not.toContain('upstreamBilling.summaryUserScope')
    wrapper.unmount()
  })

  it('does not present stale costs as a successful result after a load error', async () => {
    const wrapper = render()
    await flushPromises()
    report.mockRejectedValueOnce(new Error('offline'))
    await wrapper.setProps({ month: '2026-08' })
    await flushPromises()
    expect(wrapper.text()).toContain('upstreamBilling.summaryFailed')
    expect(wrapper.text()).not.toContain('USD 1.00000001')
    wrapper.unmount()
  })
})

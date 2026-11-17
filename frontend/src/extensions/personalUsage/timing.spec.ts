import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import TimingComparisonCell from './TimingComparisonCell.vue'
import type { UsageLog } from '@/types'
import { fetchTimingComparison, type TimingComparison } from './timing'

vi.mock('./timing', async () => ({ ...await vi.importActual('./timing'), fetchTimingComparison: vi.fn() }))
const row = { id: 1, account_id: 2, first_token_ms: 50, duration_ms: 2000, output_tokens: 10 } as UsageLog
const mountCell = (allowComparison = true) => mount(TimingComparisonCell, { props: { row, allowComparison }, global: { stubs: { Teleport: true }, plugins: [createI18n({ legacy: false, locale: 'zh', missingWarn: false, fallbackWarn: false })] } })
const exact = { status: 'exact', local: { duration_ms: 2000, first_token_ms: 50 }, station: { id: 'r', precision_ms: 1000, duration_ms: 1000, first_token_ms: 10 }, duration_difference_ms: 1000, first_token_difference_ms: 40 } as TimingComparison
async function hover(wrapper: ReturnType<typeof mountCell>) {
  await wrapper.get('[data-testid="timing-hover-target"]').trigger('mouseenter')
  await vi.advanceTimersByTimeAsync(300); await flushPromises()
}
beforeEach(() => { vi.useFakeTimers(); vi.mocked(fetchTimingComparison).mockReset() })
afterEach(() => { vi.useRealTimers() })

describe('hover station timing', () => {
  it('does not query on rendering, brief hover, or for ordinary users', async () => {
    const wrapper = mountCell()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(fetchTimingComparison).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="timing-hover-target"]').trigger('mouseenter')
    await wrapper.get('[data-testid="timing-hover-target"]').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(500)
    expect(fetchTimingComparison).not.toHaveBeenCalled()
    await wrapper.setProps({ allowComparison: false }); await hover(wrapper)
    expect(fetchTimingComparison).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('shows exact measurements on hover and reuses them without repeat queries', async () => {
    vi.mocked(fetchTimingComparison).mockResolvedValue(exact)
    const wrapper = mountCell(); await hover(wrapper)
    expect(fetchTimingComparison).toHaveBeenCalledWith(2, 1, expect.any(AbortSignal))
    expect(wrapper.get('[role="tooltip"]').text()).toContain('+1000 ms')
    expect(wrapper.text()).toContain('总耗时精度为秒')
    await wrapper.get('[data-testid="timing-hover-target"]').trigger('mouseleave')
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    await hover(wrapper)
    expect(fetchTimingComparison).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it.each(['unmatched', 'candidate', 'ambiguous', 'error'])('shows no popup for %s', async status => {
    if (status === 'error') vi.mocked(fetchTimingComparison).mockRejectedValue(new Error('synthetic-private-diagnostic'))
    else vi.mocked(fetchTimingComparison).mockResolvedValue({ ...exact, status } as TimingComparison)
    const wrapper = mountCell(); await hover(wrapper)
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('synthetic-private')
    await hover(wrapper); expect(fetchTimingComparison).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
  it('supports focus and rejects stale replies after changing usage rows', async () => {
    let resolve!: (result: TimingComparison) => void
    vi.mocked(fetchTimingComparison).mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mountCell()
    await wrapper.get('[data-testid="timing-hover-target"]').trigger('focus')
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.setProps({ row: { ...row, id: 3 } })
    resolve(exact); await flushPromises()
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

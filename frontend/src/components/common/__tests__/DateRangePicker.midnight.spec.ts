import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import DateRangePicker from '../DateRangePicker.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: ref('en') }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 30, 23, 59)) })
afterEach(() => vi.useRealTimers())
async function reopenNextDay() {
  const w = mount(DateRangePicker, { props: { startDate: '2026-09-30', endDate: '2026-09-30' }, global: { stubs: { Icon: true } } })
  await w.get('.date-picker-trigger').trigger('click')
  expect(w.findAll('input[type="date"]')[1].attributes('max')).toBe('2026-10-01')
  await w.get('.date-picker-trigger').trigger('click')
  vi.setSystemTime(new Date(2026, 9, 1, 0, 1))
  await w.get('.date-picker-trigger').trigger('click')
  return w
}
describe('date presets after midnight', () => {
  it.each([
    ['dates.today', '2026-10-01'],
    ['dates.last7Days', '2026-09-25'],
    ['dates.thisMonth', '2026-10-01'],
  ])('refreshes %s when the page stays mounted overnight', async (label, startDate) => {
    const w = await reopenNextDay()
    await w.findAll('.date-picker-preset').find(b => b.text() === label)!.trigger('click')
    await w.get('.date-picker-apply').trigger('click')
    expect(w.emitted('change')?.[0]?.[0]).toMatchObject({ startDate, endDate: '2026-10-01' })
  })
  it('updates the maximum selectable date when reopened', async () => {
    const w = await reopenNextDay()
    expect(w.findAll('input[type="date"]')[1].attributes('max')).toBe('2026-10-02')
  })
})

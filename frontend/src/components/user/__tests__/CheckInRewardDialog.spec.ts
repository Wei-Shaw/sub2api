import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import CheckInRewardDialog from '../CheckInRewardDialog.vue'
import zh from '@/i18n/locales/zh/checkin'
import en from '@/i18n/locales/en/checkin'

enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.spyOn(Math, 'random').mockReturnValue(0)
})
afterEach(() => vi.restoreAllMocks())

function render(date: string, locale = 'zh', show = true) {
  return mount(CheckInRewardDialog, {
    props: { show, record: { id: 1, date, reward: 7, created_at: `${date}T08:00:00+08:00` } },
    global: {
      plugins: [createI18n({
        legacy: false, locale, missingWarn: false, fallbackWarn: false,
        // Vitest 使用无消息编译器的 runtime 构建；这些祝福均为纯文本。
        messageCompiler: (message) => () => String(message),
        messages: { zh, en }
      })],
      stubs: { teleport: true }
    }
  })
}

describe('CheckInRewardDialog', () => {
  it.each([
    ['2026-09-24', null],
    ['2026-09-25', 'midAutumn'],
    ['2026-09-30', 'midAutumn'],
    ['2026-10-01', 'nationalDay'],
    ['2026-10-07', 'nationalDay'],
    ['2026-10-08', null],
    ['2027-09-25', null]
  ] as const)('uses the recorded date %s for the campaign greeting', (date, holiday) => {
    const wrapper = render(date)
    expect(wrapper.text()).toContain('+$7.00')
    for (const festival of ['midAutumn', 'nationalDay'] as const) {
      const title = zh.checkIn[`${festival}Title`]
      const blessing = zh.checkIn[`${festival}Blessing`]
      expect(wrapper.text().includes(title)).toBe(festival === holiday)
      expect(wrapper.text().includes(blessing)).toBe(festival === holiday)
    }
  })

  it('localizes the greeting and closes on confirmation', async () => {
    vi.mocked(Math.random).mockReturnValue(0.99)
    const wrapper = render('2026-10-01', 'en')
    expect(wrapper.text()).toContain(en.checkIn.nationalDayTitle)
    expect(wrapper.text()).toContain(en.checkIn.nationalDayBlessing5)
    await wrapper.get('.modal-footer button').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('picks a greeting on opening and keeps it stable during reward refreshes', async () => {
    const wrapper = render('2026-09-25', 'zh', false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('7.00')
    expect(Math.random).not.toHaveBeenCalled()

    await wrapper.setProps({ show: true })
    expect(wrapper.text()).toContain(zh.checkIn.midAutumnBlessing)
    expect(Math.random).toHaveBeenCalledTimes(1)

    vi.mocked(Math.random).mockReturnValue(0.99)
    await wrapper.setProps({ record: { id: 1, date: '2026-09-25', reward: 8, created_at: '' } })
    expect(wrapper.text()).toContain('+$8.00')
    expect(wrapper.text()).toContain(zh.checkIn.midAutumnBlessing)
    expect(Math.random).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    expect(wrapper.text()).toContain(zh.checkIn.midAutumnBlessing5)
    expect(Math.random).toHaveBeenCalledTimes(2)
  })
})

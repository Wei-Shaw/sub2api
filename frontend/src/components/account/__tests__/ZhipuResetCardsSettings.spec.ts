import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ZhipuResetCardsSettings from '../ZhipuResetCardsSettings.vue'

const { get, act, quota, start, poll } = vi.hoisted(() => ({
  get: vi.fn(), act: vi.fn(), quota: vi.fn(), start: vi.fn(), poll: vi.fn()
}))
vi.mock('@/api/admin/zhipuResetCards', () => ({
  getZhipuResetCards: get,
  actZhipuResetCard: act,
  startZhipuResetAuthorization: start,
  pollZhipuResetAuthorization: poll,
  unbindZhipuResetAuthorization: vi.fn()
}))
vi.mock('@/api/admin/cnProviders', () => ({ queryQuota: quota }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: {
    props: ['show'],
    emits: ['confirm', 'cancel'],
    template: `<div v-if="show"><slot />
      <button data-testid="confirm-ok" @click="$emit('confirm')">confirm</button>
    </div>`
  }
}))

const cards = {
  available_five_hour_resets: [{ expire_at: 4102444800000 }],
  available_week_resets: []
}

describe('GLM reset cards', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    get.mockResolvedValue({ status: 'bound', binding_id: 'binding-1', cards })
    quota.mockResolvedValue({})
  })

  it('waits for browser approval and never claims or uses a card as part of authorization', async () => {
    get.mockResolvedValue({ status: 'unbound' })
    start.mockResolvedValue({
      attempt_id: 'attempt-one', authorize_url: 'https://bigmodel.cn/login',
      expires_at: new Date(Date.now() + 120000).toISOString(), poll_interval: 2
    })
    poll.mockResolvedValueOnce({ status: 'pending' })
      .mockResolvedValueOnce({ status: 'bound', state: { status: 'bound', binding_id: 'binding-1', cards } })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42 } })
    await flushPromises()
    vi.useFakeTimers()
    try {
      await wrapper.get('[data-testid="zhipu-reset-auth"]').trigger('click')
      expect(wrapper.get('a').attributes('href')).toBe('https://bigmodel.cn/login')
      expect(poll).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(2000)
      expect(poll).toHaveBeenCalledWith(42, 'attempt-one', expect.any(AbortSignal))
      expect(wrapper.find('[data-testid="zhipu-reset-use-FIVE_HOUR"]').exists()).toBe(false)
      await vi.advanceTimersByTimeAsync(2000)
      expect(wrapper.find('[data-testid="zhipu-reset-use-FIVE_HOUR"]').exists()).toBe(true)
      expect(act).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it('only uses a card after confirmation, and refreshes quota on confirmed use', async () => {
    act.mockResolvedValue({ outcome: 'used', cards: { ...cards, available_five_hour_resets: [] } })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42 } })
    await flushPromises()
    expect(get).toHaveBeenCalledWith(42, expect.any(AbortSignal))
    expect(wrapper.get('[data-testid="zhipu-reset-use-WEEK"]').attributes()).toHaveProperty('disabled')
    const use = wrapper.get('[data-testid="zhipu-reset-use-FIVE_HOUR"]')
    await use.trigger('click')
    expect(act).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(act).toHaveBeenCalledTimes(1)
    expect(act).toHaveBeenCalledWith(42, 'binding-1', 'FIVE_HOUR', expect.any(String))
    expect(quota).toHaveBeenCalledTimes(1)
    expect(quota).toHaveBeenCalledWith(42)
    expect(use.attributes()).toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('shows separate count and reset actions, and refreshes quota after a confirmed reset', async () => {
    act.mockResolvedValue({ outcome: 'used', cards: { ...cards, available_five_hour_resets: [] } })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42, compact: true } })
    const count = wrapper.get('[data-testid="zhipu-reset-query"]')
    const reset = wrapper.get('[data-testid="zhipu-reset-action"]')
    await flushPromises()
    expect(get).not.toHaveBeenCalled()
    expect(count.text()).toContain('listCount')
    expect(reset.attributes()).toHaveProperty('disabled')
    await count.trigger('click')
    await flushPromises()
    expect(get).toHaveBeenCalledTimes(1)
    expect(count.text()).toContain('1')
    expect(reset.attributes()).not.toHaveProperty('disabled')
    expect(act).not.toHaveBeenCalled()
    await reset.trigger('click')
    expect(wrapper.find('[data-testid="zhipu-reset-window-WEEK"]').exists()).toBe(false)
    expect(get).toHaveBeenCalledTimes(1)
    expect(act).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(act).toHaveBeenCalledTimes(1)
    expect(act).toHaveBeenCalledWith(42, 'binding-1', 'FIVE_HOUR', expect.any(String))
    expect(wrapper.emitted('redeemed')).toEqual([[]])
    expect(quota).not.toHaveBeenCalled()
    expect(count.text()).toContain('0')
    expect(reset.attributes()).toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('lets the operator select one window when both have cards', async () => {
    get.mockResolvedValue({ status: 'bound', binding_id: 'binding-1', cards: {
      ...cards, available_week_resets: [{ expire_at: 4102444800000 }]
    } })
    act.mockResolvedValue({ outcome: 'used', cards })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42, compact: true } })
    await wrapper.get('[data-testid="zhipu-reset-query"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="zhipu-reset-query"]').text()).toContain('2')
    await wrapper.get('[data-testid="zhipu-reset-action"]').trigger('click')
    await wrapper.get('[data-testid="zhipu-reset-window-WEEK"]').setValue(true)
    expect(act).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(act).toHaveBeenCalledTimes(1)
    expect(act).toHaveBeenCalledWith(42, 'binding-1', 'WEEK', expect.any(String))
    expect(wrapper.get('[data-testid="zhipu-reset-query"]').text()).toContain('1')
    wrapper.unmount()
  })

  it('hides the stale count after an unconfirmed result until queried again', async () => {
    act.mockRejectedValue({ reason: 'ZHIPU_RESET_OUTCOME_UNKNOWN' })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42, compact: true } })
    const count = wrapper.get('[data-testid="zhipu-reset-query"]')
    const reset = wrapper.get('[data-testid="zhipu-reset-action"]')
    await count.trigger('click')
    await flushPromises()
    await reset.trigger('click')
    await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(act).toHaveBeenCalledTimes(1)
    expect(count.text()).not.toContain('1')
    expect(reset.attributes()).toHaveProperty('disabled')
    expect(wrapper.get('[role=alert]').text()).toContain('actionUnknown')
    await count.trigger('click')
    await flushPromises()
    expect(count.text()).toContain('1')
    expect(reset.attributes()).not.toHaveProperty('disabled')
    wrapper.unmount()
  })

  it('does not replay an uncertain use and requires a fresh status read', async () => {
    act.mockRejectedValue({ reason: 'ZHIPU_RESET_OUTCOME_UNKNOWN' })
    const wrapper = mount(ZhipuResetCardsSettings, { props: { accountId: 42 } })
    await flushPromises()
    const use = wrapper.get('[data-testid="zhipu-reset-use-FIVE_HOUR"]')
    await use.trigger('click')
    await wrapper.get('[data-testid="confirm-ok"]').trigger('click')
    await flushPromises()
    expect(act).toHaveBeenCalledTimes(1)
    expect(quota).not.toHaveBeenCalled()
    expect(use.attributes()).toHaveProperty('disabled')
    expect(wrapper.get('[role=alert]').text()).toContain('actionUnknown')
    wrapper.unmount()
  })
})

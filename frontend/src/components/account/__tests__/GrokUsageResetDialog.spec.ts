import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GrokUsageResetDialog from '../GrokUsageResetDialog.vue'
import type { Account } from '@/types'

const { queryUsageResetCards, redeemUsageResetCard } = vi.hoisted(() => ({
  queryUsageResetCards: vi.fn(),
  redeemUsageResetCard: vi.fn()
}))

vi.mock('@/api/admin/grok', () => ({ queryUsageResetCards, redeemUsageResetCard }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))

const card = { token_id: 'card-one', valid_from: '2026-01-01T00:00:00Z', expires_at: '2100-01-01T00:00:00Z' }
const account = { id: 12, name: 'Test Grok', platform: 'grok', type: 'oauth' } as Account

function setup() {
  return mount(GrokUsageResetDialog, {
    props: { show: true, account },
    global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } }
  })
}

function button(wrapper: ReturnType<typeof setup>, label: string) {
  const match = wrapper.findAll('button').find(item => item.text() === label)
  expect(match).toBeDefined()
  return match!
}

async function loadCards(wrapper: ReturnType<typeof setup>) {
  await wrapper.get('input[type=password]').setValue('web-session-secret')
  await button(wrapper, 'admin.accounts.grokReset.query').trigger('click')
  await flushPromises()
}

describe('GrokUsageResetDialog', () => {
  beforeEach(() => {
    queryUsageResetCards.mockReset().mockResolvedValue({ cards: [card] })
    redeemUsageResetCard.mockReset().mockResolvedValue({ cards: [] })
  })

  it('queries without redeeming and requires explicit confirmation', async () => {
    const wrapper = setup()
    await loadCards(wrapper)
    expect(queryUsageResetCards).toHaveBeenCalledWith(12, 'web-session-secret')
    expect(redeemUsageResetCard).not.toHaveBeenCalled()
    await button(wrapper, 'admin.accounts.grokReset.redeem').trigger('click')
    expect(wrapper.text()).toContain('admin.accounts.grokReset.confirmMessage')
    expect(redeemUsageResetCard).not.toHaveBeenCalled()
    await button(wrapper, 'admin.accounts.grokReset.confirm').trigger('click')
    await flushPromises()
    expect(redeemUsageResetCard).toHaveBeenCalledTimes(1)
    expect(redeemUsageResetCard).toHaveBeenCalledWith(12, 'web-session-secret', 'card-one')
    expect(wrapper.emitted('redeemed')).toHaveLength(1)
    expect(wrapper.find('input[type=password]').exists()).toBe(false)
  })

  it('requires another query after an uncertain redemption and hides raw errors', async () => {
    redeemUsageResetCard.mockRejectedValue(new Error('timeout with web-session-secret'))
    const wrapper = setup()
    await loadCards(wrapper)
    await button(wrapper, 'admin.accounts.grokReset.redeem').trigger('click')
    await button(wrapper, 'admin.accounts.grokReset.confirm').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.grokReset.redeemUnknown')
    expect(wrapper.text()).not.toContain('web-session-secret')
    expect(wrapper.find('input[type=radio]').exists()).toBe(false)
    expect(redeemUsageResetCard).toHaveBeenCalledTimes(1)
  })

  it('clears SSO and cards on close and ignores a late query for another account', async () => {
    let resolveQuery!: (value: { cards: typeof card[] }) => void
    queryUsageResetCards.mockReturnValue(new Promise(resolve => { resolveQuery = resolve }))
    const wrapper = setup()
    await loadCards(wrapper)
    await wrapper.setProps({ account: { ...account, id: 13 } })
    resolveQuery({ cards: [card] })
    await flushPromises()
    expect((wrapper.get('input[type=password]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.find('input[type=radio]').exists()).toBe(false)
    await wrapper.get('input[type=password]').setValue('new-session')
    await button(wrapper, 'common.close').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect((wrapper.get('input[type=password]').element as HTMLInputElement).value).toBe('')
  })

  it('drops the selected card when the supplied web session changes', async () => {
    const wrapper = setup()
    await loadCards(wrapper)
    await wrapper.get('input[type=password]').setValue('different-web-session')
    expect(wrapper.find('input[type=radio]').exists()).toBe(false)
    expect(redeemUsageResetCard).not.toHaveBeenCalled()
  })

  it('does not submit duplicate redemptions while the first is pending', async () => {
    let resolveRedeem!: (value: { cards: typeof card[] }) => void
    redeemUsageResetCard.mockReturnValue(new Promise(resolve => { resolveRedeem = resolve }))
    const wrapper = setup()
    await loadCards(wrapper)
    await button(wrapper, 'admin.accounts.grokReset.redeem').trigger('click')
    const confirm = button(wrapper, 'admin.accounts.grokReset.confirm')
    await confirm.trigger('click')
    await confirm.trigger('click')
    expect(redeemUsageResetCard).toHaveBeenCalledTimes(1)
    resolveRedeem({ cards: [] })
    await flushPromises()
  })
})

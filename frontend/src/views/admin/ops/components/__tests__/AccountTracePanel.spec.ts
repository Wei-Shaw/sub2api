import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AccountLifecycleEvent, AccountTrace, TraceAttempt } from '@/api/admin/opsTrace'
import AccountTracePanel from '../trace/AccountTracePanel.vue'
import AccountLedgerTimeline from '../trace/AccountLedgerTimeline.vue'
import AccountStatusTimeline from '../trace/AccountStatusTimeline.vue'
import { expiryDeltaDays, summarizeAttempts } from '../trace/accountTraceUtils'
import zhLocale from '@/i18n/locales/zh'
import enLocale from '@/i18n/locales/en'

const { getAccountTrace, getAccountLedger, copyToClipboard } = vi.hoisted(() => ({
  getAccountTrace: vi.fn(),
  getAccountLedger: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/admin/opsTrace', () => ({ getAccountTrace, getAccountLedger }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  // 带参数时把参数拼进去，便于断言插值内容
  useI18n: () => ({ t: (key: string, p?: Record<string, unknown>) => (p ? `${key}${JSON.stringify(p)}` : key) })
}))

const FP_OLD = 'aaaa1111bbbb2222'
const FP_NEW = 'cccc3333dddd4444'

function attempt(kind: string, fp: string, created_at: string, extra: Partial<TraceAttempt> = {}): TraceAttempt {
  return { attempt_no: 1, account_id: 9201, platform: 'anthropic', failure_kind: kind, credential_fp: fp, created_at, ...extra }
}

const attempts: TraceAttempt[] = [
  attempt('auth', FP_NEW, '2026-11-11T10:00:00Z', { upstream_status: 401, error_code: 'invalid_api_key', retry_from: 9100 }),
  attempt('auth', FP_NEW, '2026-11-11T11:30:00Z', { upstream_status: 401 }),
  attempt('rate_limit', FP_OLD, '2026-11-11T08:00:00Z', { upstream_status: 429 }),
  attempt('overload', FP_OLD, '2026-11-11T07:00:00Z', { upstream_status: 529 }),
  attempt('client', FP_NEW, '2026-11-11T09:00:00Z', { upstream_status: 400 }),
  attempt('none', FP_NEW, '2026-11-11T12:00:00Z', { upstream_status: 200 })
]

function ev(id: number, event_type: string, changes: Record<string, unknown>, extra: Partial<AccountLifecycleEvent> = {}): AccountLifecycleEvent {
  return { id, account_id: 9201, event_type, changes, source: 'db', created_at: `2026-11-${String(id).padStart(2, '0')}T00:00:00Z`, ...extra }
}

const ledgerEvents: AccountLifecycleEvent[] = [
  ev(1, 'created', { platform: 'anthropic', type: 'apikey', status: 'active', expires_at: '2026-12-01T00:00:00Z', plan: { plan_type: 'pro' } }, { credential_fp_after: FP_OLD }),
  ev(2, 'renewed', { expires_at: { from: '2026-12-01T00:00:00Z', to: '2026-12-31T00:00:00Z' } }, {
    expires_at_before: '2026-12-01T00:00:00Z', expires_at_after: '2026-12-31T00:00:00Z', credential_fp_before: FP_OLD, credential_fp_after: FP_OLD
  }),
  ev(3, 'plan_changed', { 'extra.plan_type': { from: 'pro', to: 'max' } }, { credential_fp_before: FP_OLD, credential_fp_after: FP_OLD }),
  ev(4, 'credential_rotated', { credential_fp: { from: FP_OLD, to: FP_NEW } }, { credential_fp_before: FP_OLD, credential_fp_after: FP_NEW }),
  ev(5, 'token_refreshed', { credential_fp: { from: FP_NEW, to: 'eeee5555ffff6666' } }, { credential_fp_before: FP_NEW, credential_fp_after: 'eeee5555ffff6666' }),
  ev(6, 'expiry_shortened', { expires_at: { from: '2026-12-31T00:00:00Z', to: '2026-12-28T12:00:00Z' } }, { credential_fp_before: FP_NEW, credential_fp_after: FP_NEW }),
  ev(7, 'disabled', { status: { from: 'active', to: 'disabled' }, schedulable: { from: true, to: false } }, { credential_fp_before: FP_NEW, credential_fp_after: FP_NEW })
]

const trace: AccountTrace = {
  account_id: 9201,
  since: '2026-11-10T12:00:00Z',
  attempts,
  status_events: [
    { account_id: 9201, from_state: 'active', to_state: 'cooling', reason: 'rate_limit', cooling_until: '2026-11-11T08:30:00Z', source: 'classify', request_id: 'req-cool-1', created_at: '2026-11-11T08:00:00Z' },
    { account_id: 9201, from_state: 'active', to_state: 'invalid', reason: 'auth', source: 'refresh', created_at: '2026-11-11T11:30:00Z' }
  ],
  lifecycle_events: []
}

const BaseDialogStub = { props: ['show', 'title'], template: '<div v-if="show"><slot /></div>' }

async function mountPanel() {
  const wrapper = mount(AccountTracePanel, {
    props: { show: true, account: { id: 9201, name: 'demo-acct-a', platform: 'anthropic' } },
    global: { stubs: { BaseDialog: BaseDialogStub, LoadingSpinner: true, Icon: true } }
  })
  await flushPromises()
  return wrapper
}

describe('accountTraceUtils', () => {
  it('summarizes failure_kind distribution, ignoring successful attempts', () => {
    const s = summarizeAttempts(attempts)
    expect(s.total).toBe(5)
    expect(s.byKind).toEqual({ auth: 2, rate_limit: 1, overload: 1, transient: 0, client: 1 })
    expect(s.lastFailureAt).toBe('2026-11-11T11:30:00Z')
    expect(s.fingerprints.sort()).toEqual([FP_OLD, FP_NEW].sort())
  })

  it('computes expiry delta in days (extended / shortened)', () => {
    expect(expiryDeltaDays(ledgerEvents[1])).toBe(30)
    expect(expiryDeltaDays(ledgerEvents[5])).toBe(-2.5)
    expect(expiryDeltaDays(ledgerEvents[2])).toBeNull()
  })
})

describe('AccountTracePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getAccountTrace.mockResolvedValue(trace)
    getAccountLedger.mockResolvedValue({ account_id: 9201, events: ledgerEvents })
  })

  it('loads 24h trace by default and renders the failure_kind distribution', async () => {
    const wrapper = await mountPanel()
    expect(getAccountTrace).toHaveBeenCalledWith(9201, { hours: 24, limit: 200 })
    expect(getAccountLedger).toHaveBeenCalledWith(9201, { limit: 500 })
    expect(wrapper.get('[data-testid="failure-total"]').text()).toBe('5')
    const count = (k: string) => wrapper.get(`[data-testid="kind-${k}"] [data-testid="kind-count"]`).text()
    expect([count('auth'), count('rate_limit'), count('overload'), count('transient'), count('client')]).toEqual(['2', '1', '1', '0', '1'])
    expect(wrapper.get('[data-testid="fp-count"]').text()).toBe('2')
    // 成功尝试不进失败列表
    expect(wrapper.findAll('[data-testid="attempt-row"]')).toHaveLength(5)
    expect(wrapper.findAll('[data-testid="failover-badge"]')).toHaveLength(1)
  })

  it('switches the time window to 1h / 7d', async () => {
    const wrapper = await mountPanel()
    await wrapper.get('[data-testid="range-168"]').trigger('click')
    await flushPromises()
    expect(getAccountTrace).toHaveBeenLastCalledWith(9201, { hours: 168, limit: 200 })
    await wrapper.get('[data-testid="range-1"]').trigger('click')
    await flushPromises()
    expect(getAccountTrace).toHaveBeenLastCalledWith(9201, { hours: 1, limit: 200 })
  })

  it('links failing fingerprints in the overview to the ledger rotation time', async () => {
    const wrapper = await mountPanel()
    const chip = wrapper.findAll('[data-testid="fp-chip"]').find((c) => c.attributes('data-fp') === FP_NEW)
    expect(chip?.attributes('title')).toContain('admin.traceAccount.overview.fpSwitchedAt')
    expect(chip?.attributes('title')).toContain('time')
  })

  it('shows status events with translated source and owner', async () => {
    const wrapper = await mountPanel()
    await wrapper.get('[data-testid="tab-status"]').trigger('click')
    const rows = wrapper.findAll('[data-testid="status-event"]')
    expect(rows).toHaveLength(2)
    // 新 → 旧：先 invalid(refresh)，再 cooling(classify)
    expect(rows[0].text()).toContain('admin.traceAccount.status.enterInvalid')
    expect(rows[0].text()).toContain('admin.traceAccount.status.sources.refresh.owner')
    expect(rows[1].text()).toContain('admin.traceAccount.status.coolingUntil')
    expect(rows[1].text()).toContain('{"minutes":30}')
    expect(rows[1].text()).toContain('req-cool-1')
  })

  it('shows an error state with retry when the trace request fails', async () => {
    getAccountTrace.mockRejectedValueOnce(new Error('boom'))
    const wrapper = await mountPanel()
    expect(wrapper.get('[data-testid="trace-error"]').text()).toContain('boom')
    await wrapper.get('[data-testid="trace-error"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="trace-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="failure-total"]').text()).toBe('5')
  })
})

describe('AccountLedgerTimeline', () => {
  const failuresByFp = { [FP_NEW]: 3, [FP_OLD]: 2 }

  function mountLedger(events: AccountLifecycleEvent[]) {
    return mount(AccountLedgerTimeline, { props: { events, failuresByFp }, global: { stubs: { Icon: true } } })
  }

  it('renders changes as old → new with expiry day delta', () => {
    const wrapper = mountLedger(ledgerEvents)
    const renewed = wrapper.get('[data-type="renewed"]')
    expect(renewed.get('[data-testid="event-label"]').text()).toBe('admin.traceAccount.ledger.events.renewed')
    const row = renewed.get('[data-field="expires_at"]')
    expect(row.get('[data-testid="change-from"]').text()).not.toBe(row.get('[data-testid="change-to"]').text())
    expect(row.get('[data-testid="expiry-delta"]').text()).toBe('admin.traceAccount.ledger.extended{"days":30}')

    const shortened = wrapper.get('[data-type="expiry_shortened"] [data-testid="expiry-delta"]')
    expect(shortened.text()).toBe('admin.traceAccount.ledger.shortened{"days":2.5}')

    const plan = wrapper.get('[data-type="plan_changed"] [data-field="extra.plan_type"]')
    expect(plan.get('[data-testid="change-from"]').text()).toBe('pro')
    expect(plan.get('[data-testid="change-to"]').text()).toBe('max')

    const disabled = wrapper.get('[data-type="disabled"] [data-field="schedulable"]')
    expect(disabled.get('[data-testid="change-from"]').text()).toBe('admin.traceAccount.ledger.yes')
    expect(disabled.get('[data-testid="change-to"]').text()).toBe('admin.traceAccount.ledger.no')

    // created 是快照：只有新值
    const created = wrapper.get('[data-type="created"]')
    expect(created.find('[data-testid="change-from"]').exists()).toBe(false)
    expect(created.get('[data-field="extra.plan_type"] [data-testid="change-to"]').text()).toBe('pro')
  })

  it('highlights credential fingerprint changes and matches them with failures', () => {
    const wrapper = mountLedger(ledgerEvents)
    const highlighted = wrapper.findAll('[data-credential-changed="true"]')
    // token_refreshed 默认隐藏，只剩 credential_rotated
    expect(highlighted).toHaveLength(1)
    const rotated = wrapper.get('[data-type="credential_rotated"]')
    const chips = rotated.findAll('[data-testid="fp-chip"]').map((c) => c.attributes('data-fp'))
    expect(chips).toEqual([FP_OLD, FP_NEW])
    expect(rotated.get('[data-testid="credential-failures"]').text()).toContain('{"count":3}')
    // 指纹已在高亮块展示，不重复出现在 changes 行
    expect(rotated.find('[data-field="credential_fp"]').exists()).toBe(false)
    // 未换 key 的事件不高亮
    expect(wrapper.get('[data-type="renewed"] > div').attributes('data-credential-changed')).toBeUndefined()
  })

  it('hides token_refreshed by default and shows them when toggled off', async () => {
    const wrapper = mountLedger(ledgerEvents)
    expect(wrapper.find('[data-type="token_refreshed"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="hidden-count"]').text()).toContain('{"count":1}')
    const toggle = wrapper.get<HTMLInputElement>('[data-testid="toggle-token-refresh"]')
    expect(toggle.element.checked).toBe(true)
    await toggle.setValue(false)
    expect(wrapper.find('[data-type="token_refreshed"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="ledger-event"]')).toHaveLength(ledgerEvents.length)
    expect(wrapper.findAll('[data-credential-changed="true"]')).toHaveLength(2)
  })

  it('explains the empty ledger (accounts created before the ledger was enabled)', () => {
    const wrapper = mountLedger([])
    const empty = wrapper.get('[data-testid="ledger-empty"]')
    expect(empty.text()).toContain('admin.traceAccount.ledger.emptyTitle')
    expect(empty.text()).toContain('admin.traceAccount.ledger.emptyDesc')
    expect((zhLocale as any).admin.traceAccount.ledger.emptyDesc).toContain('台账启用之前')
  })

  it('distinguishes "only token refreshes, all hidden" from an empty ledger', () => {
    const wrapper = mountLedger([ledgerEvents[4]])
    expect(wrapper.find('[data-testid="ledger-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ledger-all-hidden"]').exists()).toBe(true)
  })
})

describe('AccountStatusTimeline', () => {
  it('renders an empty hint without events', () => {
    const wrapper = mount(AccountStatusTimeline, { props: { events: [] }, global: { stubs: { Icon: true } } })
    expect(wrapper.find('[data-testid="status-empty"]').exists()).toBe(true)
  })
})

describe('traceAccount i18n', () => {
  function keys(obj: unknown, prefix = ''): string[] {
    if (!obj || typeof obj !== 'object') return [prefix]
    return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => keys(v, prefix ? `${prefix}.${k}` : k))
  }

  it('zh and en have identical keys', () => {
    const zh = keys((zhLocale as any).admin.traceAccount).sort()
    const en = keys((enLocale as any).admin.traceAccount).sort()
    expect(zh.length).toBeGreaterThan(50)
    expect(zh).toEqual(en)
  })
})

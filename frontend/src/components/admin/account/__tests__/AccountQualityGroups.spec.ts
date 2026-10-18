import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AccountQualityGroups from '../AccountQualityGroups.vue'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'

const api = vi.hoisted(() => ({
  groups: vi.fn(),
  page: vi.fn(),
  runs: vi.fn(),
  results: vi.fn(),
  save: vi.fn(),
  run: vi.fn(),
  cancel: vi.fn(),
  preview: vi.fn(),
  events: vi.fn(),
  setSchedulable: vi.fn()
}))
vi.mock('@/api/admin/accountQuality', () => ({
  qualityRoutingPreview: api.preview,
  qualityRoutingEvents: api.events,
  qualityCatalog: async () => ({
    reference_models: ['reference-model'],
    bank_revision: 'test'
  }),
  listQualityGroups: api.groups,
  qualityPage: api.page,
  qualityRuns: api.runs,
  qualityResults: api.results,
  saveQualityPolicy: api.save,
  runQuality: api.run,
  cancelQuality: api.cancel
}))
vi.mock('@/api/admin/accounts', () => ({ setSchedulable: api.setSchedulable }))
vi.mock('vue-i18n', async () => {
  const { ref } = await import('vue')
  return { useI18n: () => ({ t: (key: string) => key, locale: ref('en') }) }
})
let wrapper: ReturnType<typeof mount> | undefined
const policy = {
  enabled: false,
  model: '',
  effort: 'low',
  service_tier: '',
  interval_minutes: 15,
  daily_budget: 200,
  max_accounts: 50,
  endpoint: 'responses',
  trace_on_anomaly: false,
  reference_model: '',
  context_check: false,
  context_chars: 32000,
  score_weights: { quality: 40, ttft: 25, cache: 15, stability: 20 },
  mode: 'observe'
}
const account = {
  id: 7,
  name: 'Pinned account',
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  concurrency: 10,
  capacity: { platform: 'openai', type: 'apikey', concurrency: 10, current_concurrency: 3, quota_daily_limit: 20, quota_daily_used: 5 },
  candy_status: 'untested',
  quality_samples: 0,
  quality_passed: 0,
  last_test_at: null,
  p50_ms: null,
  p95_ms: null,
  cache_rate: null,
  score: null,
  suggested_weight: null,
  failures: 0,
  reliability_known: false,
  success_rate: null,
  trace_status: 'untested',
  trace_candidate: '',
  effective_multiplier: null,
  components: [],
  requests: 0,
  ttft_samples: 0
}
async function render(overrides: Record<string, unknown> = {}) {
  const fixturePolicy = { ...policy, ...overrides }
  let accountSnapshot = { ...account }
  api.setSchedulable.mockImplementation(async (_id, schedulable) => { accountSnapshot = { ...accountSnapshot, schedulable }; return { id: 7, schedulable } })
  api.groups.mockResolvedValue([
    {
      id: 1,
      name: 'Group A',
      platform: 'openai',
      total: 9,
      available: 9,
      policy: fixturePolicy
    },
    {
      id: 2,
      name: 'Group B',
      platform: 'openai',
      total: 1,
      available: 1,
      policy
    }
  ])
  api.page.mockImplementation(async (id: number, params: { page: number }) => ({
    items: [{ ...accountSnapshot }],
    page: params.page,
    page_size: 8,
    total: id === 1 ? 9 : 1,
    window_hours: 24,
    model: '',
    mode: 'observe'
  }))
  api.runs.mockResolvedValue([])
  api.results.mockResolvedValue([])
  api.preview.mockResolvedValue({
    configured_mode: 'observe',
    applied: false,
    reason: 'not_automatic',
    weights: {}
  })
  api.events.mockResolvedValue([])
  api.save.mockImplementation(async (_id, p) => p)
  api.run.mockResolvedValue({
    id: 1,
    status: 'queued',
    planned: 3,
    completed: 0,
    benchmark: 'candy'
  })
  wrapper = mount(AccountQualityGroups, {
    global: { stubs: { Teleport: true } }
  })
  await flushPromises()
  return wrapper
}
afterEach(() => {
  wrapper?.unmount()
  vi.clearAllMocks()
})
describe('group account observations', () => {
  it('renders the original capacity cell with runtime concurrency and configured quotas', async () => {
    const view = await render()
    const cells = view.findAllComponents(AccountCapacityCell)
    expect(cells).toHaveLength(2)
    expect(cells[0].props('account')).toEqual(account.capacity)
    expect(cells[0].text()).toContain('3/10')
    expect(cells[0].text()).toContain('$5.00/$20.00')
  })
  it('saves one scheduling change and synchronizes duplicate account cards', async () => {
    const view = await render()
    await view.findAll('[role="switch"]')[0].trigger('click')
    await flushPromises()
    expect(api.setSchedulable).toHaveBeenCalledTimes(1)
    expect(api.setSchedulable).toHaveBeenCalledWith(7, false)
    expect(view.findAll('[role="switch"]').map(s => s.attributes('aria-checked'))).toEqual(['false', 'false'])
    expect(view.emitted('schedulingChange')?.at(-1)).toEqual([7, false])
    await view.findAll('[role="switch"]')[1].trigger('click')
    await flushPromises()
    expect(api.setSchedulable).toHaveBeenLastCalledWith(7, true)
    expect(view.findAll('[role="switch"]').map(s => s.attributes('aria-checked'))).toEqual(['true', 'true'])
    expect(api.run).not.toHaveBeenCalled()
  })
  it('locks duplicate switches while saving and preserves state on failure', async () => {
    const view = await render()
    let rejectSave!: (error: Error) => void
    api.setSchedulable.mockReturnValueOnce(new Promise((_resolve, reject) => { rejectSave = reject }))
    await view.findAll('[role="switch"]')[0].trigger('click')
    expect(view.findAll('[role="switch"]').every(s => s.attributes('disabled') !== undefined)).toBe(true)
    rejectSave(new Error('upstream database unavailable'))
    await flushPromises()
    expect(view.findAll('[role="switch"]').map(s => s.attributes('aria-checked'))).toEqual(['true', 'true'])
    expect(view.findAll('[role="switch"]').every(s => s.attributes('disabled') === undefined)).toBe(true)
    expect(view.text()).toContain('admin.accounts.failedToToggleSchedulable')
    expect(view.emitted('schedulingChange')).toBeUndefined()
  })
  it('loads each group independently and does not fabricate missing quality', async () => {
    const view = await render()
    expect(view.findAll('article')).toHaveLength(2)
    expect(api.page).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ page: 1, page_size: 8 }),
      expect.any(AbortSignal)
    )
    expect(api.page).toHaveBeenCalledWith(
      2,
      expect.objectContaining({ page: 1 }),
      expect.any(AbortSignal)
    )
    expect(view.text()).toContain('accountQuality.noData')
    expect(view.text()).toContain('accountQuality.untested')
    expect(api.run).not.toHaveBeenCalled()
  })
  it('paginates only the selected group', async () => {
    const view = await render()
    api.page.mockClear()
    const next = view
      .findAll('article')[0]
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.next')
    await next!.trigger('click')
    await flushPromises()
    expect(api.page).toHaveBeenCalledTimes(1)
    expect(api.page).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ page: 2 }),
      expect.any(AbortSignal)
    )
  })
  it('requires model configuration before starting any paid inspection', async () => {
    const view = await render()
    const inspect = view
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.inspect')
    await inspect!.trigger('click')
    await flushPromises()
    expect(view.text()).toContain('accountQuality.configure')
    expect(api.run).not.toHaveBeenCalled()
    expect(api.save).not.toHaveBeenCalled()
  })
  it('shows an empty evidence state without inventing test results', async () => {
    const view = await render()
    const evidence = view
      .findAll('tbody button')
      .find((b) => b.text() === 'accountQuality.history')
    await evidence!.trigger('click')
    await flushPromises()
    expect(api.results).toHaveBeenCalledWith(1, 7)
    expect(view.text()).toContain('accountQuality.noHistory')
  })
  it('deduplicates the same account across multiple group cards', async () => {
    const view = await render()
    const rows = view.findAll(
      'input[aria-label="accountQuality.selectAccount"]'
    )
    expect(rows).toHaveLength(2)
    await rows[0].setValue(true)
    const first = view.emitted('selection')!.at(-1)![0] as { id: number }[]
    expect(first.map((r) => r.id)).toEqual([7])
    await view.setProps({ selectedIds: [7] })
    expect((rows[1].element as HTMLInputElement).checked).toBe(true)
    await rows[1].setValue(false)
    expect(view.emitted('selection')!.at(-1)![0]).toEqual([])
  })
  it('sends an explicitly selected local fingerprint benchmark after confirmation', async () => {
    const view = await render({
      model: 'fixture-model',
      reference_model: 'reference-model'
    })
    await view
      .find('select[aria-label="accountQuality.benchmark"]')
      .setValue('modeltrace')
    await view
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.inspect')!
      .trigger('click')
    expect(api.run).not.toHaveBeenCalled()
    await view
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.start')!
      .trigger('click')
    await flushPromises()
    expect(api.run).toHaveBeenCalledWith(1, 0, 'modeltrace')
  })
  it('keeps edited routing modes local until settings are saved', async () => {
    const view = await render({ model: 'fixture-model' })
    await view
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.settings')!
      .trigger('click')
    const select = view
      .findAll('select')
      .find((s) => s.find('option[value="auto"]').exists())!
    await select.setValue('auto')
    expect(api.save).not.toHaveBeenCalled()
    await view.find('form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        mode: 'auto',
        enabled: false,
        model: 'fixture-model',
        interval_minutes: 15
      })
    )
  })
  it('shows runtime diagnostics without enabling weighting', async () => {
    const view = await render()
    await view
      .findAll('button')
      .find((b) => b.text() === 'accountQuality.routingDiagnostics')!
      .trigger('click')
    await flushPromises()
    expect(api.preview).toHaveBeenCalled()
    expect(api.events).toHaveBeenCalledWith(1)
    expect(view.text()).toContain('accountQuality.routeReason.not_automatic')
    expect(api.save).not.toHaveBeenCalled()
    expect(api.run).not.toHaveBeenCalled()
  })
})

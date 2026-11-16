import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupQuotaPolicyFields from '../GroupQuotaPolicyFields.vue'
import * as policyAPI from '@/api/admin/groupQuotaPolicy'
import type { QuotaPolicyView } from '@/api/admin/groupQuotaPolicy'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/groupQuotaPolicy', () => ({ get: vi.fn(), save: vi.fn(), preview: vi.fn(), refresh: vi.fn(), release: vi.fn(), getModels: vi.fn(), validateMapping: vi.fn() }))

const view = (): QuotaPolicyView => ({
  group_id: 7, revision: 3,
  config: { rules: [{ id: 'one', name: 'Quota guard', priority: 1, mode: 'observe', account_ids: [5], account_match: 'any', window_match: 'any', conditions: [{ window: '5h', trigger_remaining: 20, recover_remaining: 50 }], recovery: 'auto', mappings: [{ source: 'source-model', target: 'target-model' }] }] },
  accounts: [{ id: 5, name: 'Account A', status: 'active' }, { id: 6, name: 'Account B', status: 'active' }],
  runtime: { snapshots: [], rules: [], events: [] },
})
const wrappers: ReturnType<typeof mount>[] = []
const mountField = () => {
  const wrapper = mount(GroupQuotaPolicyFields, {
  props: { groupId: 7 },
  global: { stubs: { Icon: { template: '<i />' }, Select: { props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' } } },
})
  wrappers.push(wrapper)
  return wrapper
}
const open = async () => {
  const wrapper = mountField()
  await wrapper.get('[data-testid="quota-toggle"]').trigger('click')
  await flushPromises()
  return wrapper
}

describe('GroupQuotaPolicyFields', () => {
  beforeEach(() => {
    vi.mocked(policyAPI.get).mockResolvedValue(view())
    vi.mocked(policyAPI.getModels).mockResolvedValue({ group_id: 7, checked_at: 1, models: [], accounts: [] })
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.resetAllMocks() })

  it('loads only when expanded and never creates a default policy on read', async () => {
    vi.mocked(policyAPI.get).mockResolvedValue({ ...view(), config: { rules: [] } })
    const wrapper = mountField()
    expect(policyAPI.get).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="quota-toggle"]').trigger('click')
    await flushPromises()
    expect(policyAPI.get).toHaveBeenCalledWith(7)
    expect(wrapper.findAll('[data-testid="quota-rule"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('quotaPolicy.empty')
    expect(policyAPI.save).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="quota-save"]').attributes('disabled')).toBeDefined()
  })

  it('does not allow a failed load to overwrite an existing policy', async () => {
    vi.mocked(policyAPI.get).mockRejectedValue(new Error('unavailable'))
    const wrapper = await open()
    expect(wrapper.text()).toContain('quotaPolicy.loadFailed')
    expect(wrapper.find('[data-testid="quota-save"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="quota-add"]').exists()).toBe(false)
    expect(policyAPI.save).not.toHaveBeenCalled()
  })

  it('saves selected accounts and mode with the original revision', async () => {
    vi.mocked(policyAPI.save).mockImplementation(async (_id, revision, config) => ({ ...view(), revision: revision + 1, config }))
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-account-6"]').setValue(true)
    await wrapper.get('[data-testid="quota-account-match"]').setValue('all')
    await wrapper.get('[data-testid="quota-mode"]').setValue('enforce')
    await wrapper.get('[data-testid="quota-save"]').trigger('click')
    await flushPromises()
    expect(policyAPI.save).toHaveBeenCalledWith(7, 3, { rules: [expect.objectContaining({ account_ids: [5, 6], account_match: 'all', mode: 'enforce' })] })
    expect(wrapper.get('[data-testid="quota-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('quotaPolicy.saved')
  })

  it.each(['', '20', '101'])('blocks invalid recovery threshold %j', async value => {
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-recover"]').setValue(value)
    await wrapper.get('[data-testid="quota-save"]').trigger('click')
    expect(wrapper.text()).toContain('quotaPolicy.validationThreshold')
    expect(policyAPI.save).not.toHaveBeenCalled()
  })

  it('starts new rules disabled with no preset accounts, thresholds, or model names', async () => {
    vi.mocked(policyAPI.get).mockResolvedValue({ ...view(), config: { rules: [] } })
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-add"]').trigger('click')
    expect((wrapper.get('[data-testid="quota-mode"]').element as HTMLSelectElement).value).toBe('disabled')
    expect((wrapper.get('[data-testid="quota-source"]').element as HTMLInputElement).value).toBe('')
    expect((wrapper.get('[data-testid="quota-account-5"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('[data-testid="quota-window-5h"]').setValue(true)
    expect((wrapper.get('[data-testid="quota-trigger"]').element as HTMLInputElement).value).toBe('')
    expect((wrapper.get('[data-testid="quota-recover"]').element as HTMLInputElement).value).toBe('')
    expect(policyAPI.save).not.toHaveBeenCalled()
  })

  it('refreshes runtime without erasing the draft or rebasing its revision', async () => {
    vi.mocked(policyAPI.refresh).mockResolvedValue({ ...view(), revision: 8, runtime: { snapshots: [], rules: [{ rule_id: 'one', active: true, suppressed: false, status: 'observing', changed_at: 1, checked_at: 2 }], events: [] } })
    vi.mocked(policyAPI.save).mockRejectedValue({ status: 409 })
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-name"]').setValue('My unsaved rule')
    await wrapper.get('[data-testid="quota-refresh"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="quota-name"]').element as HTMLInputElement).value).toBe('My unsaved rule')
    expect(wrapper.text()).toContain('quotaPolicy.observing')
    expect(wrapper.text()).toContain('quotaPolicy.conflict')
    await wrapper.get('[data-testid="quota-save"]').trigger('click')
    await flushPromises()
    expect(policyAPI.save).toHaveBeenCalledWith(7, 3, expect.anything())
    expect((wrapper.get('[data-testid="quota-name"]').element as HTMLInputElement).value).toBe('My unsaved rule')
    expect(wrapper.find('[data-testid="quota-reload"]').exists()).toBe(true)
  })

  it('previews unsaved rules without saving and clears obsolete preview after an edit', async () => {
    vi.mocked(policyAPI.preview).mockResolvedValue({ runtime: view().runtime, resolution: { applied: true, source_model: 'source-model', target_model: 'target-model', rule_id: 'one', rule_name: 'Draft', revision: 3 } })
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-name"]').setValue('Draft')
    await wrapper.get('[data-testid="quota-preview-model"]').setValue('source-model')
    await wrapper.get('[data-testid="quota-preview"]').trigger('click')
    await flushPromises()
    expect(policyAPI.preview).toHaveBeenCalledWith(7, { rules: [expect.objectContaining({ name: 'Draft' })] }, 'source-model')
    expect(policyAPI.save).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="quota-preview-result"]').exists()).toBe(true)
    await wrapper.get('[data-testid="quota-target"]').setValue('other-target')
    expect(wrapper.find('[data-testid="quota-preview-result"]').exists()).toBe(false)
  })

  it('ignores responses for a previously selected group', async () => {
    let resolveOld!: (value: QuotaPolicyView) => void
    vi.mocked(policyAPI.get).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
      .mockResolvedValueOnce({ ...view(), group_id: 8, config: { rules: [] } })
    const wrapper = mountField()
    await wrapper.get('[data-testid="quota-toggle"]').trigger('click')
    await wrapper.setProps({ groupId: 8 })
    await flushPromises()
    resolveOld(view())
    await flushPromises()
    expect(wrapper.findAll('[data-testid="quota-rule"]')).toHaveLength(0)
    expect(policyAPI.get).toHaveBeenCalledWith(8)
  })

  it('releases only saved rules and includes the revision', async () => {
    const active = { ...view(), runtime: { snapshots: [], events: [], rules: [{ rule_id: 'one', active: true, suppressed: false, status: 'active', changed_at: 1, checked_at: 1 }] } }
    vi.mocked(policyAPI.get).mockResolvedValue(active)
    vi.mocked(policyAPI.release).mockResolvedValue(view())
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-name"]').setValue('Unsaved')
    expect(wrapper.get('[data-testid="quota-release"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="quota-name"]').setValue('Quota guard')
    await wrapper.get('[data-testid="quota-release"]').trigger('click')
    await flushPromises()
    expect(policyAPI.release).toHaveBeenCalledWith(7, 3, 'one')
  })

  it('allows disabling a rule even if a monitored account was removed from the group', async () => {
    vi.mocked(policyAPI.get).mockResolvedValue({ ...view(), accounts: [] })
    vi.mocked(policyAPI.save).mockImplementation(async (_id, revision, config) => ({ ...view(), revision: revision + 1, config }))
    const wrapper = await open()
    await wrapper.get('[data-testid="quota-mode"]').setValue('disabled')
    await wrapper.get('[data-testid="quota-save"]').trigger('click')
    await flushPromises()
    expect(policyAPI.save).toHaveBeenCalledWith(7, 3, { rules: [expect.objectContaining({ mode: 'disabled', account_ids: [5] })] })
  })
})

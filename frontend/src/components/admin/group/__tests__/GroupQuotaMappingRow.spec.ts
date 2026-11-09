import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupQuotaMappingRow from '../GroupQuotaMappingRow.vue'
import * as policyAPI from '@/api/admin/groupQuotaPolicy'
import type { QuotaMappingCheck, QuotaModelCatalog } from '@/api/admin/groupQuotaPolicy'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/groupQuotaPolicy', () => ({ validateMapping: vi.fn() }))
const catalog: QuotaModelCatalog = {
  group_id: 7, checked_at: 10,
  models: [{ id: 'other-model', account_ids: [6] }, { id: 'target-model', account_ids: [5, 6] }],
  accounts: [{ id: 5, name: 'Account A', status: 'active', schedulable: true, models: [] }, { id: 6, name: 'Account B', status: 'active', schedulable: true, models: [] }],
}
const success = (): QuotaMappingCheck => ({ source: 'source-model', target: 'new-target', status: 'passed', checked_at: 1, accounts: [{ account_id: 5, account_name: 'Account A', upstream_model: 'real-target', status: 'passed', latency_ms: 10 }] })
const wrappers: ReturnType<typeof mount>[] = []
const mountRow = () => {
  const wrapper = mount(GroupQuotaMappingRow, {
    props: { groupId: 7, source: 'source-model', target: 'target-model', accountIds: [5], catalog },
    global: { stubs: { Icon: true, Select: { name: 'Select', props: ['modelValue', 'options'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' } } },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('GroupQuotaMappingRow', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.mocked(policyAPI.validateMapping).mockResolvedValue(success()) })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers(); vi.resetAllMocks() })

  it('lists group models with monitored accounts first and does not bill when opened', async () => {
    const wrapper = mountRow()
    const selects = wrapper.findAllComponents({ name: 'Select' })
    const options = selects[0].props('options')
    expect(options[0].value).toBe('target-model')
    expect(options[0].label).toContain('Account A')
    await vi.advanceTimersByTimeAsync(1000)
    expect(policyAPI.validateMapping).not.toHaveBeenCalled()
    await wrapper.setProps({ catalog: { ...catalog, checked_at: 11 } })
    await vi.advanceTimersByTimeAsync(1000)
    expect(policyAPI.validateMapping).not.toHaveBeenCalled()
  })

  it('debounces completed edits into one target test and preserves manual names', async () => {
    const wrapper = mountRow()
    await wrapper.get('[data-testid="quota-target"]').setValue('custom-alias')
    expect(wrapper.emitted('update:target')?.[0]).toEqual(['custom-alias'])
    await wrapper.setProps({ target: 'typing-target' })
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.setProps({ target: 'new-target' })
    await vi.advanceTimersByTimeAsync(650)
    await flushPromises()
    expect(policyAPI.validateMapping).toHaveBeenCalledTimes(1)
    expect(policyAPI.validateMapping).toHaveBeenCalledWith(7, { source: 'source-model', target: 'new-target', account_ids: [5], force: false }, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('quotaPolicy.check_passed')
  })

  it('blocks equal names and duplicate sources without upstream requests', async () => {
    const wrapper = mountRow()
    await wrapper.setProps({ target: 'source-model' })
    await vi.advanceTimersByTimeAsync(700)
    expect(wrapper.text()).toContain('quotaPolicy.validationMappings')
    await wrapper.setProps({ target: 'new-target', duplicateSource: true })
    await vi.advanceTimersByTimeAsync(700)
    expect(policyAPI.validateMapping).not.toHaveBeenCalled()
  })

  it('shows partial success and sends force only on manual retest', async () => {
    vi.mocked(policyAPI.validateMapping).mockResolvedValue({ ...success(), status: 'partial', accounts: [...success().accounts, { account_id: 6, account_name: 'Account B', upstream_model: 'real-target', status: 'failed', error_code: 'rate_limited', message: 'Temporarily rate limited' }] })
    const wrapper = mountRow()
    await wrapper.get('[data-testid="quota-check-retry"]').trigger('click')
    await flushPromises()
    expect(policyAPI.validateMapping).toHaveBeenCalledWith(7, expect.objectContaining({ force: true }), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('quotaPolicy.check_partial')
    expect(wrapper.text()).toContain('Account B')
    expect(wrapper.text()).toContain('rate_limited')
    expect(wrapper.get('[data-testid="quota-check-status"]').classes()).not.toContain('text-green-700')
  })

  it.each(['target\u0000model', 'target\u007fmodel', '模型'.repeat(34)])('rejects invalid model name %j without a probe', async target => {
    const wrapper = mountRow()
    await wrapper.setProps({ target })
    await vi.advanceTimersByTimeAsync(700)
    expect(wrapper.text()).toContain('quotaPolicy.validationMappings')
    expect(policyAPI.validateMapping).not.toHaveBeenCalled()
  })

  it('cancels and ignores an obsolete response after changing a target', async () => {
    let resolveOld!: (result: QuotaMappingCheck) => void
    vi.mocked(policyAPI.validateMapping).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mountRow()
    await wrapper.get('[data-testid="quota-check-retry"]').trigger('click')
    const oldSignal = vi.mocked(policyAPI.validateMapping).mock.calls[0][2]
    await wrapper.setProps({ target: '' })
    expect(oldSignal?.aborted).toBe(true)
    resolveOld(success())
    await flushPromises()
    expect(wrapper.text()).not.toContain('quotaPolicy.check_passed')
    expect(wrapper.find('[data-testid="quota-check-details"]').exists()).toBe(false)
  })

  it('invalidates a completed result when monitoring accounts change', async () => {
    const wrapper = mountRow()
    await wrapper.get('[data-testid="quota-check-retry"]').trigger('click')
    await flushPromises()
    await wrapper.setProps({ accountIds: [5, 6] })
    expect(wrapper.find('[data-testid="quota-check-details"]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(650)
    expect(policyAPI.validateMapping).toHaveBeenLastCalledWith(7, expect.objectContaining({ account_ids: [5, 6], force: false }), expect.any(AbortSignal))
  })
})

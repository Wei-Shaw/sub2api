import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelEvaluationModal from '../ModelEvaluationModal.vue'

const mocks = vi.hoisted(() => ({ create: vi.fn(), list: vi.fn(), get: vi.fn(), run: vi.fn(), accounts: vi.fn(), groups: vi.fn(), export: vi.fn() }))
vi.mock('@/api/admin/modelEvaluation', () => ({ createEvaluation: mocks.create, listEvaluations: mocks.list, getEvaluation: mocks.get, runEvaluation: mocks.run }))
vi.mock('@/api/admin/accounts', () => ({ getAvailableModels: mocks.accounts }))
vi.mock('@/api/admin/groups', () => ({ getModelAllowlistCandidates: mocks.groups }))
vi.mock('file-saver', () => ({ saveAs: mocks.export }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: unknown) => `${key}${params ? JSON.stringify(params) : ''}` }) }))

function mountModal(type: 'account' | 'group' = 'account') {
  return mount(ModelEvaluationModal, {
    props: { target: { type, id: 42, name: 'Eval target' } },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
  })
}

function result(status = 'correct') {
  return { id: 'round-1', status, account_id: 42, account_name: 'Eval account', duration_ms: 1000, input_tokens: 3, output_tokens: 2, reasoning_tokens: null, text: 'FINAL_ANSWER: 21' }
}

function report() {
  return { id: 'report-1', target_type: 'account', target_id: 42, target_name: 'Eval target', model: 'gpt-5.4', effort: 'high', rounds: 5, benchmark: 'candy-shape-v1', visibility: 'private', created_at: '2026-09-26T00:00:00Z', completed: 0, correct: 0, graded: 0 }
}

function row(status = 'correct', round = 1) {
  return { round, result: result(status), saved: true }
}

describe('ModelEvaluationModal', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    mocks.create.mockResolvedValue(report())
    mocks.list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    mocks.accounts.mockResolvedValue([{ id: 'gpt-5.4' }])
    mocks.groups.mockResolvedValue(['gpt-5.4'])
    mocks.run.mockImplementation((_id, round) => Promise.resolve(row('correct', round)))
  })

  it('validates the model and starts directly on click without a module switch', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.find('[data-testid="evaluation-toggle"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="evaluation-start"]').attributes('disabled')).toBeUndefined()
    expect(mocks.run).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain('invalid')
    expect(mocks.run).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-rounds"]').setValue(1)
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('100.0%')
    expect(wrapper.get('[role="status"]').text()).toContain('finished')
    wrapper.unmount()
  })

  it('uses group scope, records actual accounts, and separates ungraded rounds from accuracy', async () => {
    mocks.run.mockResolvedValueOnce(row()).mockResolvedValueOnce(row('ungraded', 2))
    const wrapper = mountModal('group')
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-rounds"]').setValue(2)
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).toHaveBeenCalledTimes(2)
    expect(mocks.create.mock.calls[0][0]).toEqual({ target_type: 'group', target_id: 42, model: 'gpt-5.4', effort: 'high', rounds: 2 })
    expect(mocks.run.mock.calls[0].slice(0, 2)).toEqual(['report-1', 1])
    expect(wrapper.text()).toContain('100.0%')
    expect(wrapper.get('[role="alert"]').text()).toContain('Eval account #42')
    expect(wrapper.get('[role="alert"]').text()).toContain('"round":2')
    expect(wrapper.text()).toContain('ungradedHint')
    expect(wrapper.text()).toContain('—')
    wrapper.unmount()
  })

  it('stops remaining rounds after an API failure and shows target and next action visibly', async () => {
    mocks.run.mockRejectedValue(new Error('account unavailable'))
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).toHaveBeenCalledTimes(1)
    const alert = wrapper.get('[role="alert"]').text()
    expect(alert).toContain('Eval target #42')
    expect(alert).toContain('account unavailable')
    expect(alert).toContain('retryHint')
    wrapper.unmount()
  })

  it('cancels the active request and never starts the next round', async () => {
    mocks.run.mockImplementation((_id, _round, signal: AbortSignal) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    }))
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-cancel"]').trigger('click')
    await flushPromises()
    expect(mocks.run.mock.calls[0][2].aborted).toBe(true)
    expect(mocks.run).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="status"]').text()).toContain('canceled')
    wrapper.unmount()
  })

  it('allows manual model entry and evaluation when the model catalog fails', async () => {
    mocks.accounts.mockRejectedValue(new Error('offline'))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="evaluation-start"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[role="alert"]').text()).toContain('Eval target #42')
    expect(wrapper.get('[role="alert"]').text()).toContain('modelsFailed')
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-rounds"]').setValue(1)
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('aborts an active evaluation on unmount', async () => {
    mocks.run.mockImplementation((_id, _round, signal: AbortSignal) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
    }))
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    const signal = mocks.run.mock.calls[0][2] as AbortSignal
    wrapper.unmount()
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(mocks.run).toHaveBeenCalledTimes(1)
  })

  it('loads saved results after reopening without sending new evaluation requests', async () => {
    mocks.list.mockResolvedValue({ items: [report()], total: 1, page: 1, page_size: 20 })
    mocks.get.mockResolvedValue({ ...report(), completed: 2, correct: 1, graded: 2, results: [row(), row('incorrect', 2)] })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-report-report-1"]').trigger('click')
    await flushPromises()
    expect(mocks.get).toHaveBeenCalledWith('report-1', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('50.0%')
    expect(wrapper.text()).toContain('FINAL_ANSWER: 21')
    expect(wrapper.get('[role="status"]').text()).toContain('loadedReport')
    expect(mocks.run).not.toHaveBeenCalled()
    expect(mocks.create).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('stops when persistence fails while retaining the answer for export', async () => {
    mocks.run.mockResolvedValue({ ...row(), saved: false, save_error: 'report-1 round 1: export and refresh history' })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toContain('report-1 round 1')
    expect(wrapper.text()).toContain('FINAL_ANSWER: 21')
    expect(wrapper.text()).toContain('100.0%')
    wrapper.unmount()
  })

  it('never calls a model when the report cannot be created', async () => {
    mocks.create.mockRejectedValue(new Error('database unavailable'))
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="evaluation-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="evaluation-start"]').trigger('click')
    await flushPromises()
    expect(mocks.run).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('Eval target #42')
    expect(wrapper.get('[role="alert"]').text()).toContain('createFailed')
    wrapper.unmount()
  })

  it('shows history loading failures at the top with a retry action', async () => {
    mocks.list.mockRejectedValue(new Error('database unavailable'))
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Eval target #42')
    expect(wrapper.get('[role="alert"]').text()).toContain('historyFailed')
    mocks.list.mockResolvedValue({ items: [report()], total: 1, page: 1, page_size: 20 })
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="evaluation-report-report-1"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

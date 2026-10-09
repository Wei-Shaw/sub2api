import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DatabaseView from '../DatabaseView.vue'
import type { DatabaseMaintenanceJob } from '@/api/admin/database'

const { getStats, getJob, start, showSuccess, showError } = vi.hoisted(() => ({
  getStats: vi.fn(), getJob: vi.fn(), start: vi.fn(), showSuccess: vi.fn(), showError: vi.fn()
}))
vi.mock('@/api/admin/database', () => ({ getDatabaseStats: getStats, getDatabaseMaintenanceJob: getJob, startDatabaseMaintenance: start }))
vi.mock('@/components/auth/TotpStepUpDialog.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('@/composables/useStepUp', () => ({ useStepUp: () => ({ run: (fn: () => unknown) => fn() }), isStepUpCancelled: () => false, isStepUpBlocked: () => false }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key}${params ? JSON.stringify(params) : ''}` }) }))

const table = (name: string, cleanup: boolean) => ({
  schema: 'public', name, data_bytes: 1024, index_bytes: 512, total_bytes: 1536,
  live_rows: 10, dead_rows: 2, last_vacuum: null, cleanup_supported: cleanup
})
const runningJob = (): DatabaseMaintenanceJob => ({
  id: 'job-1', operation: 'cleanup', tables: ['usage_logs'], status: 'running', created_by: 1,
  started_at: '2026-10-09T02:00:00Z', updated_at: '2026-10-09T02:00:00Z',
  finished_at: null, cutoff: '2026-09-09T02:00:00Z', current_table: 'usage_logs', completed_tables: 0, deleted_rows: 0, error: ''
})
const wrappers: ReturnType<typeof mount>[] = []
function mountView() {
  const wrapper = mount(DatabaseView, {
    global: { stubs: {
      TotpStepUpDialog: true,
      ConfirmDialog: {
        props: ['show', 'message'], emits: ['confirm', 'cancel'],
        template: '<div v-if="show" data-testid="confirmation"><p>{{ message }}</p><button type="button" data-testid="confirm" @click="$emit(\'confirm\')">Confirm</button><button type="button" data-testid="cancel" @click="$emit(\'cancel\')">Cancel</button></div>'
      }
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  getStats.mockResolvedValue({ name: 'sub2api', size_bytes: 1024 ** 3, tables: [table('usage_logs', true), table('users', false)] })
  getJob.mockResolvedValue(null)
  start.mockResolvedValue(runningJob())
})
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  vi.useRealTimers()
})

describe('Database maintenance', () => {
  it('shows disk usage and prevents cleanup of business tables or invalid retention', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-testid="database-size"]').text()).toBe('1.00 GiB')
    expect(wrapper.text()).toContain('1.50 KiB')
    await wrapper.get('input[value="usage_logs"]').setValue(true)
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('#database-retention').setValue(0)
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#database-retention').setValue(30)
    await wrapper.get('input[value="users"]').setValue(true)
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="vacuum"]').attributes('disabled')).toBeUndefined()
  })

  it('requires confirmation and sends the selected tables and retention days', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('input[value="usage_logs"]').setValue(true)
    await wrapper.get('#database-retention').setValue(7)
    await wrapper.get('[data-testid="cleanup"]').trigger('click')
    expect(start).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="confirmation"]').text()).toContain('"days":7')
    await wrapper.get('[data-testid="confirm"]').trigger('click')
    await flushPromises()
    expect(start).toHaveBeenCalledWith({ operation: 'cleanup', tables: ['usage_logs'], retention_days: 7, confirm: true }, expect.any(String))
    expect(wrapper.get('[data-testid="vacuum-full"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="maintenance-job"]').text()).toContain('admin.database.status.running')
  })

  it('does not submit when confirmation is cancelled', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('input[value="users"]').setValue(true)
    await wrapper.get('[data-testid="vacuum-full"]').trigger('click')
    expect(wrapper.get('[data-testid="confirmation"]').text()).toContain('admin.database.confirm.vacuum_full')
    await wrapper.get('[data-testid="cancel"]').trigger('click')
    expect(start).not.toHaveBeenCalled()
  })

  it('recovers running jobs on page load and refreshes usage after completion', async () => {
    getJob.mockResolvedValueOnce(runningJob()).mockResolvedValue({ ...runningJob(), status: 'succeeded', completed_tables: 1, deleted_rows: 5017, current_table: '' })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeDefined()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    expect(showSuccess).toHaveBeenCalledWith('admin.database.completed')
    expect(getStats).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="maintenance-job"]').text()).toContain('5,017')
    wrapper.unmount()
    const calls = getJob.mock.calls.length
    await vi.advanceTimersByTimeAsync(6000)
    expect(getJob).toHaveBeenCalledTimes(calls)
  })

  it('blocks new operations while status is unavailable and retries', async () => {
    getJob.mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValue(null)
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('input[value="usage_logs"]').setValue(true)
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeDefined()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    await wrapper.get('input[value="usage_logs"]').setValue(true)
    expect(wrapper.get('[data-testid="cleanup"]').attributes('disabled')).toBeUndefined()
  })
})

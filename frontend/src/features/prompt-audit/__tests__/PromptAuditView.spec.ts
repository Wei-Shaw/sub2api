import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { PromptAuditConfig, PromptAuditRuntime } from '../types'
import { SCANNER_CATALOG } from '../viewModel'
import PromptAuditView from '../PromptAuditView.vue'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(), updateConfig: vi.fn(), probeEndpoint: vi.fn(), getRuntime: vi.fn(), listEvents: vi.fn(),
  getEvent: vi.fn(), deleteEvent: vi.fn(), batchDeleteEvents: vi.fn(), previewDelete: vi.fn(), deleteEventsByFilter: vi.fn(), listGroups: vi.fn(),
  showSuccess: vi.fn(), showError: vi.fn(),
}))

vi.mock('../api', () => ({ default: mocks }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'en' }, t: (key: string, params?: Record<string, unknown>) => key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)) }) }
})

const baseConfig = (): PromptAuditConfig => ({
  enabled: true, blocking_enabled: false, store_pass_events: false, effective_mode: 'async_audit', strategy: 'priority',
  worker_count: 4, queue_capacity: 100, scanners: SCANNER_CATALOG.map((item) => item.id), all_groups: true, group_ids: [],
  endpoints: [{ id: 'guard-1', name: 'Guard One', protocol: 'openai_compatible', base_url: 'http://127.0.0.1:8000', model: 'guard-model', timeout_ms: 3000, input_limit: 4000, enabled: true, has_token: true, token_status: 'configured' }],
  config_version: 7, updated_at: '2026-07-16T00:00:00Z', updated_by: 1, change_summary: '{}',
})
const runtime = (): PromptAuditRuntime => ({
  process_status: 'running', effective_mode: 'async_audit', expected_config_version: 7, active_config_version: 7,
  worker_total: 4, worker_active: 1, queue_capacity: 100,
  queue: { staging: 0, queued: 0, processing: 1, retry: 0, done: 5, failed: 0, active: 1 },
  processed_total: 5, failed_total: 0, enqueued_total: 5, dropped_total: 0, database_status: 'ok', redis_status: 'ok', endpoints: {},
  guard_metrics: { total: 1, allowed: 1, flagged: 0, blocked: 0, unavailable: 0, invalid: 0, timeouts: 0, failovers: 0, bulkhead_full: 0, record_failed: 0 },
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const RuntimeStub = defineComponent({ props: ['runtime', 'loading', 'error'], emits: ['refresh'], template: '<div data-test="runtime">{{ error }}</div>' })
const EndpointStub = defineComponent({
  props: ['endpoints', 'probeResults', 'probingIds'], emits: ['update:endpoints', 'probe'],
  template: '<div data-test="endpoint"><button data-test="inject-secret" @click="$emit(\'update:endpoints\', endpoints.map((e) => ({ ...e, token: \'PROMPT_AUDIT_CANARY_SECRET_DO_NOT_PERSIST\' })))">secret</button><button data-test="probe" @click="$emit(\'probe\', endpoints[0])">probe</button></div>',
})
const PolicyStub = defineComponent({ props: ['draft', 'groups'], emits: ['update:draft'], template: '<div data-test="policy" />' })
const EventsStub = defineComponent({
  props: ['events', 'filters', 'selectedIds', 'loading', 'error', 'total', 'page', 'pageSize'],
  emits: ['filters-change', 'search', 'selection', 'page', 'page-size', 'view', 'delete', 'batch-delete', 'preview-delete'],
  template: '<div data-test="events"><button data-test="preview" @click="$emit(\'preview-delete\')">preview</button><button data-test="change-filter" @click="$emit(\'filters-change\', { ...filters, keyword: \'changed\' })">change</button><button data-test="delete-one" @click="$emit(\'delete\', 5)">delete</button><button data-test="select-batch" @click="$emit(\'selection\', [5, 6])">select</button><button data-test="delete-batch" @click="$emit(\'batch-delete\')">batch</button></div>',
})
const DetailStub = defineComponent({ props: ['show', 'event', 'loading'], emits: ['close'], template: '<div data-test="detail" />' })
const ConfirmStub = defineComponent({ props: ['show', 'title', 'message'], emits: ['confirm', 'cancel'], template: '<div v-if="show" data-test="confirm"><button data-test="confirm-action" @click="$emit(\'confirm\')">confirm</button></div>' })
const FilterDeleteStub = defineComponent({
  props: ['show', 'initialFilters', 'preview', 'previewing', 'deleting'],
  emits: ['close', 'preview', 'confirm', 'criteria-change'],
  template: '<div v-if="show" data-test="filter-delete-dialog"><button data-test="dialog-preview" @click="$emit(\'preview\', { ...initialFilters, start_at: \'2026-07-15T00:00\', end_at: \'2026-07-16T00:00\' })">run</button><button data-test="dialog-confirm" @click="$emit(\'confirm\', { ...initialFilters, start_at: \'2026-07-15T00:00\', end_at: \'2026-07-16T00:00\' })">confirm</button><span data-test="dialog-preview-state">{{ preview ? preview.matched_count : \'none\' }}</span></div>',
})

function mountView() {
  return mount(PromptAuditView, {
    global: { stubs: { AppLayout: AppLayoutStub, RuntimeOverview: RuntimeStub, EndpointPool: EndpointStub, PolicyPanel: PolicyStub, EventWorkspace: EventsStub, EventDetailDialog: DetailStub, FilterDeleteDialog: FilterDeleteStub, ConfirmDialog: ConfirmStub } },
  })
}

describe('PromptAuditView', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.getConfig.mockResolvedValue(baseConfig())
    mocks.getRuntime.mockResolvedValue(runtime())
    mocks.listGroups.mockResolvedValue([])
    mocks.listEvents.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    mocks.updateConfig.mockImplementation(async () => ({ ...baseConfig(), config_version: 8 }))
    mocks.probeEndpoint.mockResolvedValue({ ok: true, status: 'healthy', message: 'ok', latency_ms: 2, http_status: 200, retryable: false, checked_at: '2026-07-16T00:00:00Z', token_applied: true })
    mocks.previewDelete.mockResolvedValue({ matched_count: 2, filter_summary: {}, snapshot_max_id: 10, filter_hash: 'a'.repeat(64), confirmation_token: 'opaque-confirmation', expires_at: '2026-07-16T00:05:00Z' })
    mocks.deleteEventsByFilter.mockResolvedValue({ deleted_events: 2, deleted_jobs: 2 })
    mocks.deleteEvent.mockResolvedValue({ deleted_events: 1, deleted_jobs: 1 })
    mocks.batchDeleteEvents.mockResolvedValue({ deleted_events: 2, deleted_jobs: 2 })
  })

  it('starts config, runtime, groups, and events loads independently', async () => {
    mocks.getRuntime.mockRejectedValue(new Error('runtime offline'))
    const wrapper = mountView()
    expect(mocks.getConfig).toHaveBeenCalledOnce()
    expect(mocks.getRuntime).toHaveBeenCalledOnce()
    expect(mocks.listGroups).toHaveBeenCalledOnce()
    expect(mocks.listEvents).toHaveBeenCalledOnce()
    await flushPromises()
    expect(wrapper.get('[data-test="runtime"]').text()).toContain('runtime offline')
    expect(wrapper.find('[data-test="endpoint"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="events"]').exists()).toBe(true)
  })

  it('separates configuration and audit events into page tabs', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="tab-events"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-test="tab-config"]').attributes('aria-selected')).toBe('false')
    expect(wrapper.get('[data-test="tab-panel-events"]').attributes('style') || '').not.toContain('display: none')
    expect(wrapper.get('[data-test="tab-panel-config"]').attributes('style') || '').toContain('display: none')
    expect(wrapper.find('[data-test="save-config"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="events"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="pass-events-disabled-notice"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="tab-events"]').text()).toContain('admin.promptAudit.tabs.events')
    expect(wrapper.get('[data-test="tab-config"]').text()).toContain('admin.promptAudit.tabs.config')

    await wrapper.get('[data-test="tab-config"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="tab-config"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-test="tab-panel-config"]').attributes('style') || '').not.toContain('display: none')
    expect(wrapper.get('[data-test="tab-panel-events"]').attributes('style') || '').toContain('display: none')
    expect(wrapper.find('[data-test="save-config"]').exists()).toBe(true)

    await wrapper.get('[data-test="tab-events"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="tab-events"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-test="save-config"]').exists()).toBe(false)

    await wrapper.get('[data-test="pass-events-disabled-notice"] button').trigger('click')
    expect(wrapper.get('[data-test="tab-config"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-test="save-config"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="tab-panel-config"]').attributes('style') || '').not.toContain('display: none')
  })

  it('requires confirmation for blocking and disables it when audit is turned off', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="tab-config"]').trigger('click')
    await wrapper.get('[data-test="blocking-toggle"]').trigger('click')
    expect(wrapper.find('[data-test="confirm"]').exists()).toBe(true)
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    expect(wrapper.get('[data-test="blocking-toggle"]').attributes('aria-checked')).toBe('true')
    await wrapper.get('[data-test="enabled-toggle"]').trigger('click')
    expect(wrapper.get('[data-test="enabled-toggle"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[data-test="blocking-toggle"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[data-test="blocking-toggle"]').attributes()).toHaveProperty('disabled')
  })

  it('clears plaintext token state after a successful save', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="tab-config"]').trigger('click')
    await wrapper.get('[data-test="inject-secret"]').trigger('click')
    expect(wrapper.text()).toContain('admin.promptAudit.saveBar.dirty')
    await wrapper.get('[data-test="save-config"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ endpoints: [expect.objectContaining({ token: 'PROMPT_AUDIT_CANARY_SECRET_DO_NOT_PERSIST' })] }))
    const endpointProps = wrapper.getComponent(EndpointStub).props('endpoints') as Array<{ token: string }>
    expect(endpointProps[0].token).toBe('')
    expect(wrapper.html()).not.toContain('PROMPT_AUDIT_CANARY_SECRET_DO_NOT_PERSIST')
  })

  it('reports real probe progress/results and invalidates filter confirmation when filters change', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="tab-config"]').trigger('click')
    await wrapper.get('[data-test="probe"]').trigger('click')
    await flushPromises()
    expect(mocks.probeEndpoint).toHaveBeenCalledOnce()
    expect((wrapper.getComponent(EndpointStub).props('probeResults') as Record<string, unknown>)).toHaveProperty('guard-1')

    await wrapper.get('[data-test="tab-events"]').trigger('click')
    await wrapper.get('[data-test="preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filter-delete-dialog"]').exists()).toBe(true)
    expect(mocks.previewDelete).not.toHaveBeenCalled()
    await wrapper.get('[data-test="dialog-preview"]').trigger('click')
    await flushPromises()
    expect(mocks.previewDelete).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-test="dialog-preview-state"]').text()).toBe('2')
    await wrapper.get('[data-test="change-filter"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filter-delete-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="dialog-preview-state"]').text()).toBe('none')
  })

  it('uses native labeled switches and a responsive fixed save surface', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="tab-config"]').trigger('click')
    const switches = wrapper.findAll('[role="switch"]')
    expect(switches).toHaveLength(3)
    expect(switches.every((item) => Boolean(item.attributes('aria-label')))).toBe(true)
    expect(wrapper.html()).toContain('fixed inset-x-0 bottom-0')
    expect(wrapper.html()).toContain('flex-wrap')
  })

  it('executes single, selected-batch, and preview-confirmed filter deletion flows', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="delete-one"]').trigger('click')
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    await flushPromises()
    expect(mocks.deleteEvent).toHaveBeenCalledWith(5)

    await wrapper.get('[data-test="select-batch"]').trigger('click')
    await wrapper.get('[data-test="delete-batch"]').trigger('click')
    await wrapper.get('[data-test="confirm-action"]').trigger('click')
    await flushPromises()
    expect(mocks.batchDeleteEvents).toHaveBeenCalledWith([5, 6])

    await wrapper.get('[data-test="preview"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="dialog-preview"]').trigger('click')
    await flushPromises()
    expect(mocks.previewDelete).toHaveBeenCalledWith(expect.objectContaining({ start_at: '2026-07-15T00:00', end_at: '2026-07-16T00:00' }))
    await wrapper.get('[data-test="dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.deleteEventsByFilter).toHaveBeenCalledWith(expect.objectContaining({
      start_at: '2026-07-15T00:00',
      end_at: '2026-07-16T00:00',
    }), expect.objectContaining({
      snapshot_max_id: 10,
      confirmation_token: 'opaque-confirmation',
    }))
    expect(wrapper.find('[data-test="filter-delete-dialog"]').exists()).toBe(false)
  })

  it('mints the confirmation token on the fly for one-click filter deletion without a manual preview', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="filter-delete-dialog"]').exists()).toBe(true)
    expect(mocks.previewDelete).not.toHaveBeenCalled()

    await wrapper.get('[data-test="dialog-confirm"]').trigger('click')
    await flushPromises()
    expect(mocks.previewDelete).toHaveBeenCalledOnce()
    expect(mocks.previewDelete).toHaveBeenCalledWith(expect.objectContaining({ start_at: '2026-07-15T00:00', end_at: '2026-07-16T00:00' }))
    expect(mocks.deleteEventsByFilter).toHaveBeenCalledWith(expect.objectContaining({
      start_at: '2026-07-15T00:00',
      end_at: '2026-07-16T00:00',
    }), expect.objectContaining({
      snapshot_max_id: 10,
      confirmation_token: 'opaque-confirmation',
    }))
    expect(wrapper.find('[data-test="filter-delete-dialog"]').exists()).toBe(false)
  })
})

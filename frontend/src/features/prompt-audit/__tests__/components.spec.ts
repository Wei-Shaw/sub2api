import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { defineComponent REDACTED from 'vue'
import { mount REDACTED from '@vue/test-utils'
import EndpointPool from '../components/EndpointPool.vue'
import PolicyPanel from '../components/PolicyPanel.vue'
import EventWorkspace from '../components/EventWorkspace.vue'
import type { PromptAuditDraft, PromptAuditEndpointDraft, PromptAuditEvent REDACTED from '../types'
import { emptyEventFilters, SCANNER_CATALOG REDACTED from '../viewModel'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'en' REDACTED, t: (key: string, params?: Record<string, unknown>) => key.replace(/\{(\w+)\REDACTED/g, (_, token) => String(params?.[token] ?? `{${tokenREDACTEDREDACTED`)) REDACTED) REDACTED
REDACTED)

const DialogStub = defineComponent({ props: ['show', 'title'], emits: ['close'], template: '<div v-if="show" data-test="dialog"><slot /><slot name="footer" /></div>' REDACTED)
const PaginationStub = defineComponent({ props: ['total', 'page', 'pageSize'], emits: ['update:page', 'update:pageSize'], template: '<div data-test="pagination" />' REDACTED)

const endpoint = (): PromptAuditEndpointDraft => ({
  id: 'guard-1', name: 'Guard One', protocol: 'openai_compatible', base_url: 'http://127.0.0.1:8000',
  model: 'guard-model', timeout_ms: 3000, input_limit: 4000, enabled: true,
  has_token: true, token_status: 'configured', token: '', clear_token: false,
REDACTED)

describe('Prompt Audit components', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('edits a saved endpoint with blank-secret keep, explicit clear, replacement, and probe actions', async () => {
    const wrapper = mount(EndpointPool, {
      props: { endpoints: [endpoint()], probeResults: {REDACTED, probingIds: [] REDACTED,
      global: { stubs: { BaseDialog: DialogStub REDACTED REDACTED,
    REDACTED)
    expect(wrapper.text()).toContain('admin.promptAudit.pool.configured')
    const edit = wrapper.findAll('button').find((button) => button.text().includes('common.edit'))
    expect(edit).toBeTruthy()
    await edit!.trigger('click')
    const token = wrapper.get<HTMLInputElement>('[aria-label="admin.promptAudit.pool.apiKey"]')
    expect(token.element.value).toBe('')
    expect(token.attributes('placeholder')).toContain('admin.promptAudit.pool.keepSecret')

    await wrapper.get<HTMLInputElement>('[aria-label="admin.promptAudit.pool.clearSecret"]').setValue(true)
    await token.setValue('replacement-canary')
    await wrapper.get('[data-test="save-endpoint"]').trigger('click')
    const updated = wrapper.emitted('update:endpoints')?.at(-1)?.[0] as PromptAuditEndpointDraft[]
    expect(updated[0]).toMatchObject({ token: 'replacement-canary', clear_token: false REDACTED)

    const probe = wrapper.findAll('button').find((button) => button.text().includes('admin.promptAudit.pool.probe'))
    await probe!.trigger('click')
    expect(wrapper.emitted('probe')?.[0]?.[0]).toMatchObject({ id: 'guard-1' REDACTED)
  REDACTED)

  it('supports group search, stale configured groups, nine scanners, and bounded worker inputs', async () => {
    const draft: PromptAuditDraft = {
      enabled: true, blocking_enabled: false, store_pass_events: false, effective_mode: 'async_audit', strategy: 'priority',
      worker_count: 4, queue_capacity: 100, scanners: SCANNER_CATALOG.map((item) => item.id), all_groups: false, group_ids: [1, 99],
      endpoints: [endpoint()], config_version: 1, updated_at: '', updated_by: 0, change_summary: '',
    REDACTED
    const wrapper = mount(PolicyPanel, {
      props: { draft, groups: [{ id: 1, name: 'Alpha', platform: 'openai', status: 'active' REDACTED, { id: 2, name: 'Beta', platform: 'claude', status: 'inactive' REDACTED] REDACTED,
    REDACTED)
    expect(wrapper.text()).toContain('99')
    expect(wrapper.findAll('input[type="checkbox"]').filter((input) => SCANNER_CATALOG.some((scanner) => input.attributes('aria-label') === scanner.label))).toHaveLength(9)
    await wrapper.get('[aria-label="admin.promptAudit.policy.searchGroups"]').setValue('Beta')
    expect(wrapper.text()).toContain('Beta')
    expect(wrapper.text()).not.toContain('Alpha')
    await wrapper.get('[aria-label="admin.promptAudit.policy.workerCount"]').setValue('6')
    const emitted = wrapper.emitted('update:draft')?.at(-1)?.[0] as PromptAuditDraft
    expect(emitted.worker_count).toBe(6)
  REDACTED)

  it('keeps identity fields separate, supports selection, and gates filter deletion on a time range', async () => {
    const event: PromptAuditEvent = {
      id: 1, job_id: 1, decision: 'critical', risk_level: 'critical', action: 'Block', categories: ['pii'], matched_scanners: ['pii'], scanner_scores: { pii: 1 REDACTED, scanner_evidence: { pii: 'redacted' REDACTED, scanner_backend: 'qwen3guard-openai', scanner_version: '1', guard_endpoint_id: 'guard-1', policy_id: 'priority', policy_version: 1, config_version: 1, chunk_total: 1, latency_ms: 10, issue_summaries: [], created_at: '2026-07-16T00:00:00Z',
      snapshot: { request_id: 'req-1', user_id: 1, username: 'alice', user_email: 'alice@example.test', api_key_id: 2, api_key_name: 'alice-key', group_id: 3, group_name: 'Alpha', provider: 'openai', endpoint: '/v1/chat/completions', protocol: 'openai_chat', model: 'gpt-test', prompt_hash: 'a'.repeat(64), redacted_preview: 'redacted preview', prompt_length: 10, message_count: 1, stage: 'http' REDACTED,
    REDACTED
    const wrapper = mount(EventWorkspace, {
      props: { events: [event], total: 1, page: 1, pageSize: 20, filters: emptyEventFilters(), selectedIds: [], loading: false, error: '' REDACTED,
      global: { stubs: { Pagination: PaginationStub REDACTED REDACTED,
    REDACTED)
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('alice@example.test')
    expect(wrapper.text()).toContain('alice-key')
    expect(wrapper.get('[data-test="filter-delete"]').attributes()).toHaveProperty('disabled')
    await wrapper.get('[aria-label="admin.promptAudit.events.startAt"]').setValue('2026-07-15T00:00')
    await wrapper.get('[aria-label="admin.promptAudit.events.endAt"]').setValue('2026-07-16T00:00')
    expect(wrapper.get('[data-test="filter-delete"]').attributes()).not.toHaveProperty('disabled')
    await wrapper.get('[aria-label="admin.promptAudit.events.selectEvent"]').setValue(true)
    expect(wrapper.emitted('selection')?.at(-1)?.[0]).toEqual([1])
  REDACTED)
REDACTED)

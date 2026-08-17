import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import OllamaCloudUsageCell from '../OllamaCloudUsageCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import type { Account, OllamaCloudUsageState REDACTED from '@/types'

const { refreshOllamaCloudUsage REDACTED = vi.hoisted(() => ({
  refreshOllamaCloudUsage: vi.fn()
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      refreshOllamaCloudUsage
    REDACTED
  REDACTED
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key REDACTED)
  REDACTED
REDACTED)

const usageState = (): OllamaCloudUsageState => ({
  account_id: 7,
  eligible: true,
  configured: true,
  auto_refresh_enabled: false,
  encryption_key_configured: true,
  snapshot: {
    status: 'ok',
    fetched_at: '2026-07-22T12:00:00Z',
    last_attempt_at: '2026-07-22T12:00:00Z',
    next_refresh_at: '2026-07-22T13:00:00Z',
    data: {
      plan: 'max',
      five_hour: { used_percent: 5.6, reset_at: '2026-07-23T03:00:00Z' REDACTED,
      seven_day: { used_percent: 14.2, reset_at: '2026-07-29T00:00:00Z' REDACTED,
      balance: '$0',
      models: [
        { model: 'gpt-oss:120b-cloud', window: 'five_hour', requests: 2 REDACTED,
        { model: 'gpt-oss:120b-cloud', window: 'seven_day', requests: 12 REDACTED
      ]
    REDACTED
  REDACTED
REDACTED)

const account = (state = usageState()): Account => ({
  id: 7,
  name: 'ollama',
  platform: 'openai',
  type: 'apikey',
  ollama_cloud_usage: state,
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
REDACTED)

describe('OllamaCloudUsageCell', () => {
  beforeEach(() => {
    refreshOllamaCloudUsage.mockReset()
  REDACTED)

  it('renders native 5h and 7d windows with the configured-account query action', () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() REDACTED REDACTED)
    const cell = wrapper.get('[data-testid="ollama-cloud-usage-cell"]')
    expect(cell.classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full']))
    expect(cell.classes()).not.toContain('min-w-[12rem]')

    const bars = wrapper.findAllComponents(UsageProgressBar)
    expect(bars).toHaveLength(2)
    expect(bars[0].props()).toMatchObject({
      label: '5h',
      utilization: 5.6,
      resetsAt: '2026-07-23T03:00:00Z'
    REDACTED)
    expect(bars[1].props()).toMatchObject({
      label: '7d',
      utilization: 14.2,
      resetsAt: '2026-07-29T00:00:00Z'
    REDACTED)

    expect(wrapper.find('[data-testid="ollama-cloud-usage-details"]').exists()).toBe(false)
    const query = wrapper.get('[data-testid="ollama-cloud-usage-query"]')
    expect(query.classes()).toEqual(expect.arrayContaining(['text-blue-600', 'hover:bg-blue-50']))
    expect(query.text()).toContain('admin.accounts.usageWindow.activeQuery')
    expect(wrapper.text()).not.toContain('max')
    expect(wrapper.text()).not.toContain('$0')
    expect(wrapper.text()).not.toContain('gpt-oss:120b-cloud')
  REDACTED)

  it('reacts to an account snapshot update', async () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() REDACTED REDACTED)
    const next = usageState()
    next.snapshot!.data!.five_hour!.used_percent = 43

    await wrapper.setProps({ account: account(next) REDACTED)

    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(43)
  REDACTED)

  it('queries through the edit-page refresh endpoint and emits the updated state', async () => {
    const next = usageState()
    next.snapshot!.data!.five_hour!.used_percent = 43
    refreshOllamaCloudUsage.mockResolvedValueOnce(next)
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() REDACTED REDACTED)

    await wrapper.get('[data-testid="ollama-cloud-usage-query"]').trigger('click')
    await flushPromises()

    expect(refreshOllamaCloudUsage).toHaveBeenCalledWith(7)
    expect(wrapper.findAllComponents(UsageProgressBar)[0].props('utilization')).toBe(43)
    expect(wrapper.emitted<OllamaCloudUsageState[]>('updated')?.[0]?.[0]).toEqual(next)
  REDACTED)

  it('hides the query action until a browser session is configured', () => {
    const state = usageState()
    state.configured = false

    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account(state) REDACTED REDACTED)

    expect(wrapper.find('[data-testid="ollama-cloud-usage-query"]').exists()).toBe(false)
  REDACTED)
REDACTED)

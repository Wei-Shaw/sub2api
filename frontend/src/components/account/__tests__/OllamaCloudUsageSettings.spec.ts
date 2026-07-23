import { flushPromises, mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import OllamaCloudUsageSettings from '../OllamaCloudUsageSettings.vue'
import type { Account, OllamaCloudUsageState REDACTED from '@/types'

const api = vi.hoisted(() => ({
  getOllamaCloudUsage: vi.fn(),
  saveOllamaCloudUsageSession: vi.fn(),
  deleteOllamaCloudUsageSession: vi.fn(),
  setOllamaCloudUsageAutoRefresh: vi.fn(),
  refreshOllamaCloudUsage: vi.fn()
REDACTED))
const notifications = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() REDACTED))

vi.mock('@/api/admin', () => ({ adminAPI: { accounts: api REDACTED REDACTED))
vi.mock('@/stores/app', () => ({ useAppStore: () => notifications REDACTED))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.accounts.ollamaCloud.errors.OLLAMA_CLOUD_USAGE_REFRESH_RATE_LIMITED') {
          return `retry in ${params?.retry_after_secondsREDACTED seconds`
        REDACTED
        if (key === 'admin.accounts.ollamaCloud.fiveHourShort') return '5h'
        if (key === 'admin.accounts.ollamaCloud.sevenDayShort') return '7d'
        return key
      REDACTED
    REDACTED)
  REDACTED
REDACTED)

const state = (overrides: Partial<OllamaCloudUsageState> = {REDACTED): OllamaCloudUsageState => ({
  account_id: 7,
  eligible: true,
  configured: false,
  auto_refresh_enabled: false,
  encryption_key_configured: true,
  ...overrides
REDACTED)

const detailedState = (plan = 'max'): OllamaCloudUsageState => state({
  configured: true,
  snapshot: {
    status: 'ok',
    fetched_at: '2026-07-22T12:00:00Z',
    last_attempt_at: '2026-07-22T12:00:00Z',
    next_refresh_at: '2026-07-22T13:00:00Z',
    data: {
      plan,
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

const account = (usage: OllamaCloudUsageState = state()): Account => ({
  id: 7,
  name: 'ollama',
  platform: 'anthropic',
  type: 'apikey',
  ollama_cloud_usage: usage,
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

describe('OllamaCloudUsageSettings', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    notifications.showSuccess.mockReset()
    notifications.showError.mockReset()
  REDACTED)

  it('uses the existing account state without an immediate duplicate GET', async () => {
    api.saveOllamaCloudUsageSession.mockResolvedValueOnce(state({ configured: true REDACTED))
    const wrapper = mount(OllamaCloudUsageSettings, { props: { account: account() REDACTED REDACTED)
    await flushPromises()

    const input = wrapper.get('#ollama-cloud-session')
    await input.setValue('wos-session=browser-secret')
    await wrapper.get('[data-testid="ollama-cloud-session-save"]').trigger('click')
    await flushPromises()

    expect(api.getOllamaCloudUsage).not.toHaveBeenCalled()
    expect(api.saveOllamaCloudUsageSession).toHaveBeenCalledWith(7, 'wos-session=browser-secret')
    expect((input.element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.text()).not.toContain('browser-secret')
    expect(wrapper.emitted('updated')?.at(-1)?.[0]).toMatchObject({ configured: true REDACTED)
  REDACTED)

  it('fails closed when the persistent encryption key is unavailable', async () => {
    const wrapper = mount(OllamaCloudUsageSettings, {
      props: { account: account(state({ encryption_key_configured: false REDACTED)) REDACTED
    REDACTED)
    await flushPromises()
    await wrapper.get('#ollama-cloud-session').setValue('wos-session=secret')

    expect(wrapper.get('[data-testid="ollama-cloud-session-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.accounts.ollamaCloud.encryptionKeyRequired')
    expect(api.saveOllamaCloudUsageSession).not.toHaveBeenCalled()
  REDACTED)

  it('updates the account-level automatic refresh switch through its dedicated endpoint', async () => {
    api.setOllamaCloudUsageAutoRefresh.mockResolvedValueOnce(state({ configured: true, auto_refresh_enabled: true REDACTED))
    const wrapper = mount(OllamaCloudUsageSettings, {
      props: { account: account(state({ configured: true REDACTED)) REDACTED
    REDACTED)
    await flushPromises()

    await wrapper.get('[data-testid="ollama-cloud-auto-refresh"]').trigger('click')
    await flushPromises()
    expect(api.setOllamaCloudUsageAutoRefresh).toHaveBeenCalledWith(7, true)
  REDACTED)

  it('keeps plan, balance, model, status, and manual refresh details in the edit settings', async () => {
    api.refreshOllamaCloudUsage.mockResolvedValueOnce(detailedState('pro'))
    const wrapper = mount(OllamaCloudUsageSettings, {
      props: { account: account(detailedState()) REDACTED
    REDACTED)
    await flushPromises()

    const details = wrapper.get('[data-testid="ollama-cloud-usage-details"]')
    expect(details.text()).toContain('max')
    expect(details.text()).toContain('$0')
    expect(details.text()).toContain('5h gpt-oss:120b-cloud: 2')
    expect(details.text()).toContain('7d gpt-oss:120b-cloud: 12')
    expect(details.text()).toContain('admin.accounts.ollamaCloud.ok')

    await wrapper.get('[data-testid="ollama-cloud-refresh"]').trigger('click')
    await flushPromises()

    expect(api.refreshOllamaCloudUsage).toHaveBeenCalledWith(7)
    expect(wrapper.get('[data-testid="ollama-cloud-usage-details"]').text()).toContain('pro')
    expect(notifications.showSuccess).toHaveBeenCalled()
  REDACTED)

  it('shows the structured manual refresh limit from the edit settings', async () => {
    api.refreshOllamaCloudUsage.mockRejectedValueOnce({
      status: 429,
      reason: 'OLLAMA_CLOUD_USAGE_REFRESH_RATE_LIMITED',
      metadata: { retry_after_seconds: '18' REDACTED
    REDACTED)
    const wrapper = mount(OllamaCloudUsageSettings, {
      props: { account: account(detailedState()) REDACTED
    REDACTED)

    await wrapper.get('[data-testid="ollama-cloud-refresh"]').trigger('click')
    await flushPromises()

    expect(notifications.showError).toHaveBeenCalledWith('retry in 18 seconds')
  REDACTED)
REDACTED)

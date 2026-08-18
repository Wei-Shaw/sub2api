import { defineComponent, nextTick REDACTED from 'vue'
import { flushPromises, mount, type VueWrapper REDACTED from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'

import type { ChannelMonitor REDACTED from '@/api/admin/channelMonitor'
import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'

const {
  listTemplates,
  accountsList,
  accountsGetById,
  monitorCreate,
  monitorUpdate,
REDACTED = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  accountsList: vi.fn(),
  accountsGetById: vi.fn(),
  monitorCreate: vi.fn(),
  monitorUpdate: vi.fn(),
REDACTED))


vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorV1Mode: () => true,
  isChannelMonitorV2Mode: () => false,
  getChannelMonitorMode: () => 'v1' as const,
REDACTED))

vi.mock('@/features/channel-monitor-v2/MonitorSettingsPanel.vue', () => ({
  default: { name: 'MonitorSettingsPanel', template: '<div data-testid="v2-settings" />' REDACTED,
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: monitorCreate,
      update: monitorUpdate,
    REDACTED,
    channelMonitorTemplate: {
      list: listTemplates,
    REDACTED,
    accounts: {
      list: (...args: unknown[]) => accountsList(...args),
      getById: (...args: unknown[]) => accountsGetById(...args),
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() REDACTED,
REDACTED))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() REDACTED,
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  REDACTED),
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key REDACTED),
  REDACTED
REDACTED)

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false REDACTED REDACTED,
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
REDACTED)

function makeMonitor(overrides: Partial<ChannelMonitor> = {REDACTED): ChannelMonitor {
  return {
    id: 42,
    name: 'primary',
    provider: 'openai',
    api_mode: 'chat_completions',
    endpoint: 'https://api.example.com',
    api_key_masked: 'sk-t***',
    primary_model: 'gpt-4o-mini',
    extra_models: [],
    group_name: '',
    enabled: true,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    primary_status: '',
    primary_latency_ms: null,
    availability_7d: 0,
    extra_models_status: [],
    template_id: null,
    extra_headers: {REDACTED,
    body_override_mode: 'off',
    body_override: null,
    check_mode: 'probe',
    account_id: null,
    ...overrides,
  REDACTED
REDACTED

let unmountWrapper: (() => void) | undefined

function mountDialog(monitor: ChannelMonitor | null = null) {
  const wrapper = mount(MonitorFormDialog, {
    props: { show: true, monitor REDACTED,
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      REDACTED,
    REDACTED,
  REDACTED)
  unmountWrapper = () => wrapper.unmount()
  return wrapper
REDACTED

const accountTrigger = (wrapper: VueWrapper) =>
  wrapper.get('[data-testid="monitor-linked-account"] button')

const openAccountDropdown = async (wrapper: VueWrapper) => {
  await accountTrigger(wrapper).trigger('click')
  await nextTick()
  const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
  expect(dropdown).not.toBeNull()
  return dropdown as HTMLElement
REDACTED

const clickOption = (dropdown: HTMLElement, text: string) => {
  const option = [...dropdown.querySelectorAll('.select-option')].find((el) =>
    el.textContent?.includes(text),
  )
  expect(option, `option containing "${textREDACTED" not found`).toBeDefined()
  ;(option as HTMLElement).click()
REDACTED

const typeAccountSearch = async (dropdown: HTMLElement, query: string) => {
  const input = dropdown.querySelector<HTMLInputElement>('.select-search-input')
  expect(input).not.toBeNull()
  input!.value = query
  input!.dispatchEvent(new Event('input'))
  await nextTick()
REDACTED

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.restoreAllMocks()
REDACTED)

describe('MonitorFormDialog linked account selector', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue({ items: [] REDACTED)
    accountsList.mockReset().mockResolvedValue({ items: [] REDACTED)
    accountsGetById.mockReset()
    monitorCreate.mockReset().mockResolvedValue({REDACTED)
    monitorUpdate.mockReset().mockResolvedValue({REDACTED)
  REDACTED)

  it('loads the first page of provider accounts when quota mode is enabled', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'a', platform: 'anthropic' REDACTED] REDACTED)
    const wrapper = mountDialog()
    await flushPromises()
    expect(accountsList).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()

    expect(accountsList).toHaveBeenCalledTimes(1)
    expect(accountsList).toHaveBeenCalledWith(
      1,
      50,
      { platform: 'anthropic' REDACTED,
      { signal: expect.any(AbortSignal) REDACTED,
    )
  REDACTED)

  it('debounced typing hits server-side search and aborts the previous request', async () => {
    vi.useFakeTimers()
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const dropdown = await openAccountDropdown(wrapper)
    await typeAccountSearch(dropdown, 'hidden')
    expect(accountsList).toHaveBeenCalledTimes(1, 'debounce window must not fire a request')

    await vi.advanceTimersByTimeAsync(300)

    expect(accountsList).toHaveBeenCalledTimes(2)
    const lastCall = accountsList.mock.calls[accountsList.mock.calls.length - 1]
    expect(lastCall[2]).toMatchObject({ platform: 'anthropic', search: 'hidden' REDACTED)
    const firstSignal = (accountsList.mock.calls[0][3] as { signal: AbortSignal REDACTED).signal
    expect(firstSignal.aborted).toBe(true)
  REDACTED)

  it('hydrates a bound account missing from the first page and keeps the binding', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'first page account', platform: 'anthropic' REDACTED] REDACTED)
    accountsGetById.mockResolvedValue({ id: 999, name: 'hidden gem', platform: 'anthropic' REDACTED)
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    REDACTED))
    await flushPromises()

    expect(accountsGetById).toHaveBeenCalledWith(999)
    expect(accountTrigger(wrapper).text()).toContain('hidden gem (#999)')

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({ account_id: 999 REDACTED))
  REDACTED)

  it('clears the binding with a visible hint when the bound account cannot be loaded', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    REDACTED))
    await flushPromises()

    expect(wrapper.text()).toContain('admin.channelMonitor.form.linkedAccountMissing')
    expect(accountTrigger(wrapper).text()).not.toContain('hidden gem')

    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).not.toHaveBeenCalled()
  REDACTED)

  it('shows the OpenAI codex probe hint only in quota mode on openai', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.text()).not.toContain('openAIQuotaProbeHint')

    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()
    // anthropic 平台不显示。
    expect(wrapper.text()).not.toContain('openAIQuotaProbeHint')

    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('openAIQuotaProbeHint')
  REDACTED)

  it('keeps a picked account visible when a later search hides it', async () => {
    vi.useFakeTimers()
    accountsList
      .mockResolvedValueOnce({ items: [
        { id: 1, name: 'alpha', platform: 'anthropic' REDACTED,
        { id: 2, name: 'beta', platform: 'anthropic' REDACTED,
      ] REDACTED)
      .mockResolvedValueOnce({ items: [{ id: 2, name: 'beta', platform: 'anthropic' REDACTED] REDACTED)
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    const dropdown = await openAccountDropdown(wrapper)
    clickOption(dropdown, 'alpha (#1)')
    await nextTick()
    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    // 搜索 beta：结果页只剩 beta，但已选中的 alpha 仍固定展示、提交仍保留绑定。
    const reopened = await openAccountDropdown(wrapper)
    await typeAccountSearch(reopened, 'beta')
    await vi.advanceTimersByTimeAsync(300)

    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    await wrapper.findAll('input[type="text"]')[0].setValue('my monitor')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await vi.advanceTimersByTimeAsync(0)
    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({ account_id: 1 REDACTED))
  REDACTED)

  it('clears the account binding when the provider switches', async () => {
    accountsList.mockResolvedValue({ items: [{ id: 1, name: 'alpha', platform: 'anthropic' REDACTED] REDACTED)
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    await flushPromises()

    const dropdown = await openAccountDropdown(wrapper)
    clickOption(dropdown, 'alpha (#1)')
    await nextTick()
    expect(accountTrigger(wrapper).text()).toContain('alpha (#1)')

    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    await flushPromises()

    expect(accountTrigger(wrapper).text()).not.toContain('alpha')
    expect(accountTrigger(wrapper).text()).toContain('linkedAccountPlaceholder')
  REDACTED)

  // P2-7(b)：antigravity 强制的 quota 在切走时要对称还原，占位模型一并清掉。
  it('restores probe mode and clears the quota placeholder when switching away from antigravity', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'antigravity',
      check_mode: 'quota',
      account_id: 7,
      endpoint: '',
      primary_model: 'quota',
    REDACTED))
    await flushPromises()

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    await flushPromises()

    // 占位模型已被清空：probe 模式必填校验拦下提交，而不是拿 'quota' 探活。
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()
    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({
      check_mode: 'probe',
      primary_model: 'claude-sonnet-4-5',
    REDACTED))
  REDACTED)

  // P2-7(c)：update 切回 probe 显式发 account_id=0 解绑存量关联（null=不动）。
  it('unbinds the linked account with account_id=0 when a quota monitor switches back to probe', async () => {
    accountsGetById.mockRejectedValue(new Error('gone'))
    const wrapper = mountDialog(makeMonitor({
      provider: 'anthropic',
      check_mode: 'quota',
      account_id: 999,
      endpoint: '',
      primary_model: 'quota',
    REDACTED))
    await flushPromises()

    await wrapper.get('[data-testid="monitor-check-mode-probe"]').trigger('click')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorUpdate).toHaveBeenCalledWith(42, expect.objectContaining({ account_id: 0 REDACTED))
  REDACTED)

  // P2-7(c)：create 绝不发 0（后端会把 0 存成 &0 触发外键违约），保持 null。
  it('keeps account_id null when creating a probe monitor', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.findAll('input[type="text"]')[0].setValue('my monitor')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('claude-sonnet-4-5')
    await wrapper.get('#channel-monitor-form').trigger('submit')
    await flushPromises()

    expect(monitorCreate).toHaveBeenCalledWith(expect.objectContaining({
      check_mode: 'probe',
      account_id: null,
    REDACTED))
  REDACTED)
REDACTED)

import { describe, it, expect, vi, beforeEach REDACTED from 'vitest'
import { mount, flushPromises REDACTED from '@vue/test-utils'
import { createPinia, setActivePinia REDACTED from 'pinia'

const apiMocks = vi.hoisted(() => ({
  getPlatformQuotas: vi.fn(),
  updatePlatformQuotas: vi.fn(),
  resetPlatformQuotaWindow: vi.fn(),
REDACTED))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      getPlatformQuotas: apiMocks.getPlatformQuotas,
      updatePlatformQuotas: apiMocks.updatePlatformQuotas,
      resetPlatformQuotaWindow: apiMocks.resetPlatformQuotaWindow,
    REDACTED,
  REDACTED,
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  REDACTED),
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (params) {
          return key.replace(/\{(\w+)\REDACTED/g, (_, k) => params[k] ?? '')
        REDACTED
        return key
      REDACTED,
    REDACTED),
  REDACTED
REDACTED)

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show', 'title', 'width'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>',
  REDACTED,
REDACTED))

import UserPlatformQuotaModal from '../UserPlatformQuotaModal.vue'
import type { UserSubscription REDACTED from '@/types'

function makeUser(overrides: { subscriptions?: UserSubscription[] REDACTED = {REDACTED) {
  return { id: 99, email: 'u@example.com', ...overrides REDACTED as any
REDACTED

/** 挂载并触发 show：false → true，确保 watch 被激活 */
async function mountAndOpen(extraProps: Record<string, unknown> = {REDACTED) {
  const w = mount(UserPlatformQuotaModal, {
    props: { show: false, user: makeUser(), ...extraProps REDACTED,
  REDACTED)
  await w.setProps({ show: true REDACTED)
  await flushPromises()
  return w
REDACTED

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  apiMocks.getPlatformQuotas.mockResolvedValue({ platform_quotas: [] REDACTED)
  apiMocks.updatePlatformQuotas.mockResolvedValue({ platform_quotas: [] REDACTED)
  apiMocks.resetPlatformQuotaWindow.mockResolvedValue({ platform_quotas: [] REDACTED)
REDACTED)

describe('UserPlatformQuotaModal', () => {
  it('挂载并 show=true 时调用 getPlatformQuotas', async () => {
    await mountAndOpen()
    expect(apiMocks.getPlatformQuotas).toHaveBeenCalledWith(99)
  REDACTED)

  it('空数据渲染 5 个 platform 行', async () => {
    const w = await mountAndOpen()
    const html = w.html()
    expect(html).toContain('anthropic')
    expect(html).toContain('openai')
    expect(html).toContain('gemini')
    expect(html).toContain('antigravity')
    expect(html).toContain('grok')
  REDACTED)

  it('已有数据正确填充 limit input', async () => {
    apiMocks.getPlatformQuotas.mockResolvedValueOnce({
      platform_quotas: [
        { platform: 'anthropic', daily_limit_usd: 10, weekly_limit_usd: null, monthly_limit_usd: null,
          daily_usage_usd: 3.2, weekly_usage_usd: 0, monthly_usage_usd: 0 REDACTED,
      ],
    REDACTED)
    const w = await mountAndOpen()
    const inputs = w.findAll('input[type=number]')
    // 5 platforms × 3 windows = 15 inputs
    expect(inputs.length).toBe(15)
    // 第一个 input 是 anthropic.daily = 10
    expect((inputs[0].element as HTMLInputElement).value).toBe('10')
  REDACTED)

  it('保存提交完整 5 platform payload', async () => {
    apiMocks.getPlatformQuotas.mockResolvedValueOnce({
      platform_quotas: [
        { platform: 'openai', daily_limit_usd: null, weekly_limit_usd: 20, monthly_limit_usd: null,
          daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0 REDACTED,
      ],
    REDACTED)
    const w = await mountAndOpen()
    // 找到「保存」按钮（包含中文「保存」字样的按钮）
    const buttons = w.findAll('button')
    const saveBtn = buttons.find((b) => b.text() === 'admin.users.platformQuota.save')
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')
    await flushPromises()
    expect(apiMocks.updatePlatformQuotas).toHaveBeenCalledTimes(1)
    const [uid, payload] = apiMocks.updatePlatformQuotas.mock.calls[0]
    expect(uid).toBe(99)
    expect(payload).toHaveLength(5) // 5 platforms always submitted
    const openai = payload.find((p: any) => p.platform === 'openai')
    expect(openai.weekly_limit_usd).toBe(20)
  REDACTED)

  it('全部清空把所有 limit 置 null（确认通过）', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMocks.getPlatformQuotas.mockResolvedValueOnce({
      platform_quotas: [
        { platform: 'anthropic', daily_limit_usd: 10, weekly_limit_usd: 50, monthly_limit_usd: 100,
          daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0 REDACTED,
      ],
    REDACTED)
    const w = await mountAndOpen()
    const buttons = w.findAll('button')
    const clearBtn = buttons.find((b) => b.text() === 'admin.users.platformQuota.clearAll')
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    const inputs = w.findAll('input[type=number]')
    for (const inp of inputs) {
      expect((inp.element as HTMLInputElement).value).toBe('')
    REDACTED
    confirmSpy.mockRestore()
  REDACTED)

  it('全部清空 confirm 取消则保持原值', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    apiMocks.getPlatformQuotas.mockResolvedValueOnce({
      platform_quotas: [
        { platform: 'anthropic', daily_limit_usd: 10, weekly_limit_usd: 50, monthly_limit_usd: 100,
          daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0 REDACTED,
      ],
    REDACTED)
    const w = await mountAndOpen()
    const clearBtn = w.findAll('button').find((b) => b.text() === 'admin.users.platformQuota.clearAll')
    await clearBtn!.trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    // anthropic daily 应保持 10（未被清空）
    const inputs = w.findAll('input[type=number]')
    const dailyVal = (inputs[0].element as HTMLInputElement).value
    expect(dailyVal).toBe('10')
    confirmSpy.mockRestore()
  REDACTED)

  it('重置按钮 confirm 取消则不调用 API', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const w = await mountAndOpen()
    const resetBtns = w.findAll('button').filter((b) => b.text() === '↻')
    expect(resetBtns.length).toBeGreaterThan(0)
    await resetBtns[0].trigger('click')
    await flushPromises()
    expect(apiMocks.resetPlatformQuotaWindow).not.toHaveBeenCalled()
    confirmSpy.mockRestore()
  REDACTED)

  it('重置按钮 confirm 确认则调用 API', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const w = await mountAndOpen()
    const resetBtns = w.findAll('button').filter((b) => b.text() === '↻')
    await resetBtns[0].trigger('click') // 第一个是 anthropic.daily
    await flushPromises()
    expect(apiMocks.resetPlatformQuotaWindow).toHaveBeenCalledWith(99, 'anthropic', 'daily')
    confirmSpy.mockRestore()
  REDACTED)

  describe('subscription warning banner', () => {
    it('displays subscription warning when user has active subscription', async () => {
      const w = mount(UserPlatformQuotaModal, {
        props: {
          show: true,
          user: makeUser({
            subscriptions: [
              {
                id: 1, user_id: 99, group_id: 1, status: 'active',
                starts_at: '2026-01-01T00:00:00Z', expires_at: null,
                daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
                daily_window_start: null, weekly_window_start: null, monthly_window_start: null,
                created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
              REDACTED as UserSubscription,
            ],
          REDACTED),
        REDACTED,
      REDACTED)
      await flushPromises()
      expect(w.html()).toContain('admin.users.platformQuota.subscriptionWarning')
    REDACTED)

    it('hides subscription warning when user has only expired subscriptions', async () => {
      const w = mount(UserPlatformQuotaModal, {
        props: {
          show: true,
          user: makeUser({
            subscriptions: [
              {
                id: 2, user_id: 99, group_id: 1, status: 'expired',
                starts_at: '2025-01-01T00:00:00Z', expires_at: '2025-12-31T00:00:00Z',
                daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
                daily_window_start: null, weekly_window_start: null, monthly_window_start: null,
                created_at: '2025-01-01T00:00:00Z', updated_at: '2025-12-31T00:00:00Z',
              REDACTED as UserSubscription,
            ],
          REDACTED),
        REDACTED,
      REDACTED)
      await flushPromises()
      expect(w.html()).not.toContain('admin.users.platformQuota.subscriptionWarning')
    REDACTED)

    it('hides subscription warning when subscriptions is empty array', async () => {
      const w = mount(UserPlatformQuotaModal, {
        props: {
          show: true,
          user: makeUser({ subscriptions: [] REDACTED),
        REDACTED,
      REDACTED)
      await flushPromises()
      expect(w.html()).not.toContain('admin.users.platformQuota.subscriptionWarning')
    REDACTED)
  REDACTED)
REDACTED)

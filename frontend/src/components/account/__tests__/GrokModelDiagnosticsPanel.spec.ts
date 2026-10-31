import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { i18n } from '@/i18n'
import { formatDateTime } from '@/utils/format'

const originalI18nT = i18n.global.t.bind(i18n.global)
import enAccounts from '@/i18n/locales/en/admin/accounts'
import zhAccounts from '@/i18n/locales/zh/admin/accounts'
import enCommon from '@/i18n/locales/en/common'
import zhCommon from '@/i18n/locales/zh/common'
import type { GrokModelDiagnostics } from '@/types'

const { tMock } = vi.hoisted(() => ({
  tMock: vi.fn<(key: string, params?: Record<string, unknown>) => string>()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => tMock(key, params)
    })
  }
})

import GrokModelDiagnosticsPanel from '../GrokModelDiagnosticsPanel.vue'

const NOT_LISTED_ZH = '未在该账号此端点的当前目录中发现'
const SAVED_CONFIG_ZH = '诊断基于已保存配置，请保存后重新同步'
const REQUESTS_NOT_BLOCKED_ZH = '请求不会被阻断'
const REQUIRED_KEYS = [
  'title',
  'configuredModels',
  'upstreamModels',
  'lastSuccess',
  'lastAttempt',
  'fresh',
  'stale',
  'unknown',
  'sourceChanged',
  'notListed',
  'unverifiable',
  'outOfScope',
  'compatibilityNotice',
  'configuredFallback',
  'savedConfigOnly',
  'refreshFailed',
  'syncSavedConfig',
  'catalogTitle',
  'syncCatalog',
  'details',
  'viewDifferences',
  'hideDifferences',
  'missingSummary',
  'summaryNotice',
  'modelCount',
  'historicalModelCount',
  'lastSuccessRelative',
  'source',
  'historicalSource',
  'historicalModels',
  'savedMappings',
  'currentListed',
  'historicalComparison',
  'syncFailed',
  'attemptSuccess',
  'attemptFallback',
  'attemptUnknown',
  'attemptUnavailable',
  'previousCatalogValid',
  'staleNotice',
  'changedNotice',
  'unknownNotice',
  'fallbackNotice',
  'savedConfigHint',
  'requestColumn',
  'targetColumn',
  'errorConfiguration',
  'errorHTTP',
  'errorTransport',
  'errorInvalidResponse',
  'errorEmptyCatalog',
  'errorTooLarge',
  'errorPersistence'
] as const

const START = Date.parse('2026-10-25T12:00:00.000Z')
const FETCHED_AT = '2026-10-25T11:00:00.000Z'
const EXPIRES_AT = '2026-10-25T12:00:05.000Z'
const LONG_ENDPOINT = `https://relay.example.com/${'endpoint-segment-'.repeat(12)}v1/models`
const LONG_MODEL = `grok-${'very-long-model-id-'.repeat(8)}custom`

let locale: 'en' | 'zh' = 'zh'
const initialI18nLocale = i18n.global.locale.value
const initialI18nMessages = {
  en: i18n.global.getLocaleMessage('en'),
  zh: i18n.global.getLocaleMessage('zh')
}

function lookup(key: string, params?: Record<string, unknown>): string {
  const root = locale === 'zh'
    ? { ...zhCommon, admin: zhAccounts }
    : { ...enCommon, admin: enAccounts }
  let current: unknown = root
  for (const segment of key.split('.')) {
    if (!current || typeof current !== 'object') return key
    current = (current as Record<string, unknown>)[segment]
  }
  return typeof current === 'string'
    ? current.replace(/\{(\w+)\}/g, (token, name) => String(params?.[name] ?? token))
    : key
}

function syncI18nLocale(nextLocale: 'en' | 'zh') {
  const dict = nextLocale === 'zh' ? zhCommon : enCommon
  i18n.global.locale.value = nextLocale
  // The test build of vue-i18n is runtime-only, so formatRelativeTime cannot
  // compile `{n}` itself. Route that global helper through the same locale objects.
  ;(i18n.global as { t: (key: string, params?: Record<string, unknown>) => string }).t = (key, params) => {
    let current: unknown = dict
    for (const segment of key.split('.')) {
      if (!current || typeof current !== 'object') return key
      current = (current as Record<string, unknown>)[segment]
    }
    return typeof current === 'string'
      ? current.replace(/\{(\w+)\}/g, (token, name) => String(params?.[name] ?? token))
      : key
  }
}

function mountPanel(
  props: Record<string, unknown> = {},
  nextLocale: 'en' | 'zh' = 'zh'
) {
  locale = nextLocale
  syncI18nLocale(nextLocale)
  tMock.mockImplementation(lookup)
  return mount(GrokModelDiagnosticsPanel, {
    props: {
      diagnostics: null,
      ...props
    }
  })
}

async function openDetails(wrapper: VueWrapper) {
  const button = wrapper.get('[data-testid="grok-diagnostics-details-toggle"]')
  if (button.attributes('aria-expanded') === 'false') {
    await button.trigger('click')
  }
}

function freshDiagnostics(overrides: Partial<GrokModelDiagnostics> = {}): GrokModelDiagnostics {
  return {
    state: 'fresh',
    endpoint: 'https://cli-chat-proxy.grok.com/v1/models',
    fetched_at: FETCHED_AT,
    expires_at: EXPIRES_AT,
    models: ['grok-4.5', 'grok-4.6', 'grok-4.7', 'grok-4.7-build'],
    last_attempt: {
      started_at: FETCHED_AT,
      finished_at: '2026-10-25T11:00:01.000Z',
      status: 'success',
      scope: {
        account_id: 25,
        account_type: 'oauth',
        endpoint: 'https://cli-chat-proxy.grok.com/v1/models',
        fingerprint: 'abc'
      }
    },
    mappings: [
      { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' },
      { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' },
      { requested: 'custom-*', status: 'unverifiable' },
      { requested: 'grok-imagine-image', upstream: 'grok-imagine-image', status: 'out_of_scope' }
    ],
    ...overrides
  }
}

const mounted: VueWrapper[] = []

function track(wrapper: VueWrapper) {
  mounted.push(wrapper)
  return wrapper
}

describe('GrokModelDiagnosticsPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    vi.setSystemTime(START)
  })

  afterEach(() => {
    for (const wrapper of mounted.splice(0)) wrapper.unmount()
    vi.useRealTimers()
    vi.restoreAllMocks()
    ;(i18n.global as { t: typeof originalI18nT }).t = originalI18nT
    i18n.global.setLocaleMessage('en', initialI18nMessages.en)
    i18n.global.setLocaleMessage('zh', initialI18nMessages.zh)
    i18n.global.locale.value = initialI18nLocale
  })

  it('keeps English and Chinese diagnostic copy isomorphic and non-empty', () => {
    const en = (enAccounts as { accounts: { grokModelDiagnostics?: Record<string, string> } }).accounts.grokModelDiagnostics
    const zh = (zhAccounts as { accounts: { grokModelDiagnostics?: Record<string, string> } }).accounts.grokModelDiagnostics
    expect(en).toBeTruthy()
    expect(zh).toBeTruthy()
    expect(Object.keys(en!).sort()).toEqual(Object.keys(zh!).sort())
    for (const key of REQUIRED_KEYS) {
      expect(en![key]?.trim()).toBeTruthy()
      expect(zh![key]?.trim()).toBeTruthy()
    }
    expect(zh!.notListed).toBe(NOT_LISTED_ZH)
    expect(zh!.savedConfigOnly).toBe(SAVED_CONFIG_ZH)
    expect(zh!.compatibilityNotice).toBe(REQUESTS_NOT_BLOCKED_ZH)
    expect(zhCommon.common.loading.trim()).toBeTruthy()
    expect(enCommon.common.loading.trim()).toBeTruthy()
    expect(zhCommon.common.refresh.trim()).toBeTruthy()
    expect(enCommon.common.refresh.trim()).toBeTruthy()
  })

  it('shows source, last success, the latest attempt, and mapping differences for a fresh catalog', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('https://cli-chat-proxy.grok.com/v1/models')
    expect(text).toContain(formatDateTime(FETCHED_AT))
    expect(text).toContain('最后成功')
    expect(text).toContain('最近尝试')
    expect(text).toContain('同步成功')
    expect(text).not.toContain('success')
    expect(text).toContain('当前有效')
    expect(text).toContain('grok-4.5')
    expect(text).toContain('grok-4.6')
    expect(text).toContain('grok-4.7')
    expect(text).toContain('grok-4.7-build')
    expect(text).toContain('composer-2.5')
    expect(text).toContain('grok-composer-2.5-fast')
    expect(text).toContain(NOT_LISTED_ZH)
    expect(text).toContain(REQUESTS_NOT_BLOCKED_ZH)
    expect(text).toContain('friendly')
    expect(text).toContain('custom-*')
    expect(text).toContain('无法逐项核验')
    expect(text).toContain('grok-imagine-image')
    expect(text).toContain('不在此文本目录范围内')
    expect(text).toContain('已保存配置核对')
    expect(text).toContain('上游目录')
  })

  it('renders the same sections in English', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }, 'en'))
    await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    await openDetails(wrapper)
    const text = wrapper.text()
    const copy = (enAccounts as { accounts: { grokModelDiagnostics: Record<string, string> } }).accounts.grokModelDiagnostics
    expect(text).toContain(copy.catalogTitle)
    expect(text).toContain(copy.fresh)
    expect(text).toContain(copy.notListed)
    expect(text).toContain(copy.compatibilityNotice)
    expect(text).toContain(copy.unverifiable)
    expect(text).toContain(copy.outOfScope)
    expect(text).toContain(copy.lastSuccess)
    expect(text).toContain(copy.lastAttempt)
    expect(text).not.toContain('configured_fallback')
  })

  it('becomes stale at ExpiresAt and stops claiming current catalog gaps without a request', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(START)
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    expect(wrapper.text()).toContain('1 个映射目标未在当前目录中发现')
    expect(wrapper.text()).toContain('当前有效')

    await vi.advanceTimersByTimeAsync(5000)

    expect(wrapper.text()).toContain('历史目录')
    expect(wrapper.text()).not.toContain('当前有效')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(wrapper.text()).not.toContain(REQUESTS_NOT_BLOCKED_ZH)
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('composer-2.5')
    expect(text).toContain(formatDateTime(FETCHED_AT))
    expect(text).toContain('历史记录，需重新同步后核验')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('shows unknown for invalid timestamps instead of parsing them in local time', async () => {
    const invalid = '2026-10-25 12:00:00'
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'fresh',
        fetched_at: invalid,
        expires_at: 'tomorrow',
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('未知')
    expect(text).not.toContain(invalid)
    expect(text).not.toContain('tomorrow')
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('当前有效')
  })

  it.each([
    '2026-02-29T12:00:00Z',
    '2026-02-30T12:00:00Z',
    '2026-10-25T24:00:00Z',
    '2026-10-25T12:60:00Z',
    '2026-10-25T12:00:60Z',
    '2026-10-25T12:00:00+24:00',
    '2026-10-25T12:00:00+08:60'
  ])('does not present an impossible catalog time %s as a successful observation', async (invalid) => {
    vi.useFakeTimers()
    vi.setSystemTime(START)
    const normalized = formatDateTime(invalid)
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'fresh',
        fetched_at: invalid,
        expires_at: invalid,
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('未知')
    expect(text).not.toContain(invalid)
    if (normalized) expect(text).not.toContain(normalized)
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('当前有效')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).not.toContain('历史目录')
    expect(text).not.toContain('来源已变更')
    const details = wrapper.get('[data-testid="grok-diagnostics-details"]')
    expect(details.get('dt').text()).toBe('历史目录来源')
    expect(details.findAll('h4')[0].text()).toBe('历史目录')
  })

  it('keeps a real leap day, numeric offset, and fractional second as the last success', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(START)
    const leapDay = '2024-02-29T23:30:00.5+08:00'
    const expiresAt = '2026-10-25T20:00:00.123+08:00'
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'fresh',
        fetched_at: leapDay,
        expires_at: expiresAt,
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('当前有效')
    expect(text).toContain(formatDateTime(leapDay))
    expect(text).toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('未知')
  })

  it('does not present a future fetched_at echo as a successful observation', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(START)
    const future = '2026-10-25T13:00:00.000Z'
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'unknown',
        fetched_at: future,
        expires_at: undefined,
        models: ['grok-4.7'],
        mappings: [
          { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' },
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('未知')
    expect(text).not.toContain(formatDateTime(future))
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('当前有效')
    expect(text).toContain('grok-4.7')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).not.toContain('历史目录')
    const details = wrapper.get('[data-testid="grok-diagnostics-details"]')
    expect(details.get('dt').text()).toBe('历史目录来源')
    expect(details.findAll('h4')[0].text()).toBe('历史目录')
  })

  it('treats listed rows in a stale catalog as history rather than a current result', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'stale',
        fetched_at: '2026-10-25T05:00:00.000Z',
        expires_at: '2026-10-25T11:00:00.000Z',
        mappings: [
          { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' },
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('历史目录')
    expect(text).toContain('friendly')
    expect(text).toContain('grok-4.7')
    expect(text).toContain('历史记录，需重新同步后核验')
    expect(text).not.toContain('当前目录中已发现')
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('当前有效')
  })

  it('shows a source change without borrowing the previous catalog as current', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'source_changed',
        endpoint: LONG_ENDPOINT,
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    expect(wrapper.text()).toContain('来源已变更')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain(LONG_ENDPOINT)
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').classes()).toContain('break-all')
  })

  it('shows the latest failure together with the previous successful catalog', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: 'upstream_unavailable'
        }
      })
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('同步失败')
    expect(wrapper.text()).toContain('上次成功的目录仍在有效期内')
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain(formatDateTime(FETCHED_AT))
    expect(text).not.toContain('failed')
    expect(text).not.toContain('upstream_unavailable')
    expect(text).toContain('同步未成功，请稍后重试')
    expect(text).toContain(formatDateTime('2026-10-25T11:50:02.000Z'))
    expect(text).toContain(NOT_LISTED_ZH)
  })

  it('keeps the historical catalog visible when refreshing the latest status fails', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      refreshFailed: true
    }))
    expect(wrapper.text()).toContain('无法获取最新状态')
    expect(wrapper.text()).toContain('当前有效')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('1 个映射目标未在当前目录中发现')
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('grok-4.7')
    expect(wrapper.text()).toContain(NOT_LISTED_ZH)
  })

  it('warns that unsaved edits are not part of the diagnosis without hiding gaps', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      dirty: true
    }))
    const text = wrapper.text()
    expect(text).toContain('诊断基于已保存配置。当前修改尚未保存，保存后请重新同步。')
    expect(text).toContain('1 个映射目标未在当前目录中发现')
    expect(text).toContain('当前有效')
  })

  it('shows unknown when no diagnostics are available and does not offer sync by default', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel())
    expect(wrapper.text()).toContain('未知')
    expect(wrapper.find('[data-testid="grok-diagnostics-sync"]').exists()).toBe(false)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it.each([
    {
      locale: 'zh' as const,
      visibleLabel: zhAccounts.accounts.grokModelDiagnostics.syncCatalog,
      savedConfig: zhAccounts.accounts.grokModelDiagnostics.syncSavedConfig
    },
    {
      locale: 'en' as const,
      visibleLabel: enAccounts.accounts.grokModelDiagnostics.syncCatalog,
      savedConfig: enAccounts.accounts.grokModelDiagnostics.syncSavedConfig
    }
  ])('keeps sync accessible name containing the $locale visible label and saved-config title', async ({
    locale: nextLocale,
    visibleLabel,
    savedConfig
  }) => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics(), canSync: true }, nextLocale))
    const button = wrapper.get('[data-testid="grok-diagnostics-sync"]')
    const accessibleName = button.attributes('aria-label') ?? ''
    expect(button.text()).toContain(visibleLabel)
    expect(accessibleName).toContain(visibleLabel)
    expect(accessibleName).toContain(savedConfig)
    expect(button.attributes('title')).toBe(savedConfig)
    await button.trigger('click')
    expect(wrapper.emitted('sync')).toEqual([[]])

    await wrapper.setProps({ syncing: true })
    await wrapper.get('[data-testid="grok-diagnostics-sync"]').trigger('click')
    expect(wrapper.emitted('sync')).toEqual([[]])
    expect(wrapper.get('[data-testid="grok-diagnostics-sync"]').attributes('disabled')).toBeDefined()
  })

  it('shows the existing loading copy without hiding the catalog', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics(), loading: true }))
    expect(wrapper.text()).toContain(zhCommon.common.loading)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('grok-4.7')
  })

  it('wraps long model ids and stays inside a narrow dark layout', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        endpoint: LONG_ENDPOINT,
        models: [LONG_MODEL],
        mappings: [{ requested: LONG_MODEL, upstream: LONG_MODEL, status: 'listed' }]
      })
    }))
    const root = wrapper.get('[data-testid="grok-model-diagnostics"]')
    expect(root.classes()).toEqual(expect.arrayContaining(['max-w-full', 'overflow-hidden', 'min-w-0']))
    expect(root.classes().some(className => className.startsWith('dark:'))).toBe(true)
    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').classes()).toContain('break-all')
    const model = wrapper.get('[data-testid="grok-diagnostics-model"]')
    expect(model.classes()).toEqual(expect.arrayContaining(['break-all', 'min-w-0', 'font-mono', 'text-xs']))
    expect(model.text()).toContain(LONG_MODEL)
    expect(wrapper.get('[data-testid="grok-diagnostics-details"]').find('ul').classes()).toEqual(
      expect.arrayContaining(['max-h-48', 'overflow-y-auto'])
    )
  })

  it('summarizes only current gaps and opens full saved diagnostics on demand', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    const detailToggle = wrapper.find('[data-testid="grok-diagnostics-details-toggle"]')
    expect(detailToggle.exists()).toBe(true)
    expect(detailToggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('1 个映射目标未在当前目录中发现')
    await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    expect(wrapper.get('[data-testid="grok-diagnostics-difference-list"]').text())
      .toContain('grok-composer-2.5-fast')
    await detailToggle.trigger('click')
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text())
      .toBe('https://cli-chat-proxy.grok.com/v1/models')
    const saved = wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()
    expect(saved).toContain('custom-*')
    expect(saved).toContain('无法逐项核验')
    expect(saved).toContain('grok-imagine-image')
    expect(saved).toContain('不在此文本目录范围内')
  })

  it('omits an empty gap warning and still opens wildcard and media rows', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [
          { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' },
          { requested: 'custom-*', status: 'unverifiable' },
          { requested: 'grok-imagine-image', upstream: 'grok-imagine-image', status: 'out_of_scope' }
        ]
      })
    }))
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-difference-toggle"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    await openDetails(wrapper)
    const saved = wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()
    expect(saved).toContain('custom-*')
    expect(saved).toContain('无法逐项核验')
    expect(saved).toContain('grok-imagine-image')
    expect(saved).toContain('不在此文本目录范围内')
    expect(saved).not.toContain(NOT_LISTED_ZH)
  })

  it('counts each not_listed mapping without folding suffixes or wildcard rows', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [
          { requested: 'composer-a', upstream: 'grok-composer-2.5-fast', status: 'not_listed' },
          { requested: 'composer-b', upstream: 'grok-composer-2.5-fast', status: 'not_listed' },
          { requested: 'custom-*', status: 'unverifiable' },
          { requested: 'grok-imagine-image', upstream: 'grok-imagine-image', status: 'out_of_scope' }
        ]
      })
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-summary"]').text()).toContain('4 个模型')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('2 个映射目标未在当前目录中发现')
    expect(wrapper.text()).toContain('仅作提醒，不会阻断请求或删除映射。')
    await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    const diffs = wrapper.get('[data-testid="grok-diagnostics-difference-list"]').text()
    expect(diffs).toContain('composer-a')
    expect(diffs).toContain('composer-b')
    expect(diffs.split(NOT_LISTED_ZH).length - 1).toBe(1)
    expect(diffs.split(REQUESTS_NOT_BLOCKED_ZH).length - 1).toBe(1)
    expect(diffs).not.toContain('custom-*')
    expect(diffs).not.toContain('grok-imagine-image')
    await openDetails(wrapper)
    const saved = wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()
    expect(saved).toContain('custom-*')
    expect(saved).toContain('无法逐项核验')
    expect(saved).toContain('grok-imagine-image')
    expect(saved).toContain('不在此文本目录范围内')
  })

  it.each([
    ['configuration', '上游配置不可用'],
    ['http_error', '上游请求失败'],
    ['transport', '连接上游失败'],
    ['invalid_response', '无法识别返回的目录'],
    ['empty_catalog', '未获得有效模型目录'],
    ['too_large', '目录超过读取上限'],
    ['persistence', '目录保存失败']
  ])('explains a fresh catalog failure %s without the raw code', async (code, label) => {
    const finished = '2026-10-25T11:50:02.000Z'
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: finished,
          status: 'failed',
          error_code: code
        }
      })
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('同步失败')
    expect(wrapper.text()).toContain('上次成功的目录仍在有效期内')
    expect(wrapper.text()).not.toContain(code)
    expect(wrapper.text()).not.toContain('failed')
    await openDetails(wrapper)
    const attempt = wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()
    expect(attempt).toContain(label)
    expect(attempt).toContain(formatDateTime(finished))
    expect(attempt).not.toContain(code)
    expect(wrapper.text()).toContain(formatDateTime(FETCHED_AT))
    expect(wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()).not.toContain(formatDateTime(finished))
  })

  it('does not echo an unknown failure code or invent an HTTP status', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: 'upstream_unavailable'
        }
      })
    }))
    await openDetails(wrapper)
    const attempt = wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()
    expect(attempt).toContain('同步未成功，请稍后重试')
    expect(attempt).not.toContain('upstream_unavailable')
    expect(attempt).not.toContain('failed')
    expect(attempt).not.toMatch(/\b[45]\d\d\b/)
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('同步失败')
    expect(wrapper.text()).toContain(formatDateTime(FETCHED_AT))
  })

  it('labels an unrecognized attempt status without splicing its raw value', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'weird_status',
          error_code: 'http_error'
        }
      })
    }))
    await openDetails(wrapper)
    const attempt = wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()
    expect(attempt).toContain('状态未知')
    expect(attempt).not.toContain('weird_status')
    expect(attempt).not.toContain('http_error')
    expect(attempt).not.toContain('上游请求失败')
  })

  it.each(['__proto__', 'constructor', 'toString'] as const)(
    'treats inherited attempt status %s as unknown without showing it',
    async (status) => {
      const wrapper = track(mountPanel({
        diagnostics: freshDiagnostics({
          last_attempt: {
            started_at: '2026-10-25T11:50:00.000Z',
            finished_at: '2026-10-25T11:50:02.000Z',
            status,
            error_code: 'http_error'
          }
        })
      }))
      expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('当前有效')
      await openDetails(wrapper)
      const attempt = wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()
      expect(attempt).toContain('状态未知')
      expect(attempt).not.toContain(status)
      expect(attempt).not.toContain('http_error')
      expect(attempt).not.toContain('上游请求失败')
      expect(attempt).not.toContain('[object Object]')
    }
  )

  it.each(['__proto__', 'constructor', 'toString'] as const)(
    'treats inherited error code %s as unavailable without showing it',
    async (code) => {
      const wrapper = track(mountPanel({
        diagnostics: freshDiagnostics({
          last_attempt: {
            started_at: '2026-10-25T11:50:00.000Z',
            finished_at: '2026-10-25T11:50:02.000Z',
            status: 'failed',
            error_code: code
          }
        })
      }))
      expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('同步失败')
      expect(wrapper.text()).toContain('同步未成功，请稍后重试')
      expect(wrapper.text()).not.toContain(code)
      expect(wrapper.text()).not.toContain('[object Object]')
      await openDetails(wrapper)
      const attempt = wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()
      expect(attempt).toContain('同步失败')
      expect(attempt).toContain('同步未成功，请稍后重试')
      expect(attempt).not.toContain(code)
      expect(attempt).not.toContain('failed')
    }
  )

  it('shows a fresh snapshot gap as the previous catalog when the latest attempt fell back', async () => {
    const attemptFinished = '2026-10-25T11:40:00.000Z'
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:39:00.000Z',
          finished_at: attemptFinished,
          status: 'configured_fallback'
        }
      })
    }))
    const state = wrapper.get('[data-testid="grok-diagnostics-state"]').text()
    expect(state).toContain('使用已配置模型补充能力')
    expect(wrapper.text()).toContain('本次未获得实时目录，仅用已配置模型补充能力信息。')
    expect(wrapper.text()).toContain('上次成功的目录仍在有效期内')
    expect(wrapper.text()).not.toContain('configured_fallback')
    expect(wrapper.text()).not.toContain('同步成功')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('1 个映射目标未在当前目录中发现')
    await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    const diffs = wrapper.get('[data-testid="grok-diagnostics-difference-list"]').text()
    expect(diffs).toContain('grok-composer-2.5-fast')
    expect(diffs).toContain('上次成功')
    expect(diffs).not.toContain('同步成功')
    await openDetails(wrapper)
    expect(wrapper.text()).toContain(formatDateTime(FETCHED_AT))
    expect(wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()).not.toContain(formatDateTime(attemptFinished))
    expect(wrapper.get('[data-testid="grok-diagnostics-last-attempt"]').text()).toContain(formatDateTime(attemptFinished))
  })

  it('does not invent a success time or current gaps when fallback has no trusted catalog', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'unknown',
        fetched_at: undefined,
        expires_at: undefined,
        models: ['configured-only'],
        last_attempt: {
          started_at: '2026-10-25T11:39:00.000Z',
          finished_at: '2026-10-25T11:40:00.000Z',
          status: 'configured_fallback'
        },
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).not.toContain('当前有效')
    expect(wrapper.text()).toContain('本次未获得实时目录，仅用已配置模型补充能力信息。')
    expect(wrapper.text()).toContain('暂无可信目录，不能判断当前映射差异。')
    expect(wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()).toContain('未知')
    expect(wrapper.text()).not.toContain(formatDateTime('2026-10-25T11:40:00.000Z'))
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('configured-only')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(wrapper.text()).toContain('历史记录，需重新同步后核验')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('未知')
    const details = wrapper.get('[data-testid="grok-diagnostics-details"]')
    expect(details.get('dt').text()).toBe('历史目录来源')
    expect(details.findAll('h4')[0].text()).toBe('历史目录')
  })

  it.each([
    ['stale', '2026-10-25T05:00:00.000Z', '2026-10-25T11:00:00.000Z', '历史目录', '目录已过期，重新同步后再比较映射。'],
    ['source_changed', FETCHED_AT, EXPIRES_AT, '来源已变更', '来源已变更，请重新同步。']
  ] as const)('keeps a %s catalog readable without current gap claims', async (state, fetched, expires, label, notice) => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state,
        fetched_at: fetched,
        expires_at: expires,
        endpoint: LONG_ENDPOINT,
        models: ['grok-4.7', LONG_MODEL],
        mappings: [
          { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' },
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' },
          { requested: 'custom-*', status: 'unverifiable' }
        ]
      })
    }))
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain(label)
    expect(wrapper.get('[data-testid="grok-diagnostics-summary"]').text()).toContain('2 个历史模型')
    expect(wrapper.text()).toContain(notice)
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    await openDetails(wrapper)
    const text = wrapper.text()
    expect(text).toContain('grok-4.7')
    expect(text).toContain(LONG_MODEL)
    expect(text).toContain('历史目录')
    expect(text).toContain('历史记录，需重新同步后核验')
    expect(text).toContain('无法逐项核验')
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('当前目录中已发现')
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text()).toBe(LONG_ENDPOINT)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').classes()).toContain('break-all')
  })

  it('labels an unknown catalog and still opens the saved model list', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        state: 'mystery',
        fetched_at: undefined,
        expires_at: undefined,
        models: ['grok-4.7'],
        mappings: [
          { requested: 'composer-2.5', upstream: 'grok-composer-2.5-fast', status: 'not_listed' }
        ]
      })
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('未知')
    expect(wrapper.text()).toContain('暂无可信目录，不能判断当前映射差异。')
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('grok-4.7')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(wrapper.text()).toContain('历史记录，需重新同步后核验')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).not.toContain('历史目录')
    const details = wrapper.get('[data-testid="grok-diagnostics-details"]')
    expect(details.get('dt').text()).toBe('历史目录来源')
    expect(details.findAll('h4')[0].text()).toBe('历史目录')
  })

  it('keeps refresh failure, the unsaved hint, and trusted gaps together', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      refreshFailed: true,
      dirty: true
    }))
    const dirty = wrapper.get('[data-testid="grok-diagnostics-dirty"]')
    expect(wrapper.get('[data-testid="grok-diagnostics-refresh-failed"]').text()).toContain('无法获取最新状态')
    expect(dirty.text()).toContain('诊断基于已保存配置。当前修改尚未保存，保存后请重新同步。')
    expect(dirty.classes().join(' ')).not.toMatch(/amber/)
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('1 个映射目标未在当前目录中发现')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).toContain('当前有效')
    expect(wrapper.get('[data-testid="grok-diagnostics-state"]').text()).not.toContain('同步失败')
  })

  it.each([false, true])('withdraws current gaps at expiry (open=%s)', async (open) => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    if (open) await wrapper.get('[data-testid="grok-diagnostics-difference-toggle"]').trigger('click')
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-difference-list"]').exists()).toBe(false)
    await wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').trigger('click')
    expect(wrapper.text()).toContain('历史记录，需重新同步后核验')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('advances the relative success time when the existing clock crosses a minute', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        fetched_at: '2026-10-25T11:58:30.000Z',
        expires_at: '2026-10-25T18:00:00.000Z'
      })
    }))
    const line = () => wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()
    expect(line()).toContain('上次成功：')
    expect(line()).toContain('1分钟前')
    expect(line()).not.toContain(formatDateTime('2026-10-25T11:58:30.000Z'))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(line()).toContain('2分钟前')
    expect(line()).not.toContain('1分钟前')
  })

  it('uses the English relative clock when the fixture locale is English', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        fetched_at: '2026-10-25T11:58:30.000Z',
        expires_at: '2026-10-25T18:00:00.000Z'
      })
    }, 'en'))
    const line = () => wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()
    expect(line()).toContain('Last success:')
    expect(line()).toContain('1m ago')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(line()).toContain('2m ago')
    expect(wrapper.text()).not.toContain('分钟前')
  })

  it('shows only the parsed host in the summary and keeps the full endpoint in details', async () => {
    const endpoint = 'https://user:secret@relay.example.com/v1/models?token=abc'
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics({ endpoint }) }))
    const summary = wrapper.get('[data-testid="grok-diagnostics-summary"]').text()
    expect(summary).toContain('relay.example.com')
    expect(summary).not.toContain('secret')
    expect(summary).not.toContain('token')
    expect(summary).not.toContain('/v1/models')
    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text()).toBe(endpoint)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('does not invent an official host for an unparseable source', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({ endpoint: 'not a url' })
    }))
    const summary = wrapper.get('[data-testid="grok-diagnostics-summary"]').text()
    expect(summary).not.toContain('grok.com')
    expect(summary).not.toContain('not a url')
    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text()).toBe('not a url')
  })

  it('connects disclosures with unique ids in one application and clears its clock on unmount', async () => {
    const clearSpy = vi.spyOn(globalThis, 'clearInterval')
    locale = 'zh'
    syncI18nLocale('zh')
    tMock.mockImplementation(lookup)
    const diagnostics = freshDiagnostics()
    const Harness = defineComponent({
      components: { GrokModelDiagnosticsPanel },
      setup() {
        return { diagnostics }
      },
      template: `<div>
        <GrokModelDiagnosticsPanel :diagnostics="diagnostics" can-sync />
        <GrokModelDiagnosticsPanel :diagnostics="diagnostics" can-sync />
      </div>`
    })
    const wrapper = track(mount(Harness))
    const detailToggles = wrapper.findAll('[data-testid="grok-diagnostics-details-toggle"]')
    const diffToggles = wrapper.findAll('[data-testid="grok-diagnostics-difference-toggle"]')
    const syncButtons = wrapper.findAll('[data-testid="grok-diagnostics-sync"]')
    expect(detailToggles).toHaveLength(2)
    expect(diffToggles).toHaveLength(2)
    const detailIds = detailToggles.map((button) => button.attributes('aria-controls'))
    const diffIds = diffToggles.map((button) => button.attributes('aria-controls'))
    expect(new Set(detailIds).size).toBe(2)
    expect(new Set(diffIds).size).toBe(2)
    expect(detailIds[0]).toMatch(/^grok-catalog-/)
    expect(diffIds[0]).toMatch(/^grok-differences-/)
    for (const button of [...detailToggles, ...diffToggles, ...syncButtons]) {
      expect(button.attributes('type')).toBe('button')
    }
    expect(detailToggles[0].attributes('aria-expanded')).toBe('false')
    await detailToggles[0].trigger('click')
    expect(detailToggles[0].attributes('aria-expanded')).toBe('true')
    expect(wrapper.get(`[id="${detailIds[0]}"]`).attributes('data-testid')).toBe('grok-diagnostics-details')
    expect(wrapper.findAll('[data-testid="grok-diagnostics-details"]')).toHaveLength(1)
    wrapper.unmount()
    expect(clearSpy).toHaveBeenCalled()
  })

  it('keeps details open when the same panel receives updated diagnostics', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    await wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').trigger('click')
    await wrapper.setProps({
      diagnostics: freshDiagnostics({
        models: ['grok-4.5', 'grok-4.6', 'grok-4.7', 'grok-4.7-build', 'grok-extra']
      })
    })
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('grok-extra')
  })
})

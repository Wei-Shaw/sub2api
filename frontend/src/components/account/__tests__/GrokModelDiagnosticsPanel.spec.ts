import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import { i18n } from '@/i18n'

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

const NOT_LISTED_ZH = '上游无此模型'
const REQUIRED_KEYS = [
  'catalogTitle',
  'upstreamModels',
  'historicalModels',
  'fresh',
  'stale',
  'unknown',
  'sourceChanged',
  'notListed',
  'unverifiable',
  'outOfScope',
  'configuredFallback',
  'refreshFailed',
  'syncSavedConfig',
  'syncCatalog',
  'viewDetails',
  'hideDetails',
  'badgeMissing',
  'missingDetail',
  'modelCount',
  'historicalModelCount',
  'syncedRelative',
  'sourcePrefix',
  'mappingCheck',
  'mappingCheckHint',
  'currentListed',
  'historicalComparison',
  'syncFailed',
  'attemptFallback',
  'attemptUnavailable',
  'previousCatalogValid',
  'retryHint',
  'staleNotice',
  'changedNotice',
  'unknownNotice',
  'fallbackNotice',
  'savedConfigHint',
  'showMoreModels',
  'collapseModels',
  'modelsWithCount',
  'mappingFromAria',
  'mappingTargetAria',
  'removeMappingAria',
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

function stateBadge(wrapper: VueWrapper) {
  return wrapper.get('[data-testid="grok-diagnostics-state"]')
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
    expect(zh!.fresh).toBe('正常')
    expect(en!.fresh).toBe('Healthy')
    expect(zh!.notListed).toBe(NOT_LISTED_ZH)
    expect(zhCommon.common.loading.trim()).toBeTruthy()
    expect(enCommon.common.loading.trim()).toBeTruthy()
  })

  it('uses badge-success for a healthy fresh catalog without gaps', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [
          { requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }
        ]
      })
    }))
    const badge = stateBadge(wrapper)
    expect(badge.text()).toBe('正常')
    expect(badge.classes()).toEqual(expect.arrayContaining(['badge', 'badge-success']))
  })

  it('uses badge-warning with a missing-count label when fresh gaps exist', () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    const badge = stateBadge(wrapper)
    expect(badge.text()).toBe('1 项上游无此模型')
    expect(badge.classes()).toEqual(expect.arrayContaining(['badge', 'badge-warning']))
  })

  it('uses badge-gray for an unknown catalog', () => {
    const wrapper = track(mountPanel())
    const badge = stateBadge(wrapper)
    expect(badge.text()).toContain('未知')
    expect(badge.classes()).toEqual(expect.arrayContaining(['badge', 'badge-gray']))
  })

  it('renders the same healthy and missing badges in English', () => {
    const healthy = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }]
      })
    }, 'en'))
    expect(stateBadge(healthy).text()).toBe('Healthy')

    const missing = track(mountPanel({ diagnostics: freshDiagnostics() }, 'en'))
    expect(stateBadge(missing).text()).toBe('1 not in upstream')
  })

  it('shows a missing reminder with a view-details control that expands the details panel', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    const summary = wrapper.get('[data-testid="grok-diagnostics-missing-summary"]')
    expect(summary.text()).toContain('有 1 个映射的目标模型不在该账号的上游模型清单里，转发时可能报错。')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-difference-toggle"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('仅作提醒，不会阻断请求或删除映射。')

    await summary.get('button').trigger('click')
    expect(wrapper.get('[data-testid="grok-diagnostics-details-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()).toContain('composer-2.5')
  })

  it('puts failure ahead of the missing sentence and keeps both in one alert', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: 'http_error'
        }
      })
    }))
    const text = wrapper.text()
    expect(text).toContain('上游请求失败，上次成功的目录仍有效，点「同步目录」重试。')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('有 1 个映射的目标模型不在该账号的上游模型清单里，转发时可能报错。')
    expect(text).not.toContain('仅作提醒，不会阻断请求或删除映射。')
    expect(wrapper.findAll('.bg-amber-50').length).toBe(1)
  })

  it('renders four mapping badge kinds in details', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    await openDetails(wrapper)
    const mappings = wrapper.get('[data-testid="grok-diagnostics-mappings"]')
    expect(mappings.text()).toContain('映射检查')
    expect(mappings.text()).toContain('按已保存的配置，核对目标模型是否在上游模型清单里')
    expect(mappings.text()).toContain('composer-2.5 → grok-composer-2.5-fast')
    expect(mappings.text()).toContain(NOT_LISTED_ZH)
    expect(mappings.text()).toContain('friendly → grok-4.7')
    expect(mappings.text()).toContain('已找到')
    expect(mappings.text()).toContain('custom-*')
    expect(mappings.text()).toContain('无法逐项核验')
    expect(mappings.text()).toContain('grok-imagine-image → grok-imagine-image')
    expect(mappings.text()).toContain('不在此文本目录范围内')
    expect(mappings.text()).not.toContain('请求不会被阻断')
  })

  it('labels non-fresh listed and not_listed rows as pending re-sync', async () => {
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
    const mappings = wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()
    expect(mappings).toContain('待重新同步')
    expect(mappings).not.toContain('已找到')
    expect(mappings).not.toContain(NOT_LISTED_ZH)
  })

  it('shows six upstream models by default and expands the rest on demand', async () => {
    const models = Array.from({ length: 8 }, (_, i) => `grok-model-${i + 1}`)
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({ models })
    }))
    await openDetails(wrapper)
    expect(wrapper.findAll('[data-testid="grok-diagnostics-model"]')).toHaveLength(6)
    expect(wrapper.text()).toContain('+2 个')
    expect(wrapper.text()).not.toContain('grok-model-7')

    await wrapper.get('[data-testid="grok-diagnostics-models-more"]').trigger('click')
    expect(wrapper.findAll('[data-testid="grok-diagnostics-model"]')).toHaveLength(8)
    expect(wrapper.text()).toContain('grok-model-7')
    expect(wrapper.text()).toContain('收起')

    await wrapper.get('[data-testid="grok-diagnostics-models-more"]').trigger('click')
    expect(wrapper.findAll('[data-testid="grok-diagnostics-model"]')).toHaveLength(6)
  })

  it('omits mapping, model, and endpoint blocks when those values are empty', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        endpoint: '',
        models: [],
        mappings: []
      })
    }))
    await openDetails(wrapper)
    expect(wrapper.find('[data-testid="grok-diagnostics-mappings"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-model"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-endpoint"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
  })

  it('shows meta count with relative sync time and keeps the endpoint only in details', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        fetched_at: '2026-10-25T11:58:30.000Z',
        expires_at: '2026-10-25T18:00:00.000Z',
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }]
      })
    }))
    const summary = wrapper.get('[data-testid="grok-diagnostics-summary"]').text()
    expect(summary).toContain('4 个模型')
    expect(wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()).toContain('1分钟前同步')
    expect(summary).not.toContain('cli-chat-proxy.grok.com')
    expect(summary).not.toContain('最后成功')
    expect(summary).not.toContain('最近尝试')

    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text())
      .toBe('来源 https://cli-chat-proxy.grok.com/v1/models')
    expect(wrapper.text()).not.toContain('最后成功')
    expect(wrapper.text()).not.toContain('最近尝试')
  })

  it('omits the sync-time fragment when there is no usable success time', () => {
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
        mappings: []
      })
    }))
    expect(wrapper.find('[data-testid="grok-diagnostics-last-success"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('本次未获得实时目录，仅用已配置模型补充能力信息。')
    expect(wrapper.text()).not.toContain('暂无可信目录，不能判断当前映射差异。')
  })

  it('becomes stale at ExpiresAt and stops claiming current catalog gaps without a request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(true)
    expect(stateBadge(wrapper).text()).toContain('上游无此模型')

    await vi.advanceTimersByTimeAsync(5000)

    expect(stateBadge(wrapper).text()).toContain('历史目录')
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('composer-2.5')
    expect(wrapper.text()).toContain('待重新同步')
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
    expect(stateBadge(wrapper).text()).toContain('未知')
    expect(text).not.toContain(invalid)
    expect(text).not.toContain('tomorrow')
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('正常')
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
    expect(stateBadge(wrapper).text()).toContain('未知')
    expect(stateBadge(wrapper).text()).not.toContain('历史目录')
    expect(wrapper.find('[data-testid="grok-diagnostics-last-success"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    const text = wrapper.text()
    expect(text).not.toContain(invalid)
    expect(text).not.toContain(NOT_LISTED_ZH)
    expect(text).not.toContain('正常')
    expect(text).not.toContain('来源已变更')
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('历史目录')
  })

  it('keeps a real leap day, numeric offset, and fractional second as the last success', async () => {
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
    expect(stateBadge(wrapper).text()).toBe('1 项上游无此模型')
    expect(wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()).toMatch(/\d+天前同步/)
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('有 1 个映射的目标模型不在该账号的上游模型清单里，转发时可能报错。')
    expect(wrapper.text()).not.toContain('未知')
  })

  it('does not present a future fetched_at echo as a successful observation', async () => {
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
    expect(stateBadge(wrapper).text()).toContain('未知')
    expect(stateBadge(wrapper).text()).not.toContain('历史目录')
    expect(wrapper.find('[data-testid="grok-diagnostics-last-success"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
    expect(wrapper.text()).not.toContain('正常')
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('grok-4.7')
    expect(wrapper.text()).toContain('历史目录')
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
    expect(stateBadge(wrapper).text()).toBe('2 项上游无此模型')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').text())
      .toContain('有 2 个映射的目标模型不在该账号的上游模型清单里，转发时可能报错。')
    expect(wrapper.find('[data-testid="grok-diagnostics-difference-toggle"]').exists()).toBe(false)
    await openDetails(wrapper)
    const saved = wrapper.get('[data-testid="grok-diagnostics-mappings"]').text()
    expect(saved).toContain('composer-a')
    expect(saved).toContain('composer-b')
    expect(saved.split(NOT_LISTED_ZH).length - 1).toBe(2)
    expect(saved).toContain('custom-*')
    expect(saved).toContain('无法逐项核验')
    expect(saved).toContain('grok-imagine-image')
    expect(saved).toContain('不在此文本目录范围内')
  })

  it('labels an unrecognized attempt status without splicing its raw value', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }],
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'weird_status',
          error_code: 'http_error'
        }
      })
    }))
    expect(stateBadge(wrapper).text()).toBe('正常')
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    const text = wrapper.text()
    expect(text).not.toContain('weird_status')
    expect(text).not.toContain('http_error')
    expect(text).not.toContain('上游请求失败')
    expect(text).not.toContain('同步失败')
    expect(wrapper.find('[data-testid="grok-diagnostics-last-attempt"]').exists()).toBe(false)
  })

  it.each(['__proto__', 'constructor', 'toString'] as const)(
    'treats inherited attempt status %s as unknown without showing it',
    (status) => {
      const wrapper = track(mountPanel({
        diagnostics: freshDiagnostics({
          mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }],
          last_attempt: {
            started_at: '2026-10-25T11:50:00.000Z',
            finished_at: '2026-10-25T11:50:02.000Z',
            status,
            error_code: 'http_error'
          }
        })
      }))
      expect(stateBadge(wrapper).text()).toBe('正常')
      expect(stateBadge(wrapper).classes()).toEqual(expect.arrayContaining(['badge', 'badge-success']))
      expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
      const text = wrapper.text()
      expect(text).not.toContain('同步失败')
      expect(text).not.toContain('上游请求失败')
      expect(text).not.toContain(status)
      expect(text).not.toContain('http_error')
      expect(text).not.toContain('[object Object]')
      expect(wrapper.findAll('.bg-amber-50').length).toBe(0)
    }
  )

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
    expect(stateBadge(wrapper).text()).toContain('来源已变更')
    expect(wrapper.find('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(false)
    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').text()).toContain(LONG_ENDPOINT)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').classes()).toContain('break-all')
  })

  it('explains a fresh catalog failure without the raw code', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: 'transport'
        }
      })
    }))
    expect(stateBadge(wrapper).text()).toContain('上游无此模型')
    expect(wrapper.text()).toContain('连接上游失败，上次成功的目录仍有效，点「同步目录」重试。')
    expect(wrapper.text()).not.toContain('transport')
    expect(wrapper.text()).not.toContain('failed')
    expect(wrapper.find('[data-testid="grok-diagnostics-last-attempt"]').exists()).toBe(false)
  })

  it.each([
    ['configuration', '上游配置不可用'],
    ['http_error', '上游请求失败'],
    ['transport', '连接上游失败'],
    ['invalid_response', '无法识别返回的目录'],
    ['empty_catalog', '未获得有效模型目录'],
    ['too_large', '目录超过读取上限'],
    ['persistence', '目录保存失败']
  ])('maps failure code %s to a localized notice', (code, label) => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }],
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: code
        }
      })
    }))
    expect(stateBadge(wrapper).text()).toContain('同步失败')
    expect(wrapper.text()).toContain(`${label}，上次成功的目录仍有效，点「同步目录」重试。`)
    expect(wrapper.text()).not.toContain(code)
  })

  it('does not echo an unknown failure code', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }],
        last_attempt: {
          started_at: '2026-10-25T11:50:00.000Z',
          finished_at: '2026-10-25T11:50:02.000Z',
          status: 'failed',
          error_code: 'upstream_unavailable'
        }
      })
    }))
    expect(wrapper.text()).toContain('同步未成功，请稍后重试，上次成功的目录仍有效，点「同步目录」重试。')
    expect(wrapper.text()).not.toContain('upstream_unavailable')
    expect(stateBadge(wrapper).text()).toContain('同步失败')
  })

  it('keeps the historical catalog visible when refreshing the latest status fails', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      refreshFailed: true
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-refresh-failed"]').text()).toContain('无法获取最新状态')
    expect(stateBadge(wrapper).text()).toContain('上游无此模型')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(true)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('grok-4.7')
  })

  it('warns that unsaved edits are not part of the diagnosis without hiding gaps', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      dirty: true
    }))
    const dirty = wrapper.get('[data-testid="grok-diagnostics-dirty"]')
    expect(dirty.text()).toContain('诊断基于已保存配置。当前修改尚未保存，保存后请重新同步。')
    expect(dirty.classes().join(' ')).not.toMatch(/amber/)
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(true)
  })

  it('shows unknown when no diagnostics are available and does not offer sync by default', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response())
    const wrapper = track(mountPanel())
    expect(stateBadge(wrapper).text()).toContain('未知')
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

  it('uses a light card shell without overflow clipping and keeps long endpoints breakable', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        endpoint: LONG_ENDPOINT,
        models: [LONG_MODEL],
        mappings: [{ requested: LONG_MODEL, upstream: LONG_MODEL, status: 'listed' }]
      })
    }))
    const root = wrapper.get('[data-testid="grok-model-diagnostics"]')
    expect(root.classes()).toEqual(expect.arrayContaining(['mt-4', 'rounded-lg', 'border', 'p-4']))
    expect(root.classes()).not.toContain('overflow-hidden')
    await openDetails(wrapper)
    expect(wrapper.get('[data-testid="grok-diagnostics-endpoint"]').classes()).toContain('break-all')
    expect(wrapper.get('[data-testid="grok-diagnostics-model"]').text()).toContain(LONG_MODEL)
  })

  it('toggles details with aria-expanded and aria-controls', async () => {
    const wrapper = track(mountPanel({ diagnostics: freshDiagnostics() }))
    const toggle = wrapper.get('[data-testid="grok-diagnostics-details-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.text()).toContain('查看详情')
    expect(wrapper.find('[data-testid="grok-diagnostics-details"]').exists()).toBe(false)
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(toggle.text()).toContain('收起详情')
    const details = wrapper.get('[data-testid="grok-diagnostics-details"]')
    expect(toggle.attributes('aria-controls')).toBe(details.attributes('id'))
  })

  it('shows fallback notice without inventing current gaps when the catalog is untrusted', async () => {
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
    expect(stateBadge(wrapper).text()).not.toContain('正常')
    expect(wrapper.text()).toContain('本次未获得实时目录，仅用已配置模型补充能力信息。')
    expect(wrapper.text()).not.toContain('暂无可信目录，不能判断当前映射差异。')
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('configured-only')
    expect(wrapper.text()).toContain('待重新同步')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
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
    expect(stateBadge(wrapper).text()).toContain(label)
    expect(wrapper.get('[data-testid="grok-diagnostics-summary"]').text()).toContain('2 个历史模型')
    expect(wrapper.text()).toContain(notice)
    await openDetails(wrapper)
    expect(wrapper.text()).toContain('历史目录')
    expect(wrapper.text()).toContain('待重新同步')
    expect(wrapper.text()).toContain('无法逐项核验')
    expect(wrapper.text()).not.toContain(NOT_LISTED_ZH)
  })

  it('keeps refresh failure, the unsaved hint, and trusted gaps together', () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics(),
      refreshFailed: true,
      dirty: true
    }))
    expect(wrapper.get('[data-testid="grok-diagnostics-refresh-failed"]').text()).toContain('无法获取最新状态')
    expect(wrapper.get('[data-testid="grok-diagnostics-dirty"]').text())
      .toContain('诊断基于已保存配置。当前修改尚未保存，保存后请重新同步。')
    expect(wrapper.get('[data-testid="grok-diagnostics-missing-summary"]').exists()).toBe(true)
    expect(stateBadge(wrapper).text()).toContain('上游无此模型')
  })

  it('advances the relative success time when the existing clock crosses a minute', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        fetched_at: '2026-10-25T11:58:30.000Z',
        expires_at: '2026-10-25T18:00:00.000Z',
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }]
      })
    }))
    const line = () => wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()
    expect(line()).toContain('1分钟前同步')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(line()).toContain('2分钟前同步')
    expect(line()).not.toContain('1分钟前')
  })

  it('uses the English relative clock when the fixture locale is English', async () => {
    const wrapper = track(mountPanel({
      diagnostics: freshDiagnostics({
        fetched_at: '2026-10-25T11:58:30.000Z',
        expires_at: '2026-10-25T18:00:00.000Z',
        mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }]
      })
    }, 'en'))
    const line = () => wrapper.get('[data-testid="grok-diagnostics-last-success"]').text()
    expect(line()).toContain('synced 1m ago')
    await vi.advanceTimersByTimeAsync(30_000)
    expect(line()).toContain('synced 2m ago')
    expect(wrapper.text()).not.toContain('分钟前')
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
    const syncButtons = wrapper.findAll('[data-testid="grok-diagnostics-sync"]')
    expect(detailToggles).toHaveLength(2)
    const detailIds = detailToggles.map((button) => button.attributes('aria-controls'))
    expect(new Set(detailIds).size).toBe(2)
    expect(detailIds[0]).toMatch(/^grok-catalog-/)
    for (const button of [...detailToggles, ...syncButtons]) {
      expect(button.attributes('type')).toBe('button')
    }
    expect(detailToggles[0].attributes('aria-expanded')).toBe('false')
    await detailToggles[0].trigger('click')
    expect(detailToggles[0].attributes('aria-expanded')).toBe('true')
    expect(wrapper.get(`[id="${detailIds[0]}"]`).attributes('data-testid')).toBe('grok-diagnostics-details')
    expect(wrapper.findAll('[data-testid="grok-diagnostics-details"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="grok-diagnostics-difference-toggle"]').exists()).toBe(false)
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

  it.each(['__proto__', 'constructor', 'toString'] as const)(
    'treats inherited error code %s as unavailable without showing it',
    (code) => {
      const wrapper = track(mountPanel({
        diagnostics: freshDiagnostics({
          mappings: [{ requested: 'friendly', upstream: 'grok-4.7', status: 'listed' }],
          last_attempt: {
            started_at: '2026-10-25T11:50:00.000Z',
            finished_at: '2026-10-25T11:50:02.000Z',
            status: 'failed',
            error_code: code
          }
        })
      }))
      expect(stateBadge(wrapper).text()).toContain('同步失败')
      expect(wrapper.text()).toContain('同步未成功，请稍后重试')
      expect(wrapper.text()).not.toContain(code)
      expect(wrapper.text()).not.toContain('[object Object]')
    }
  )
})

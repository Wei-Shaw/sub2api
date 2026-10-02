import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import type { CodexVersionSyncStatus } from '@/api/admin/settings'
import zhSettings from '@/i18n/locales/zh/admin/settings'
import CodexVersionSyncStatusView from '../CodexVersionSyncStatus.vue'

const { getStatus } = vi.hoisted(() => ({ getStatus: vi.fn() }))
vi.mock('@/api', () => ({ adminAPI: { settings: { getOpenAICodexVersionSyncStatus: getStatus } } }))

function snapshot(overrides: Partial<CodexVersionSyncStatus> = {}): CodexVersionSyncStatus {
  return {
    effective_version: '0.159.0', version_source: 'synced', synced_version: '0.159.0',
    auto_sync_enabled: true, status: 'success',
    last_checked_at: '2026-10-01T14:00:00Z', last_succeeded_at: '2026-10-01T14:00:00Z',
    last_version_updated_at: '2026-09-25T03:00:00Z', next_check_at: '2026-10-01T20:00:00Z',
    retry_count: 0, retry_exhausted: false,
    ...overrides,
  }
}

let wrapper: VueWrapper | undefined
function mountStatus(props: { active?: boolean; refreshToken?: number } = {}) {
  wrapper = mount(CodexVersionSyncStatusView, {
    props,
    global: { plugins: [createI18n({
      legacy: false, locale: 'zh-CN', messages: { 'zh-CN': { admin: zhSettings } },
      // Vitest 使用无编译器的 i18n runtime，测试显式提供简单插值编译器。
      messageCompiler: message => ctx => typeof message === 'string'
        ? message.replace(/\{(\w+)\}/g, (_match, key: string) => String(ctx.named(key)))
        : '',
    })] },
  })
  return wrapper
}

describe('Codex 版本同步的紧凑状态', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    getStatus.mockReset().mockResolvedValue(snapshot())
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('默认只展示版本、来源、结果，详情折叠且时间语义不同', async () => {
    const view = mountStatus()
    await flushPromises()
    expect(view.get('summary').text()).toContain('生效：0.159.0')
    expect(view.get('summary').text()).toContain('自动同步值')
    expect(view.get('summary').text()).toContain('最近检查成功')
    expect((view.get('details').element as HTMLDetailsElement).open).toBe(false)
    expect(view.get('summary').text()).not.toContain('最近版本更新')
    const definitions = view.findAll('dd').map(node => node.text())
    expect(definitions[1]).toBe(new Date('2026-10-01T14:00:00Z').toLocaleString('zh-CN'))
    expect(definitions[3]).toBe(new Date('2026-09-25T03:00:00Z').toLocaleString('zh-CN'))
  })

  it('限流时展示受控原因和重试期限，不误称已同步最新', async () => {
    getStatus.mockResolvedValue(snapshot({
      status: 'failed', error_code: 'github_primary_rate_limit', http_status: 403,
      rate_limit_reset_at: '2026-10-01T15:12:27Z', next_check_at: '2026-10-01T15:12:35Z',
      retry_count: 3, retry_exhausted: true,
    }))
    const view = mountStatus()
    await flushPromises()
    expect(view.get('summary').text()).toContain('检查失败')
    expect(view.get('summary').text()).not.toContain('最近检查成功')
    expect(view.text()).toContain('GitHub 主额度耗尽')
    expect(view.text()).toContain('HTTP 403')
    expect(view.text()).toContain('本轮追加重试已用完')
    expect(view.text()).toContain(new Date('2026-10-01T15:12:35Z').toLocaleString('zh-CN'))
  })

  it('手动覆盖明确提示不会被自动同步覆盖；关闭时不显示故障状态徽标', async () => {
    getStatus.mockResolvedValue(snapshot({
      version_source: 'manual', synced_version: '0.160.0', auto_sync_enabled: false,
      status: 'failed', error_code: 'github_forbidden', next_check_at: null,
    }))
    const view = mountStatus()
    await flushPromises()
    expect(view.get('summary').text()).toContain('手动固定')
    expect(view.get('summary').text()).toContain('自动同步已关闭')
    expect(view.get('summary').text()).not.toContain('检查失败')
    expect(view.text()).toContain('清空并保存后才跟随同步值')
    expect(view.text()).toContain('尚无足够证据判定为限流')
  })

  it('结果保存失败时提示可能丢失，不用旧的成功记录掩盖持久化问题', async () => {
    getStatus.mockResolvedValue(snapshot({ persistence_failed: true }))
    const view = mountStatus()
    await flushPromises()
    expect(view.get('summary').text()).toContain('检查结果未保存')
    expect(view.get('summary').text()).not.toContain('最近检查成功')
    expect(view.text()).toContain('重启后可能丢失')
  })

  it('读取失败不继续展示旧成功状态，手动刷新只是重新读取快照', async () => {
    const view = mountStatus()
    await flushPromises()
    getStatus.mockRejectedValueOnce(new Error('unavailable'))
    await view.setProps({ refreshToken: 1 })
    await flushPromises()
    expect(view.get('summary').text()).toBe('同步状态暂不可用')
    expect(view.text()).not.toContain('最近检查成功')
    await view.get('button').trigger('click')
    await flushPromises()
    expect(view.get('summary').text()).toContain('最近检查成功')
    expect(getStatus).toHaveBeenCalledTimes(3)
  })

  it('仅在网关页可见时读取并每分钟刷新，切页和卸载后停止', async () => {
    const view = mountStatus({ active: false })
    await flushPromises()
    expect(getStatus).not.toHaveBeenCalled()
    await view.setProps({ active: true })
    await flushPromises()
    expect(getStatus).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getStatus).toHaveBeenCalledTimes(2)
    await view.setProps({ active: false })
    await vi.advanceTimersByTimeAsync(120_000)
    expect(getStatus).toHaveBeenCalledTimes(2)
    view.unmount()
    wrapper = undefined
    await vi.advanceTimersByTimeAsync(120_000)
    expect(getStatus).toHaveBeenCalledTimes(2)
  })

  it('浏览器隐藏时停止，恢复可见后立即更新', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get')
    visibility.mockReturnValue('hidden')
    mountStatus()
    await flushPromises()
    expect(getStatus).not.toHaveBeenCalled()
    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getStatus).toHaveBeenCalledTimes(1)
    visibility.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(120_000)
    expect(getStatus).toHaveBeenCalledTimes(1)
  })

  it('保存后刷新时丢弃迟到的旧响应', async () => {
    let resolveOld!: (value: CodexVersionSyncStatus) => void
    getStatus.mockReturnValueOnce(new Promise<CodexVersionSyncStatus>(resolve => { resolveOld = resolve }))
    const view = mountStatus()
    getStatus.mockResolvedValueOnce(snapshot({ effective_version: '0.160.0' }))
    await view.setProps({ refreshToken: 1 })
    await flushPromises()
    expect(view.get('summary').text()).toContain('0.160.0')
    resolveOld(snapshot())
    await flushPromises()
    expect(view.get('summary').text()).toContain('0.160.0')
    expect(view.get('summary').text()).not.toContain('0.159.0')
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SiteVersionBadge from '../SiteVersionBadge.vue'

const { checkUpdates, auth, app } = vi.hoisted(() => ({
  checkUpdates: vi.fn(), auth: { isAdmin: true }, app: { siteVersion: '0.2.2' }
}))
vi.mock('@/api/admin/system', () => ({ checkUpdates }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
const latest = { current_version: '0.2.2', latest_version: '0.2.3', has_update: true,
  release_info: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.3' } }
let wrapper: ReturnType<typeof mount>
beforeEach(() => { auth.isAdmin = true; checkUpdates.mockReset(); checkUpdates.mockResolvedValue(latest) })
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = ''; vi.useRealTimers() })

describe('SiteVersionBadge', () => {
  it('keeps update notices but explains custom-image-only upgrades', async () => {
    checkUpdates.mockResolvedValue({ ...latest, build_type: 'custom' })
    wrapper = mount(SiteVersionBadge, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('有新版本可用')
    expect(document.body.textContent).toContain('已验证的定制镜像')
    expect(document.body.textContent).not.toContain('立即更新')
  })
  it('shows current version, update dot, safe release link and no update mutation', async () => {
    wrapper = mount(SiteVersionBadge, { attachTo: document.body })
    await flushPromises()
    expect(wrapper.text()).toContain('v0.2.2')
    expect(wrapper.find('.version-dot').exists()).toBe(true)
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('有新版本可用')
    expect(document.querySelector('a')?.getAttribute('href')).toBe(latest.release_info.html_url)
    expect(document.body.textContent).not.toContain('立即更新')
    ;(document.querySelector('[aria-label="检查更新"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(checkUpdates).toHaveBeenLastCalledWith(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })
  it('does not call administrator APIs for ordinary users', async () => {
    auth.isAdmin = false
    wrapper = mount(SiteVersionBadge)
    await flushPromises()
    expect(wrapper.text()).toContain('v0.2.2')
    expect(checkUpdates).not.toHaveBeenCalled()
    expect(wrapper.find('button').exists()).toBe(false)
  })
  it('never claims latest when the check failed', async () => {
    checkUpdates.mockRejectedValue(new Error('offline'))
    wrapper = mount(SiteVersionBadge, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('检查更新失败')
    expect(document.body.textContent).not.toContain('已是最新版本')
  })
  it('treats cached warnings as unconfirmed and rejects unexpected release URLs', async () => {
    checkUpdates.mockResolvedValue({ ...latest, has_update: false, warning: 'cached', release_info: { html_url: 'javascript:alert(1)' } })
    wrapper = mount(SiteVersionBadge, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('暂时无法确认最新版本')
    expect(document.body.textContent).not.toContain('已是最新版本')
    expect(document.querySelector('.version-release')).toBeNull()
  })
})

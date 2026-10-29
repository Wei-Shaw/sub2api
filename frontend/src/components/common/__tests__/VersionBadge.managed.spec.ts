import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  app: { buildType: 'managed', versionLoading: false, currentVersion: '0.2.14-custom',
    latestVersion: '0.2.15', hasUpdate: true, releaseInfo: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.15' },
    fetchVersion: vi.fn(), clearVersionCache: vi.fn() },
  performUpdate: vi.fn(), rollback: vi.fn(), getRollbackVersions: vi.fn()
}))
vi.mock('@/stores', () => ({ useAuthStore: () => ({ isAdmin: true }), useAppStore: () => mocks.app }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/system', () => ({ performUpdate: mocks.performUpdate, rollback: mocks.rollback,
  getRollbackVersions: mocks.getRollbackVersions, restartService: vi.fn() }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))
import VersionBadge from '../VersionBadge.vue'

describe('Managed deployment update controls', () => {
  beforeEach(() => { vi.clearAllMocks(); mocks.app.buildType = 'managed'; mocks.app.hasUpdate = true })
  afterEach(() => { document.body.innerHTML = '' })
  for (const hasUpdate of [true, false]) {
    it(`keeps deployment guidance visible (upstream update: ${hasUpdate})`, async () => {
      mocks.app.hasUpdate = hasUpdate
      const wrapper = mount(VersionBadge, { attachTo: document.body })
      await wrapper.find('button').trigger('click')
      await flushPromises()
      expect(document.body.querySelector('[data-testid="managed-update-hint"]')).not.toBeNull()
      const buttons = [...document.body.querySelectorAll('button')].map(b => b.textContent)
      expect(buttons.join(' ')).not.toContain('version.updateNow')
      expect(buttons.join(' ')).not.toContain('version.rollback')
      expect(document.body.textContent).not.toContain('version.sourceModeHint')
      expect(mocks.performUpdate).not.toHaveBeenCalled()
      expect(mocks.getRollbackVersions).not.toHaveBeenCalled()
      wrapper.unmount()
    })
  }
  it('preserves online update controls for official release builds', async () => {
    mocks.app.buildType = 'release'
    const wrapper = mount(VersionBadge, { attachTo: document.body })
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('version.updateNow')
    expect(document.body.querySelector('[data-testid="managed-update-hint"]')).toBeNull()
    wrapper.unmount()
  })
})

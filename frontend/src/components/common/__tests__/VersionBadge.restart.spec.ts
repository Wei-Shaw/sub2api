import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import VersionBadge from '../VersionBadge.vue'

const performUpdate = vi.fn()
const restartService = vi.fn()

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/api/admin/system', () => ({
  performUpdate: (...args: any[]) => performUpdate(...args),
  restartService: (...args: any[]) => restartService(...args),
  getRollbackVersions: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => ({
    versionLoading: false,
    currentVersion: '0.2.16',
    latestVersion: '0.2.17',
    hasUpdate: true,
    releaseInfo: null,
    buildType: 'release',
    fetchVersion: vi.fn(),
    clearVersionCache: vi.fn()
  })
}))

vi.mock('@/composables/useClipboard', async () => {
  const { ref } = await vi.importActual<typeof import('vue')>('vue')
  return { useClipboard: () => ({ copied: ref(false), copyToClipboard: vi.fn() }) }
})

async function mountRestartPanel() {
  performUpdate.mockResolvedValue({ need_restart: true })
  const wrapper = mount(VersionBadge, {
    attachTo: document.body,
    global: { stubs: { Icon: true } }
  })
  await wrapper.find('button').trigger('click')
  const updateButton = wrapper.findAll('button').find((b) => b.text().includes('version.updateNow'))
  await updateButton!.trigger('click')
  await flushPromises()
  const restartButton = wrapper.findAll('button').find((b) => b.text().includes('version.restartNow'))
  expect(restartButton).toBeDefined()
  return { wrapper, restartButton: restartButton! }
}

describe('VersionBadge restart', () => {
  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    performUpdate.mockReset()
    restartService.mockReset()
    document.body.innerHTML = ''
  })

  it('shows a manual-restart hint instead of a fake countdown when the server cannot restart itself', async () => {
    const { wrapper, restartButton } = await mountRestartPanel()
    restartService.mockRejectedValue({ status: 501, reason: 'SYSTEM_RESTART_UNSUPPORTED' })

    await restartButton.trigger('click')
    await flushPromises()

    expect(restartService).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('version.restartUnsupported')
    expect(wrapper.text()).not.toContain('version.restarting')
    wrapper.unmount()
  })

  it('keeps waiting for the service when the restart request loses its connection', async () => {
    vi.useFakeTimers()
    const { wrapper, restartButton } = await mountRestartPanel()
    restartService.mockRejectedValue({ status: 0, code: 'ERR_NETWORK' })

    await restartButton.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('version.restarting')
    expect(wrapper.text()).not.toContain('version.restartUnsupported')
    wrapper.unmount()
  })
})

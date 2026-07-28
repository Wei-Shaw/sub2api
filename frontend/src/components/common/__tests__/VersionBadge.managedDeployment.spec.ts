import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentJob, DeploymentJobStatus } from '@/api/admin/system'

const mocks = vi.hoisted(() => ({
  appStore: {
    versionLoading: false,
    currentVersion: '0.1.164-ts.1',
    latestVersion: '0.1.165-ts.1',
    hasUpdate: true,
    versionWarning: '',
    releaseInfo: null,
    buildType: 'release',
    deploymentMode: 'docker-managed',
    deploymentReady: true,
    deploymentMessage: '',
    fetchVersion: vi.fn(),
    clearVersionCache: vi.fn()
  },
  getCurrentDeploymentJob: vi.fn(),
  getDeploymentJob: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ isAdmin: true }),
  useAppStore: () => mocks.appStore
}))

vi.mock('@/api/admin/system', () => ({
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  getVersion: vi.fn(),
  getRollbackVersions: vi.fn(),
  getDeploymentJob: mocks.getDeploymentJob,
  getCurrentDeploymentJob: mocks.getCurrentDeploymentJob,
  replayOrRecoverCurrentDeployment: vi.fn(),
  reconcileDeploymentVersion: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: false,
    copyToClipboard: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

import VersionBadge from '../VersionBadge.vue'

function terminalJob(status: DeploymentJobStatus): DeploymentJob {
  return {
    id: `job-${status}`,
    action: 'update',
    target_version: '0.1.165-ts.1',
    status,
    stage: 'failed',
    error: 'candidate deployment failed',
    rollback_error: status === 'rollback_failed' ? 'previous container did not recover' : undefined,
    rollback_performed: false,
    background_activated: false,
    created_at: '2026-07-24T00:00:00Z',
    started_at: '2026-07-24T00:00:00Z',
    updated_at: '2026-07-24T00:01:00Z',
    finished_at: '2026-07-24T00:01:00Z'
  }
}

describe('VersionBadge managed deployment recovery', () => {
  beforeEach(() => {
    mocks.appStore.currentVersion = '0.1.164-ts.1'
    mocks.appStore.latestVersion = '0.1.165-ts.1'
    mocks.appStore.hasUpdate = true
    mocks.appStore.deploymentMode = 'docker-managed'
    mocks.appStore.deploymentReady = true
    mocks.appStore.fetchVersion.mockReset().mockResolvedValue(null)
    mocks.appStore.versionWarning = ''
    mocks.appStore.clearVersionCache.mockReset()
    mocks.getCurrentDeploymentJob.mockReset()
    mocks.getDeploymentJob.mockReset()
  })

  it('ignores a historical successful job when a newer release is available', async () => {
    mocks.appStore.currentVersion = '0.1.164-ts.6'
    mocks.appStore.latestVersion = '0.1.166-ts.1'
    mocks.appStore.hasUpdate = true
    mocks.getCurrentDeploymentJob.mockResolvedValue({
      ...terminalJob('succeeded'),
      target_version: '0.1.164-ts.6',
      stage: 'completed',
      error: ''
    })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.6' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')

    expect(wrapper.text()).toContain('version.updateAvailable')
    expect(wrapper.text()).toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.updateComplete')

    wrapper.unmount()
  })

  it('does not restore a historical success when no update remains', async () => {
    mocks.appStore.currentVersion = '0.1.165-ts.1'
    mocks.appStore.latestVersion = '0.1.165-ts.1'
    mocks.appStore.hasUpdate = false
    mocks.getCurrentDeploymentJob.mockResolvedValue({
      ...terminalJob('succeeded'),
      stage: 'completed',
      error: ''
    })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.165-ts.1' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')

    expect(wrapper.text()).toContain('version.upToDate')
    expect(wrapper.text()).not.toContain('version.updateComplete')

    wrapper.unmount()
  })

  it('does not let a historical rollback success hide an available update', async () => {
    mocks.appStore.currentVersion = '0.1.164-ts.6'
    mocks.appStore.latestVersion = '0.1.166-ts.1'
    mocks.appStore.hasUpdate = true
    mocks.getCurrentDeploymentJob.mockResolvedValue({
      ...terminalJob('succeeded'),
      action: 'rollback',
      target_version: '0.1.164-ts.6',
      stage: 'completed',
      error: ''
    })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.6' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')

    expect(wrapper.text()).toContain('version.updateAvailable')
    expect(wrapper.text()).toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.rollbackComplete')

    wrapper.unmount()
  })

  it('still shows success when the deployment started by this page completes', async () => {
    mocks.getCurrentDeploymentJob.mockRejectedValue({ response: { status: 404 } })
    mocks.getDeploymentJob.mockResolvedValue({
      ...terminalJob('succeeded'),
      stage: 'completed',
      error: ''
    })
    const systemAPI = await import('@/api/admin/system')
    vi.mocked(systemAPI.performUpdate).mockResolvedValue({
      message: 'started',
      need_restart: false,
      job: {
        ...terminalJob('running'),
        stage: 'pulling',
        error: ''
      }
    })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')
    await wrapper.get('button.bg-primary-500').trigger('click')
    await flushPromises()

    expect(mocks.getDeploymentJob).toHaveBeenCalledWith('job-running')
    expect(wrapper.text()).toContain('version.updateComplete')

    wrapper.unmount()
  })

  it.each([
    ['degraded', 'version.deploymentDegraded'],
    ['rollback_failed', 'version.deploymentRollbackFailed']
  ] as const)('keeps a recovered %s alert visible after an ordinary version refresh', async (status, messageKey) => {
    mocks.getCurrentDeploymentJob.mockResolvedValue(terminalJob(status))
    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: {
        stubs: {
          Icon: { template: '<span />' }
        }
      }
    })

    await flushPromises()

    const badge = wrapper.get('[data-testid="version-badge"]')
    expect(badge.classes()).toContain('bg-red-100')
    expect(badge.attributes('title')).toContain(messageKey)
    expect(badge.attributes('title')).toContain('candidate deployment failed')

    await badge.trigger('click')
    await flushPromises()

    const alertBeforeRefresh = wrapper.get('[data-testid="deployment-error"]')
    expect(alertBeforeRefresh.text()).toContain(messageKey)
    expect(alertBeforeRefresh.text()).toContain('candidate deployment failed')
    if (status === 'rollback_failed') {
      expect(alertBeforeRefresh.text()).toContain('previous container did not recover')
    }
    expect(wrapper.text()).not.toContain('version.retry')

    await wrapper.get('[data-testid="version-refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="deployment-error"]').text()).toContain(messageKey)
    expect(mocks.appStore.fetchVersion).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('recovers a dangerous job even when the first version request fails before managed mode is known', async () => {
    mocks.appStore.deploymentMode = 'source'
    mocks.appStore.fetchVersion.mockResolvedValueOnce(null)
    mocks.getCurrentDeploymentJob.mockResolvedValue(terminalJob('degraded'))

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: {
        stubs: {
          Icon: { template: '<span />' }
        }
      }
    })
    await flushPromises()

    expect(mocks.getCurrentDeploymentJob).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="version-badge"]').classes()).toContain('bg-red-100')

    wrapper.unmount()
  })

  it('retries durable job recovery on a manual version refresh', async () => {
    mocks.getCurrentDeploymentJob
      .mockRejectedValueOnce(new Error('temporary upstream switch'))
      .mockResolvedValueOnce(terminalJob('rollback_failed'))

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: {
        stubs: {
          Icon: { template: '<span />' }
        }
      }
    })
    await flushPromises()

    await wrapper.get('[data-testid="version-badge"]').trigger('click')
    await wrapper.get('[data-testid="version-refresh"]').trigger('click')
    await flushPromises()

    expect(mocks.getCurrentDeploymentJob).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="deployment-error"]').text()).toContain(
      'version.deploymentRollbackFailed'
    )

    wrapper.unmount()
  })

  it('shows an update feed warning instead of claiming the version is current', async () => {
    mocks.appStore.hasUpdate = false
    mocks.appStore.versionWarning = 'GitHub release feed is unavailable'
    mocks.getCurrentDeploymentJob.mockRejectedValue({ response: { status: 404 } })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()

    const badge = wrapper.get('[data-testid="version-badge"]')
    expect(badge.classes()).toContain('bg-amber-100')
    expect(badge.attributes('title')).toBe('version.updateCheckWarning')
    await badge.trigger('click')
    expect(wrapper.get('[data-testid="version-warning"]').text()).toContain(
      'GitHub release feed is unavailable'
    )
    expect(wrapper.text()).not.toContain('version.upToDate')

    wrapper.unmount()
  })

  it('keeps a host deployer upgrade failure visible after reload', async () => {
    mocks.getCurrentDeploymentJob.mockResolvedValue({
      ...terminalJob('succeeded'),
      stage: 'completed',
      error: '',
      cleanup_warning: 'deployer control-plane upgrade failed: health verification failed',
      control_plane_upgrade_status: 'failed',
      control_plane_upgrade_error: 'health verification failed'
    })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')

    const alert = wrapper.get('[data-testid="deployment-error"]')
    expect(alert.text()).toContain('version.controlPlaneUpgradeFailed')
    expect(alert.text()).toContain('health verification failed')
    expect(wrapper.text()).not.toContain('version.retry')
    wrapper.unmount()
  })

  it('restores the rollback target when recovering a failed rollback job', async () => {
    const failedRollback = {
      ...terminalJob('failed'),
      action: 'rollback' as const,
      target_version: '0.1.163-ts.4'
    }
    mocks.getCurrentDeploymentJob.mockResolvedValue(failedRollback)
    const systemAPI = await import('@/api/admin/system')
    vi.mocked(systemAPI.rollback).mockResolvedValue({ message: 'started', need_restart: false })

    const wrapper = mount(VersionBadge, {
      props: { version: '0.1.164-ts.1' },
      global: { stubs: { Icon: { template: '<span />' } } }
    })
    await flushPromises()
    await wrapper.get('[data-testid="version-badge"]').trigger('click')
    await wrapper.get('[data-testid="deployment-error"] button').trigger('click')
    await flushPromises()

    expect(systemAPI.rollback).toHaveBeenCalledWith('0.1.163-ts.4')
    wrapper.unmount()
  })
})

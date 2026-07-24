import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('../client', () => ({
  apiClient: {
    get,
    post,
  },
}))

import {
  getCurrentDeploymentJob,
  getDeploymentJob,
  getMatchingCurrentDeploymentJob,
  getRollbackVersions,
  performUpdate,
  reconcileDeploymentVersion,
  replayOrRecoverCurrentDeployment,
  restartService,
  rollback,
  type DeploymentJob,
  type RollbackVersionInfo
} from '@/api/admin/system'

describe('admin system rollback API', () => {
  beforeEach(() => {
    sessionStorage.clear()
    get.mockReset()
    post.mockReset()
    vi.spyOn(globalThis.crypto, 'randomUUID').mockReturnValue(
      '11111111-1111-4111-8111-111111111111'
    )
  })

  it('getRollbackVersions fetches the rollback version list', async () => {
    const versions: RollbackVersionInfo[] = [
      {
        version: '0.1.146',
        published_at: '2026-07-07T00:00:00Z',
        html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.146'
      }
    ]
    get.mockResolvedValue({ data: { versions } })

    const result = await getRollbackVersions()

    expect(get).toHaveBeenCalledWith('/admin/system/rollback-versions')
    expect(result.versions).toEqual(versions)
  })

  it('does not run deployment recovery when the rollback versions GET fails', async () => {
    get.mockRejectedValueOnce(new Error('offline'))

    await expect(getRollbackVersions()).rejects.toThrow('offline')
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/admin/system/rollback-versions')
    expect(post).not.toHaveBeenCalled()
  })

  it('rollback posts the target version in the request body', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true } })

    const result = await rollback('0.1.146')

    expect(post).toHaveBeenCalledWith(
      '/admin/system/rollback',
      { version: '0.1.146' },
      {
        timeout: 900000,
        headers: {
          'Idempotency-Key':
            'system-rollback-0.1.146-11111111-1111-4111-8111-111111111111'
        }
      }
    )
    expect(result.need_restart).toBe(true)
  })

  it('rollback without a version posts no body (legacy backup rollback)', async () => {
    post.mockResolvedValue({ data: { message: 'ok', need_restart: true } })

    await rollback()

    expect(post).toHaveBeenCalledWith('/admin/system/rollback', undefined, {
      timeout: 900000,
      headers: {
        'Idempotency-Key':
          'system-rollback-local-backup-11111111-1111-4111-8111-111111111111'
      }
    })
  })

  it('restart posts a persistent idempotency key and clears it after success', async () => {
    post.mockResolvedValue({ data: { message: 'restarting' } })

    await expect(restartService()).resolves.toEqual({ message: 'restarting' })

    expect(post).toHaveBeenCalledWith('/admin/system/restart', undefined, {
      headers: {
        'Idempotency-Key': 'system-restart-11111111-1111-4111-8111-111111111111'
      }
    })
    expect(sessionStorage.length).toBe(0)
  })

  it('reuses the restart idempotency key after an ambiguous response failure', async () => {
    post.mockRejectedValueOnce(new Error('connection closed during restart'))
    await expect(restartService()).rejects.toThrow('connection closed during restart')
    const firstHeaders = post.mock.calls[0][2].headers
    expect(sessionStorage.length).toBe(1)

    vi.resetModules()
    post.mockResolvedValueOnce({ data: { message: 'restarting' } })
    const { restartService: restartAfterReload } = await import('@/api/admin/system')
    await restartAfterReload()

    expect(post.mock.calls[1][2].headers).toEqual(firstHeaders)
    expect(sessionStorage.length).toBe(0)
  })

  it('reuses the update idempotency key after an ambiguous response failure', async () => {
    post.mockRejectedValueOnce(new Error('network timeout'))
    await expect(performUpdate()).rejects.toThrow('network timeout')

    const runningJob = {
      id: 'job-recovered',
      action: 'update',
      target_version: '0.1.165-ts.1',
      status: 'running',
      stage: 'switching_traffic',
      rollback_performed: false,
      background_activated: false,
      created_at: new Date().toISOString(),
      started_at: new Date().toISOString(),
      updated_at: new Date().toISOString()
    } satisfies DeploymentJob

    post.mockResolvedValueOnce({
      data: { message: 'started', need_restart: false, job: runningJob }
    })
    await expect(performUpdate()).resolves.toMatchObject({ job: runningJob })

    expect(post).toHaveBeenCalledTimes(2)
    expect(post.mock.calls[1][2].headers).toEqual(post.mock.calls[0][2].headers)
    expect(sessionStorage.length).toBe(0)
  })

  it('does not adopt a stale same-target current job', async () => {
    const attemptStartedAt = Date.now()
    get.mockResolvedValueOnce({
      data: {
        id: 'job-stale',
        action: 'update',
        target_version: '0.1.165-ts.1',
        status: 'succeeded',
        stage: 'completed',
        rollback_performed: false,
        background_activated: true,
        created_at: new Date(attemptStartedAt - 60000).toISOString(),
        started_at: new Date(attemptStartedAt - 60000).toISOString(),
        updated_at: new Date(attemptStartedAt - 60000).toISOString()
      } satisfies DeploymentJob
    })

    await expect(
      getMatchingCurrentDeploymentJob('update', '0.1.165-ts.1', attemptStartedAt)
    ).resolves.toBeNull()
  })

  it('reuses the rollback idempotency key after a page reload', async () => {
    post.mockRejectedValueOnce(new Error('connection reset'))
    await expect(rollback('0.1.145')).rejects.toThrow('connection reset')
    const firstHeaders = post.mock.calls[0][2].headers

    vi.resetModules()
    post.mockResolvedValueOnce({ data: { message: 'started', need_restart: false } })
    const { rollback: rollbackAfterReload } = await import('@/api/admin/system')
    await rollbackAfterReload('0.1.145')

    expect(post.mock.calls[1][2].headers).toEqual(firstHeaders)
    expect(sessionStorage.length).toBe(0)
  })

  it('replays an ambiguous operation before consulting the current job', async () => {
    const replayed = { message: 'accepted', need_restart: false }
    const replay = vi.fn().mockResolvedValue(replayed)

    await expect(
      replayOrRecoverCurrentDeployment(replay, 'update', '0.1.165-ts.1', Date.now())
    ).resolves.toEqual({ result: replayed })
    expect(replay).toHaveBeenCalledOnce()
    expect(get).not.toHaveBeenCalled()
  })

  it('adopts only a fresh matching current job when replay remains unavailable', async () => {
    const attemptStartedAt = Date.now()
    const job = {
      id: 'job-current',
      action: 'update',
      target_version: '0.1.165-ts.1',
      status: 'running',
      stage: 'switching_traffic',
      rollback_performed: false,
      background_activated: false,
      created_at: new Date(attemptStartedAt).toISOString(),
      started_at: new Date(attemptStartedAt).toISOString(),
      updated_at: new Date(attemptStartedAt).toISOString()
    } satisfies DeploymentJob
    const replay = vi.fn().mockRejectedValue(new Error('still offline'))
    get.mockResolvedValueOnce({ data: job })

    await expect(
      replayOrRecoverCurrentDeployment(replay, 'update', job.target_version, attemptStartedAt)
    ).resolves.toEqual({ job })
    expect(replay).toHaveBeenCalledOnce()
    expect(get).toHaveBeenCalledWith('/admin/system/deployment-jobs/current')
  })

  it('reconciles both update and rollback when the target version is serving', () => {
    const updateJob = {
      id: 'job-update',
      action: 'update',
      target_version: '0.1.165-ts.1',
      status: 'running',
      stage: 'switching_traffic',
      rollback_performed: false,
      background_activated: false,
      created_at: '',
      started_at: '',
      updated_at: ''
    } satisfies DeploymentJob
    const rollbackJob = { ...updateJob, id: 'job-rollback', action: 'rollback' as const }

    expect(reconcileDeploymentVersion(updateJob, 'v0.1.165-ts.1')).toBe('succeeded')
    expect(reconcileDeploymentVersion(rollbackJob, '0.1.165-ts.1')).toBe('succeeded')

    const degradedJob: DeploymentJob = { ...updateJob, status: 'degraded' }
    expect(degradedJob.status).toBe('degraded')
  })

  it('loads durable deployment progress across a container switch', async () => {
    const job: DeploymentJob = {
      id: 'sysop-job-1',
      action: 'update',
      target_version: '0.1.165-ts.1',
      status: 'running',
      stage: 'switching_traffic',
      rollback_performed: false,
      background_activated: false,
      created_at: '2026-07-23T00:00:00Z',
      started_at: '2026-07-23T00:00:00Z',
      updated_at: '2026-07-23T00:00:05Z'
    }
    get.mockResolvedValue({ data: job })

    await expect(getDeploymentJob(job.id)).resolves.toEqual(job)
    expect(get).toHaveBeenCalledWith('/admin/system/deployment-jobs/sysop-job-1')

    await expect(getCurrentDeploymentJob()).resolves.toEqual(job)
    expect(get).toHaveBeenCalledWith('/admin/system/deployment-jobs/current')
  })
})

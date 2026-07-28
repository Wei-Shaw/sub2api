/**
 * System API endpoints for admin operations
 */

import { apiClient } from '../client'

export interface ReleaseInfo {
  name: string
  body: string
  published_at: string
  html_url: string
}

export interface VersionInfo {
  current_version: string
  latest_version: string
  has_update: boolean
  release_info?: ReleaseInfo
  cached: boolean
  warning?: string
  build_type: string // "source" for manual builds, "release" for CI builds
  deployment_mode: 'source' | 'standalone-binary' | 'docker-manual' | 'docker-managed'
  deployment_ready: boolean
  deployment_message?: string
}

export type DeploymentJobStatus =
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'rollback_failed'
  | 'degraded'

export interface DeploymentJob {
  id: string
  action: 'update' | 'rollback'
  target_version: string
  status: DeploymentJobStatus
  stage: string
  message?: string
  error?: string
  from_version?: string
  target_image?: string
  target_digest?: string
  rollback_performed: boolean
  background_activated: boolean
  rollback_error?: string
  cleanup_warning?: string
  control_plane_upgrade_status?: 'pending' | 'succeeded' | 'failed'
  control_plane_upgrade_error?: string
  created_at: string
  started_at: string
  updated_at: string
  finished_at?: string
}

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

/**
 * Check for updates
 * @param force - Force refresh from GitHub API
 */
export async function checkUpdates(force = false): Promise<VersionInfo> {
  const { data } = await apiClient.get<VersionInfo>('/admin/system/check-updates', {
    params: force ? { force: 'true' } : undefined
  })
  return data
}

export interface UpdateResult {
  message: string
  need_restart: boolean
  deployment_mode?: string
  job?: DeploymentJob
}

export interface RollbackVersionInfo {
  version: string
  published_at: string
  html_url: string
}

/**
 * Get versions available for rollback (up to 3 versions older than current)
 */
export async function getRollbackVersions(): Promise<{ versions: RollbackVersionInfo[] }> {
  const { data } = await apiClient.get<{ versions: RollbackVersionInfo[] }>(
    '/admin/system/rollback-versions'
  )
  return data
}

/**
 * In-place update/rollback downloads a full release binary from GitHub, which
 * can take several minutes on slow links. The global 30s axios timeout would
 * abort the request mid-download (#4504), so these calls wait as long as the
 * backend allows (15 minutes server-side).
 */
const UPDATE_REQUEST_TIMEOUT_MS = 15 * 60 * 1000

const systemOperationKeys = new Map<string, string>()

function systemOperationStorageKey(operation: string): string {
  return `sub2api:admin:system-operation:${operation}`
}

function readSystemOperationKey(operation: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(systemOperationStorageKey(operation)) ?? null
  } catch {
    return null
  }
}

function writeSystemOperationKey(operation: string, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(systemOperationStorageKey(operation), key)
    else globalThis.sessionStorage?.removeItem(systemOperationStorageKey(operation))
  } catch {
    // The in-memory copy still protects retries when browser storage is blocked.
  }
}

function getSystemOperationKey(operation: string): string {
  let key = systemOperationKeys.get(operation) ?? readSystemOperationKey(operation)
  if (!key) {
    const requestID =
      globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    key = `system-${operation}-${requestID}`
  }
  systemOperationKeys.set(operation, key)
  writeSystemOperationKey(operation, key)
  return key
}

function completeSystemOperation(operation: string): void {
  systemOperationKeys.delete(operation)
  writeSystemOperationKey(operation, null)
}

/**
 * Perform system update
 * Downloads and applies the latest version
 */
export async function performUpdate(): Promise<UpdateResult> {
  const operation = 'update'
  const idempotencyKey = getSystemOperationKey(operation)
  const { data } = await apiClient.post<UpdateResult>('/admin/system/update', undefined, {
    timeout: UPDATE_REQUEST_TIMEOUT_MS,
    headers: { 'Idempotency-Key': idempotencyKey }
  })
  completeSystemOperation(operation)
  return data
}

/**
 * Rollback to a previous version
 * @param version - Target version (e.g. "0.1.146"); omit to restore the local backup binary
 */
export async function rollback(version?: string): Promise<UpdateResult> {
  const normalizedVersion = version?.replace(/^v/, '') || 'local-backup'
  const operation = `rollback-${normalizedVersion}`
  const idempotencyKey = getSystemOperationKey(operation)
  const { data } = await apiClient.post<UpdateResult>(
    '/admin/system/rollback',
    version ? { version } : undefined,
    {
      timeout: UPDATE_REQUEST_TIMEOUT_MS,
      headers: { 'Idempotency-Key': idempotencyKey }
    }
  )
  completeSystemOperation(operation)
  return data
}

export type DeploymentVersionReconciliation = 'succeeded' | 'rolled_back' | null

export function reconcileDeploymentVersion(
  job: DeploymentJob,
  currentVersion: string
): DeploymentVersionReconciliation {
  const current = currentVersion.replace(/^v/, '')
  const target = job.target_version.replace(/^v/, '')
  const previous = job.from_version?.replace(/^v/, '')
  if (current === target) return 'succeeded'
  if (previous && current === previous) return 'rolled_back'
  return null
}

export async function getDeploymentJob(id: string): Promise<DeploymentJob> {
  const { data } = await apiClient.get<DeploymentJob>(`/admin/system/deployment-jobs/${id}`)
  return data
}

export async function getCurrentDeploymentJob(): Promise<DeploymentJob> {
  const { data } = await apiClient.get<DeploymentJob>('/admin/system/deployment-jobs/current')
  return data
}

export function deploymentJobMatchesOperation(
  job: DeploymentJob,
  action: DeploymentJob['action'],
  targetVersion: string,
  attemptStartedAt?: number
): boolean {
  const target = targetVersion.replace(/^v/, '')
  if (job.action !== action || (target && job.target_version.replace(/^v/, '') !== target)) {
    return false
  }
  if (attemptStartedAt === undefined) return true
  const createdAt = Date.parse(job.created_at)
  const updatedAt = Date.parse(job.updated_at)
  const lastObservedAt = Math.max(
    Number.isNaN(createdAt) ? 0 : createdAt,
    Number.isNaN(updatedAt) ? 0 : updatedAt
  )
  // Allow a small browser/server clock skew without adopting an unrelated old job.
  return lastObservedAt >= attemptStartedAt - 5000
}

export async function getMatchingCurrentDeploymentJob(
  action: DeploymentJob['action'],
  targetVersion: string,
  attemptStartedAt?: number
): Promise<DeploymentJob | null> {
  const job = await getCurrentDeploymentJob()
  return deploymentJobMatchesOperation(job, action, targetVersion, attemptStartedAt) ? job : null
}

export interface DeploymentRequestRecovery<T> {
  result?: T
  job?: DeploymentJob
}

export async function replayOrRecoverCurrentDeployment<T>(
  replay: () => Promise<T>,
  action: DeploymentJob['action'],
  targetVersion: string,
  attemptStartedAt: number
): Promise<DeploymentRequestRecovery<T> | null> {
  try {
    return { result: await replay() }
  } catch {
    try {
      const job = await getMatchingCurrentDeploymentJob(action, targetVersion, attemptStartedAt)
      return job ? { job } : null
    } catch {
      return null
    }
  }
}

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string }> {
  const operation = 'restart'
  const idempotencyKey = getSystemOperationKey(operation)
  const { data } = await apiClient.post<{ message: string }>('/admin/system/restart', undefined, {
    headers: { 'Idempotency-Key': idempotencyKey }
  })
  completeSystemOperation(operation)
  return data
}

export const systemAPI = {
  getVersion,
  checkUpdates,
  performUpdate,
  getRollbackVersions,
  rollback,
  getDeploymentJob,
  getCurrentDeploymentJob,
  getMatchingCurrentDeploymentJob,
  replayOrRecoverCurrentDeployment,
  restartService
}

export default systemAPI

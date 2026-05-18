import { apiClient } from '../client'

export interface BackupS3Config {
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
}

export interface BackupScheduleConfig {
  enabled: boolean
  cron_expr: string
  retain_days: number
  retain_count: number
}

export interface BackupRecord {
  id: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  backup_type: string
  file_name: string
  s3_key: string
  size_bytes: number
  triggered_by: string
  error_message?: string
  started_at: string
  finished_at?: string
  expires_at?: string
  progress?: string
  restore_status?: string
  restore_error?: string
  restored_at?: string
}

export interface BackupImportPreviewItem {
  file_name: string
  s3_key: string
  size_bytes: number
  last_modified?: string
  has_record: boolean
  record_id?: string
  can_import: boolean
  reason?: string
}

export interface BackupImportPreview {
  prefix: string
  total_objects: number
  existing_count: number
  missing_count: number
  importable_count: number
  items: BackupImportPreviewItem[]
}

export interface BackupImportResult {
  imported_count: number
  skipped_count: number
  items: BackupRecord[]
}

export interface CreateBackupRequest {
  expire_days?: number
}

export interface TestS3Response {
  ok: boolean
  message: string
}

// S3 Config
export async function getS3Config(): Promise<BackupS3Config> {
  const { data } = await apiClient.get<BackupS3Config>('/admin/backups/s3-config')
  return data
}

export async function updateS3Config(config: BackupS3Config): Promise<BackupS3Config> {
  const { data } = await apiClient.put<BackupS3Config>('/admin/backups/s3-config', config)
  return data
}

export async function testS3Connection(config: BackupS3Config): Promise<TestS3Response> {
  const { data } = await apiClient.post<TestS3Response>('/admin/backups/s3-config/test', config)
  return data
}

// Schedule
export async function getSchedule(): Promise<BackupScheduleConfig> {
  const { data } = await apiClient.get<BackupScheduleConfig>('/admin/backups/schedule')
  return data
}

export async function updateSchedule(config: BackupScheduleConfig): Promise<BackupScheduleConfig> {
  const { data } = await apiClient.put<BackupScheduleConfig>('/admin/backups/schedule', config)
  return data
}

// Backup operations
export async function createBackup(req?: CreateBackupRequest): Promise<BackupRecord> {
  const { data } = await apiClient.post<BackupRecord>('/admin/backups', req || {})
  return data
}

export async function listBackups(): Promise<{ items: BackupRecord[] }> {
  const { data } = await apiClient.get<{ items: BackupRecord[] }>('/admin/backups')
  return data
}

export async function getBackup(id: string): Promise<BackupRecord> {
  const { data } = await apiClient.get<BackupRecord>(`/admin/backups/${id}`)
  return data
}

export async function deleteBackup(id: string): Promise<void> {
  await apiClient.delete(`/admin/backups/${id}`)
}

export async function getDownloadURL(id: string): Promise<{ url: string }> {
  const { data } = await apiClient.get<{ url: string }>(`/admin/backups/${id}/download-url`)
  return data
}

export async function previewImportBackups(): Promise<BackupImportPreview> {
  const { data } = await apiClient.get<BackupImportPreview>('/admin/backups/import-preview')
  return data
}

export async function importMissingBackups(): Promise<BackupImportResult> {
  const { data } = await apiClient.post<BackupImportResult>('/admin/backups/import', {})
  return data
}

// Restore
export async function restoreBackup(id: string, password: string): Promise<BackupRecord> {
  const { data } = await apiClient.post<BackupRecord>(`/admin/backups/${id}/restore`, { password })
  return data
}

export const backupAPI = {
  getS3Config,
  updateS3Config,
  testS3Connection,
  getSchedule,
  updateSchedule,
  createBackup,
  listBackups,
  getBackup,
  deleteBackup,
  getDownloadURL,
  previewImportBackups,
  importMissingBackups,
  restoreBackup,
}

export default backupAPI

import { apiClient REDACTED from '../client'

export interface BackupS3Config {
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
REDACTED

export interface BackupScheduleConfig {
  enabled: boolean
  cron_expr: string
  retain_days: number
  retain_count: number
REDACTED

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
REDACTED

export interface CreateBackupRequest {
  expire_days?: number
REDACTED

export interface TestS3Response {
  ok: boolean
  message: string
REDACTED

// S3 Config
export async function getS3Config(): Promise<BackupS3Config> {
  const { data REDACTED = await apiClient.get<BackupS3Config>('/admin/backups/s3-config')
  return data
REDACTED

export async function updateS3Config(config: BackupS3Config): Promise<BackupS3Config> {
  const { data REDACTED = await apiClient.put<BackupS3Config>('/admin/backups/s3-config', config)
  return data
REDACTED

export async function testS3Connection(config: BackupS3Config): Promise<TestS3Response> {
  const { data REDACTED = await apiClient.post<TestS3Response>('/admin/backups/s3-config/test', config)
  return data
REDACTED

// Async image object storage
//
// Shares the S3 client with backups, so `reuse_backup_s3` borrows the endpoint and
// credentials configured above and only keeps its own bucket/prefix.
export interface ImageStorageConfig {
  enabled: boolean
  reuse_backup_s3: boolean
  bucket: string
  prefix: string
  public_base_url: string
  presign_expiry_hours: number
  max_download_bytes: number
  endpoint: string
  region: string
  access_key_id: string
  secret_access_key?: string
  force_path_style: boolean
REDACTED

export interface ImageStorageConfigResponse {
  config: ImageStorageConfig
  secret_configured: boolean
REDACTED

export async function getImageStorageConfig(): Promise<ImageStorageConfigResponse> {
  const { data REDACTED = await apiClient.get<ImageStorageConfigResponse>('/admin/backups/image-storage')
  return data
REDACTED

export async function updateImageStorageConfig(
  config: ImageStorageConfig,
): Promise<ImageStorageConfig> {
  const { data REDACTED = await apiClient.put<ImageStorageConfig>('/admin/backups/image-storage', config)
  return data
REDACTED

export async function testImageStorageConnection(
  config: ImageStorageConfig,
): Promise<TestS3Response> {
  const { data REDACTED = await apiClient.post<TestS3Response>(
    '/admin/backups/image-storage/test',
    config,
  )
  return data
REDACTED

// Schedule
export async function getSchedule(): Promise<BackupScheduleConfig> {
  const { data REDACTED = await apiClient.get<BackupScheduleConfig>('/admin/backups/schedule')
  return data
REDACTED

export async function updateSchedule(config: BackupScheduleConfig): Promise<BackupScheduleConfig> {
  const { data REDACTED = await apiClient.put<BackupScheduleConfig>('/admin/backups/schedule', config)
  return data
REDACTED

// Backup operations
export async function createBackup(req?: CreateBackupRequest): Promise<BackupRecord> {
  const { data REDACTED = await apiClient.post<BackupRecord>('/admin/backups', req || {REDACTED)
  return data
REDACTED

export async function listBackups(): Promise<{ items: BackupRecord[] REDACTED> {
  const { data REDACTED = await apiClient.get<{ items: BackupRecord[] REDACTED>('/admin/backups')
  return data
REDACTED

export async function getBackup(id: string): Promise<BackupRecord> {
  const { data REDACTED = await apiClient.get<BackupRecord>(`/admin/backups/${idREDACTED`)
  return data
REDACTED

export async function deleteBackup(id: string): Promise<void> {
  await apiClient.delete(`/admin/backups/${idREDACTED`)
REDACTED

export async function getDownloadURL(id: string): Promise<{ url: string REDACTED> {
  const { data REDACTED = await apiClient.get<{ url: string REDACTED>(`/admin/backups/${idREDACTED/download-url`)
  return data
REDACTED

// Restore
export async function restoreBackup(id: string, password: string): Promise<BackupRecord> {
  const { data REDACTED = await apiClient.post<BackupRecord>(`/admin/backups/${idREDACTED/restore`, { password REDACTED)
  return data
REDACTED

export const backupAPI = {
  getS3Config,
  updateS3Config,
  testS3Connection,
  getImageStorageConfig,
  updateImageStorageConfig,
  testImageStorageConnection,
  getSchedule,
  updateSchedule,
  createBackup,
  listBackups,
  getBackup,
  deleteBackup,
  getDownloadURL,
  restoreBackup,
REDACTED

export default backupAPI

import { apiClient } from '../client'

export interface DatabaseTableStats {
  schema: string
  name: string
  data_bytes: number
  index_bytes: number
  total_bytes: number
  live_rows: number
  dead_rows: number
  last_vacuum: string | null
  cleanup_supported: boolean
}

export interface DatabaseStats {
  name: string
  size_bytes: number
  tables: DatabaseTableStats[]
}

export type DatabaseOperation = 'cleanup' | 'vacuum' | 'vacuum_full'

export interface DatabaseMaintenanceRequest {
  operation: DatabaseOperation
  tables: string[]
  retention_days: number
  confirm: boolean
}

export interface DatabaseMaintenanceJob {
  id: string
  operation: DatabaseOperation
  tables: string[]
  status: 'running' | 'succeeded' | 'failed'
  created_by: number
  started_at: string
  updated_at: string
  finished_at: string | null
  cutoff: string | null
  current_table: string
  completed_tables: number
  deleted_rows: number
  error: string
}

export async function getDatabaseStats(): Promise<DatabaseStats> {
  const { data } = await apiClient.get<DatabaseStats>('/admin/system/database')
  return data
}

export async function getDatabaseMaintenanceJob(): Promise<DatabaseMaintenanceJob | null> {
  const { data } = await apiClient.get<{ job: DatabaseMaintenanceJob | null }>('/admin/system/database/job')
  return data.job
}

export async function startDatabaseMaintenance(request: DatabaseMaintenanceRequest, idempotencyKey: string): Promise<DatabaseMaintenanceJob> {
  const { data } = await apiClient.post<DatabaseMaintenanceJob>('/admin/system/database/maintenance', request, { headers: { 'Idempotency-Key': idempotencyKey } })
  return data
}

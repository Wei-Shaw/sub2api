/**
 * Cluster API endpoints for admin operations
 * Provides cluster status monitoring for distributed deployments
 */

import { apiClient } from '../client'

export interface ClusterInstance {
  id: string
  hostname: string
  ip?: string
  role: 'master' | 'slave' | 'standalone'
  started_at: string
  last_heartbeat: string
  status: 'healthy' | 'unhealthy' | 'unknown'
  running_tasks?: string[]
  version?: string
}

export interface ClusterStatusResponse {
  current_instance: ClusterInstance | null
  instances: ClusterInstance[]
  master_id: string
  cluster_mode: 'master-slave' | 'standalone'
  cluster_healthy: boolean
  total_instances: number
  healthy_count: number
  master_count: number
  slave_count: number
  cluster_enabled: boolean
}

export interface MasterStatusResponse {
  is_master: boolean
  instance_id: string
  role: 'master' | 'slave' | 'standalone'
}

/**
 * Get cluster status
 * Returns status of all instances in the cluster
 */
export async function getClusterStatus(): Promise<ClusterStatusResponse> {
  const { data } = await apiClient.get<ClusterStatusResponse>('/admin/cluster/status')
  return data
}

/**
 * Get current instance info
 * Returns information about this specific instance
 */
export async function getCurrentInstance(): Promise<ClusterInstance> {
  const { data } = await apiClient.get<ClusterInstance>('/admin/cluster/instance')
  return data
}

/**
 * Check master status
 * Returns whether this instance is the master (runs background tasks)
 */
export async function getMasterStatus(): Promise<MasterStatusResponse> {
  const { data } = await apiClient.get<MasterStatusResponse>('/admin/cluster/master')
  return data
}

export const clusterAPI = {
  getClusterStatus,
  getCurrentInstance,
  getMasterStatus
}

export default clusterAPI

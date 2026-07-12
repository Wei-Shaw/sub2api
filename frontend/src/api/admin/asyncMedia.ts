import { apiClient } from '../client'

// AsyncMediaRuntimeConfig 对应后端 service.AsyncMediaRuntimeConfig，
// 用于异步媒体（fal 等）reconciler 的运行时配置（可在后台热更新）。
export interface AsyncMediaRuntimeConfig {
  // reconciler 扫描未终结任务的间隔（秒）。
  reconcile_interval_seconds: number
  // 任务从创建到强制判失（退费兜底）的最长时间（秒）。
  fail_timeout_seconds: number
}

export async function getAsyncMediaConfig(): Promise<AsyncMediaRuntimeConfig> {
  const { data } = await apiClient.get<AsyncMediaRuntimeConfig>('/admin/async-media/config')
  return data
}

export async function updateAsyncMediaConfig(
  config: AsyncMediaRuntimeConfig,
): Promise<AsyncMediaRuntimeConfig> {
  const { data } = await apiClient.put<AsyncMediaRuntimeConfig>('/admin/async-media/config', config)
  return data
}

export const asyncMediaAPI = {
  getAsyncMediaConfig,
  updateAsyncMediaConfig,
}

export default asyncMediaAPI

/**
 * Video Playground API
 *
 * 独立的 axios 实例，用于演练台直调视频门面（/api/v1/model/{model}...）。
 * 关键约束：
 *   - 不复用主 apiClient：主实例会自动注入 JWT、剥壳 {code,data}、401 跳登录，
 *     这些都不适合"用户拿一把 API Key 走真实调用链"的语义。
 *   - Authorization 由调用方显式传入（Bearer sk-xxx），完全 1:1 复刻用户
 *     在自己代码里的调用体验。
 *   - Response 原样透传（fal 原生 payload 不套 {code,data}）。
 *   - 5xx/4xx 不做重定向，直接抛给调用方展示错误详情。
 */

import axios, { AxiosInstance } from 'axios'

// 独立 axios 实例：baseURL 走同源相对路径，
// 由 Vite dev proxy 或生产 Caddy 转发到后端。
const playgroundClient: AxiosInstance = axios.create({
  baseURL: '/api/v1/model',
  timeout: 60_000, // 视频任务提交/查询本身很快；60s 兜底防止 hang 死
  headers: {
    'Content-Type': 'application/json',
  },
})

export interface SubmitResponse {
  request_id: string
  status?: string
  status_url?: string
  response_url?: string
  cancel_url?: string
  [k: string]: unknown
}

export interface StatusResponse {
  status: string // IN_QUEUE | IN_PROGRESS | COMPLETED | FAILED | CANCELED（fal 原生大小写）
  request_id?: string
  queue_position?: number
  logs?: Array<{ message?: string; level?: string; timestamp?: string } | string>
  [k: string]: unknown
}

// fal 原生 result payload：video 通常是 { url, content_type, ... } 或 { videos: [...] }
export interface ResultResponse {
  video?: { url?: string; content_type?: string; file_name?: string; file_size?: number }
  videos?: Array<{ url?: string; content_type?: string; file_name?: string; file_size?: number }>
  images?: Array<{
    url?: string
    content_type?: string
    file_name?: string
    file_size?: number
    width?: number
    height?: number
  }>
  seed?: number
  [k: string]: unknown
}

function authHeader(apiKey: string): Record<string, string> {
  return { Authorization: `Bearer ${apiKey.trim()}` }
}

/**
 * 提交视频任务
 * @param slug 完整 fal slug（例如 fal-ai/bytedance/seedance-2.5/text-to-video）
 * @param body fal 原生请求体（prompt / image_url / duration / resolution ...）
 * @param apiKey 用户 API Key（明文 sk-...）
 * @param options.internalRequestId 可选，前端预生成的稳定 ID；
 *   传入后作为 x-client-request-id header，后端会把它作为 async_video_tasks.internal_request_id
 *   落库，前端可用它在任务终态后查询 final_cost（实扣费用）。
 */
export async function submit(
  slug: string,
  body: Record<string, unknown>,
  apiKey: string,
  options?: { signal?: AbortSignal; internalRequestId?: string }
): Promise<SubmitResponse> {
  const headers: Record<string, string> = authHeader(apiKey)
  if (options?.internalRequestId) {
    headers['x-client-request-id'] = options.internalRequestId
  }
  const { data } = await playgroundClient.post<SubmitResponse>(
    `/${slug}`,
    body,
    { headers, signal: options?.signal }
  )
  return data
}

/**
 * 查询任务状态
 */
export async function status(
  slug: string,
  requestId: string,
  apiKey: string,
  options?: { signal?: AbortSignal }
): Promise<StatusResponse> {
  const { data } = await playgroundClient.get<StatusResponse>(
    `/${slug}/requests/${requestId}/status`,
    { headers: authHeader(apiKey), signal: options?.signal }
  )
  return data
}

/**
 * 拿最终结果（fal 原生 payload）
 */
export async function result(
  slug: string,
  requestId: string,
  apiKey: string,
  options?: { signal?: AbortSignal }
): Promise<ResultResponse> {
  const { data } = await playgroundClient.get<ResultResponse>(
    `/${slug}/requests/${requestId}`,
    { headers: authHeader(apiKey), signal: options?.signal }
  )
  return data
}

/**
 * 从 fal result payload 中抽出可播放的 video URL 列表（尽量鲁棒）。
 */
export function extractVideoUrls(payload: ResultResponse | undefined | null): string[] {
  if (!payload || typeof payload !== 'object') return []
  const urls: string[] = []
  if (payload.video && typeof payload.video.url === 'string' && payload.video.url) {
    urls.push(payload.video.url)
  }
  if (Array.isArray(payload.videos)) {
    for (const v of payload.videos) {
      if (v && typeof v.url === 'string' && v.url) urls.push(v.url)
    }
  }
  return urls
}

export const videoPlaygroundAPI = {
  submit,
  status,
  result,
  extractVideoUrls,
}

export default videoPlaygroundAPI

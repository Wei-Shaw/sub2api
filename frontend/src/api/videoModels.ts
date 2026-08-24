import apiClient from './client'
import type { OutputFieldSpec, ResultMediaType } from './admin/modelIntros'

export type { OutputFieldSpec, ResultMediaType } from './admin/modelIntros'

export interface VideoModelPricingItem {
  resolution: string
  price_per_second: number
  currency: string
  enabled: boolean
}

// VideoModelIntro 由管理员在"模型介绍"菜单中配置：
//   - title / description / cover_url 用于卡片展示；
//   - default_params 用于生成 curl 示例的示例 payload，同时（新格式下）作为
//     演练台表单字段声明的来源：每个 key 的 value 可为
//       { value, required?, description?, enum?, options? }
//     旧格式（value 直接是原始值）也保留，只是演练台会退化为纯 JSON 模式。
//   - output_fields 用于演练台在任务完成后按声明的字段列表提取并渲染结果：
//     支持 video / image / url / text / json / number 六种展示类型。未配置或为空时演练台
//     不展示结果字段区，仅展示原始 payload。
//   - result_field / result_type 用于指示"主结果字段"：result_field 为空时演练台
//     按 output_fields 顺序取第一个 video/image 字段作为主结果；非空时强制将
//     result_field 匹配到的字段以 result_type（video / image）大尺寸展示。
// 若管理员未配置或已禁用 enabled，后端不会下发此字段。
export interface VideoModelIntro {
  title: string
  description: string
  /**
   * description_en：模型介绍的英文版本。后端下发（可能为空串）；
   * 前端按当前 locale 选择：英文界面优先展示 description_en，
   * 缺失时回落到 description；中文界面反之。
   */
  description_en: string
  cover_url: string
  default_params: Record<string, unknown>
  output_fields: OutputFieldSpec[]
  result_field: string
  result_type: ResultMediaType
}

export interface VideoModelItem {
  slug: string
  family: string
  variant: string
  display_name: string
  submit_path: string
  status_path: string
  result_path: string
  cancel_path: string
  pricing: VideoModelPricingItem[]
  available: boolean
  intro?: VideoModelIntro | null
}

export interface VideoModelListResponse {
  items: VideoModelItem[]
  total: number
  supported_resolutions: string[]
}

// VideoTaskItem 是演练历史列表的单条数据（对应后端 videoTaskItem DTO）。
// request_payload / result_payload 是 fal 上游原样的 JSON，前端"重放"时把
// request_payload 塞回演练表单即可；未终结任务的 result_payload 为空对象。
export interface VideoTaskItem {
  id: number
  internal_request_id: string
  upstream_request_id: string
  requested_model: string
  status: string // pending / running / succeeded / failed / refunded / expired / refund_failed
  resolution: string
  duration_seconds: number
  aspect_ratio: string
  final_cost: number
  held_cost: number
  error_reason: string
  video_urls: string[]
  cos_urls: string[]
  image_urls?: string[]
  media_type?: string
  request_payload: Record<string, unknown> | null
  result_payload: Record<string, unknown> | null
  created_at: string
  finished_at?: string
}

export interface VideoTasksResponse {
  items: VideoTaskItem[]
  total: number
  page: number
  page_size: number
}

const videoModelsAPI = {
  list() {
    return apiClient.get<VideoModelListResponse>('/user/video-models')
  },
  /**
   * listTasks：分页拉取当前用户在指定 slug 下的演练历史。
   *
   * 后端强制要求携带 slug（Q3-1 B 方案），空 slug 会得到 400。
   */
  listTasks(slug: string, page = 1, pageSize = 20) {
    return apiClient.get<VideoTasksResponse>('/user/video-models/tasks', {
      params: { slug, page, page_size: pageSize },
    })
  },
  /**
   * getTaskByRequestId：按 internal_request_id 拉单条任务详情。
   *
   * 用途：演练台任务终态后拉一次，前端展示 final_cost（实扣费用）。
   * 权限：后端强制校验 task.user_id === current user，非本人返回 404。
   */
  getTaskByRequestId(rid: string) {
    return apiClient.get<VideoTaskItem>(`/user/video-models/tasks/by-request/${encodeURIComponent(rid)}`)
  },
  /**
   * getTaskById：按 async_video_tasks.id 拉单条任务详情。
   *
   * 用途：使用记录页视频行"详情"入口——usage_logs.task_id 存的即为该 id。
   * 权限：后端强制校验 task.user_id === current user，非本人返回 404。
   */
  getTaskById(id: number) {
    return apiClient.get<VideoTaskItem>(`/user/video-models/tasks/by-id/${id}`)
  },
  /**
   * getTaskByIdAdmin：管理员按 async_video_tasks.id 拉任意用户的任务详情。
   *
   * 用途：管理员使用记录页视频行"详情"入口。
   * 权限：AdminAuthMiddleware 保证；不做归属校验，管理员可查看所有用户任务。
   */
  getTaskByIdAdmin(id: number) {
    return apiClient.get<VideoTaskItem>(`/admin/video-tasks/by-id/${id}`)
  },
  completeManualBillingAdmin(id: number, finalCost: number) {
    return apiClient.patch<VideoTaskItem>(`/admin/video-tasks/by-id/${id}/billing`, {
      final_cost: finalCost,
    })
  },
}

export default videoModelsAPI

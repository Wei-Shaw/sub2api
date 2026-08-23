/**
 * userMaterials.ts
 *
 * 用户素材库 API（对应后端 /user/materials/*）：
 *   - list(kind?, keyword?, page?, pageSize?)  按类型 / 文件名过滤 + 分页
 *   - upload(file)                             multipart 上传单文件
 *   - importFromUrl(url)                       后端下载外链再转存到 COS
 *   - rename(id, fileName)                     修改自己的素材展示名称
 *   - remove(id)                               软删自己的素材
 *
 * 所有接口都以当前 JWT 用户为主体，后端强制过滤 user_id，无需在前端传 userId。
 */
import apiClient from './client'

/**
 * UserMaterialKind：素材大类。演练台的"图片输入控件"只会拉 kind='image' 的素材，
 * 独立"素材库"页面则可切换 image / audio / video。
 */
export type UserMaterialKind = 'image' | 'audio' | 'video'

/**
 * UserMaterialItem：单条素材记录。url 就是最终写回业务侧、传给上游模型的 URL。
 * created_at 为 ISO 8601 UTC 字符串。
 */
export interface UserMaterialItem {
  id: number
  file_name: string
  url: string
  content_type: string
  size_bytes: number
  kind: UserMaterialKind | 'other'
  source: 'upload' | 'url_import'
  created_at: string
}

export interface UserMaterialListResponse {
  items: UserMaterialItem[]
  total: number
  page: number
  page_size: number
}

const userMaterialsAPI = {
  /**
   * list：按 kind / keyword 过滤，按 created_at 倒序分页。
   * kind 传空串或省略时不按类型过滤。
   */
  list(params: { kind?: string; keyword?: string; page?: number; pageSize?: number } = {}) {
    const { kind = '', keyword = '', page = 1, pageSize = 20 } = params
    return apiClient.get<UserMaterialListResponse>('/user/materials', {
      params: { kind, keyword, page, page_size: pageSize },
    })
  },

  /**
   * upload：multipart/form-data 单文件上传。
   * 后端会把字节转存到 COS 的用户目录下并落库，返回带 COS URL 的完整记录。
   */
  upload(file: File) {
    const fd = new FormData()
    fd.append('file', file)
    return apiClient.post<UserMaterialItem>('/user/materials/upload', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  /**
   * importFromUrl：后端拉取外部 URL、转存 COS 再落库；最终保存的是 COS URL。
   * 参考理由：外链会失效、CORS/防盗链会踩坑，且能与"用户 ID 前缀目录"这条规则统一。
   */
  importFromUrl(url: string) {
    return apiClient.post<UserMaterialItem>('/user/materials/import-url', { url })
  },

  /**
   * rename：只修改展示名称，不移动 COS 对象，也不改变素材 URL。
   */
  rename(id: number, fileName: string) {
    return apiClient.patch<UserMaterialItem>(`/user/materials/${id}`, { file_name: fileName })
  },

  /**
   * remove：软删（DB 打 deleted_at；COS 对象保留一段时间由后台任务清理）。
   */
  remove(id: number) {
    return apiClient.delete<{ deleted: number }>(`/user/materials/${id}`)
  },
}

export default userMaterialsAPI

import { apiClient } from '../client'

/**
 * 管理员「文件管理」API：直接操作图片转存（COS / S3 兼容）桶里的对象。
 *
 * 全部接口依赖图片转存已启用；未启用时后端返回 reason=COS_NOT_CONFIGURED，
 * 前端据此渲染引导页而不是空列表。
 */

/** AdminFileEntry 单条文件或"目录"。目录是按 "/" 聚合出的逻辑层级（S3 CommonPrefix）。 */
export interface AdminFileEntry {
  /** 完整对象键（含前缀）。目录以 "/" 结尾。 */
  key: string
  /** 去掉当前前缀后的展示名。 */
  name: string
  size: number
  last_modified: string
  etag: string
  is_dir: boolean
  /** 按配置拼出的对外地址；桶为私有读时不可直接访问，仅供复制。 */
  public_url: string
}

export interface AdminFileListResult {
  entries: AdminFileEntry[]
  /** 非空表示还有下一页，回传即可继续。对象存储只支持游标分页，没有跳页。 */
  next_token: string
  /** 本次列举使用的前缀（已归一化），用于渲染面包屑。 */
  prefix: string
}

export interface AdminFileStatus {
  enabled: boolean
  bucket: string
  prefix: string
}

export interface AdminFileDeleteResult {
  deleted: number
  failed: number
  /** key → 失败原因。 */
  failures: Record<string, string>
}

/** getStatus：文件管理是否可用 + 正在管理的桶/前缀。 */
export async function getStatus(): Promise<AdminFileStatus> {
  const { data } = await apiClient.get<AdminFileStatus>('/admin/files/status')
  return data
}

/**
 * list：列举对象。
 * @param prefix 目录前缀，如 "images/2026/"；空表示桶根
 * @param token 上一页返回的 next_token
 * @param flat true 时递归平铺该前缀下所有对象（跨层级查看），默认按目录聚合
 */
export async function list(params: {
  prefix?: string
  token?: string
  limit?: number
  flat?: boolean
}): Promise<AdminFileListResult> {
  const { data } = await apiClient.get<AdminFileListResult>('/admin/files', {
    params: {
      prefix: params.prefix || '',
      token: params.token || '',
      limit: params.limit ?? 100,
      ...(params.flat ? { flat: 1 } : {}),
    },
  })
  return data
}

/** getDownloadURL：取预签名下载直链（10 分钟内有效）。 */
export async function getDownloadURL(key: string): Promise<{ url: string; key: string; expires_in: number }> {
  const { data } = await apiClient.get<{ url: string; key: string; expires_in: number }>(
    '/admin/files/download-url',
    { params: { key } }
  )
  return data
}

/**
 * upload：上传文件到指定目录。
 * 最终 key = prefix + (name ?? 文件原名)；同名对象会被覆盖。
 * 超时放宽到 10 分钟：管理端可能上传几百 MB 的视频。
 */
export async function upload(
  file: File,
  options?: { prefix?: string; name?: string; onProgress?: (percent: number) => void }
): Promise<AdminFileEntry> {
  const form = new FormData()
  form.append('file', file)
  if (options?.prefix) form.append('prefix', options.prefix)
  if (options?.name) form.append('name', options.name)
  const { data } = await apiClient.post<AdminFileEntry>('/admin/files/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 600_000,
    onUploadProgress: (e) => {
      if (!options?.onProgress || !e.total) return
      options.onProgress(Math.round((e.loaded / e.total) * 100))
    },
  })
  return data
}

/**
 * rename：改名或移动。
 * 只传 name 时在原目录内改名；传 newKey 可跨目录移动。
 * 后端实现为服务端 copy + 删源；目标键已存在时返回 409 OBJECT_KEY_EXISTS。
 */
export async function rename(key: string, opts: { name?: string; newKey?: string }): Promise<AdminFileEntry> {
  const { data } = await apiClient.put<AdminFileEntry>('/admin/files/rename', {
    key,
    name: opts.name || '',
    new_key: opts.newKey || '',
  })
  return data
}

/** remove：批量删除（单次最多 100 条）。部分失败会在 failures 里逐条给出原因。 */
export async function remove(keys: string[]): Promise<AdminFileDeleteResult> {
  const { data } = await apiClient.delete<AdminFileDeleteResult>('/admin/files', {
    data: { keys },
  })
  return data
}

const adminFilesAPI = {
  getStatus,
  list,
  getDownloadURL,
  upload,
  rename,
  remove,
}

export default adminFilesAPI

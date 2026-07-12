import { apiClient } from '../client'

// COSImageConfig 对应后端 service.COSImageConfig，用于全局图片转存（腾讯云 COS / S3 兼容）。
export interface COSImageConfig {
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  // 写入时填入新密钥；留空表示保持原值。读取时后端不回显明文。
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
  public_base_url: string
}

export interface COSImageConfigResponse {
  config: COSImageConfig
  // 标识后端是否已设置 SecretKey（明文不回显）。
  secret_access_key_set: boolean
}

export async function getCOSImageConfig(): Promise<COSImageConfigResponse> {
  const { data } = await apiClient.get<COSImageConfigResponse>('/admin/cos-image/config')
  return data
}

export async function updateCOSImageConfig(config: COSImageConfig): Promise<COSImageConfig> {
  const { data } = await apiClient.put<COSImageConfig>('/admin/cos-image/config', config)
  return data
}

export const cosImageAPI = {
  getCOSImageConfig,
  updateCOSImageConfig,
}

export default cosImageAPI

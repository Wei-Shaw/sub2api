import { apiClient REDACTED from '@/api/client'

export async function getPlatformModels(platform: string): Promise<string[]> {
  const { data REDACTED = await apiClient.get<string[]>('/admin/models', {
    params: { platform REDACTED
  REDACTED)
  return data
REDACTED

export const modelsAPI = {
  getPlatformModels
REDACTED

export default modelsAPI

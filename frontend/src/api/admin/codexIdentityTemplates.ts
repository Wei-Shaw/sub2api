import { apiClient } from '../client'
import type {
  CodexIdentityTemplate,
  CodexIdentityTemplateUpdateRequest,
  CodexIdentityTemplateWriteRequest,
} from '@/types/codexIdentity'

const basePath = '/admin/settings/codex-identity-templates'

export async function list(): Promise<CodexIdentityTemplate[]> {
  const { data } = await apiClient.get<{ items: CodexIdentityTemplate[] }>(basePath)
  return data.items ?? []
}

export async function getByID(id: number): Promise<CodexIdentityTemplate> {
  const { data } = await apiClient.get<CodexIdentityTemplate>(`${basePath}/${id}`)
  return data
}

export async function create(
  payload: CodexIdentityTemplateWriteRequest,
): Promise<CodexIdentityTemplate> {
  const { data } = await apiClient.post<CodexIdentityTemplate>(basePath, payload)
  return data
}

export async function update(
  id: number,
  payload: CodexIdentityTemplateUpdateRequest,
): Promise<CodexIdentityTemplate> {
  const { data } = await apiClient.put<CodexIdentityTemplate>(`${basePath}/${id}`, payload)
  return data
}

export async function deleteTemplate(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`${basePath}/${id}`)
  return data
}

export const codexIdentityTemplatesAPI = {
  list,
  getByID,
  create,
  update,
  delete: deleteTemplate,
}

export default codexIdentityTemplatesAPI

import { apiClient } from '../client'
import type { PaginatedResponse, ProxyGroup } from '@/types'

export interface ProxyGroupInput {
  name: string
  description?: string | null
  status?: 'active' | 'inactive'
  proxy_ids: number[]
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: { status?: 'active' | 'inactive'; search?: string }
): Promise<PaginatedResponse<ProxyGroup>> {
  const { data } = await apiClient.get<PaginatedResponse<ProxyGroup>>('/admin/proxy-groups', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function getAll(): Promise<ProxyGroup[]> {
  const { data } = await apiClient.get<ProxyGroup[]>('/admin/proxy-groups/all')
  return data
}

export async function getById(id: number): Promise<ProxyGroup> {
  const { data } = await apiClient.get<ProxyGroup>(`/admin/proxy-groups/${id}`)
  return data
}

export async function create(input: ProxyGroupInput): Promise<ProxyGroup> {
  const { data } = await apiClient.post<ProxyGroup>('/admin/proxy-groups', input)
  return data
}

export async function update(id: number, input: ProxyGroupInput): Promise<ProxyGroup> {
  const { data } = await apiClient.put<ProxyGroup>(`/admin/proxy-groups/${id}`, input)
  return data
}

export async function deleteGroup(id: number): Promise<{ id: number }> {
  const { data } = await apiClient.delete<{ id: number }>(`/admin/proxy-groups/${id}`)
  return data
}

export default { list, getAll, getById, create, update, deleteGroup }

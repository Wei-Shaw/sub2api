/**
 * Admin Composite route scheme API
 */

import { apiClient } from '../client'
import type {
  CompositeModelRoute,
  CompositeModelRouteInput,
  CompositeRouteDecision,
  CompositeRoutePreviewRequest,
  CompositeRouteScheme,
  CompositeRouteSchemeInput
} from '@/types'

export async function listRouteSchemes(): Promise<CompositeRouteScheme[]> {
  const { data } = await apiClient.get<CompositeRouteScheme[]>('/admin/composite-route-schemes')
  return data
}

export async function getRouteScheme(id: number): Promise<CompositeRouteScheme> {
  const { data } = await apiClient.get<CompositeRouteScheme>(`/admin/composite-route-schemes/${id}`)
  return data
}

export async function createRouteScheme(
  input: CompositeRouteSchemeInput
): Promise<CompositeRouteScheme> {
  const { data } = await apiClient.post<CompositeRouteScheme>('/admin/composite-route-schemes', input)
  return data
}

export async function updateRouteScheme(
  id: number,
  input: CompositeRouteSchemeInput
): Promise<CompositeRouteScheme> {
  const { data } = await apiClient.put<CompositeRouteScheme>(
    `/admin/composite-route-schemes/${id}`,
    input
  )
  return data
}

export async function deleteRouteScheme(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/composite-route-schemes/${id}`
  )
  return data
}

export async function duplicateRouteScheme(
  id: number,
  name?: string
): Promise<CompositeRouteScheme> {
  const { data } = await apiClient.post<CompositeRouteScheme>(
    `/admin/composite-route-schemes/${id}/duplicate`,
    name ? { name } : {}
  )
  return data
}

export async function listRouteSchemeRoutes(id: number): Promise<CompositeModelRoute[]> {
  const { data } = await apiClient.get<CompositeModelRoute[]>(
    `/admin/composite-route-schemes/${id}/routes`
  )
  return data
}

export async function createRouteSchemeRoute(
  id: number,
  route: CompositeModelRouteInput
): Promise<CompositeModelRoute> {
  const { data } = await apiClient.post<CompositeModelRoute>(
    `/admin/composite-route-schemes/${id}/routes`,
    route
  )
  return data
}

export async function updateRouteSchemeRoute(
  id: number,
  routeId: number,
  route: CompositeModelRouteInput
): Promise<CompositeModelRoute> {
  const { data } = await apiClient.put<CompositeModelRoute>(
    `/admin/composite-route-schemes/${id}/routes/${routeId}`,
    route
  )
  return data
}

export async function deleteRouteSchemeRoute(
  id: number,
  routeId: number
): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/composite-route-schemes/${id}/routes/${routeId}`
  )
  return data
}

export async function previewRouteScheme(
  id: number,
  request: CompositeRoutePreviewRequest
): Promise<CompositeRouteDecision> {
  const { data } = await apiClient.post<CompositeRouteDecision>(
    `/admin/composite-route-schemes/${id}/preview`,
    request
  )
  return data
}

export const routeSchemesAPI = {
  list: listRouteSchemes,
  get: getRouteScheme,
  create: createRouteScheme,
  update: updateRouteScheme,
  delete: deleteRouteScheme,
  duplicate: duplicateRouteScheme,
  listRoutes: listRouteSchemeRoutes,
  createRoute: createRouteSchemeRoute,
  updateRoute: updateRouteSchemeRoute,
  deleteRoute: deleteRouteSchemeRoute,
  preview: previewRouteScheme
}

export default routeSchemesAPI

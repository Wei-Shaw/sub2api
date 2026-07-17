import { apiClient REDACTED from '@/api/client'
import type {
  PromptAuditConfig,
  PromptAuditEvent,
  PromptAuditGroup,
  PromptAuditRuntime,
  PromptAuditUpdateRequest,
  PromptDeletePreview,
  PromptDeleteResult,
  PromptEventFilters,
  PromptEventPage,
  PromptProbeResult,
  PromptAuditEndpointDraft,
REDACTED from './types'
import { eventFilterPayload, eventQueryParams REDACTED from './viewModel'

const basePath = '/admin/prompt-audit'

export async function getConfig(): Promise<PromptAuditConfig> {
  const { data REDACTED = await apiClient.get<PromptAuditConfig>(`${basePathREDACTED/config`)
  return data
REDACTED

export async function updateConfig(payload: PromptAuditUpdateRequest): Promise<PromptAuditConfig> {
  const { data REDACTED = await apiClient.put<PromptAuditConfig>(`${basePathREDACTED/config`, payload)
  return data
REDACTED

export async function probeEndpoint(endpoint: PromptAuditEndpointDraft): Promise<PromptProbeResult> {
  const { data REDACTED = await apiClient.post<PromptProbeResult>(`${basePathREDACTED/endpoints/probe`, {
    endpoint: {
      id: endpoint.id,
      name: endpoint.name,
      protocol: 'openai_compatible',
      base_url: endpoint.base_url,
      model: endpoint.model,
      token: endpoint.token || undefined,
      timeout_ms: endpoint.timeout_ms,
      input_limit: endpoint.input_limit,
      enabled: endpoint.enabled,
    REDACTED,
  REDACTED)
  return data
REDACTED

export async function getRuntime(): Promise<PromptAuditRuntime> {
  const { data REDACTED = await apiClient.get<PromptAuditRuntime>(`${basePathREDACTED/runtime`)
  return data
REDACTED

export async function listEvents(
  filters: PromptEventFilters,
  page: number,
  pageSize: number,
): Promise<PromptEventPage> {
  const { data REDACTED = await apiClient.get<PromptEventPage>(`${basePathREDACTED/events`, {
    params: { page, page_size: pageSize, ...eventQueryParams(filters) REDACTED,
  REDACTED)
  return data
REDACTED

export async function getEvent(id: number): Promise<PromptAuditEvent> {
  const { data REDACTED = await apiClient.get<PromptAuditEvent>(`${basePathREDACTED/events/${idREDACTED`)
  return data
REDACTED

export async function deleteEvent(id: number): Promise<PromptDeleteResult> {
  const { data REDACTED = await apiClient.delete<PromptDeleteResult>(`${basePathREDACTED/events/${idREDACTED`)
  return data
REDACTED

export async function batchDeleteEvents(ids: number[]): Promise<PromptDeleteResult> {
  const { data REDACTED = await apiClient.post<PromptDeleteResult>(`${basePathREDACTED/events/batch-delete`, { ids REDACTED)
  return data
REDACTED

export async function previewDelete(filters: PromptEventFilters): Promise<PromptDeletePreview> {
  const { data REDACTED = await apiClient.post<PromptDeletePreview>(
    `${basePathREDACTED/events/delete-preview`,
    eventFilterPayload(filters),
  )
  return data
REDACTED

export async function deleteEventsByFilter(
  filters: PromptEventFilters,
  preview: PromptDeletePreview,
): Promise<PromptDeleteResult> {
  const { data REDACTED = await apiClient.post<PromptDeleteResult>(`${basePathREDACTED/events/delete-by-filter`, {
    filter: eventFilterPayload(filters),
    snapshot_max_id: preview.snapshot_max_id,
    filter_hash: preview.filter_hash,
    confirmation_token: preview.confirmation_token,
    confirm: true,
  REDACTED)
  return data
REDACTED

export async function listGroups(): Promise<PromptAuditGroup[]> {
  const { data REDACTED = await apiClient.get<PromptAuditGroup[]>('/admin/groups/all', {
    params: { include_inactive: true REDACTED,
  REDACTED)
  return data
REDACTED

export const promptAuditAPI = {
  getConfig,
  updateConfig,
  probeEndpoint,
  getRuntime,
  listEvents,
  getEvent,
  deleteEvent,
  batchDeleteEvents,
  previewDelete,
  deleteEventsByFilter,
  listGroups,
REDACTED

export default promptAuditAPI

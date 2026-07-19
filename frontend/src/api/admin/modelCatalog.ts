import apiClient from '../client'

export type CatalogOperatorState = 'enabled' | 'disabled' | 'retired'

export interface CatalogPricing {
  input_cost_per_token: number
  output_cost_per_token: number
  cache_creation_input_token_cost: number
  cache_read_input_token_cost: number
  [key: string]: unknown
}

export interface CatalogAdminModel {
  id: number
  canonical_key: string
  operator_state: CatalogOperatorState
  operator_reason: string
  operator_version: number
  source_state: string
  provider: string
  platform: string
  mode: string
  pricing_schema_version: number
  pricing: CatalogPricing | null
  pricing_valid: boolean
  pricing_source: string
  source_hash: string
}

export interface CatalogAdminStatus {
  initialized: boolean
  scope: string
  epoch: number
  revision_id: number
  revision: number
  checksum: string
  published_at: string
  pricing_read_mode: string
  admission_mode: string
  import_mode: string
  legacy_model_count: number
  models: CatalogAdminModel[]
}

export interface CatalogAdminMutationResult {
  snapshot: CatalogAdminStatus
  runtime_reloaded: boolean
}

export interface UpdateCatalogStateRequest {
  expected_version: number
  state: Extract<CatalogOperatorState, 'enabled' | 'disabled'>
  reason: string
}

export interface BulkCatalogStateModel {
  model_id: number
  expected_version: number
}

export interface BulkUpdateCatalogStateRequest {
  expected_epoch: number
  expected_revision_id: number
  state: Extract<CatalogOperatorState, 'enabled' | 'disabled'>
  models: BulkCatalogStateModel[]
  reason: string
}

export interface UpdateCatalogPricingRequest {
  expected_epoch: number
  expected_revision_id: number
  expected_source_hash: string
  input_cost_per_token: number
  output_cost_per_token: number
  cache_creation_input_token_cost: number
  cache_read_input_token_cost: number
  reason: string
}

function mutationHeaders(idempotencyKey: string) {
  return { headers: { 'Idempotency-Key': idempotencyKey } }
}

export function createModelCatalogIdempotencyKey(operation: string, modelID?: number): string {
  return `model-catalog-${operation}-${modelID ?? 'all'}-${globalThis.crypto.randomUUID()}`
}

export async function getModelCatalog(): Promise<CatalogAdminStatus> {
  const { data } = await apiClient.get<CatalogAdminStatus>('/admin/model-catalog')
  return data
}

export async function syncModelCatalog(idempotencyKey: string): Promise<CatalogAdminStatus> {
  const { data } = await apiClient.post<CatalogAdminStatus>(
    '/admin/model-catalog/sync',
    undefined,
    mutationHeaders(idempotencyKey)
  )
  return data
}

export async function updateModelCatalogState(
  modelID: number,
  request: UpdateCatalogStateRequest,
  idempotencyKey: string
): Promise<CatalogAdminMutationResult> {
  const { data } = await apiClient.patch<CatalogAdminMutationResult>(
    `/admin/model-catalog/models/${modelID}/state`,
    request,
    mutationHeaders(idempotencyKey)
  )
  return data
}

export async function bulkUpdateModelCatalogState(
  request: BulkUpdateCatalogStateRequest,
  idempotencyKey: string
): Promise<CatalogAdminMutationResult> {
  const { data } = await apiClient.patch<CatalogAdminMutationResult>(
    '/admin/model-catalog/models/bulk-state',
    request,
    mutationHeaders(idempotencyKey)
  )
  return data
}

export async function updateModelCatalogPricing(
  modelID: number,
  request: UpdateCatalogPricingRequest,
  idempotencyKey: string
): Promise<CatalogAdminMutationResult> {
  const { data } = await apiClient.patch<CatalogAdminMutationResult>(
    `/admin/model-catalog/models/${modelID}/pricing`,
    request,
    mutationHeaders(idempotencyKey)
  )
  return data
}

export default {
  getModelCatalog,
  syncModelCatalog,
  updateModelCatalogState,
  bulkUpdateModelCatalogState,
  updateModelCatalogPricing,
  createModelCatalogIdempotencyKey
}

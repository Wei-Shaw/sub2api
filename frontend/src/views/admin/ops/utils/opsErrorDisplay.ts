export interface OpsErrorDisplayRecord {
  phase?: string | null
  status_code?: number | null
  message?: string | null
  inbound_endpoint?: string | null
  request_path?: string | null
  upstream_endpoint?: string | null
  platform?: string | null
  scheduled_account_id?: number | null
  scheduled_account_name?: string | null
  account_id?: number | null
  account_name?: string | null
  upstream_status_code?: number | null
  upstream_error_message?: string | null
  upstream_error_detail?: string | null
  upstream_errors?: string | null
}

const LOCAL_METADATA_ENDPOINTS = new Set([
  '/v1/models',
  '/v1/usage',
  '/antigravity/models',
  '/antigravity/v1/models',
  '/antigravity/v1/usage'
])

export function normalizeOpsPath(path: string | null | undefined): string {
  const text = String(path || '').trim()
  if (!text) return ''

  const hashIndex = text.indexOf('#')
  const withoutHash = hashIndex >= 0 ? text.slice(0, hashIndex) : text
  const queryIndex = withoutHash.indexOf('?')
  const withoutQuery = queryIndex >= 0 ? withoutHash.slice(0, queryIndex) : withoutHash
  return withoutQuery.trim()
}

function hasValue(value: string | null | undefined): boolean {
  const text = String(value || '').trim()
  if (!text) return false

  const lowered = text.toLowerCase()
  return text !== '[]' && text !== '{}' && lowered !== 'null'
}

export function hasScheduledAccount(record: OpsErrorDisplayRecord | null | undefined): boolean {
  if (!record) return false
  return String(record.scheduled_account_name || '').trim() !== '' || record.scheduled_account_id != null
}

export function isLocalMetadataEndpoint(record: OpsErrorDisplayRecord | null | undefined): boolean {
  if (!record) return false

  const inboundEndpoint = normalizeOpsPath(record.inbound_endpoint)
  if (LOCAL_METADATA_ENDPOINTS.has(inboundEndpoint)) {
    return true
  }

  const requestPath = normalizeOpsPath(record.request_path)
  return LOCAL_METADATA_ENDPOINTS.has(requestPath)
}

export function hasUpstreamCallContext(record: OpsErrorDisplayRecord | null | undefined): boolean {
  if (!record) return false
  if (record.upstream_status_code != null) return true

  return [
    record.upstream_error_message,
    record.upstream_error_detail,
    record.upstream_errors
  ].some(hasValue)
}

function looksLikePreSchedulingAuthFailure(record: OpsErrorDisplayRecord | null | undefined): boolean {
  if (!record) return false

  const statusCode = record.status_code ?? 0
  const message = String(record.message || '').trim().toLowerCase()
  if ((statusCode !== 401 && statusCode !== 403) || !message) {
    return false
  }

  return [
    'api key',
    'authorization header',
    'bearer scheme',
    'invalid api key',
    'api_key_required',
    'invalid_api_key'
  ].some(token => message.includes(token))
}

export function isAuthFailureBeforeScheduling(record: OpsErrorDisplayRecord | null | undefined): boolean {
  const phase = String(record?.phase || '').trim().toLowerCase()
  if (!isLocalMetadataEndpoint(record) || hasUpstreamCallContext(record)) {
    return false
  }

  return phase === 'auth' || looksLikePreSchedulingAuthFailure(record)
}

export interface UsageDiagnosisDetail {
  source: 'usage' | 'error' | string
  id: number
  request_id: string
  client_ip?: string
  path?: string
  created_at: string
  status_code?: number
  method?: string
  stream?: boolean
  duration_ms?: number | null
  first_token_ms?: number | null
  requested_model?: string
  upstream_model?: string
  api_key_name?: string
  group_name?: string
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  total_cost?: number
  actual_cost?: number
  upstream_url?: string
  upstream_status?: number
  has_detail?: boolean
  req_headers?: Record<string, string>
  res_headers?: Record<string, string>
  req_body?: string
  res_body?: string
  upstream_req_body?: string
  dialog?: unknown
  error_chain?: unknown
}

export type DiagnosisPrimaryTab = 'overview' | 'request' | 'upstream'
export type UpstreamSubTab = 'overview' | 'request' | 'response' | 'error_chain' | 'more'
export type MoreSide = 'request' | 'response'
export type MoreResponsePart = 'thinking' | 'reply'

export interface DialogTurn {
  role: 'system' | 'user' | 'assistant' | 'thinking' | string
  text: string
  images: MediaItem[]
  files: MediaItem[]
}

export interface MediaItem {
  id: string
  name: string
  mime?: string
  url?: string
  dataUrl?: string
  kind: 'image' | 'file'
}

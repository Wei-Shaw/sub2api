export const CODEX_DEVICE_SLOT_MIN = 1 as const
export const CODEX_DEVICE_SLOT_MAX = 3 as const
export const CODEX_SESSION_SLOT_MIN = 1 as const
export const CODEX_SESSION_SLOT_MAX = 3 as const
export const CODEX_AFFINITY_TTL_MIN_SECONDS = 60 as const
export const CODEX_AFFINITY_TTL_MAX_SECONDS = 86400 as const

export const CODEX_OS_PROFILE_IDS = ['windows', 'macos', 'linux', 'generic'] as const
export type CodexOSProfileID = (typeof CODEX_OS_PROFILE_IDS)[number]

export type CodexIdentityPolicyMode = 'off' | 'os_profile_device_pool'
export type CodexIdentityBindingScope = 'api_key_os_surface' | 'api_key_os'
export type CodexClientSurface = 'desktop' | 'cli' | 'sdk' | 'third_party'
export type CodexArchitecture = 'x86_64' | 'arm64' | ''
export type CodexProxyMode = 'inherit' | 'proxy' | 'direct'
export type CodexSessionPolicyMode =
  | 'conversation_isolated'
  | 'api_key_shared'
  | 'session_pool'
  | 'device_shared'

export interface CodexDeviceSlotPolicy {
  id?: number
  index: number
  proxy_mode: CodexProxyMode
  /** Present only when proxy_mode is proxy. */
  proxy_id?: number
}

export interface CodexOSProfilePolicy {
  id?: number
  os_class: CodexOSProfileID
  /** Profiles are uniquely identified by the os_class + canonical_surface pair. */
  canonical_surface: CodexClientSurface
  /** Generic profiles must serialize this as an empty string. */
  architecture: CodexArchitecture
  slot_count: number
  proxy_mode: CodexProxyMode
  /** Present only when proxy_mode is proxy. */
  proxy_id?: number
  slots?: CodexDeviceSlotPolicy[]
}

export interface CodexIdentityTemplate {
  id: number
  name: string
  description: string
  revision: number
  session_policy: CodexSessionPolicy
  affinity_ttl_seconds: number
  unsupported_policy: 'reject'
  profiles: Array<CodexOSProfilePolicy & { catalog_version: 1 }>
  assigned_account_count: number
  created_at: string
  updated_at: string
}

export interface CodexIdentityTemplateWriteRequest {
  name: string
  description: string
  session_policy: CodexSessionPolicy
  affinity_ttl_seconds: number
  unsupported_policy: 'reject'
  profiles: Array<CodexOSProfilePolicy & { catalog_version: 1 }>
}

export interface CodexIdentityTemplateUpdateRequest extends CodexIdentityTemplateWriteRequest {
  expected_revision: number
  confirm_assigned_accounts: boolean
}

export interface CodexIdentityAssignment {
  enabled: boolean
  template_id?: number
  expected_revision?: number
  expected_template_name?: string
  expected_runtime_sha256?: string
}

export type CodexSessionPolicy =
  | { mode: 'conversation_isolated' }
  | { mode: 'api_key_shared' }
  | { mode: 'session_pool'; sessions_per_device: number }
  | {
      mode: 'device_shared'
      max_active_conversations_per_slot: 1
      disable_cross_key_continuation: true
    }

/** Exact JSON shape consumed by backend AccountProvisioningSpec. */
export interface CodexIdentityPolicy {
  mode: CodexIdentityPolicyMode
  binding_scope: CodexIdentityBindingScope
  session_policy: CodexSessionPolicy
  affinity_ttl_seconds: number
  unsupported_policy: 'reject'
  profiles?: CodexOSProfilePolicy[]
}

export interface AccountProvisioningSpec {
  codex_identity_policy: CodexIdentityPolicy
}

export type AccountProvisioningStatus = 'pending' | 'active'

export interface CodexIdentityProxyOption {
  id: number
  name: string
  protocol?: string
  host?: string
  port?: number
  status?: 'active' | 'inactive' | 'expired'
}

export interface CodexIdentityValidationIssue {
  code: string
  path: string
  severity: 'error' | 'warning'
  message: string
}

export interface CodexIdentityValidationResult {
  valid: boolean
  errors: CodexIdentityValidationIssue[]
  warnings: CodexIdentityValidationIssue[]
}

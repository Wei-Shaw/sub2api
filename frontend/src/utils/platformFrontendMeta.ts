/**
 * Typed accessors for PlatformDeclaration.frontend_meta, AccountTypeDeclaration.frontend_meta,
 * and GroupConfigDeclaration.frontend_meta.
 *
 * These extract strongly-typed capability flags and configuration from the opaque
 * `frontend_meta?: Record<string, unknown>` field that gateway plugins populate.
 *
 * All accessors return a typed object with sensible defaults so callers never need
 * null-checks on individual flags.
 */

import type { CcSwitchPlatformEntry } from '@/utils/ccSwitchImport'
import type { PlatformCliConfig } from '@/components/keys/platformCliConfigs'

// ---------------------------------------------------------------------------
// PlatformFrontendMeta (from PlatformDeclaration.frontend_meta)
// ---------------------------------------------------------------------------

export interface PresetMapping {
  label: string
  from: string
  to: string
  color: string
}

export interface PlatformFrontendMeta {
  /** Preset model mappings shown in bulk edit and create account forms */
  preset_mappings?: PresetMapping[]
  /** CC Switch deeplink configuration */
  cc_switch_config?: CcSwitchPlatformEntry
  /** CLI config for UseKeyModal */
  cli_config?: Partial<PlatformCliConfig>
  /** Whether this platform supports mixed-channel pre-check (bulk edit group assignment) */
  supports_mixed_channel_check?: boolean
}

export function getPlatformMeta(
  decl?: { frontend_meta?: Record<string, unknown> } | null,
): PlatformFrontendMeta {
  return (decl?.frontend_meta ?? {}) as PlatformFrontendMeta
}

// ---------------------------------------------------------------------------
// AccountTypeFrontendMeta (from AccountTypeDeclaration.frontend_meta)
// ---------------------------------------------------------------------------

export interface AccountTypeFrontendMeta {
  /** Whether this account type supports OAuth passthrough (e.g. OpenAI OAuth/APIKey) */
  supports_passthrough?: boolean
  /** Whether this account type supports WebSocket mode (e.g. OpenAI OAuth) */
  supports_ws_mode?: boolean
  /** Whether this account type supports compact mode (e.g. OpenAI OAuth/APIKey) */
  supports_compact_mode?: boolean
  /** Whether this account type supports Codex CLI only mode (e.g. OpenAI OAuth) */
  supports_codex_cli_only?: boolean
  /** Whether this account type supports allow overages (e.g. Antigravity) */
  supports_allow_overages?: boolean
  /** Whether this account type supports RPM limit config (e.g. Anthropic OAuth/SetupToken) */
  supports_rpm_limit?: boolean
  /** Whether this account type supports advanced quota control (quota+notify, e.g. Anthropic apikey/bedrock) */
  supports_advanced_quota_control?: boolean
  /** Default usage source for initial load: 'passive' for sampling-based, undefined for active */
  default_usage_source?: 'passive' | 'active'
  /** Extra fields from account.extra that should be included in usage refresh key */
  usage_refresh_extra_fields?: string[]
}

export function getAccountTypeMeta(
  decl?: { frontend_meta?: Record<string, unknown> } | null,
): AccountTypeFrontendMeta {
  return (decl?.frontend_meta ?? {}) as AccountTypeFrontendMeta
}

// ---------------------------------------------------------------------------
// GroupConfigFrontendMeta (from GroupConfigDeclaration.frontend_meta)
// ---------------------------------------------------------------------------

export interface GroupConfigFrontendMeta {
  /** Whether this platform's groups support messages dispatch (e.g. OpenAI) */
  supports_messages_dispatch?: boolean
  /** Whether this platform's groups support fallback on invalid request (e.g. Anthropic/Antigravity) */
  supports_fallback_on_invalid_request?: boolean
}

export function getGroupConfigMeta(
  decl?: { frontend_meta?: Record<string, unknown> } | null,
): GroupConfigFrontendMeta {
  return (decl?.frontend_meta ?? {}) as GroupConfigFrontendMeta
}

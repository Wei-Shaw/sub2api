/**
 * Account form types — re-exported from @sub2api/plugin-sdk.
 *
 * This file preserves backward compatibility for existing host imports.
 * Canonical definitions now live in plugin-sdk/src/account-form-types.ts.
 */
export {
  type CommonAccountFields,
  type ModelMapping,
  type PlatformFormContext,
  type PlatformFormPayload,
  type EditFormPayload,
  type PlatformFormValidation,
  type OAuthFlowConfig,
  type OAuthComposableState,
  type PlatformFormExposed,
  type SdkAccount,
  type SdkCreateAccountRequest,
} from '@sub2api/plugin-sdk'

// Re-export SDK types under their original host-specific names for backward compat.
// Host code uses Account/CreateAccountRequest from @/types which are structurally
// compatible with SdkAccount/SdkCreateAccountRequest.
import type { Account, AccountPlatform, CreateAccountRequest } from '@/types'

/**
 * @deprecated Use OAuthFlowConfig['platform'] (string) from SDK instead.
 * This re-declaration exists only so existing host forms that pass
 * AccountPlatform to oauthConfig.platform keep compiling without changes.
 */
export type { Account, AccountPlatform, CreateAccountRequest }

// ---------------------------------------------------------------------------
// OAuth flow types — previously lived in @/composables/useAccountOAuth.
// Moved here because the composable was platform-specific (Anthropic OAuth)
// and has been migrated to the gateway-anthropic plugin.
// ---------------------------------------------------------------------------

/** OAuth add method — 'oauth' (browser-based) or 'setup-token' (long-lived token) */
export type AddMethod = 'oauth' | 'setup-token'

/** Input method for the OAuthAuthorizationFlow component */
export type AuthInputMethod =
  | 'manual'
  | 'cookie'
  | 'refresh_token'
  | 'mobile_refresh_token'
  | 'session_token'
  | 'access_token'
  | 'codex_session'

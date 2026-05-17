/**
 * Account form types shared between host and gateway plugins.
 *
 * These types define the contract for platform-specific account forms
 * (create / edit mode). Plugin forms must implement PlatformFormExposed
 * so the host can validate, extract payloads, and drive OAuth flows.
 *
 * Note: This file lives in plugin-sdk to avoid plugins depending on
 * host-internal `@/types`. Types that referenced host-only interfaces
 * (Account, CreateAccountRequest) use minimal inline shapes instead.
 */

// ---------------------------------------------------------------------------
// Minimal host-type shapes (avoid import from @/types)
// ---------------------------------------------------------------------------

/**
 * Minimal Account shape for form edit-mode population.
 * The host passes the full Account object which is structurally compatible.
 */
export interface SdkAccount {
  id: number
  name: string
  platform: string
  type: string
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id: number | null
}

/**
 * Minimal CreateAccountRequest shape for OAuth flow results.
 * The host expects the full CreateAccountRequest; this shape is compatible.
 */
export interface SdkCreateAccountRequest {
  name: string
  platform: string
  type: string
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id?: number | null
  concurrency?: number
  priority?: number
  group_ids?: number[]
}

// ---------------------------------------------------------------------------
// Form context & payloads
// ---------------------------------------------------------------------------

export interface ModelMapping {
  from: string
  to: string
}

export interface PlatformFormContext {
  /** 'oauth-based' | 'apikey' | 'bedrock' | 'service_account' */
  accountCategory: string
  accountTypeId: string
  proxyId: number | null
  /** When 'edit', forms pre-populate from account data and hide OAuth flows */
  mode?: 'create' | 'edit'
}

export interface PlatformFormPayload {
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  typeOverride?: string
  needsOAuthFlow?: boolean
}

/**
 * Edit-mode payload returned by getEditPayload().
 * Contains fully-built credentials and extra ready to send to PUT /admin/accounts/:id.
 */
export interface EditFormPayload {
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
}

export interface PlatformFormValidation {
  valid: boolean
  error?: string
}

export interface OAuthFlowConfig {
  showCookieOption?: boolean
  showRefreshTokenOption?: boolean
  showMobileRefreshTokenOption?: boolean
  showSessionTokenOption?: boolean
  showAccessTokenOption?: boolean
  showProjectId?: boolean
  showHelp?: boolean
  showProxyWarning?: boolean
  allowMultiple?: boolean
  needsMixedChannelCheck?: boolean
  showCodexSessionImportOption?: boolean
  /** Show a warning notice in Step 2 (e.g. OpenAI's "Important Notice") */
  showImportantNotice?: boolean
  /** Show a state-parameter warning under the auth-code input (e.g. Gemini) */
  showStateWarning?: boolean
  /**
   * i18n key prefix for platform-specific OAuth translations.
   * Falls back to 'admin.accounts.oauth' when unset.
   * Example: 'admin.accounts.oauth.openai' resolves 'title' as
   * 'admin.accounts.oauth.openai.title'.
   */
  i18nPrefix?: string
  platform: string
}

export interface OAuthComposableState {
  authUrl: string
  sessionId: string
  loading: boolean
  error: string
}

export interface PlatformFormExposed {
  validate(): PlatformFormValidation
  getPayload(): PlatformFormPayload
  isOAuthFlow(): boolean
  reset(): void
  /** Populate form fields from an existing account (edit mode) */
  initFromAccount?(account: SdkAccount): void
  /** Build the update payload for PUT /admin/accounts/:id (edit mode) */
  getEditPayload?(account: SdkAccount): EditFormPayload
  oauthConfig?: OAuthFlowConfig
  /** Reactive OAuth state for OAuthAuthorizationFlow binding */
  getOAuthState?(): OAuthComposableState
  /** Trigger URL generation */
  generateOAuthUrl?(proxyId: number | null, projectId?: string): Promise<void>
  /** Reset OAuth composable state */
  resetOAuth?(): void
  handleOAuthExchange?(code: string, oauthState?: string, projectId?: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null>
  handleCookieAuth?(sessionKey: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null>
  handleRefreshToken?(rt: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null>
  handleMobileRefreshToken?(rt: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null>
  handleSessionToken?(token: string): Promise<SdkCreateAccountRequest | SdkCreateAccountRequest[] | null>
}

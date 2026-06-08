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

/** Proxy data passed by the host for ProxySelector rendering. */
export interface SdkProxy {
  id: number
  name: string
  protocol?: string
  host?: string
  port?: number
}

/** Group data passed by the host for GroupSelector rendering. */
export interface SdkGroup {
  id: number
  name: string
  platform?: string
  rate_multiplier?: number
  account_count?: number
}

export interface PlatformFormContext {
  /** 'oauth-based' | 'apikey' | 'bedrock' | 'service_account' */
  accountCategory: string
  accountTypeId: string
  proxyId: number | null
  /** When 'edit', forms pre-populate from account data and hide OAuth flows */
  mode?: 'create' | 'edit'
  /** Host-provided data for common field rendering */
  hostData?: {
    proxies: SdkProxy[]
    groups: SdkGroup[]
    isSimpleMode: boolean
    quotaNotifyGlobalEnabled: boolean
    platform?: string
    compatiblePlatforms?: string[]
  }
}

/** Common fields managed by AccountCommonFields SDK component. */
export interface CommonAccountFields {
  name: string
  notes: string
  proxy_id: number | null
  concurrency: number
  load_factor: number | null
  priority: number
  rate_multiplier: number
  expires_at: number | null
  auto_pause_on_expired: boolean
  group_ids: number[]
  quota_enabled: boolean
  quota_limit: number | null
  quota_daily_limit: number | null
  quota_weekly_limit: number | null
}

export interface PlatformFormPayload {
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  typeOverride?: string
  needsOAuthFlow?: boolean
  common?: CommonAccountFields
}

/**
 * Edit-mode payload returned by getEditPayload().
 * Contains fully-built credentials and extra ready to send to PUT /admin/accounts/:id.
 */
export interface EditFormPayload {
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  common?: Partial<CommonAccountFields>
  /** Validation error from platform form — if set, credentials is undefined and save should be aborted. */
  error?: string
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


// ---------------------------------------------------------------------------
// Test panel types (test component contract)
// ---------------------------------------------------------------------------

/**
 * Exposed interface from a plugin test panel component.
 *
 * The host's AccountTestModal uses template-ref to call startTest / abort
 * and read isRunning. This is symmetric with PlatformFormExposed for forms.
 */
export interface AccountTestExposed {
  startTest(): void
  abort(): void
  readonly isRunning: boolean
}

/**
 * Context passed by the host to the plugin test panel component as a prop.
 *
 * Contains the account being tested and host-side data the plugin may need
 * (e.g. pre-fetched available models so the plugin doesn't re-fetch).
 */
export interface SdkTestContext {
  account: SdkAccount
  hostData: {
    /** Available models fetched by host, or plugin can fetch its own. */
    availableModels?: Array<{ id: string; display_name: string }>
  }
}

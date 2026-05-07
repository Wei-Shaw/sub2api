import type { Account, AccountPlatform, AccountType, CreateAccountRequest } from '@/types'

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
  typeOverride?: AccountType
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
  platform: AccountPlatform
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
  initFromAccount?(account: Account): void
  /** Build the update payload for PUT /admin/accounts/:id (edit mode) */
  getEditPayload?(account: Account): EditFormPayload
  oauthConfig?: OAuthFlowConfig
  /** Reactive OAuth state for OAuthAuthorizationFlow binding */
  getOAuthState?(): OAuthComposableState
  /** Trigger URL generation */
  generateOAuthUrl?(proxyId: number | null, projectId?: string): Promise<void>
  /** Reset OAuth composable state */
  resetOAuth?(): void
  handleOAuthExchange?(code: string, oauthState?: string, projectId?: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null>
  handleCookieAuth?(sessionKey: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null>
  handleRefreshToken?(rt: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null>
  handleMobileRefreshToken?(rt: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null>
  handleSessionToken?(token: string): Promise<CreateAccountRequest | CreateAccountRequest[] | null>
}
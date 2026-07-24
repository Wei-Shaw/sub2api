/**
 * Shared OAuth composable factory.
 *
 * The four provider composables (useGrokOAuth, useAntigravityOAuth, useGeminiOAuth,
 * useOpenAIOAuth) share identical ref state management and try/catch/finally
 * scaffolding. Only the provider-specific bits vary (API endpoints, i18n keys,
 * payload shape, credential mapping, error extraction). This factory captures the
 * shared logic verbatim and parameterizes the rest via {@link OAuthComposableConfig}.
 *
 * The extraction is purely behavior-preserving: per-provider logic moves into config
 * strategies, the shared scaffolding (loading/error/try/catch/finally, showError,
 * return values) lives here unchanged.
 */

import { ref, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'

/** Minimal i18n translate-function shape (mirrors utils/apiError.TranslateFn). */
export type OAuthTranslateFn = (key: string, params?: Record<string, unknown>) => string

/** Response shape returned by every OAuth `generate-auth-url` endpoint. */
export interface OAuthAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

/** Optional hook deriving the oauth `state` from the auth-url response. */
export type GenerateAuthUrlOnSuccess = (
  response: OAuthAuthUrlResponse,
  setState: (value: string) => void
) => void

export interface GenerateAuthUrlStrategy {
  buildPayload: (...args: any[]) => Record<string, unknown>
  call: (payload: Record<string, unknown>) => Promise<OAuthAuthUrlResponse>
  extractError: (err: any, t: OAuthTranslateFn) => string
  /**
   * Optional post-success hook. When omitted, the response `state` is assigned
   * directly (Grok/Antigravity/Gemini). OpenAI parses the `state` query param
   * out of the auth URL instead.
   */
  onSuccess?: GenerateAuthUrlOnSuccess
}

export interface ExchangeAuthCodeStrategy {
  /** Returns a validation error message to surface (and short-circuit on), or null to proceed. */
  validate: (t: OAuthTranslateFn, ...args: any[]) => string | null
  buildPayload: (...args: any[]) => Record<string, unknown>
  call: (payload: Record<string, unknown>) => Promise<unknown>
  extractError: (err: any, t: OAuthTranslateFn) => string
}

export interface ValidateRefreshTokenStrategy {
  /** Returns a validation error message to surface (and short-circuit on), or null to proceed. */
  validate: (t: OAuthTranslateFn, ...args: any[]) => string | null
  call: (...args: any[]) => Promise<unknown>
  extractError: (err: any, t: OAuthTranslateFn) => string
  /** Whether to call `appStore.showError` on failure. Grok/Antigravity: false; OpenAI: true. */
  showErrorOnFailure?: boolean
}

export interface OAuthComposableConfig<TToken> {
  generateAuthUrl: GenerateAuthUrlStrategy
  exchangeAuthCode: ExchangeAuthCodeStrategy
  validateRefreshToken?: ValidateRefreshTokenStrategy
  buildCredentials: (tokenInfo: TToken, ...args: any[]) => Record<string, unknown>
  buildExtraInfo?: (tokenInfo: TToken, ...args: any[]) => Record<string, unknown> | undefined
}

/**
 * Derives the token-info type carried by a config (the first parameter of its
 * `buildCredentials`). Lets callers pass a plain config literal while still
 * recovering a concrete `TToken` (e.g. `GrokTokenInfo`) for the return type.
 */
type OAuthConfigToken<C> = C extends OAuthComposableConfig<infer TToken> ? TToken : never

/**
 * Return type of {@link createOAuthComposable}. `validateRefreshToken` and
 * `buildExtraInfo` are present only when the corresponding config key is provided,
 * mirroring the per-provider public shapes exactly (e.g. Gemini has no
 * `validateRefreshToken`, Antigravity has no `buildExtraInfo`).
 */
export type OAuthComposableReturn<TToken, C extends OAuthComposableConfig<TToken>> = {
  authUrl: Ref<string>
  sessionId: Ref<string>
  state: Ref<string>
  loading: Ref<boolean>
  error: Ref<string>
  resetState: () => void
  generateAuthUrl: (...args: any[]) => Promise<boolean>
  exchangeAuthCode: (...args: any[]) => Promise<TToken | null>
  buildCredentials: C['buildCredentials']
} & ('validateRefreshToken' extends keyof C
  ? { validateRefreshToken: (...args: any[]) => Promise<TToken | null> }
  : {}) &
  ('buildExtraInfo' extends keyof C ? { buildExtraInfo: C['buildExtraInfo'] } : {})

export function createOAuthComposable<C extends OAuthComposableConfig<any>>(
  config: C
): OAuthComposableReturn<OAuthConfigToken<C>, C> {
  const appStore = useAppStore()
  const { t } = useI18n()

  const authUrl = ref('')
  const sessionId = ref('')
  const state = ref('')
  const loading = ref(false)
  const error = ref('')

  const resetState = () => {
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    loading.value = false
    error.value = ''
  }

  const generateAuthUrl = async (...args: any[]): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    state.value = ''
    error.value = ''

    try {
      const payload = config.generateAuthUrl.buildPayload(...args)
      const response = await config.generateAuthUrl.call(payload)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      if (config.generateAuthUrl.onSuccess) {
        config.generateAuthUrl.onSuccess(response, (value: string) => {
          state.value = value
        })
      } else {
        state.value = response.state
      }
      return true
    } catch (err: any) {
      error.value = config.generateAuthUrl.extractError(err, t)
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  const exchangeAuthCode = async (...args: any[]): Promise<OAuthConfigToken<C> | null> => {
    const validationError = config.exchangeAuthCode.validate(t, ...args)
    if (validationError) {
      error.value = validationError
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const payload = config.exchangeAuthCode.buildPayload(...args)
      const tokenInfo = await config.exchangeAuthCode.call(payload)
      return tokenInfo as OAuthConfigToken<C>
    } catch (err: any) {
      error.value = config.exchangeAuthCode.extractError(err, t)
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  const validateRefreshToken = config.validateRefreshToken
    ? async (...args: any[]): Promise<OAuthConfigToken<C> | null> => {
        const strategy = config.validateRefreshToken!
        const validationError = strategy.validate(t, ...args)
        if (validationError) {
          error.value = validationError
          return null
        }

        loading.value = true
        error.value = ''

        try {
          return (await strategy.call(...args)) as OAuthConfigToken<C>
        } catch (err: any) {
          error.value = strategy.extractError(err, t)
          if (strategy.showErrorOnFailure) {
            appStore.showError(error.value)
          }
          return null
        } finally {
          loading.value = false
        }
      }
    : undefined

  return {
    authUrl,
    sessionId,
    state,
    loading,
    error,
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    buildCredentials: config.buildCredentials,
    ...(config.validateRefreshToken ? { validateRefreshToken } : {}),
    ...(config.buildExtraInfo ? { buildExtraInfo: config.buildExtraInfo } : {})
  } as OAuthComposableReturn<OAuthConfigToken<C>, C>
}

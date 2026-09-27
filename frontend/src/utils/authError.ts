import { extractApiErrorCode, extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'

type TranslateFn = (key: string, params?: Record<string, unknown>) => string

/**
 * Message for a failed login/registration request. With `t`, the backend
 * reason is looked up under `auth.errors.<REASON>` first; either way it falls
 * back to the backend message and then to `fallback`.
 */
export function buildAuthErrorMessage(
  error: unknown,
  options: { fallback: string; t?: TranslateFn }
): string {
  const { fallback, t } = options
  if (!t) return extractApiErrorMessage(error, fallback)
  if (extractApiErrorCode(error) === 'EMAIL_DOMAIN_REGISTRATION_LIMIT') {
    return t('auth.emailDomainRegistrationLimit')
  }
  return extractI18nErrorMessage(error, t, 'auth.errors', fallback)
}

function translateIfExists(t: TranslateFn, key: string): string {
  const translated = t(key)
  return translated && translated !== key ? translated : ''
}

/**
 * Localized message for the `#error=` fragment the backend appends when an
 * OAuth callback fails. `error_message` often carries a backend reason (for
 * session_error), so it is tried under `auth.errors` before the generic
 * `auth.oauth.error.<code>`. For provider_error it carries the provider's own
 * code (e.g. access_denied), whose text beats the provider's description,
 * which in turn beats the generic "provider returned an error".
 */
export function buildOAuthCallbackErrorMessage(t: TranslateFn, params: URLSearchParams): string {
  const error = params.get('error') || ''
  const reason = params.get('error_message') || ''
  const description = params.get('error_description') || ''
  const fromProvider = error === 'provider_error'
  return (
    (reason && translateIfExists(t, `auth.errors.${reason}`)) ||
    (fromProvider && reason && translateIfExists(t, `auth.oauth.error.${reason}`)) ||
    (fromProvider && description) ||
    translateIfExists(t, `auth.oauth.error.${error}`) ||
    description ||
    reason ||
    error
  )
}

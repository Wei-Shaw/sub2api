import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'

type TranslateFn = (key: string, params?: Record<string, unknown>) => string

/**
 * Localized message for a failed login/registration request: the backend
 * reason is looked up under `auth.errors.<REASON>`, falling back to the
 * backend message and then to `fallback`.
 */
export function buildAuthErrorMessage(error: unknown, t: TranslateFn, fallback: string): string {
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
 * `auth.oauth.error.<code>`; unknown codes keep the raw provider text.
 */
export function buildOAuthCallbackErrorMessage(t: TranslateFn, params: URLSearchParams): string {
  const error = params.get('error') || ''
  const reason = params.get('error_message') || ''
  const description = params.get('error_description') || ''
  return (
    (reason && translateIfExists(t, `auth.errors.${reason}`)) ||
    translateIfExists(t, `auth.oauth.error.${error}`) ||
    description ||
    reason ||
    error
  )
}

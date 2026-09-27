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

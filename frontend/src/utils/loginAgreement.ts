import type { LoginAgreementDocument } from '@/types'

const SUPPORTED_LOGIN_AGREEMENT_LOCALES = ['en', 'zh', 'vi', 'ko'] as const

export type LoginAgreementLocale = (typeof SUPPORTED_LOGIN_AGREEMENT_LOCALES)[number]

export function normalizeLoginAgreementLocale(locale: string): LoginAgreementLocale {
  const normalized = locale.toLowerCase().split('-')[0]
  return SUPPORTED_LOGIN_AGREEMENT_LOCALES.includes(normalized as LoginAgreementLocale)
    ? (normalized as LoginAgreementLocale)
    : 'en'
}

function localizedValue(values: Record<string, string> | undefined, locale: LoginAgreementLocale): string {
  if (!values) {
    return ''
  }

  return values[locale]?.trim() || values.en?.trim() || values.zh?.trim() || ''
}

export function resolveLoginAgreementDocument(
  doc: LoginAgreementDocument,
  locale: string,
): LoginAgreementDocument {
  const normalizedLocale = normalizeLoginAgreementLocale(locale)
  const title = localizedValue(doc.title_i18n, normalizedLocale) || doc.title.trim()
  const content = localizedValue(doc.content_md_i18n, normalizedLocale) || doc.content_md.trim()

  return {
    ...doc,
    title,
    content_md: content,
  }
}

export function resolveLoginAgreementDocuments(
  docs: LoginAgreementDocument[],
  locale: string,
): LoginAgreementDocument[] {
  return docs
    .map((doc) => resolveLoginAgreementDocument(doc, locale))
    .filter((doc) => doc.title.trim())
}

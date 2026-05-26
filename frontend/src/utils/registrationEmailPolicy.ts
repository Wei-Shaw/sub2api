const EMAIL_SUFFIX_TOKEN_SPLIT_RE = /[\s,，]+/
const EMAIL_SUFFIX_INVALID_CHAR_RE = /[^a-z0-9.-]/g
const EMAIL_SUFFIX_INVALID_CHAR_CHECK_RE = /[^a-z0-9.-]/
const EMAIL_SUFFIX_PREFIX_RE = /^@+/
const EMAIL_SUFFIX_DOMAIN_PATTERN =
  /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/

// normalizeRegistrationEmailSuffixDomain converts raw input into a canonical domain token.
// It removes leading "@", lowercases input, and strips all invalid characters.
export function normalizeRegistrationEmailSuffixDomain(raw: string): string {
  let value = String(raw || '').trim().toLowerCase()
  if (!value) {
    return ''
  }
  value = value.replace(EMAIL_SUFFIX_PREFIX_RE, '')
  value = value.replace(EMAIL_SUFFIX_INVALID_CHAR_RE, '')
  return value
}

export function normalizeRegistrationEmailSuffixDomains(
  items: string[] | null | undefined
): string[] {
  if (!items || items.length === 0) {
    return []
  }

  const seen = new Set<string>()
  const normalized: string[] = []
  for (const item of items) {
    const domain = normalizeRegistrationEmailSuffixDomain(item)
    if (!isRegistrationEmailSuffixDomainValid(domain) || seen.has(domain)) {
      continue
    }
    seen.add(domain)
    normalized.push(domain)
  }
  return normalized
}

export function parseRegistrationEmailSuffixWhitelistInput(input: string): string[] {
  if (!input || !input.trim()) {
    return []
  }

  const seen = new Set<string>()
  const normalized: string[] = []

  for (const token of input.split(EMAIL_SUFFIX_TOKEN_SPLIT_RE)) {
    const domain = normalizeRegistrationEmailSuffixDomainStrict(token)
    if (!isRegistrationEmailSuffixDomainValid(domain) || seen.has(domain)) {
      continue
    }
    seen.add(domain)
    normalized.push(domain)
  }

  return normalized
}

export function normalizeRegistrationEmailSuffixWhitelist(
  items: string[] | null | undefined
): string[] {
  return normalizeRegistrationEmailSuffixDomains(items).map((domain) => `@${domain}`)
}

function extractRegistrationEmailDomain(email: string): string {
  const raw = String(email || '').trim().toLowerCase()
  if (!raw) {
    return ''
  }
  const atIndex = raw.indexOf('@')
  if (atIndex <= 0 || atIndex >= raw.length - 1) {
    return ''
  }
  if (raw.indexOf('@', atIndex + 1) !== -1) {
    return ''
  }
  return raw.slice(atIndex + 1)
}

export function isRegistrationEmailSuffixAllowed(
  email: string,
  whitelist: string[] | null | undefined
): boolean {
  const normalizedWhitelist = normalizeRegistrationEmailSuffixWhitelist(whitelist)
  if (normalizedWhitelist.length === 0) {
    return true
  }
  const emailDomain = extractRegistrationEmailDomain(email)
  if (!emailDomain) {
    return false
  }
  const emailSuffix = `@${emailDomain}`
  return normalizedWhitelist.includes(emailSuffix)
}

// Pasted domains should be strict: any invalid character drops the whole token.
function normalizeRegistrationEmailSuffixDomainStrict(raw: string): string {
  let value = String(raw || '').trim().toLowerCase()
  if (!value) {
    return ''
  }
  value = value.replace(EMAIL_SUFFIX_PREFIX_RE, '')
  if (!value || EMAIL_SUFFIX_INVALID_CHAR_CHECK_RE.test(value)) {
    return ''
  }
  return value
}

export function isRegistrationEmailSuffixDomainValid(domain: string): boolean {
  if (!domain) {
    return false
  }
  return EMAIL_SUFFIX_DOMAIN_PATTERN.test(domain)
}

// Exact registration email blacklist input helpers. Validation follows the
// backend (NormalizeRegistrationEmailBlacklistEntry): trim, drop the domain root
// dot, local part ≤ 64 UTF-8 bytes, whole address ≤ 254 UTF-8 bytes. Only ASCII
// letters are lowercased here; the backend applies full lowercasing on save and
// returns the stored form.

const TOKEN_SPLIT_RE = /[\s,，;；]+/
const DOMAIN_PATTERN =
  /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/
const LOCAL_MAX_BYTES = 64
const EMAIL_MAX_BYTES = 254
const utf8 = new TextEncoder()

function utf8Length(value: string): number {
  return utf8.encode(value).length
}

function hasControlChar(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i)
    if (code <= 0x1f || (code >= 0x7f && code <= 0x9f)) {
      return true
    }
  }
  return false
}

/** Returns the normalized address, or null when the token is not a valid email. */
export function normalizeBlacklistEmail(raw: string): string | null {
  const value = String(raw ?? '').trim().replace(/[A-Z]+/g, (letters) => letters.toLowerCase())
  const at = value.indexOf('@')
  if (at <= 0) {
    return null
  }
  const local = value.slice(0, at)
  const domain = value.slice(at + 1).replace(/\.+$/, '')
  if (!domain || domain.includes('@')) {
    return null
  }
  if (utf8Length(local) > LOCAL_MAX_BYTES || /\s/.test(local) || hasControlChar(local)) {
    return null
  }
  if (!DOMAIN_PATTERN.test(domain)) {
    return null
  }
  const email = `${local}@${domain}`
  return utf8Length(email) > EMAIL_MAX_BYTES ? null : email
}

export interface ParsedBlacklistInput {
  /** Normalized, de-duplicated addresses in input order. */
  valid: string[]
  /** Raw tokens that are not valid addresses. */
  invalid: string[]
  /** Tokens dropped because an earlier token normalized to the same address. */
  duplicates: number
}

/** Splits pasted text on whitespace, commas and semicolons into addresses. */
export function parseBlacklistInput(input: string): ParsedBlacklistInput {
  const valid: string[] = []
  const invalid: string[] = []
  const seen = new Set<string>()
  let duplicates = 0
  for (const token of String(input ?? '').split(TOKEN_SPLIT_RE)) {
    if (!token) {
      continue
    }
    const email = normalizeBlacklistEmail(token)
    if (!email) {
      invalid.push(token)
      continue
    }
    if (seen.has(email)) {
      duplicates++
      continue
    }
    seen.add(email)
    valid.push(email)
  }
  return { valid, invalid, duplicates }
}

export interface MergeBlacklistResult {
  list: string[]
  added: string[]
  duplicates: number
}

/** Prepends addresses not already in the list; existing order is kept. */
export function mergeIntoBlacklist(existing: string[], incoming: string[]): MergeBlacklistResult {
  const present = new Set(existing)
  const added: string[] = []
  for (const email of incoming) {
    if (present.has(email)) {
      continue
    }
    present.add(email)
    added.push(email)
  }
  return {
    list: [...added, ...existing],
    added,
    duplicates: incoming.length - added.length
  }
}

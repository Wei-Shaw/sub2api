export type AccountIdentifierKind = 'email' | 'phone'

const EMAIL_PATTERN =
  /^[A-Za-z0-9._%+-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$/

export function normalizeEmailIdentifier(value: string): string {
  return String(value || '').trim().toLowerCase()
}

export function isEmailIdentifier(value: string): boolean {
  const normalized = normalizeEmailIdentifier(value)
  if (!normalized || normalized.length > 255 || /\s/.test(normalized)) {
    return false
  }
  return EMAIL_PATTERN.test(normalized)
}

export function normalizePhoneIdentifier(value: string): string {
  return String(value || '').trim().replace(/[ \-()]/g, '')
}

export function isPhoneIdentifier(value: string): boolean {
  const raw = String(value || '').trim()
  if (!raw || raw.length > 64 || !/^[0-9+\-() ]+$/.test(raw)) {
    return false
  }
  if ((raw.match(/\+/g) || []).length > 1 || (raw.includes('+') && !raw.startsWith('+'))) {
    return false
  }
  const normalized = normalizePhoneIdentifier(raw)
  return /^\+?[0-9]{6,20}$/.test(normalized)
}

export function isAccountIdentifier(value: string): boolean {
  const raw = String(value || '').trim()
  if (!raw || raw.length > 255 || /[\u0000-\u001F\u007F]/.test(raw)) {
    return false
  }
  if (raw.includes('@')) {
    return isEmailIdentifier(raw)
  }
  return isPhoneIdentifier(raw)
}

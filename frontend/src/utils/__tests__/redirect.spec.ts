import { describe, it, expect } from 'vitest'
import { sanitizeRedirectPath, DEFAULT_REDIRECT_PATH } from '../redirect'

describe('sanitizeRedirectPath', () => {
  it('keeps a normal relative in-app path', () => {
    expect(sanitizeRedirectPath('/dashboard')).toBe('/dashboard')
    expect(sanitizeRedirectPath('/admin/accounts')).toBe('/admin/accounts')
  })

  it('keeps a path with query string and hash intact', () => {
    expect(sanitizeRedirectPath('/admin/usage?page=2&size=50#chart')).toBe(
      '/admin/usage?page=2&size=50#chart'
    )
  })

  it('rejects protocol-relative URLs', () => {
    expect(sanitizeRedirectPath('//evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('//evil.com/path')).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('rejects absolute URLs', () => {
    expect(sanitizeRedirectPath('https://evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('http://evil.com/steal')).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('rejects an absolute URL smuggled after a leading slash', () => {
    expect(sanitizeRedirectPath('/redirect?to=https://evil.com')).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('rejects non-slash-prefixed values including pseudo protocols', () => {
    expect(sanitizeRedirectPath('dashboard')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('javascript:alert(1)')).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('rejects CRLF injection', () => {
    expect(sanitizeRedirectPath('/dashboard\r\nSet-Cookie: a=b')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/dashboard\n')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/dashboard\r')).toBe(DEFAULT_REDIRECT_PATH)
  })

  it('falls back for empty, null and undefined input', () => {
    expect(sanitizeRedirectPath('')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath(null)).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath(undefined)).toBe(DEFAULT_REDIRECT_PATH)
  })

  // Rule 6. For special schemes (http/https) the WHATWG URL parser treats a
  // backslash as a forward slash, so `/\\evil.com` resolves to `https://evil.com`.
  it('rejects the backslash protocol-relative variants', () => {
    expect(sanitizeRedirectPath('/\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/\\evil.com/path')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/\\\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/\\/evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('//\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
  })

  // A bare backslash never had a leading slash, so rule 2 already caught it.
  it('rejects a bare backslash authority', () => {
    expect(sanitizeRedirectPath('\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('\\\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
  })

  // The URL parser deletes TAB/LF/CR before parsing, so a tab can reassemble an
  // authority that the raw string does not show. LF/CR are already rule 5.
  it('rejects tab-smuggled protocol-relative URLs', () => {
    expect(sanitizeRedirectPath('/\t/evil.com')).toBe(DEFAULT_REDIRECT_PATH)
    expect(sanitizeRedirectPath('/\t\\evil.com')).toBe(DEFAULT_REDIRECT_PATH)
  })

  // --- Deliberately NOT blocked: boundary pinned on purpose ---

  // Normalises to `/docs/a/b` - still same-origin, so blocking it would only
  // break legitimate links. Rule 6 checks the authority position only.
  it('allows a backslash in the middle of the path', () => {
    expect(sanitizeRedirectPath('/docs/a\\b')).toBe('/docs/a\\b')
    expect(sanitizeRedirectPath('/files?path=C:\\Users\\me')).toBe('/files?path=C:\\Users\\me')
  })

  // Query/hash content is never a navigation target for router.push.
  it('allows an off-site-looking value inside query or hash', () => {
    expect(sanitizeRedirectPath('/foo?next=//evil.com')).toBe('/foo?next=//evil.com')
    expect(sanitizeRedirectPath('/foo#//evil.com')).toBe('/foo#//evil.com')
  })

  // A percent-encoded backslash stays in the path; the parser never decodes it
  // into an authority separator.
  it('allows a percent-encoded backslash', () => {
    expect(sanitizeRedirectPath('/%5Cevil.com')).toBe('/%5Cevil.com')
  })

  // Known accepted gap: rule 4 matches the raw `://` only, so the backslash
  // spelling survives inside a query string. Harmless here because the value is
  // a query param, not the navigation target - any code that later reads
  // `?to=` must sanitise it at that sink.
  it('allows a backslash-spelled absolute URL inside a query string', () => {
    expect(sanitizeRedirectPath('/redirect?to=https:/\\evil.com')).toBe(
      '/redirect?to=https:/\\evil.com'
    )
  })

  it('allows an ordinary path that merely looks like a host', () => {
    expect(sanitizeRedirectPath('/evil.com')).toBe('/evil.com')
  })

  it('exposes /dashboard as the default fallback', () => {
    expect(DEFAULT_REDIRECT_PATH).toBe('/dashboard')
  })
})

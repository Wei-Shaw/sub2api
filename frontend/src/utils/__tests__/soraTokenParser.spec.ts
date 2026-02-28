import { describe, expect, it REDACTED from 'vitest'
import { parseSoraRawTokens REDACTED from '@/utils/soraTokenParser'

describe('parseSoraRawTokens', () => {
  it('parses sessionToken and accessToken from JSON payload', () => {
    const payload = JSON.stringify({
      user: { id: 'u1' REDACTED,
      accessToken: 'at-json-1',
      sessionToken: 'st-json-1'
    REDACTED)

    const result = parseSoraRawTokens(payload)

    expect(result.sessionTokens).toEqual(['st-json-1'])
    expect(result.accessTokens).toEqual(['at-json-1'])
  REDACTED)

  it('supports plain session tokens (one per line)', () => {
    const result = parseSoraRawTokens('st-1\nst-2')

    expect(result.sessionTokens).toEqual(['st-1', 'st-2'])
    expect(result.accessTokens).toEqual([])
  REDACTED)

  it('supports non-standard object snippets via regex', () => {
    const raw = "sessionToken: 'st-snippet', access_token: \"at-snippet\""
    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['st-snippet'])
    expect(result.accessTokens).toEqual(['at-snippet'])
  REDACTED)

  it('keeps unique tokens and extracts JWT-like plain line as AT too', () => {
    const jwt = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature'
    const raw = `st-dup\nst-dup\n${jwtREDACTED\n${JSON.stringify({ sessionToken: 'st-json', accessToken: jwt REDACTED)REDACTED`
    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['st-json', 'st-dup'])
    expect(result.accessTokens).toEqual([jwt])
  REDACTED)

  it('parses session token from Set-Cookie line and strips cookie attributes', () => {
    const raw =
      '__Secure-next-auth.session-token.0=st-cookie-part-0; Domain=.chatgpt.com; Path=/; Expires=Thu, 28 May 2026 11:43:36 GMT; HttpOnly; Secure; SameSite=Lax'
    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['st-cookie-part-0'])
    expect(result.accessTokens).toEqual([])
  REDACTED)

  it('merges chunked session-token cookies by numeric suffix order', () => {
    const raw = [
      'Set-Cookie: __Secure-next-auth.session-token.1=part-1; Path=/; HttpOnly',
      'Set-Cookie: __Secure-next-auth.session-token.0=part-0; Path=/; HttpOnly'
    ].join('\n')
    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['part-0part-1'])
    expect(result.accessTokens).toEqual([])
  REDACTED)

  it('prefers latest duplicate chunk values when multiple cookie groups exist', () => {
    const raw = [
      'Set-Cookie: __Secure-next-auth.session-token.0=old-0; Path=/; HttpOnly',
      'Set-Cookie: __Secure-next-auth.session-token.1=old-1; Path=/; HttpOnly',
      'Set-Cookie: __Secure-next-auth.session-token.0=new-0; Path=/; HttpOnly',
      'Set-Cookie: __Secure-next-auth.session-token.1=new-1; Path=/; HttpOnly'
    ].join('\n')
    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['new-0new-1'])
    expect(result.accessTokens).toEqual([])
  REDACTED)

  it('uses latest complete chunk group and ignores incomplete latest group', () => {
    const raw = [
      'set-cookie',
      '__Secure-next-auth.session-token.0=ok-0; Domain=.chatgpt.com; Path=/',
      'set-cookie',
      '__Secure-next-auth.session-token.1=ok-1; Domain=.chatgpt.com; Path=/',
      'set-cookie',
      '__Secure-next-auth.session-token.0=partial-0; Domain=.chatgpt.com; Path=/'
    ].join('\n')

    const result = parseSoraRawTokens(raw)

    expect(result.sessionTokens).toEqual(['ok-0ok-1'])
    expect(result.accessTokens).toEqual([])
  REDACTED)
REDACTED)

import { readFileSync REDACTED from 'node:fs'
import { resolve REDACTED from 'node:path'
import { describe, expect, it REDACTED from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/admin/account/ReAuthAccountModal.vue'),
  'utf8'
)

describe('ReAuthAccountModal Grok re-auth paths', () => {
  it('exposes SSO cookie and refresh-token options; password auth stays hidden', () => {
    expect(source).toContain(':show-sso-option="isGrok"')
    expect(source).toContain(':show-email-password-option="false"')
    expect(source).toContain(':show-refresh-token-option="isOpenAI || isAntigravity || isGrok"')
    expect(source).not.toContain('@authorize-password=')
  REDACTED)

  it('wires SSO and RT reauth without batch account create', () => {
    expect(source).toContain('@import-sso="handleGrokImportSSO"')
    expect(source).toContain('@validate-refresh-token="handleGrokValidateRefreshToken"')
    expect(source).toContain('grokOAuth.validateSSOToken')
    expect(source).toContain('grokOAuth.buildCredentials')
    // Re-auth updates the existing account; must not call createFromSSO batch create
    expect(source).not.toContain('createFromSSO')
    expect(source).toContain('applyOAuthCredentials')
  REDACTED)

  it('hides footer code-exchange button for SSO/RT input methods', () => {
    expect(source).toContain("method === 'sso_cookie'")
    expect(source).toContain("method === 'refresh_token'")
  REDACTED)

  it('defaults reauth to refresh_token or sso_cookie (not password)', () => {
    expect(source).toContain('grokInitialInputMethod')
    expect(source).toContain(':initial-input-method="grokInitialInputMethod"')
    expect(source).toContain("return 'sso_cookie'")
    expect(source).toContain("return 'refresh_token'")
    expect(source).not.toContain("return 'email_password'")
    expect(source).not.toContain('grokPrefillEmailPassword')
  REDACTED)
REDACTED)

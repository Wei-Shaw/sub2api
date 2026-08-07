import { readFileSync REDACTED from 'node:fs'
import { resolve REDACTED from 'node:path'
import { describe, expect, it REDACTED from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/admin/account/ReAuthAccountModal.vue'),
  'utf8'
)

describe('ReAuthAccountModal Grok re-auth paths', () => {
  it('exposes SSO cookie, email-password, and refresh-token options for Grok', () => {
    expect(source).toContain(':show-sso-option="isGrok"')
    expect(source).toContain(':show-email-password-option="isGrok"')
    expect(source).toContain(':show-refresh-token-option="isOpenAI || isAntigravity || isGrok"')
  REDACTED)

  it('wires password and SSO handlers without batch account create', () => {
    expect(source).toContain('@authorize-password="handleGrokAuthorizePassword"')
    expect(source).toContain('@import-sso="handleGrokImportSSO"')
    expect(source).toContain('@validate-refresh-token="handleGrokValidateRefreshToken"')
    expect(source).toContain('handleGrokAuthorizePassword')
    expect(source).toContain('grokOAuth.authorizePassword')
    expect(source).toContain('grokOAuth.validateSSOToken')
    expect(source).toContain('grokOAuth.buildCredentials')
    // Re-auth updates the existing account; must not call createFromSSO batch create
    expect(source).not.toContain('createFromSSO')
    expect(source).toContain('applyOAuthCredentials')
  REDACTED)

  it('hides footer code-exchange button for SSO/password/RT input methods', () => {
    expect(source).toContain("method === 'sso_cookie'")
    expect(source).toContain("method === 'email_password'")
    expect(source).toContain("method === 'refresh_token'")
  REDACTED)

  it('prefills email---- and defaults to password method when email is known', () => {
    expect(source).toContain('grokPrefillEmailPassword')
    expect(source).toContain('grokInitialInputMethod')
    expect(source).toContain(':initial-email-password="grokPrefillEmailPassword"')
    expect(source).toContain(':initial-input-method="grokInitialInputMethod"')
    expect(source).toContain('email----')
    expect(source).toContain("return 'email_password'")
  REDACTED)
REDACTED)

import { readFileSync REDACTED from 'node:fs'
import { resolve REDACTED from 'node:path'
import { describe, expect, it REDACTED from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Grok account types', () => {
  it('offers API-key setup alongside OAuth with the official xAI default', () => {
    expect(source).toContain('data-testid="grok-account-type-api-key"')
    expect(source).toContain("@click=\"accountCategory = 'apikey'\"")
    expect(source).toContain("newPlatform === 'grok'")
    expect(source).toContain("? 'https://api.x.ai/v1'")
    expect(source).toContain("form.platform === 'grok'")
    expect(source).toContain("? 'xai-...'")
  REDACTED)

  it('exposes custom upstream URL and header override for the OAuth create flow', () => {
    expect(source).toContain('data-testid="grok-custom-base-url-toggle"')
    expect(source).toContain('data-testid="grok-custom-base-url-input"')
    expect(source).toContain('form.platform === \'grok\' && isOAuthFlow')
  REDACTED)

  it('validates and applies upstream config on Grok OAuth create paths', () => {
    // 授权码兑换 / RT 批量 / SSO 批量 / 密码登录（4 条路径）
    expect(source.match(/validateGrokOAuthUpstreamConfig\(\)/g)?.length).toBeGreaterThanOrEqual(4)
    expect(source.match(/applyGrokOAuthUpstreamConfig\(credentials\)/g)?.length).toBeGreaterThanOrEqual(4)
  REDACTED)

  it('wires Grok password authorize path without storing password/SSO fields', () => {
    expect(source).toContain('show-email-password-option')
    expect(source).toContain('@authorize-password="handleGrokAuthorizePassword"')
    expect(source).toContain('handleGrokAuthorizePassword')
    expect(source).toContain('grokOAuth.authorizePassword')
    expect(source).toContain('grokOAuth.buildCredentials')
    // Password only for authorize call; credentials come from buildCredentials
    expect(source).toContain('email----password')
  REDACTED)
REDACTED)

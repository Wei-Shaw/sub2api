import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

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
  })

  it('wires SSO and RT reauth without batch account create', () => {
    expect(source).toContain('@import-sso="handleGrokImportSSO"')
    expect(source).toContain('@validate-refresh-token="handleValidateRefreshToken"')
    expect(source).toContain('handleGrokValidateRefreshToken(refreshTokenInput)')
    expect(source).toContain('grokOAuth.validateSSOToken')
    expect(source).toContain('grokOAuth.buildCredentials')
    // Re-auth updates the existing account; must not call createFromSSO batch create
    expect(source).not.toContain('createFromSSO')
    expect(source).toContain('applyOAuthCredentials')
  })

  it('hides footer code-exchange button for SSO/RT input methods', () => {
    expect(source).toContain("method === 'sso_cookie'")
    expect(source).toContain("method === 'refresh_token'")
  })

  it('defaults reauth to refresh_token or sso_cookie (not password)', () => {
    expect(source).toContain('grokInitialInputMethod')
    expect(source).toContain(':initial-input-method="grokInitialInputMethod"')
    expect(source).toContain("return 'sso_cookie'")
    expect(source).toContain("return 'refresh_token'")
    expect(source).not.toContain("return 'email_password'")
    expect(source).not.toContain('grokPrefillEmailPassword')
  })
})

describe('ReAuthAccountModal OpenAI re-auth paths', () => {
  it('uses a wide dialog and exposes exactly the authorization methods available during creation', () => {
    expect(source).toContain('width="wide"')
    expect(source).toContain(':show-mobile-refresh-token-option="isOpenAI"')
    expect(source).toContain(':show-codex-session-import-option="isOpenAI"')
    expect(source).toContain(':show-agent-identity-option="isOpenAI"')
    expect(source).toContain(':show-codex-pat-option="isOpenAI"')
    expect(source).toContain(':reauthorize="isOpenAI"')
    expect(source).not.toContain('show-x-y-r-t-refresh-token-option')
  })

  it('updates the selected account through every non-code OpenAI authorization path', () => {
    expect(source).toContain('handleOpenAIReauthRefreshToken($event, OPENAI_MOBILE_RT_CLIENT_ID)')
    expect(source).toContain('@import-codex-session="handleOpenAIImportCodexSession"')
    expect(source).toContain('@import-codex-pat="handleOpenAIImportCodexPAT"')
    expect(source).toContain('account_id: props.account.id')
    expect(source).toContain('validateOpenAICodexPAT')
    expect(source).toContain('applyOAuthCredentials(props.account.id')
  })
})

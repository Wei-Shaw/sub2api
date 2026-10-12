import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal Cursor account', () => {
  it('offers a Cursor platform button with paste-token fields', () => {
    expect(source).toContain('data-testid="cursor-platform"')
    expect(source).toContain("form.platform = 'cursor'")
    expect(source).toContain('data-testid="cursor-credentials"')
    expect(source).toContain('data-testid="cursor-access-token"')
    expect(source).toContain('data-testid="cursor-machine-id"')
    expect(source).toContain('data-testid="cursor-mac-machine-id"')
  })

  it('creates Cursor accounts on step 1 instead of entering the browser OAuth flow', () => {
    expect(source).toContain("if (form.platform === 'cursor') {")
    expect(source).toContain('return false')
    expect(source).toContain("await createAccountAndFinish('cursor', 'oauth', built.credentials)")
    expect(source).toContain('buildCursorCredentials')
  })

  it('shows model restriction immediately after Cursor credentials', () => {
    const credentialsIdx = source.indexOf('data-testid="cursor-credentials"')
    const restrictionIdx = source.indexOf('data-testid="oauth-model-restriction"')
    expect(credentialsIdx).toBeGreaterThan(-1)
    expect(restrictionIdx).toBeGreaterThan(credentialsIdx)
    expect(restrictionIdx - credentialsIdx).toBeLessThan(8000)
    expect(source).toContain('admin.accounts.cursor.modelRestrictionHint')
    expect(source).toContain("form.platform === 'cursor' || ((form.platform === 'openai' || form.platform === 'grok') && isOAuthFlow)")
    expect(source).toContain("platform === 'grok' || platform === 'cursor'")
    expect(source).toContain("platform: 'cursor'")
    expect(source).toContain('access_token: accessToken')
    expect(source).toContain('list="oauth-mapping-targets"')
  })
})

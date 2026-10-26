import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/EditAccountModal.vue'),
  'utf8'
)

describe('EditAccountModal Cursor model restriction', () => {
  it('shows mapping and whitelist for Cursor OAuth accounts', () => {
    expect(source).toContain("account.platform === 'openai' || account.platform === 'grok' || account.platform === 'cursor'")
    expect(source).toContain('data-testid="oauth-model-restriction"')
    expect(source).toContain("newAccount.platform === 'openai' || newAccount.platform === 'grok' || newAccount.platform === 'cursor'")
    expect(source).toContain('list="edit-oauth-mapping-targets"')
  })
})

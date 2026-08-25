import { describe, expect, it } from 'vitest'

import {
  buildClaudeSetupTokenCredentials,
  parseClaudeSetupTokens
} from '@/composables/useAccountOAuth'

describe('Claude direct setup-token credentials', () => {
  it('stores only the long-lived inference bearer token', () => {
    expect(buildClaudeSetupTokenCredentials('  sk-ant-oat01-test-token  ')).toEqual({
      access_token: 'sk-ant-oat01-test-token',
      token_type: 'Bearer',
      scope: 'user:inference'
    })
  })

  it('does not accept a claude.ai sessionKey', () => {
    expect(() => buildClaudeSetupTokenCredentials('sk-ant-sid01-session-key')).toThrow(
      'Invalid Claude setup token'
    )
  })

  it('parses batch setup tokens one per line', () => {
    expect(parseClaudeSetupTokens('\nsk-ant-oat01-one\n  sk-ant-oat01-two  \n')).toEqual([
      'sk-ant-oat01-one',
      'sk-ant-oat01-two'
    ])
  })
})

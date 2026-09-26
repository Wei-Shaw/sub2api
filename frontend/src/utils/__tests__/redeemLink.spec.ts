import { describe, expect, it } from 'vitest'
import { parseRedeemLink } from '../redeemLink'

describe('parseRedeemLink', () => {
  it('reads an auto redeem code and removes it from the clean URL', () => {
    expect(parseRedeemLink('https://ai.gptplusch.store/redeem?auto=1#code=CODEX-100%20USD')).toEqual({
      code: 'CODEX-100 USD',
      auto: true,
      cleanUrl: '/redeem',
    })
  })

  it('does not auto submit when no code is present', () => {
    expect(parseRedeemLink('http://127.0.0.1:5174/redeem?auto=1')).toEqual({
      code: '',
      auto: false,
      cleanUrl: '/redeem',
    })
  })
})

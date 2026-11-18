import { describe, expect, it } from 'vitest'

import { cnQuotaCellVisible } from '../credentialsBuilder'

// 生产响应会把 control key 从 credentials 里剥掉，只留 credentials_status.has_<key>；
// 单元格可见性必须读状态位，否则线上永远不显示用量。
describe('cnQuotaCellVisible open_design', () => {
  it('shows the cell for a redacted account with the status flag', () => {
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: {},
      credentials_status: { has_opendesign_control_key: true }
    })).toBe(true)
  })

  it('shows the cell for unredacted credentials (local preview data)', () => {
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: { opendesign_control_key: 'vela_ctrl_x' }
    })).toBe(true)
  })

  it('hides the cell when the control key was never configured', () => {
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: {},
      credentials_status: { has_api_key: true }
    })).toBe(false)
    expect(cnQuotaCellVisible({ platform: 'open_design', type: 'apikey' })).toBe(false)
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: { opendesign_control_key: '   ' }
    })).toBe(false)
  })

  it('does not loosen the rules for other platforms', () => {
    expect(cnQuotaCellVisible({ platform: 'kimi', type: 'apikey', credentials: { account_mode: 'payg' } })).toBe(false)
    expect(cnQuotaCellVisible({ platform: 'bogus', type: 'apikey' })).toBe(false)
  })
})

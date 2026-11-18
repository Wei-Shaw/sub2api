import { describe, expect, it } from 'vitest'

import { cnQuotaCellVisible } from '../credentialsBuilder'

// OpenDesign 的用量单元格只在拿到控制面凭据时展示：没有 control key 探测必然失败，
// 展示空表格只会误导运营。
describe('cnQuotaCellVisible open_design', () => {
  it('shows the CN quota cell when the control key is present', () => {
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: { opendesign_control_key: 'vela_ctrl_x' }
    })).toBe(true)
  })

  it('hides it when the control key is missing or blank', () => {
    expect(cnQuotaCellVisible({ platform: 'open_design', type: 'apikey' })).toBe(false)
    expect(cnQuotaCellVisible({
      platform: 'open_design',
      type: 'apikey',
      credentials: { opendesign_control_key: '   ' }
    })).toBe(false)
  })

  it('does not loosen the rules for other platforms', () => {
    // Kimi 非 coding 模式仍按余额展示，不给额度单元格。
    expect(cnQuotaCellVisible({ platform: 'kimi', type: 'apikey', credentials: { account_mode: 'payg' } })).toBe(false)
    // 未知平台一律不展示。
    expect(cnQuotaCellVisible({ platform: 'bogus', type: 'apikey' })).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('upstream recharge multiplier locale keys', () => {
  it('uses explicit Chinese labels instead of the K symbol', () => {
    expect(zh.admin.upstreams.columns.rechargeMultiplier).toBe('充值倍率')
    expect(zh.admin.upstreams.form.rechargeMultiplier).toBe('充值倍率')
    expect(zh.admin.upstreams.form.rechargeSource).toBe('充值倍率来源')
    expect(zh.admin.upstreams.routes.rechargeMultiplier).toBe('充值倍率')
    expect(zh.admin.upstreams.routes.kInherited).toBe('充值倍率继承站点配置，请在站点编辑中修改。')
  })

  it('keeps the corresponding English labels available', () => {
    expect(en.admin.upstreams.columns.rechargeMultiplier).toBe('Recharge multiplier')
    expect(en.admin.upstreams.form.rechargeMultiplier).toBe('Recharge multiplier')
    expect(en.admin.upstreams.routes.rechargeMultiplier).toBe('Recharge multiplier')
  })
})

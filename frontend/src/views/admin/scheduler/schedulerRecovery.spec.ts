import { describe, expect, it } from 'vitest'
import type { OpsModelCooldown } from '@/api/admin/ops'
import { affectedAccountCount, recoveryReasonLabel, recoveryStateLabel } from './schedulerRecovery'

const row: OpsModelCooldown = { account_id: 1, model: '*', failure_streak: 1,
  last_failure_at: '', block_until: '', remaining_seconds: 0 }

describe('scheduler recovery', () => {
  it('counts accounts rather than account-model entries', () => {
    expect(affectedAccountCount([row, { ...row, model: 'a' }, { ...row, account_id: 2 }])).toBe(2)
    expect(affectedAccountCount([])).toBe(0)
  })
  it('does not call a pending probe recovered', () => {
    expect(recoveryStateLabel({ ...row, state: 'awaiting_probe' }, 'zh-CN')).toBe('等待试探')
    expect(recoveryStateLabel({ ...row, state: 'probing' }, 'en')).toBe('Probing')
    expect(recoveryStateLabel({ ...row, state: 'recovering' }, 'zh')).toBe('恢复验证中')
  })
  it('supports old snapshots and safe reason labels', () => {
    expect(recoveryStateLabel(row, 'en')).toBe('Cooling down')
    expect(recoveryReasonLabel({ ...row, reason: 'upstream_502' }, 'zh')).toBe('上游 HTTP 502')
    expect(recoveryReasonLabel({ ...row, reason: 'cross_model_503' }, 'en')).toBe('Persistent failures across models')
  })
})

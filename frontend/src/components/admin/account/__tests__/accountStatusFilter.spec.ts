import { describe, expect, it } from 'vitest'

import {
  encodeOpenAIQuotaFullStatus,
  encodeOpenAIQuotaUsedRangeStatus,
  isOpenAIQuotaEncodedStatus,
  parseOpenAIQuotaFullStatus,
  parseOpenAIQuotaUsedRangeStatus
} from '../accountStatusFilter'

describe('accountStatusFilter', () => {
  it('编码和解析指定额度状态筛选', () => {
    const encoded = encodeOpenAIQuotaUsedRangeStatus('7d', 42, 42)

    expect(encoded).toBe('openai_quota_used_range:7d:42:42')
    expect(parseOpenAIQuotaUsedRangeStatus(encoded)).toEqual({ window: '7d', min: 42, max: 42 })
    expect(isOpenAIQuotaEncodedStatus(encoded)).toBe(true)
  })

  it('指定额度状态筛选允许相同上下限并拒绝反向区间', () => {
    expect(encodeOpenAIQuotaUsedRangeStatus('5h', 10, 10)).toBe('openai_quota_used_range:5h:10:10')
    expect(encodeOpenAIQuotaUsedRangeStatus('5h', 30, 10)).toBe('')
    expect(parseOpenAIQuotaUsedRangeStatus('openai_quota_used_range:5h:30:10')).toBeNull()
  })

  it('编码和解析额度已满状态筛选', () => {
    const encoded = encodeOpenAIQuotaFullStatus('5h')

    expect(encoded).toBe('openai_quota_full:5h')
    expect(parseOpenAIQuotaFullStatus(encoded)).toBe('5h')
    expect(isOpenAIQuotaEncodedStatus(encoded)).toBe(true)
  })
})

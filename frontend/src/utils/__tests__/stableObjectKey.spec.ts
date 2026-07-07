import { describe, expect, it } from 'vitest'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'

describe('createStableObjectKeyResolver', () => {
  it('对同一对象返回稳定 key', () => {
    const resolve = createStableObjectKeyResolver<{ value: string }>('rule')
    const obj = { value: 'a' }

    const key1 = resolve(obj)
    const key2 = resolve(obj)

    expect(key1).toBe(key2)
    expect(key1.startsWith('rule-')).toBe(true)
  })

  it('不同对象返回不同 key', () => {
    const resolve = createStableObjectKeyResolver<{ value: string }>('rule')

    const key1 = resolve({ value: 'a' })
    const key2 = resolve({ value: 'a' })

    expect(key1).not.toBe(key2)
  })

  it('不同 resolver 互不影响', () => {
    const resolveA = createStableObjectKeyResolver<{ id: number }>('a')
    const resolveB = createStableObjectKeyResolver<{ id: number }>('b')
    const obj = { id: 1 }

    const keyA = resolveA(obj)
    const keyB = resolveB(obj)

    expect(keyA).not.toBe(keyB)
    expect(keyA.startsWith('a-')).toBe(true)
    expect(keyB.startsWith('b-')).toBe(true)
  })
})

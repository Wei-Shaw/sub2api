import { describe, expect, it REDACTED from 'vitest'
import { createStableObjectKeyResolver REDACTED from '@/utils/stableObjectKey'

describe('createStableObjectKeyResolver', () => {
  it('对同一对象返回稳定 key', () => {
    const resolve = createStableObjectKeyResolver<{ value: string REDACTED>('rule')
    const obj = { value: 'a' REDACTED

    const key1 = resolve(obj)
    const key2 = resolve(obj)

    expect(key1).toBe(key2)
    expect(key1.startsWith('rule-')).toBe(true)
  REDACTED)

  it('不同对象返回不同 key', () => {
    const resolve = createStableObjectKeyResolver<{ value: string REDACTED>('rule')

    const key1 = resolve({ value: 'a' REDACTED)
    const key2 = resolve({ value: 'a' REDACTED)

    expect(key1).not.toBe(key2)
  REDACTED)

  it('不同 resolver 互不影响', () => {
    const resolveA = createStableObjectKeyResolver<{ id: number REDACTED>('a')
    const resolveB = createStableObjectKeyResolver<{ id: number REDACTED>('b')
    const obj = { id: 1 REDACTED

    const keyA = resolveA(obj)
    const keyB = resolveB(obj)

    expect(keyA).not.toBe(keyB)
    expect(keyA.startsWith('a-')).toBe(true)
    expect(keyB.startsWith('b-')).toBe(true)
  REDACTED)
REDACTED)

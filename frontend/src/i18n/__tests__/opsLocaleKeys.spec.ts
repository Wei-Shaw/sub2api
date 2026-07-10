import { describe, expect, it REDACTED from 'vitest'
import en from '@/i18n/locales/en'

function flattenKeys(obj: Record<string, any>, prefix = ''): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefixREDACTED.${kREDACTED` : k
    if (typeof v === 'object' && v !== null && !Array.isArray(v)) {
      keys.push(...flattenKeys(v, fullKey))
    REDACTED else {
      keys.push(fullKey)
    REDACTED
  REDACTED
  return keys
REDACTED

describe('ops locale key completeness', () => {
  const requiredKeys = [
    'admin.ops.result',
    'admin.ops.timeRange.custom',
    'admin.ops.customTimeRange.startTime',
    'admin.ops.customTimeRange.endTime',
  ]

  for (const key of requiredKeys) {
    it(`en locale has ${keyREDACTED`, () => {
      const enKeys = flattenKeys(en)
      expect(enKeys).toContain(key)
    REDACTED)
  REDACTED
REDACTED)

describe('groups locale key completeness', () => {
  it('en locale has admin.groups.failedToSave', () => {
    const enKeys = flattenKeys(en)
    expect(enKeys).toContain('admin.groups.failedToSave')
  REDACTED)
REDACTED)

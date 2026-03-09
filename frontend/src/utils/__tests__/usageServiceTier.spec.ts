import { describe, expect, it REDACTED from 'vitest'

import { formatUsageServiceTier, getUsageServiceTierLabel, normalizeUsageServiceTier REDACTED from '@/utils/usageServiceTier'

describe('usageServiceTier utils', () => {
  it('normalizes fast/default aliases', () => {
    expect(normalizeUsageServiceTier('fast')).toBe('priority')
    expect(normalizeUsageServiceTier(' default ')).toBe('standard')
    expect(normalizeUsageServiceTier('STANDARD')).toBe('standard')
  REDACTED)

  it('preserves supported tiers', () => {
    expect(normalizeUsageServiceTier('priority')).toBe('priority')
    expect(normalizeUsageServiceTier('flex')).toBe('flex')
  REDACTED)

  it('formats empty values as standard', () => {
    expect(formatUsageServiceTier()).toBe('standard')
    expect(formatUsageServiceTier('')).toBe('standard')
  REDACTED)

  it('passes through unknown non-empty tiers for display fallback', () => {
    expect(normalizeUsageServiceTier('custom-tier')).toBe('custom-tier')
    expect(formatUsageServiceTier('custom-tier')).toBe('custom-tier')
  REDACTED)

  it('maps tiers to translated labels', () => {
    const translate = (key: string) => ({
      'usage.serviceTierPriority': 'Fast',
      'usage.serviceTierFlex': 'Flex',
      'usage.serviceTierStandard': 'Standard',
    REDACTED)[key] ?? key

    expect(getUsageServiceTierLabel('fast', translate)).toBe('Fast')
    expect(getUsageServiceTierLabel('flex', translate)).toBe('Flex')
    expect(getUsageServiceTierLabel(undefined, translate)).toBe('Standard')
    expect(getUsageServiceTierLabel('custom-tier', translate)).toBe('custom-tier')
  REDACTED)
REDACTED)

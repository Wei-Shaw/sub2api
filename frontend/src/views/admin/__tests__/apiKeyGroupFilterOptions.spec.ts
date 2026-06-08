import { describe, it, expect REDACTED from 'vitest'
import { buildApiKeyGroupFilterOptions REDACTED from '../apiKeyGroupFilterOptions'
import type { AdminGroup REDACTED from '@/types'

const labels = {
  all: 'All',
  exclusive: 'Exclusive',
  public: 'Public',
  subscription: 'Subscription',
  disabled: 'Disabled',
REDACTED

function g(partial: Partial<AdminGroup>): AdminGroup {
  return {
    id: 0,
    name: '',
    status: 'active',
    is_exclusive: false,
    subscription_type: 'standard',
    ...partial,
  REDACTED as AdminGroup
REDACTED

describe('buildApiKeyGroupFilterOptions', () => {
  it('partitions active groups into exclusive/public/subscription with headers', () => {
    const groups = [
      g({ id: 1, name: 'Excl', is_exclusive: true, subscription_type: 'standard' REDACTED),
      g({ id: 2, name: 'Pub', is_exclusive: false, subscription_type: 'standard' REDACTED),
      g({ id: 3, name: 'Sub', is_exclusive: false, subscription_type: 'subscription' REDACTED),
    ]
    expect(buildApiKeyGroupFilterOptions(groups, labels)).toEqual([
      { value: null, label: 'All' REDACTED,
      { value: -1, label: 'Exclusive', kind: 'group', disabled: true REDACTED,
      { value: 1, label: 'Excl' REDACTED,
      { value: -2, label: 'Public', kind: 'group', disabled: true REDACTED,
      { value: 2, label: 'Pub' REDACTED,
      { value: -3, label: 'Subscription', kind: 'group', disabled: true REDACTED,
      { value: 3, label: 'Sub' REDACTED,
    ])
  REDACTED)

  it('treats subscription_type=subscription as subscription even if is_exclusive', () => {
    const groups = [g({ id: 9, name: 'X', is_exclusive: true, subscription_type: 'subscription' REDACTED)]
    const opts = buildApiKeyGroupFilterOptions(groups, labels)
    expect(opts).toContainEqual({ value: 9, label: 'X' REDACTED)
    expect(opts.find((o) => o.label === 'Subscription')).toBeDefined()
    expect(opts.find((o) => o.label === 'Exclusive')).toBeUndefined()
  REDACTED)

  it('skips empty section headers', () => {
    const groups = [g({ id: 2, name: 'Pub', is_exclusive: false, subscription_type: 'standard' REDACTED)]
    const opts = buildApiKeyGroupFilterOptions(groups, labels)
    expect(opts.find((o) => o.label === 'Exclusive')).toBeUndefined()
    expect(opts.find((o) => o.label === 'Subscription')).toBeUndefined()
    expect(opts).toContainEqual({ value: -2, label: 'Public', kind: 'group', disabled: true REDACTED)
  REDACTED)

  it('places non-active groups in a separate disabled section (not omitted)', () => {
    const groups = [
      g({ id: 1, name: 'Active', is_exclusive: true REDACTED),
      g({ id: 2, name: 'Inactive', is_exclusive: true, status: 'inactive' REDACTED),
    ]
    const opts = buildApiKeyGroupFilterOptions(groups, labels)
    // Active exclusive group appears in Exclusive section
    expect(opts).toContainEqual({ value: 1, label: 'Active' REDACTED)
    // Disabled group appears in Disabled section
    expect(opts).toContainEqual({ value: 2, label: 'Inactive' REDACTED)
    // Disabled section header present
    expect(opts).toContainEqual({ value: -4, label: 'Disabled', kind: 'group', disabled: true REDACTED)
    // Not in Exclusive section
    const exclIdx = opts.findIndex((o) => o.value === -1)
    const disabledItemIdx = opts.findIndex((o) => o.value === 2)
    expect(exclIdx).toBeLessThan(disabledItemIdx)
  REDACTED)

  it('section headers use distinct negative values (no duplicate Vue :key)', () => {
    const groups = [
      g({ id: 1, name: 'E', is_exclusive: true REDACTED),
      g({ id: 2, name: 'P', is_exclusive: false REDACTED),
      g({ id: 3, name: 'S', subscription_type: 'subscription' REDACTED),
      g({ id: 4, name: 'D', status: 'inactive' REDACTED),
    ]
    const opts = buildApiKeyGroupFilterOptions(groups, labels)
    const headerValues = opts.filter((o) => o.kind === 'group').map((o) => o.value)
    const unique = new Set(headerValues)
    expect(unique.size).toBe(headerValues.length) // all distinct
    headerValues.forEach((v) => expect(v).toBeLessThan(0)) // all negative
  REDACTED)

  it('returns only the all-option when there are no groups', () => {
    expect(buildApiKeyGroupFilterOptions([], labels)).toEqual([{ value: null, label: 'All' REDACTED])
  REDACTED)
REDACTED)

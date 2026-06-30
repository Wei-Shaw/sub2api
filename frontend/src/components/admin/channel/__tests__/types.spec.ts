import { describe, expect, it REDACTED from 'vitest'
import { validateIntervals, type IntervalFormEntry REDACTED from '../types'

function makeInterval(over: Partial<IntervalFormEntry>): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: 0,
    ...over,
  REDACTED
REDACTED

function t(key: string, params?: Record<string, unknown>): string {
  return `${keyREDACTED${params ? ` ${JSON.stringify(params)REDACTED` : ''REDACTED`
REDACTED

describe('validateIntervals', () => {
  describe('token mode', () => {
    it('rejects unbounded interval that is not last', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    REDACTED)

    it('accepts unbounded interval at the end', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: null, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token', t)).toBeNull()
    REDACTED)

    it('rejects overlapping intervals', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 250000, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('overlap')
    REDACTED)

    it('rejects unbounded interval in token mode', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 100, max_tokens: 200, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token', t)).toContain('unboundedLast')
    REDACTED)
  REDACTED)

  describe('image / per_request mode', () => {
    it('allows multiple unbounded tiers identified by label', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: 0.04 REDACTED),
        makeInterval({ tier_label: '2K', per_request_price: 0.06 REDACTED),
        makeInterval({ tier_label: '4K', per_request_price: 0.08 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image', t)).toBeNull()
      expect(validateIntervals(intervals, 'per_request', t)).toBeNull()
    REDACTED)

    it('still rejects negative prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: -1 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('negativePrice')
    REDACTED)

    it('still rejects max <= min on a single tier', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', min_tokens: 100, max_tokens: 50, per_request_price: 0.04 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image', t)).toContain('maxGreaterThanMin')
    REDACTED)
  REDACTED)
REDACTED)

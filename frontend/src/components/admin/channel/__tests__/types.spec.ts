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

describe('validateIntervals', () => {
  describe('token mode', () => {
    it('rejects unbounded interval that is not last', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token')).toMatch(/unbounded interval/)
    REDACTED)

    it('accepts unbounded interval at the end', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 200000, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: null, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token')).toBeNull()
    REDACTED)

    it('rejects overlapping intervals', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: 250000, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 200000, max_tokens: 500000, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals, 'token')).toMatch(/overlap/)
    REDACTED)

    it('defaults mode to token when omitted', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ min_tokens: 0, max_tokens: null, input_price: 1, output_price: 1 REDACTED),
        makeInterval({ min_tokens: 100, max_tokens: 200, input_price: 2, output_price: 2 REDACTED),
      ]
      expect(validateIntervals(intervals)).toMatch(/unbounded interval/)
    REDACTED)
  REDACTED)

  describe('image / per_request mode', () => {
    it('allows multiple unbounded tiers identified by label', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: 0.04 REDACTED),
        makeInterval({ tier_label: '2K', per_request_price: 0.06 REDACTED),
        makeInterval({ tier_label: '4K', per_request_price: 0.08 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image')).toBeNull()
      expect(validateIntervals(intervals, 'per_request')).toBeNull()
    REDACTED)

    it('still rejects negative prices', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', per_request_price: -1 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image')).toMatch(/cannot be negative/)
    REDACTED)

    it('still rejects max <= min on a single tier', () => {
      const intervals: IntervalFormEntry[] = [
        makeInterval({ tier_label: '1K', min_tokens: 100, max_tokens: 50, per_request_price: 0.04 REDACTED),
      ]
      expect(validateIntervals(intervals, 'image')).toMatch(/must be greater/)
    REDACTED)
  REDACTED)
REDACTED)

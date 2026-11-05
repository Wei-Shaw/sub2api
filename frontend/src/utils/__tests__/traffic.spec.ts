import { describe, expect, it } from 'vitest'
import { formatTrafficBandwidth, formatTrafficBytes, formatTrafficBytesWithBandwidth, formatTrafficCompressionRate } from '../traffic'

describe('formatTrafficBytes', () => {
  it.each([
    [undefined, '—'],
    [null, '—'],
    [NaN, '—'],
    [Infinity, '—'],
    [-Infinity, '—'],
    [-1, '—'],
    [0, '0 B'],
    [-0, '0 B'],
    [1, '1 B'],
    [1023, '1023 B'],
    [1024, '1 KiB'],
    [1536, '1.5 KiB'],
    [1280, '1.25 KiB'],
    [1024 ** 2 - 1, '1 MiB'],
    [1024 ** 2, '1 MiB'],
    [1024 ** 3, '1 GiB'],
    [1024 ** 4, '1 TiB'],
    [1024 ** 5, '1 PiB'],
    [1024 ** 6, '1 EiB'],
    [2 ** 63 - 1, '8 EiB'],
    [Number.MAX_SAFE_INTEGER, '8 PiB'],
  ])('formats %s as %s', (bytes, expected) => {
    expect(formatTrafficBytes(bytes)).toBe(expected)
  })
})

describe('formatTrafficBandwidth', () => {
  it.each([
    [0, 60, '0 bps'],
    [-0, 60, '0 bps'],
    [1, 1, '8 bps'],
    [1, 1000, '< 0.01 bps'],
    [125, 1, '1 kbps'],
    [125_000, 1, '1 Mbps'],
    [187_500, 1, '1.5 Mbps'],
    [125_000_000, 1, '1 Gbps'],
    [125_000_000_000, 1, '1 Tbps'],
    [125_000_000_000_000, 1, '1 Pbps'],
    [125_000_000_000_000_000, 1, '1 Ebps'],
    [125_000 * 60, 60, '1 Mbps'],
    [125_000 * 3600, 3600, '1 Mbps'],
    [125_000 * 86400, 86400, '1 Mbps'],
    [125_000, 0.5, '2 Mbps'],
    [1024, 1, '8.19 kbps'],
    [124_999.9, 1, '1 Mbps'],
    [undefined, 60, '—'],
    [null, 60, '—'],
    [NaN, 60, '—'],
    [Infinity, 60, '—'],
    [-Infinity, 60, '—'],
    [-1, 60, '—'],
    [1024, undefined, '—'],
    [1024, null, '—'],
    [1024, 0, '—'],
    [0, 0, '—'],
    [1024, -1, '—'],
    [1024, NaN, '—'],
    [1024, Infinity, '—'],
    [1024, -Infinity, '—'],
    [Number.MAX_VALUE, Number.MIN_VALUE, '—'],
  ])('formats %s bytes over %s seconds as %s', (bytes, seconds, expected) => {
    expect(formatTrafficBandwidth(bytes, seconds)).toBe(expected)
  })
})

describe('formatTrafficBytesWithBandwidth', () => {
  it.each([
    [1024, 60, '1 KiB, 136.53 bps'],
    [1024, 3.2, '1 KiB, 2.56 kbps'],
    [187_500, 1, '183.11 KiB, 1.5 Mbps'],
    [1_006_601_503, 5368.5413493333335, '959.97 MiB, 1.5 Mbps'],
    [0, 60, '0 B, 0 bps'],
    [1, 1000, '1 B, < 0.01 bps'],
    [1024, 0, '1 KiB, —'],
    [1024, null, '1 KiB, —'],
    [1024, undefined, '1 KiB, —'],
    [undefined, 60, '—'],
    [null, 60, '—'],
    [NaN, 60, '—'],
    [-1, 60, '—'],
    [1024, -1, '1 KiB, —'],
    [1024, Infinity, '1 KiB, —'],
  ])('formats %s bytes over %s seconds as %s', (bytes, seconds, expected) => {
    expect(formatTrafficBytesWithBandwidth(bytes, seconds)).toBe(expected)
  })
})

describe('formatTrafficCompressionRate', () => {
  it.each([
    [100, 50, '50.0%'],
    [100, 100, '0.0%'],
    [100, 150, '-50.0%'],
    [100, 200, '-100.0%'],
    [100, 0, '100.0%'],
    [3, 1, '66.7%'],
    [100000, 100001, '0.0%'],
    [Number.MAX_VALUE, Number.MAX_VALUE, '0.0%'],
    [0, 0, '—'],
    [0, 10, '—'],
    [undefined, undefined, '—'],
    [null, 0, '—'],
    [100, null, '—'],
    [100, undefined, '—'],
    [NaN, 1, '—'],
    [1, NaN, '—'],
    [Infinity, 1, '—'],
    [1, Infinity, '—'],
    [-1, 1, '—'],
    [1, -1, '—'],
    [Number.MIN_VALUE, Number.MAX_VALUE, '—'],
  ])('formats original %s / actual %s as %s', (original, actual, expected) => {
    expect(formatTrafficCompressionRate(original, actual)).toBe(expected)
  })
})

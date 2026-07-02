import { describe, expect, it REDACTED from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('usage ipGeo locale keys', () => {
  it('contains zh labels for IP geolocation UI', () => {
    expect(zh.usage.ipGeo.fetch).toBe('获取地区')
    expect(zh.usage.ipGeo.fetching).toBe('获取中...')
    expect(zh.usage.ipGeo.failed).toBe('获取失败')
    expect(zh.usage.ipGeo.private).toBe('内网地址')
    expect(zh.usage.ipGeo.batchFetch).toBe('批量获取地区')
    expect(zh.usage.ipGeo.pending).toBe('{countREDACTED 个 IP 待获取地区')
  REDACTED)

  it('contains en labels for IP geolocation UI', () => {
    expect(en.usage.ipGeo.fetch).toBe('Fetch region')
    expect(en.usage.ipGeo.fetching).toBe('Fetching...')
    expect(en.usage.ipGeo.failed).toBe('Failed')
    expect(en.usage.ipGeo.private).toBe('Private address')
    expect(en.usage.ipGeo.batchFetch).toBe('Batch fetch regions')
    expect(en.usage.ipGeo.pending).toBe('{countREDACTED IPs pending')
  REDACTED)
REDACTED)

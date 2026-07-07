import { afterEach, describe, expect, it } from 'vitest'

import {
  DEFAULT_TABLE_PAGE_SIZE,
  DEFAULT_TABLE_PAGE_SIZE_OPTIONS,
  getConfiguredTableDefaultPageSize,
  getConfiguredTablePageSizeOptions,
  normalizeTablePageSize
} from '@/utils/tablePreferences'

describe('tablePreferences', () => {
  afterEach(() => {
    delete window.__APP_CONFIG__
  })

  it('returns built-in defaults when app config is missing', () => {
    expect(getConfiguredTableDefaultPageSize()).toBe(DEFAULT_TABLE_PAGE_SIZE)
    expect(getConfiguredTablePageSizeOptions()).toEqual(DEFAULT_TABLE_PAGE_SIZE_OPTIONS)
  })

  it('uses configured defaults when app config is valid', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 50,
      table_page_size_options: [20, 50, 100]
    } as any

    expect(getConfiguredTableDefaultPageSize()).toBe(50)
    expect(getConfiguredTablePageSizeOptions()).toEqual([20, 50, 100])
  })

  it('allows default page size outside selectable options', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 1000,
      table_page_size_options: [20, 50, 100]
    } as any

    expect(getConfiguredTableDefaultPageSize()).toBe(1000)
    expect(getConfiguredTablePageSizeOptions()).toEqual([20, 50, 100])
    expect(normalizeTablePageSize(1000)).toBe(100)
    expect(normalizeTablePageSize(35)).toBe(50)
  })

  it('normalizes invalid options without rewriting the configured default itself', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 35,
      table_page_size_options: [1001, 50, 10, 10, 2, 0]
    } as any

    expect(getConfiguredTableDefaultPageSize()).toBe(35)
    expect(getConfiguredTablePageSizeOptions()).toEqual([10, 50])
    expect(normalizeTablePageSize(undefined)).toBe(50)
  })

  it('normalizes page size against configured options by rounding up', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 20,
      table_page_size_options: [20, 50, 1000]
    } as any

    expect(normalizeTablePageSize(20)).toBe(20)
    expect(normalizeTablePageSize(30)).toBe(50)
    expect(normalizeTablePageSize(100)).toBe(1000)
    expect(normalizeTablePageSize(1500)).toBe(1000)
    expect(normalizeTablePageSize(undefined)).toBe(20)
  })

  it('keeps built-in selectable defaults at 10, 20, 50, 100', () => {
    window.__APP_CONFIG__ = {
      table_default_page_size: 1000
    } as any

    expect(getConfiguredTablePageSizeOptions()).toEqual([10, 20, 50, 100])
  })
})

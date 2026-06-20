import { describe, expect, it } from 'vitest'
import {
  DEFAULT_DOCUMENT_TITLE,
  DEFAULT_SITE_NAME,
  DEFAULT_SITE_SUBTITLE,
} from '@/constants/branding'

describe('branding defaults', () => {
  it('uses the Yui-facing default site name', () => {
    expect(DEFAULT_SITE_NAME).toBe('天才程序员小站')
  })

  it('keeps a concise default subtitle for the API console', () => {
    expect(DEFAULT_SITE_SUBTITLE).toBe('AI API Gateway')
  })

  it('builds the default browser title from the default site name', () => {
    expect(DEFAULT_DOCUMENT_TITLE).toBe('天才程序员小站 - AI API Gateway')
  })
})

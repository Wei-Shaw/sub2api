/**
 * Contract tests for the shipped markdown, not for the view.
 *
 * The registry names files by hand, so a renamed or missing `docs/public/*.md`
 * is only visible at runtime, on a page a reader already opened. These load
 * every page in every locale and check the cross-links resolve, which is the
 * one class of typo the view itself cannot catch.
 */
import { describe, expect, it } from 'vitest'

import { DEFAULT_DOCS_SLUG, DOCS_PAGES, findDocsPage } from '../docsPages'

const LOCALES = ['en', 'zh'] as const

/** `[Text](/docs/slug)` and `[Text](/docs/slug#anchor)`. */
const INTERNAL_LINK = /\]\(\/docs\/([a-z0-9-]+)/g

describe('docs registry', () => {
  it('has unique slugs', () => {
    const slugs = DOCS_PAGES.map((page) => page.slug)
    expect(new Set(slugs).size).toBe(slugs.length)
  })

  it('resolves its default slug', () => {
    expect(findDocsPage(DEFAULT_DOCS_SLUG)).not.toBeNull()
  })

  it('returns null for an unknown slug', () => {
    expect(findDocsPage('no-such-page')).toBeNull()
  })

  it.each(DOCS_PAGES.map((page) => page.slug))('%s ships content in every locale', async (slug) => {
    const page = findDocsPage(slug)
    expect(page).not.toBeNull()

    for (const locale of LOCALES) {
      const { default: markdown } = await page!.load[locale]()
      expect(markdown.trim().length).toBeGreaterThan(0)
      expect(markdown).toMatch(/^# /)
    }
  })

  it('only cross-links slugs that exist', async () => {
    const known = new Set(DOCS_PAGES.map((page) => page.slug))
    // `/docs/batch-image` is the authenticated in-app guide, reachable from a
    // public page on purpose and deliberately not in this registry.
    known.add('batch-image')

    for (const page of DOCS_PAGES) {
      for (const locale of LOCALES) {
        const { default: markdown } = await page.load[locale]()
        for (const match of markdown.matchAll(INTERNAL_LINK)) {
          expect(known, `${page.slug}.${locale}.md links /docs/${match[1]}`).toContain(match[1])
        }
      }
    }
  })

  it('writes the base URL as a substitutable token, never a hardcoded host', async () => {
    for (const page of DOCS_PAGES) {
      for (const locale of LOCALES) {
        const { default: markdown } = await page.load[locale]()
        expect(markdown, `${page.slug}.${locale}.md`).not.toMatch(
          /https?:\/\/(localhost|127\.0\.0\.1)/
        )
      }
    }
  })
})

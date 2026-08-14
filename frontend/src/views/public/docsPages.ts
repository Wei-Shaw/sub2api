/**
 * The public documentation table of contents.
 *
 * Content lives in `docs/public/*.{en,zh}.md` — markdown in the repository,
 * reviewed and versioned with the code it describes, rather than rows in a
 * table an operator edits. The loaders are dynamic so a reader of one page
 * never downloads the other five, and so nothing here lands in the main
 * bundle.
 *
 * Titles come from i18n rather than from the markdown's own `# heading`,
 * because the sidebar has to render every page's title before any of their
 * markdown has been fetched.
 */
import type { LocaleCode } from '@/i18n'

export interface DocsPage {
  /** URL segment: `/docs/<slug>`. */
  slug: string
  /** i18n key for the sidebar label and the page heading. */
  titleKey: string
  /** i18n key for the one-line summary under the heading. */
  summaryKey: string
  load: Record<LocaleCode, () => Promise<{ default: string }>>
}

export const DOCS_PAGES: readonly DocsPage[] = [
  {
    slug: 'overview',
    titleKey: 'docs.pages.overview.title',
    summaryKey: 'docs.pages.overview.summary',
    load: {
      en: () => import('../../../../docs/public/overview.en.md?raw'),
      zh: () => import('../../../../docs/public/overview.zh.md?raw'),
    },
  },
  {
    slug: 'quickstart',
    titleKey: 'docs.pages.quickstart.title',
    summaryKey: 'docs.pages.quickstart.summary',
    load: {
      en: () => import('../../../../docs/public/quickstart.en.md?raw'),
      zh: () => import('../../../../docs/public/quickstart.zh.md?raw'),
    },
  },
  {
    slug: 'authentication',
    titleKey: 'docs.pages.authentication.title',
    summaryKey: 'docs.pages.authentication.summary',
    load: {
      en: () => import('../../../../docs/public/authentication.en.md?raw'),
      zh: () => import('../../../../docs/public/authentication.zh.md?raw'),
    },
  },
  {
    slug: 'api-reference',
    titleKey: 'docs.pages.apiReference.title',
    summaryKey: 'docs.pages.apiReference.summary',
    load: {
      en: () => import('../../../../docs/public/api-reference.en.md?raw'),
      zh: () => import('../../../../docs/public/api-reference.zh.md?raw'),
    },
  },
  {
    slug: 'billing-and-usage',
    titleKey: 'docs.pages.billingAndUsage.title',
    summaryKey: 'docs.pages.billingAndUsage.summary',
    load: {
      en: () => import('../../../../docs/public/billing-and-usage.en.md?raw'),
      zh: () => import('../../../../docs/public/billing-and-usage.zh.md?raw'),
    },
  },
  {
    slug: 'errors',
    titleKey: 'docs.pages.errors.title',
    summaryKey: 'docs.pages.errors.summary',
    load: {
      en: () => import('../../../../docs/public/errors.en.md?raw'),
      zh: () => import('../../../../docs/public/errors.zh.md?raw'),
    },
  },
]

/** The page `/docs` itself resolves to. */
export const DEFAULT_DOCS_SLUG = DOCS_PAGES[0].slug

export function findDocsPage(slug: string): DocsPage | null {
  return DOCS_PAGES.find((page) => page.slug === slug) ?? null
}

import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import DocsView from '../DocsView.vue'

const { appStore, routeParams, push, loaders } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: null as Record<string, unknown> | null,
    fetchPublicSettings: vi.fn(),
  },
  routeParams: { slug: '' as string },
  push: vi.fn(),
  // Per-locale loaders the tests swap out, so a page body is a fixture rather
  // than whatever `docs/public` happens to say today. `docsPages.spec.ts`
  // covers the real files.
  loaders: {
    en: vi.fn(),
    zh: vi.fn(),
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ params: routeParams }),
    useRouter: () => ({ push }),
  }
})

// `@/i18n` pulls `createI18n` and every locale bundle into the graph; the view
// only needs the locale getter.
vi.mock('@/i18n', () => ({
  getLocale: () => 'en',
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: ref('en') }),
  }
})

vi.mock('../docsPages', () => {
  const pages = [
    {
      slug: 'overview',
      titleKey: 'docs.pages.overview.title',
      summaryKey: 'docs.pages.overview.summary',
      load: loaders,
    },
    {
      slug: 'quickstart',
      titleKey: 'docs.pages.quickstart.title',
      summaryKey: 'docs.pages.quickstart.summary',
      load: loaders,
    },
  ]
  return {
    DOCS_PAGES: pages,
    DEFAULT_DOCS_SLUG: 'overview',
    findDocsPage: (slug: string) => pages.find((page) => page.slug === slug) ?? null,
  }
})

const LONG_PAGE = [
  '# Overview',
  '## Base URL',
  'Send requests to {{SITE_ORIGIN}}/v1.',
  '### Prefixes',
  'More body copy.',
  '## Keys',
  'Body copy.',
  '## Errors',
  'See [Errors](/docs/errors) and [Prefixes](#docs-section-2).',
  '## Next',
  'Body copy.',
].join('\n\n')

function mountView() {
  return mount(DocsView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        Button: { template: '<button><slot /></button>' },
      },
    },
  })
}

describe('DocsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeParams.slug = ''
    appStore.cachedPublicSettings = { site_name: 'Gateway', site_logo: '' }
    appStore.fetchPublicSettings.mockResolvedValue(appStore.cachedPublicSettings)
    loaders.en.mockResolvedValue({ default: LONG_PAGE })
    loaders.zh.mockResolvedValue({ default: '# 概览' })
  })

  it('renders every page in the sidebar', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="docs-nav-overview"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="docs-nav-quickstart"]').exists()).toBe(true)
  })

  it('falls back to the first page when no slug is in the URL', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="docs-nav-overview"]').attributes('aria-current')).toBe('page')
    expect(wrapper.get('[data-testid="docs-document"]').text()).toContain(
      'docs.pages.overview.title'
    )
  })

  it('substitutes the deployment origin into the rendered markdown', async () => {
    const wrapper = mountView()
    await flushPromises()

    const html = wrapper.get('[data-testid="docs-prose"]').html()
    expect(html).not.toContain('{{SITE_ORIGIN}}')
    expect(html).toContain(`${window.location.origin}/v1`)
  })

  it('builds a table of contents once the page has enough sections', async () => {
    const wrapper = mountView()
    await flushPromises()

    const toc = wrapper.get('[data-testid="docs-toc"]')
    expect(toc.findAll('a').length).toBe(5)
    for (const href of toc.findAll('a').map((link) => link.attributes('href'))) {
      expect(wrapper.find(`[data-testid="docs-prose"] ${href}`).exists()).toBe(true)
    }
  })

  it('omits the table of contents on a short page', async () => {
    loaders.en.mockResolvedValue({ default: '# Overview\n\n## Only section\n\nBody copy.' })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="docs-prose"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="docs-toc"]').exists()).toBe(false)
  })

  it('sanitizes the markdown it renders', async () => {
    loaders.en.mockResolvedValue({
      default: '# Overview\n\n<script>alert(1)</script>\n\n<img src=x onerror="alert(1)">',
    })
    const wrapper = mountView()
    await flushPromises()

    const html = wrapper.get('[data-testid="docs-prose"]').html()
    expect(html).not.toContain('<script')
    expect(html).not.toContain('onerror')
  })

  it('routes an in-document /docs link instead of reloading', async () => {
    const wrapper = mountView()
    await flushPromises()

    const internal = wrapper
      .findAll('[data-testid="docs-prose"] a')
      .find((link) => link.attributes('href') === '/docs/errors')
    expect(internal).toBeDefined()

    await internal!.trigger('click')
    expect(push).toHaveBeenCalledWith('/docs/errors')
  })

  it('leaves links it does not own to the browser', async () => {
    const wrapper = mountView()
    await flushPromises()

    // A same-page anchor, so jsdom performs the default action rather than
    // logging a navigation it cannot carry out.
    const anchor = wrapper
      .findAll('[data-testid="docs-prose"] a')
      .find((link) => link.attributes('href') === '#docs-section-2')
    expect(anchor).toBeDefined()

    await anchor!.trigger('click')
    expect(push).not.toHaveBeenCalled()
  })

  it('reports an unknown slug without an error state', async () => {
    routeParams.slug = 'no-such-page'
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="docs-not-found"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="docs-load-error"]').exists()).toBe(false)
    expect(loaders.en).not.toHaveBeenCalled()
  })

  it('reports a failed content load', async () => {
    loaders.en.mockRejectedValue(new Error('offline'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="docs-load-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="docs-prose"]').exists()).toBe(false)
  })

  it('links the following page at the end of the article', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="docs-next"]').text()).toContain(
      'docs.pages.quickstart.title'
    )
  })

  it('offers no next link on the last page', async () => {
    routeParams.slug = 'quickstart'
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="docs-next"]').exists()).toBe(false)
  })
})

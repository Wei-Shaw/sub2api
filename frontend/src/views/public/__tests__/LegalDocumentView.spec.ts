import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import LegalDocumentView from '../LegalDocumentView.vue'

const { appStore, routeParams } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: null as Record<string, unknown> | null,
    fetchPublicSettings: vi.fn(),
  },
  routeParams: { documentId: '' as string },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRoute: () => ({ params: routeParams }),
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
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const LONG_DOCUMENT = [
  '# Terms of Service',
  '## Scope',
  'Body copy.',
  '### Detail',
  'More body copy.',
  '## Responsibilities',
  'Body copy.',
  '## Data',
  'Body copy.',
  '## Contact',
  'Reach us at [support](https://example.com/support).',
].join('\n\n')

const SHORT_DOCUMENT = ['## One', 'Body.', '## Two', 'Body.'].join('\n\n')

function mountLegal(settings: Record<string, unknown> | null, documentId: string) {
  appStore.cachedPublicSettings = settings
  routeParams.documentId = documentId

  return mount(LegalDocumentView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
      },
    },
  })
}

function settingsWith(content: string, extra: Record<string, unknown> = {}) {
  return {
    site_name: 'Test site',
    login_agreement_updated_at: '2026-01-31',
    login_agreement_documents: [{ id: 'terms', title: 'Terms of Service', content_md: content }],
    ...extra,
  }
}

describe('LegalDocumentView', () => {
  beforeEach(() => {
    appStore.fetchPublicSettings.mockReset()
    appStore.fetchPublicSettings.mockResolvedValue({})
  })

  it('renders the document header and its updated-at stamp', async () => {
    const wrapper = mountLegal(settingsWith(LONG_DOCUMENT), 'terms')
    await flushPromises()

    expect(wrapper.get('[data-testid="legal-document"]').text()).toContain('Terms of Service')
    expect(wrapper.find('[data-testid="legal-updated-at"]').exists()).toBe(true)
  })

  it('gives every section an id and links the table of contents at them', async () => {
    const wrapper = mountLegal(settingsWith(LONG_DOCUMENT), 'terms')
    await flushPromises()

    const toc = wrapper.get('[data-testid="legal-toc"]')
    const hrefs = toc.findAll('a').map((link) => link.attributes('href'))

    expect(hrefs).toEqual([
      '#legal-section-1',
      '#legal-section-2',
      '#legal-section-3',
      '#legal-section-4',
      '#legal-section-5',
    ])
    // Every anchor has to resolve inside the rendered prose, or the contents
    // list is a row of dead links.
    for (const href of hrefs) {
      expect(wrapper.find(`[data-testid="legal-prose"] ${href}`).exists()).toBe(true)
    }
  })

  it('withholds the table of contents from a short document', async () => {
    const wrapper = mountLegal(settingsWith(SHORT_DOCUMENT), 'terms')
    await flushPromises()

    expect(wrapper.find('[data-testid="legal-prose"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="legal-toc"]').exists()).toBe(false)
  })

  it('sanitizes the markdown it renders', async () => {
    const wrapper = mountLegal(
      settingsWith('Hello <script>window.pwned = 1</script><img src=x onerror="window.pwned=1">'),
      'terms'
    )
    await flushPromises()

    const html = wrapper.get('[data-testid="legal-prose"]').html()
    expect(html).not.toContain('<script')
    expect(html).not.toContain('onerror')
  })

  it('shows the empty state for a document with no body', async () => {
    const wrapper = mountLegal(settingsWith('   '), 'terms')
    await flushPromises()

    expect(wrapper.find('[data-testid="legal-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="legal-prose"]').exists()).toBe(false)
  })

  it('shows the missing state for an unknown document id', async () => {
    const wrapper = mountLegal(settingsWith(LONG_DOCUMENT), 'nope')
    await flushPromises()

    expect(wrapper.find('[data-testid="legal-missing"]').exists()).toBe(true)
  })

  it('reports a failed settings load instead of an empty document', async () => {
    appStore.fetchPublicSettings.mockResolvedValue(null)
    const wrapper = mountLegal(null, 'terms')

    expect(wrapper.find('[data-testid="legal-loading"]').exists()).toBe(true)

    await flushPromises()

    expect(wrapper.find('[data-testid="legal-load-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="legal-missing"]').exists()).toBe(false)
  })

  it('serves the bundled compliance document without any public settings entry', async () => {
    const wrapper = mountLegal(settingsWith(LONG_DOCUMENT), 'admin-compliance')
    await flushPromises()

    // The `?raw` build-time import is the source here, not the settings API.
    expect(wrapper.get('[data-testid="legal-prose"]').text()).toContain('Sub2API')
    expect(wrapper.find('[data-testid="legal-updated-at"]').exists()).toBe(false)
  })
})

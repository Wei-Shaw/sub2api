import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import TutorialDocumentView from '../TutorialDocumentView.vue'

const { getPublicSettings, updateRouteSEO } = vi.hoisted(() => ({
  getPublicSettings: vi.fn(),
  updateRouteSEO: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings,
}))

vi.mock('@/utils/seo', () => ({
  updateRouteSEO,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('public TutorialDocumentView', () => {
  beforeEach(() => {
    getPublicSettings.mockReset()
    updateRouteSEO.mockReset()

    getPublicSettings.mockResolvedValue({
      site_name: 'MyCustomSite',
      site_logo: '/logo.png',
    })

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        code: 0,
        data: {
          content_html: '<h2>教程标题</h2><p>富文本正文</p>',
        },
      }),
    }))
  })

  it('renders tutorial html from tutorial-document api', async () => {
    const wrapper = mount(TutorialDocumentView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="to"><slot /></a>',
          },
        },
      },
    })

    await flushPromises()

    expect(getPublicSettings).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledWith('/api/v1/tutorial-document', {
      headers: { Accept: 'application/json' },
    })
    expect(wrapper.html()).toContain('MyCustomSite')
    expect(wrapper.html()).toContain('<h2>教程标题</h2>')
    expect(wrapper.html()).toContain('<p>富文本正文</p>')
    expect(updateRouteSEO).toHaveBeenCalled()
  })
})

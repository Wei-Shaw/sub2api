import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TutorialView from '@/views/user/TutorialView.vue'

const { routeQuery, routerReplaceMock } = vi.hoisted(() => ({
  routeQuery: {} as Record<string, string>,
  routerReplaceMock: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ replace: routerReplaceMock })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    apiBaseUrl: 'https://api.example.com'
  })
}))

vi.mock('vue-i18n', async () => {
  const { ref } = await import('vue')
  return {
    useI18n: () => ({
      t: (key: string) => key,
    tm: (key: string) => {
      if (key === 'tutorial.guide.steps') {
        return [{ title: 'Create key', desc: 'Create one on the API keys page' }]
      }
      if (key === 'tutorial.guide.clients') {
        return [{ key: 'openai', name: 'OpenAI clients', desc: 'desc', code: 'Base URL: $BASE/v1' }]
      }
      if (key === 'tutorial.faq.items') {
        return [
          {
            id: 'billing-how',
            category: 'billing',
            question: 'How does billing work?',
            summary: 'Pay as you go',
            answer: 'Charged per request.',
            points: ['Point one'],
            note: 'TLDR billing'
          },
          {
            id: 'refund',
            category: 'support',
            question: 'Can I get a refund?',
            summary: 'Balance yes, plans no',
            answer: 'Contact support.',
            points: [],
            note: 'TLDR refund'
          }
        ]
      }
      return []
    },
      locale: ref('en')
    })
  }
})

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' }
}))

describe('TutorialView', () => {
  beforeEach(() => {
    routerReplaceMock.mockReset()
    Object.keys(routeQuery).forEach((key) => delete routeQuery[key])
  })

  it('shows the guide tab by default', () => {
    const wrapper = mount(TutorialView)
    expect(wrapper.text()).toContain('Create key')
    expect(wrapper.find('pre').text()).toContain('https://api.example.com/v1')
  })

  it('starts on the faq tab when route query has tab=faq', async () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    expect(wrapper.text()).toContain('How does billing work?')
    expect(wrapper.text()).toContain('Can I get a refund?')
  })

  it('syncs ?tab=faq to the URL when switching tabs', async () => {
    const wrapper = mount(TutorialView)
    const faqButton = wrapper.findAll('button').find((b) => b.text() === 'tutorial.tabs.faq')
    expect(faqButton).toBeTruthy()
    await faqButton!.trigger('click')
    expect(routerReplaceMock).toHaveBeenCalledWith({ query: { tab: 'faq' } })
    expect(wrapper.text()).toContain('How does billing work?')
  })

  it('expands an item to reveal the answer and TLDR note', async () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    const item = wrapper.findAll('button').find((b) => b.text().includes('How does billing work?'))
    await item!.trigger('click')
    expect(wrapper.text()).toContain('Charged per request.')
    expect(wrapper.text()).toContain('Point one')
    expect(wrapper.text()).toContain('TLDR billing')
  })

  it('filters FAQ items by search keyword', async () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    const input = wrapper.find('input')
    await input.setValue('refund')
    expect(wrapper.text()).toContain('Can I get a refund?')
    expect(wrapper.text()).not.toContain('How does billing work?')
  })

  it('filters FAQ items by category chip', async () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    const chip = wrapper.findAll('button').find((b) => b.text() === 'tutorial.faq.categories.support')
    expect(chip).toBeTruthy()
    await chip!.trigger('click')
    expect(wrapper.text()).toContain('Can I get a refund?')
    expect(wrapper.text()).not.toContain('How does billing work?')
  })
})

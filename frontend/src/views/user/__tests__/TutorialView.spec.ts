import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import TutorialView from '@/views/user/TutorialView.vue'

const { routeQuery, routerReplaceMock } = vi.hoisted(() => ({
  routeQuery: {} as Record<string, string>,
  routerReplaceMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ replace: routerReplaceMock }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    apiBaseUrl: 'https://api.example.com',
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: ref('zh'),
  }),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))

function button(wrapper: ReturnType<typeof mount>, text: string) {
  const btn = wrapper.findAll('button').find((b) => b.text() === text)
  if (!btn) throw new Error(`button not found: ${text}`)
  return btn
}

describe('TutorialView', () => {
  beforeEach(() => {
    routerReplaceMock.mockReset()
    Object.keys(routeQuery).forEach((key) => delete routeQuery[key])
  })

  it('renders model select and default Claude Code guide with substituted base URL', () => {
    const wrapper = mount(TutorialView)
    expect(wrapper.find('select').exists()).toBe(true)
    expect(wrapper.text()).toContain('npm install -g @anthropic-ai/claude-code')
    expect(wrapper.text()).toContain('"ANTHROPIC_BASE_URL": "https://api.example.com"')
    expect(wrapper.text()).toContain('"ANTHROPIC_MODEL": "deepseek-v4-pro"')
  })

  it('updates config file content when model changes', async () => {
    const wrapper = mount(TutorialView)
    await wrapper.find('select').setValue('glm-5.3')
    expect(wrapper.text()).toContain('"ANTHROPIC_MODEL": "glm-5.3"')
    expect(wrapper.text()).toContain('"ANTHROPIC_DEFAULT_HAIKU_MODEL": "glm-5.3"')
  })

  it('switches client guide via chips', async () => {
    const wrapper = mount(TutorialView)
    await button(wrapper, 'Codex CLI').trigger('click')
    expect(wrapper.text()).toContain('npm install -g @openai/codex')
    expect(wrapper.text()).toContain('wire_api = "responses"')
    await button(wrapper, 'Cursor').trigger('click')
    expect(wrapper.text()).toContain('Verify')
  })

  it('shows API code samples with selected model', async () => {
    const wrapper = mount(TutorialView)
    await button(wrapper, '代码调用').trigger('click')
    expect(wrapper.text()).toContain('curl https://api.example.com/v1/chat/completions')
    expect(wrapper.text()).toContain('"model": "deepseek-v4-pro"')
  })

  it('starts on the guide tab by default and syncs faq tab to the URL', async () => {
    const wrapper = mount(TutorialView)
    expect(wrapper.find('select').exists()).toBe(true)
    await button(wrapper, 'tutorial.tabs.faq').trigger('click')
    expect(routerReplaceMock).toHaveBeenCalledWith({ query: { tab: 'faq' } })
    expect(wrapper.text()).toContain('为什么同一个模型价格不一样?')
  })

  it('starts on the faq tab when route query has tab=faq', () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    expect(wrapper.text()).toContain('计费是怎么算的?什么时候扣余额?')
  })

  it('filters FAQ items by search keyword', async () => {
    routeQuery.tab = 'faq'
    const wrapper = mount(TutorialView)
    await wrapper.find('input').setValue('兑换码')
    expect(wrapper.text()).toContain('兑换码在哪里使用')
    expect(wrapper.text()).not.toContain('模型是原版吗')
  })
})

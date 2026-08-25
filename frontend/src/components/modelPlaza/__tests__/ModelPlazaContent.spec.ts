import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import type { ModelPlazaResponse } from '@/api/modelPlaza'

const routerState = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  replace: vi.fn(),
  setQuery: (_query: Record<string, string>) => {}
}))

vi.mock('vue-router', async () => {
  const { reactive } = await import('vue')
  const route = reactive(routerState.route)
  routerState.setQuery = (query) => {
    route.query = query
  }
  routerState.replace.mockImplementation(async ({ query }: { query: Record<string, string> }) => {
    route.query = query
  })
  return {
    useRoute: () => route,
    useRouter: () => ({ replace: routerState.replace })
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: true })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const response: ModelPlazaResponse = {
  description: '',
  groups: [
    {
      id: 1,
      name: 'Composite',
      description: '',
      platform: 'composite',
      subscription_type: 'standard',
      rate_multiplier: 1,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      is_exclusive: false,
      image_rate_independent: false,
      image_rate_multiplier: 1,
      models: [
        { name: 'shared-model', platform: 'anthropic', pricing: null, official_pricing: null },
        { name: 'shared-model', platform: 'openai', pricing: null, official_pricing: null }
      ]
    }
  ]
}

function mountContent() {
  return mount(ModelPlazaContent, {
    props: { response, loading: false },
    global: {
      stubs: {
        PlazaFilterBar,
        PlazaGroupSection: {
          props: ['group'],
          template: '<div data-testid="pricing-group">{{ group.models.map((model) => `${model.platform}:${model.name}`).join(",") }}</div>'
        },
        Icon: true
      }
    }
  })
}

describe('ModelPlazaContent', () => {
  it('exposes concrete model platforms from composite offers and filters by occurrence platform', async () => {
    routerState.route.query = {}
    const wrapper = mountContent()
    const filter = wrapper.findComponent(PlazaFilterBar)

    expect(filter.props('platforms')).toEqual(['anthropic', 'openai'])

    filter.vm.$emit('update:platform', 'openai')
    await nextTick()

    expect(wrapper.findAll('[data-testid="pricing-group"]')).toHaveLength(0)
    wrapper.vm.view = 'pricing'
    await nextTick()
    expect(wrapper.findAll('[data-testid="pricing-group"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="pricing-group"]').text()).toBe('openai:shared-model')
  })

  it('does not render a per-user model policy control for authenticated users', () => {
    routerState.route.query = {}
    const wrapper = mountContent()

    expect(wrapper.findAll('button').some((button) => button.text() === 'modelPlaza.blocked.disable')).toBe(false)
    expect(wrapper.findAll('button').some((button) => button.text() === 'modelPlaza.blocked.enable')).toBe(false)
  })

  it('hydrates controls when route query changes after mount', async () => {
    routerState.route.query = {}
    const wrapper = mountContent()
    expect(wrapper.findComponent(PlazaFilterBar).props('platform')).toBe('all')

    routerState.setQuery({ platform: 'openai', sort: 'name', view: 'pricing' })
    await nextTick()
    await nextTick()

    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('platform')).toBe('openai')
    expect(filter.props('sort')).toBe('name')
    expect(wrapper.vm.view).toBe('pricing')
  })
})

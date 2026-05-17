import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppLayout from '../AppLayout.vue'

const routeState = vi.hoisted(() => ({
  meta: {} as Record<string, unknown>,
}))

const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  user: { role: 'user' },
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ setReplayCallback: vi.fn() }),
}))

vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: () => ({ replayTour: vi.fn() }),
}))

vi.mock('../AppSidebar.vue', () => ({
  default: { template: '<aside class="app-sidebar-stub" />' },
}))

vi.mock('../AppHeader.vue', () => ({
  default: { template: '<header class="app-header-stub" />' },
}))

describe('AppLayout standalone pages', () => {
  beforeEach(() => {
    routeState.meta = {}
    authState.isAuthenticated = true
    authState.user = { role: 'user' }
  })

  it('hides the sidebar for standalone pages even when authenticated', () => {
    routeState.meta = { requiresAuth: false, standalone: true }

    const wrapper = mount(AppLayout, {
      slots: {
        default: '<div>Standalone</div>',
      },
    })

    expect(wrapper.find('.app-sidebar-stub').exists()).toBe(false)
    expect(wrapper.find('.app-header-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('Standalone')
  })
})

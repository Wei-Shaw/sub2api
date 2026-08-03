import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailOAuthButtons from '@/components/auth/EmailOAuthButtons.vue'

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    // return the key so the badge renders a stable, assertable string
    t: (key: string) => key,
  }),
}))

const BADGE_TEXT = 'auth.lastTimeUsed'

function mountButtons(lastUsedProvider: string | null) {
  return mount(EmailOAuthButtons, {
    props: {
      githubEnabled: true,
      googleEnabled: true,
      lastUsedProvider,
    },
    global: {
      stubs: { GitHubMark: true, GoogleMark: true },
    },
  })
}

describe('LastUsedBadge rendering on OAuth buttons', () => {
  beforeEach(() => {
    routeState.query = {}
    window.localStorage.clear()
    window.sessionStorage.clear()
  })

  it('renders the badge only on the last-used provider button', () => {
    const wrapper = mountButtons('google')
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)

    const githubBtn = buttons.find((b) => b.text().includes('GitHub'))!
    const googleBtn = buttons.find((b) => b.text().includes('Google'))!

    expect(googleBtn.text()).toContain(BADGE_TEXT)
    expect(githubBtn.text()).not.toContain(BADGE_TEXT)
    // exactly one badge on the page
    expect(wrapper.html().split(BADGE_TEXT).length - 1).toBe(1)
  })

  it('renders no badge when there is no last-used provider', () => {
    const wrapper = mountButtons(null)
    expect(wrapper.text()).not.toContain(BADGE_TEXT)
  })

  it('renders no badge when the last-used provider is not shown here', () => {
    // e.g. last login was linuxdo, which is not one of these email buttons
    const wrapper = mountButtons('linuxdo')
    expect(wrapper.text()).not.toContain(BADGE_TEXT)
  })
})

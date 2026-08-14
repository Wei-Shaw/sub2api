import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import NotFoundView from '../NotFoundView.vue'

const { router } = vi.hoisted(() => ({
  router: { back: vi.fn() },
}))

vi.mock('vue-router', () => ({
  useRouter: () => router,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountNotFound() {
  return mount(NotFoundView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

describe('NotFoundView', () => {
  beforeEach(() => {
    router.back.mockClear()
  })

  it('renders the 404 surface', () => {
    // Anchored on a testid, not on the old `.text-[12rem]` numeral or the
    // gradient tile — both were decoration and both are gone.
    expect(mountNotFound().find('[data-testid="not-found"]').exists()).toBe(true)
  })

  it('sends the back control through the router rather than history directly', async () => {
    await mountNotFound().get('[data-testid="not-found-back"]').trigger('click')

    expect(router.back).toHaveBeenCalledOnce()
  })

  it('keeps the dashboard as the forward destination', () => {
    const link = mountNotFound()
      .get('[data-testid="not-found-dashboard"]')
      .findComponent(RouterLinkStub)

    expect(link.props('to')).toBe('/dashboard')
  })

  it('translates every label it renders', () => {
    const text = mountNotFound().text()

    expect(text).toContain('errors.pageNotFound')
    expect(text).toContain('common.back')
    expect(text).toContain('home.goToDashboard')
  })
})

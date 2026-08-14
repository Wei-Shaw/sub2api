/**
 * Regression guard: both group modals are mounted unconditionally by GroupsView
 * (not behind v-if), so every visit to that view mounts them again. Their
 * document-level click-outside listener must therefore be torn down on unmount,
 * otherwise each visit leaks a listener that keeps firing on a destroyed
 * component.
 */
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      getGroupRateMultipliers: vi.fn().mockResolvedValue([]),
      getGroupRPMOverrides: vi.fn().mockResolvedValue([]),
      batchSetGroupRateMultipliers: vi.fn().mockResolvedValue(undefined),
      batchSetGroupRPMOverrides: vi.fn().mockResolvedValue(undefined),
    },
    users: { getUsers: vi.fn().mockResolvedValue({ users: [], total: 0 }) },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'

const stubs = {
  BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
  Pagination: true,
  Icon: true,
  PlatformIcon: true,
  Teleport: true,
}

/** Click handlers registered on `document` while `run` executes. */
function trackDocumentClickHandlers() {
  const added: EventListenerOrEventListenerObject[] = []
  const removed: EventListenerOrEventListenerObject[] = []

  const addSpy = vi.spyOn(document, 'addEventListener')
  const removeSpy = vi.spyOn(document, 'removeEventListener')

  addSpy.mockImplementation(((type: string, handler: EventListenerOrEventListenerObject) => {
    if (type === 'click') added.push(handler)
  }) as typeof document.addEventListener)
  removeSpy.mockImplementation(((type: string, handler: EventListenerOrEventListenerObject) => {
    if (type === 'click') removed.push(handler)
  }) as typeof document.removeEventListener)

  return { added, removed }
}

describe.each([
  ['GroupRateMultipliersModal', GroupRateMultipliersModal],
  ['GroupRPMOverridesModal', GroupRPMOverridesModal],
])('%s document click listener lifecycle', (_name, Component) => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('registers its click-outside listener only after mount', () => {
    const tracker = trackDocumentClickHandlers()

    // Importing/evaluating setup for a component that is not mounted must not
    // touch `document` — registration belongs in onMounted.
    expect(tracker.added).toHaveLength(0)

    const wrapper = mount(Component, {
      props: { show: false, group: null },
      global: { stubs },
    })

    expect(tracker.added.length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('removes every document click listener it added when unmounted', () => {
    const tracker = trackDocumentClickHandlers()

    const wrapper = mount(Component, {
      props: { show: false, group: null },
      global: { stubs },
    })
    expect(tracker.added.length).toBeGreaterThan(0)

    wrapper.unmount()

    for (const handler of tracker.added) {
      expect(tracker.removed).toContain(handler)
    }
  })

  it('does not accumulate listeners across repeated mount/unmount cycles', () => {
    const tracker = trackDocumentClickHandlers()

    for (let i = 0; i < 3; i += 1) {
      const wrapper = mount(Component, {
        props: { show: false, group: null },
        global: { stubs },
      })
      wrapper.unmount()
    }

    // Every registration has a matching removal, so nothing is left behind.
    expect(tracker.removed).toHaveLength(tracker.added.length)
  })
})

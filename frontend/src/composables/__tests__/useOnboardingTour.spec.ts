import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useAdminComplianceStore } from '@/stores/adminCompliance'
import { useAuthStore } from '@/stores/auth'

const driveMock = vi.fn()
const destroyMock = vi.fn()

vi.mock('driver.js', () => ({
  driver: vi.fn(() => ({
    drive: driveMock,
    destroy: destroyMock,
    isActive: vi.fn(() => false),
    getActiveIndex: vi.fn(() => 0),
    getActiveElement: vi.fn(() => null),
    moveNext: vi.fn(),
    movePrevious: vi.fn(),
  })),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/components/Guide/steps', () => ({
  getAdminSteps: () => [
    {
      popover: {
        title: 'welcome',
        description: 'welcome',
      },
    },
  ],
  getUserSteps: () => [
    {
      popover: {
        title: 'welcome',
        description: 'welcome',
      },
    },
  ],
}))

const adminUser = {
  id: 2,
  username: 'admin',
  email: 'admin@example.com',
  role: 'admin' as const,
  balance: 100,
  concurrency: 5,
  status: 'active' as const,
  allowed_groups: null,
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
}

function mountTour() {
  return mount(defineComponent({
    setup() {
      useOnboardingTour({
        storageKey: 'admin_guide',
        autoStart: true,
      })

      return () => null
    },
  }))
}

function setAdminSession(): void {
  const authStore = useAuthStore()
  authStore.$patch({
    user: adminUser,
    token: 'admin-token',
  })
}

describe('useOnboardingTour', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.useFakeTimers()
    driveMock.mockClear()
    destroyMock.mockClear()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('does not auto-start for admins before compliance status is initialized', async () => {
    setAdminSession()
    const complianceStore = useAdminComplianceStore()
    complianceStore.initialized = false

    const wrapper = mountTour()
    await nextTick()
    vi.advanceTimersByTime(1000)
    await nextTick()
    await nextTick()

    expect(driveMock).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('auto-starts after the admin compliance acknowledgement is no longer required', async () => {
    setAdminSession()
    const complianceStore = useAdminComplianceStore()
    complianceStore.status = {
      required: true,
      version: 'v2026.06.10',
      document_path_zh: 'docs/legal/admin-compliance.zh.md',
      document_path_en: 'docs/legal/admin-compliance.en.md',
      document_url_zh: 'https://example.com/zh',
      document_url_en: 'https://example.com/en',
      ack_phrase_zh: '确认',
      ack_phrase_en: 'Confirm',
    }
    complianceStore.initialized = true

    const wrapper = mountTour()
    await nextTick()
    vi.advanceTimersByTime(1000)
    await nextTick()
    await nextTick()
    expect(driveMock).not.toHaveBeenCalled()

    complianceStore.status = {
      ...complianceStore.status!,
      required: false,
    }
    await nextTick()
    vi.advanceTimersByTime(1000)
    await nextTick()
    await nextTick()

    expect(driveMock).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })
})

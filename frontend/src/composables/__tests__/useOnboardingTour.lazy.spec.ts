import { describe, it, expect, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const s = vi.hoisted(() => ({
  loads: { driver: 0, steps: 0 },
  drive: vi.fn(),
  driver: vi.fn(),
}))

vi.mock('driver.js', () => {
  s.loads.driver++
  s.driver.mockImplementation(() => ({ drive: s.drive, destroy: vi.fn(), isActive: () => true }))
  return { driver: s.driver }
})
vi.mock('@/components/Guide/steps', () => {
  s.loads.steps++
  return { getAdminSteps: () => [], getUserSteps: () => [{ popover: { title: 'step' } }] }
})
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1, role: 'user' }, isSimpleMode: false }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Sub2API' }) }))
vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    getDriverInstance: () => null,
    setDriverInstance: vi.fn(),
    setControlMethods: vi.fn(),
    clearControlMethods: vi.fn(),
    isDriverActive: () => false,
  }),
}))

import { useOnboardingTour } from '../useOnboardingTour'

describe('useOnboardingTour', () => {
  it('loads driver.js and the tour steps only when a tour starts', async () => {
    let tour!: ReturnType<typeof useOnboardingTour>
    mount(defineComponent({
      setup() {
        tour = useOnboardingTour({ autoStart: false })
        return () => null
      },
    }))

    expect(s.loads).toEqual({ driver: 0, steps: 0 })

    await tour.startTour()

    expect(s.loads).toEqual({ driver: 1, steps: 1 })
    expect(s.driver).toHaveBeenCalledWith(expect.objectContaining({ steps: [{ popover: { title: 'step' } }] }))
    expect(s.drive).toHaveBeenCalledWith(0)
  })
})

import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import SetupWizardView from '../SetupWizardView.vue'

/**
 * Anchors are `data-testid`, never style classes: this view was rebuilt on the
 * design system and every cosmetic class in it changed. What must not change is
 * the step order, the gating, and the payload.
 */
const { testDatabase, testRedis, install } = vi.hoisted(() => ({
  testDatabase: vi.fn(),
  testRedis: vi.fn(),
  install: vi.fn(),
}))

vi.mock('@/api/setup', () => ({ testDatabase, testRedis, install }))
vi.mock('@/api/client', () => ({ buildGatewayUrl: (path: string) => path }))

// Partial factory — `vue-i18n`'s real exports stay available, so nothing in the
// component graph that calls `createI18n` breaks.
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
  }
})

function mountWizard() {
  return mount(SetupWizardView, {
    global: {
      stubs: {
        transition: true,
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

type Wizard = ReturnType<typeof mountWizard>

async function passStep(wrapper: Wizard, testid: string) {
  await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
  await flushPromises()
  await wrapper.get('[data-testid="setup-next"]').trigger('click')
  await flushPromises()
}

describe('SetupWizardView', () => {
  it('marks the current step and keeps the rest as plain numbers', () => {
    const wrapper = mountWizard()
    const items = wrapper.get('[data-testid="setup-steps"]').findAll('li')

    expect(items).toHaveLength(4)
    expect(items[0].attributes('aria-current')).toBe('step')
    expect(items[1].attributes('aria-current')).toBeUndefined()
    // Zero-padded mono numbering, not a bubble.
    expect(items[1].text()).toContain('02')
  })

  it('gates step 1 on a successful database test without relabelling the button', async () => {
    let resolveTest: () => void = () => {}
    testDatabase.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveTest = resolve
        })
    )

    const wrapper = mountWizard()
    const next = wrapper.get('[data-testid="setup-next"]')
    expect(next.attributes('disabled')).toBeDefined()

    const test = wrapper.get('[data-testid="setup-test-database"]')
    await test.trigger('click')

    expect(test.attributes('aria-busy')).toBe('true')
    // The label must not change while the request is in flight.
    expect(test.text()).toBe('setup.status.testConnection')

    resolveTest()
    await flushPromises()

    expect(wrapper.find('[data-testid="setup-database-ok"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="setup-next"]').attributes('disabled')).toBeUndefined()
  })

  it('surfaces the password rules next to the field instead of only disabling next', async () => {
    testDatabase.mockResolvedValue(undefined)
    testRedis.mockResolvedValue(undefined)

    const wrapper = mountWizard()
    await passStep(wrapper, 'setup-test-database')
    await passStep(wrapper, 'setup-test-redis')

    const admin = wrapper.get('[data-testid="setup-step-admin"]')
    const [email, password, confirm] = admin.findAll('input')

    await email.setValue('admin@example.com')
    await password.setValue('short')
    expect(admin.text()).toContain('profile.passwordTooShort')
    expect(password.attributes('aria-invalid')).toBe('true')

    await password.setValue('longenough')
    await confirm.setValue('longenoug')
    expect(admin.text()).toContain('setup.admin.passwordMismatch')
    expect(wrapper.get('[data-testid="setup-next"]').attributes('disabled')).toBeDefined()

    await confirm.setValue('longenough')
    expect(admin.text()).not.toContain('setup.admin.passwordMismatch')
    expect(wrapper.get('[data-testid="setup-next"]').attributes('disabled')).toBeUndefined()
  })

  it('reviews the collected configuration and installs it unchanged', async () => {
    testDatabase.mockResolvedValue(undefined)
    testRedis.mockResolvedValue(undefined)
    install.mockResolvedValue(undefined)

    const wrapper = mountWizard()
    await passStep(wrapper, 'setup-test-database')
    await passStep(wrapper, 'setup-test-redis')

    const [email, password, confirm] = wrapper.get('[data-testid="setup-step-admin"]').findAll('input')
    await email.setValue('admin@example.com')
    await password.setValue('longenough')
    await confirm.setValue('longenough')
    await wrapper.get('[data-testid="setup-next"]').trigger('click')
    await flushPromises()

    const review = wrapper.get('[data-testid="setup-step-ready"]')
    expect(review.text()).toContain('postgres@localhost:5432/sub2api')
    expect(review.text()).toContain('localhost:6379')
    expect(review.text()).toContain('admin@example.com')

    await wrapper.get('[data-testid="setup-install"]').trigger('click')
    await flushPromises()

    expect(install).toHaveBeenCalledWith(
      expect.objectContaining({
        database: expect.objectContaining({ host: 'localhost', port: 5432, dbname: 'sub2api' }),
        redis: expect.objectContaining({ host: 'localhost', port: 6379, enable_tls: false }),
        admin: { email: 'admin@example.com', password: 'longenough' },
        server: expect.objectContaining({ host: '0.0.0.0', mode: 'release' }),
      })
    )
    expect(wrapper.find('[data-testid="setup-success"]').exists()).toBe(true)
  })

  it('renders a connection failure as an alert', async () => {
    testDatabase.mockRejectedValueOnce({ response: { data: { detail: 'boom' } } })

    const wrapper = mountWizard()
    await wrapper.get('[data-testid="setup-test-database"]').trigger('click')
    await flushPromises()

    const alert = wrapper.get('[data-testid="setup-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toContain('boom')
    expect(wrapper.find('[data-testid="setup-database-ok"]').exists()).toBe(false)
  })
})

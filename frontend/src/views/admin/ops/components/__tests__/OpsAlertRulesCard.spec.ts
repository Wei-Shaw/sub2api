import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsAlertRulesCard from '../OpsAlertRulesCard.vue'

const mocks = vi.hoisted(() => ({
  listAlertRules: vi.fn(),
  getEmailNotificationConfig: vi.fn(),
  getAll: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: mocks.getAll } } }))
vi.mock('@/api/admin/ops', () => ({
  opsAPI: { listAlertRules: mocks.listAlertRules, getEmailNotificationConfig: mocks.getEmailNotificationConfig }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

const emailConfig = (enabled: boolean, recipients: string[]) => ({ alert: { enabled, recipients }, report: {} })

async function openCreateEditor() {
  const wrapper = mount(OpsAlertRulesCard, {
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        ConfirmDialog: true,
        DataTable: true,
        Select: true
      }
    }
  })
  await flushPromises()
  ;(wrapper.vm as any).openCreate()
  await flushPromises()
  return wrapper
}

describe('OpsAlertRulesCard email notification hint', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listAlertRules.mockResolvedValue([])
    mocks.getAll.mockResolvedValue([])
  })

  it.each([
    ['alert emails are disabled', emailConfig(false, ['ops@example.com'])],
    ['there are no recipients (the default config)', emailConfig(true, [])]
  ])('warns next to "send email" when %s', async (_, cfg) => {
    mocks.getEmailNotificationConfig.mockResolvedValue(cfg)
    const wrapper = await openCreateEditor()

    expect(mocks.getEmailNotificationConfig).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="notify-email-inactive"]').text()).toBe('admin.ops.alertRules.form.notifyEmailInactive')

    ;(wrapper.vm as any).draft.notify_email = false
    await flushPromises()
    expect(wrapper.find('[data-test="notify-email-inactive"]').exists()).toBe(false)
  })

  it('shows no warning when alert emails will be delivered or the config cannot be read', async () => {
    mocks.getEmailNotificationConfig.mockResolvedValueOnce(emailConfig(true, ['ops@example.com']))
    const wrapper = await openCreateEditor()
    expect(wrapper.find('[data-test="notify-email-inactive"]').exists()).toBe(false)

    mocks.getEmailNotificationConfig.mockRejectedValueOnce(new Error('forbidden'))
    ;(wrapper.vm as any).openCreate()
    await flushPromises()
    expect(wrapper.find('[data-test="notify-email-inactive"]').exists()).toBe(false)
  })
})

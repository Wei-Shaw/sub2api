import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock, authIsSimpleMode } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  authIsSimpleMode: { value: true }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isSimpleMode() {
      return authIsSimpleMode.value
    }
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn(() => Promise.resolve({ enabled: false, providers: [] })),
      getSettings: vi.fn(() => Promise.resolve({}))
    },
    tlsFingerprintProfiles: {
      list: vi.fn(() => Promise.resolve([]))
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' },
    options: { type: Array, default: () => [] }
  },
  emits: ['update:modelValue'],
  template: '<select v-bind="$attrs" :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)" />'
})

const stubs = {
  BaseDialog: BaseDialogStub,
  Select: SelectStub,
  Icon: defineComponent({ name: 'Icon', template: '<span />' }),
  teleport: true
}

const baseAccount = {
  id: 1,
  name: 'shared-quota-account',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  credentials: {
    temp_unschedulable_enabled: true,
    temp_unschedulable_rules: [
      {
        error_code: 402,
        keywords: ['depleted', 'monthly included credits'],
        duration_minutes: 43200,
        description: 'quota exhausted',
        account_wide: true
      },
      {
        error_code: 429,
        keywords: ['rate limit'],
        duration_minutes: 10
      }
    ]
  }
}

const mountModal = async (account = baseAccount) => {
  const wrapper = mount(EditAccountModal, {
    props: {
      show: true,
      account,
      groups: [],
      proxies: []
    },
    global: {
      stubs,
      directives: { closable: {} }
    }
  })
  await flushPromises()
  return wrapper
}

describe('EditAccountModal temp unschedulable account_wide', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    updateAccountMock.mockResolvedValue(baseAccount)
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads account_wide from saved rules into the form (echo back)', async () => {
    const wrapper = await mountModal()

    const checkboxes = wrapper.findAll('input[type="checkbox"][id^="temp-unsched-account-wide-"]')
    expect(checkboxes.length).toBe(2)
    // first rule has account_wide: true, second defaults to false
    expect((checkboxes[0].element as HTMLInputElement).checked).toBe(true)
    expect((checkboxes[1].element as HTMLInputElement).checked).toBe(false)
  })

  it('preserves account_wide when submitting credentials', async () => {
    const wrapper = await mountModal()

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0][1]
    const rules = payload?.credentials?.temp_unschedulable_rules
    expect(rules).toBeDefined()
    expect(rules.length).toBe(2)
    expect(rules[0].account_wide).toBe(true)
    expect(rules[1].account_wide).toBe(false)
  })

  it('toggling account_wide updates the submitted payload', async () => {
    const wrapper = await mountModal()

    const checkbox = wrapper.find('input[type="checkbox"][id^="temp-unsched-account-wide-"]')
    await checkbox.setValue(false)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    const payload = updateAccountMock.mock.calls[0][1]
    const rules = payload?.credentials?.temp_unschedulable_rules
    expect(rules[0].account_wide).toBe(false)
  })
})

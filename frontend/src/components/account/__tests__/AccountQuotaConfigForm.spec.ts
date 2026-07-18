import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountQuotaConfigForm from '../AccountQuotaConfigForm.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getQuotaProviders: vi.fn().mockRejectedValue(new Error('offline'))
    }
  }
}))

describe('AccountQuotaConfigForm', () => {
  it('uses the standard account input style for NewAPI compatibility fields', () => {
    const wrapper = mount(AccountQuotaConfigForm, {
      props: {
        mode: 'upstream',
        provider: 'newapi',
        config: {}
      },
      global: {
        stubs: {
          Select: true
        }
      }
    })

    const inputs = wrapper.findAll('input[type="number"]')
    expect(inputs).toHaveLength(2)
    for (const input of inputs) {
      expect(input.classes()).toContain('input')
      expect(input.classes()).not.toContain('form-input')
    }
  })
})

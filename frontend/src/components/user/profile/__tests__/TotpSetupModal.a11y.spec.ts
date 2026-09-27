import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import TotpSetupModal from '../TotpSetupModal.vue'

const api = vi.hoisted(() => ({ getVerificationMethod: vi.fn(), initiateSetup: vi.fn(), enable: vi.fn() }))
vi.mock('@/api', () => ({ totpAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn() } }))
enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.resetAllMocks()
  api.getVerificationMethod.mockResolvedValue({ method: 'password' })
  api.initiateSetup.mockResolvedValue({ secret: 'EXAMPLE', qr_code_url: '', setup_token: 'setup-token' })
})

describe('TOTP setup accessibility', () => {
  it('names the icon-only copy-secret button', async () => {
    const wrapper = mount(TotpSetupModal, { attachTo: document.body, global: { stubs: { Teleport: true } } })
    await flushPromises()
    await wrapper.get('input[type="password"]').setValue('password')
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.get('button.btn-icon').attributes('aria-label')).toBe('common.copy')
  })
})

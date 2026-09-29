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
  api.enable.mockResolvedValue({ success: true })
})

async function openCodeStep() {
  const wrapper = mount(TotpSetupModal, { attachTo: document.body })
  await flushPromises()
  await wrapper.get('input[type="password"]').setValue('password')
  await wrapper.get('.btn-primary').trigger('click')
  await flushPromises()
  await wrapper.get('.btn-primary').trigger('click')
  const inputs = wrapper.findAll('input[maxlength="1"]')
  for (let i = 0; i < inputs.length; i++) await inputs[i].setValue(String(i + 1))
  return wrapper
}

describe('TOTP setup input synchronization', () => {
  it('clears displayed digits after verification fails and focuses the first cell', async () => {
    api.enable.mockRejectedValueOnce({ message: 'Invalid code' })
    const wrapper = await openCodeStep()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const inputs = wrapper.findAll('input[maxlength="1"]')
    expect(inputs.map(input => (input.element as HTMLInputElement).value)).toEqual(['', '', '', '', '', ''])
    expect(document.activeElement).toBe(inputs[0].element)
    expect(wrapper.get('[type="submit"]').attributes('disabled')).toBeDefined()
    await inputs[0].trigger('paste', { clipboardData: { getData: () => '654321' } })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenLastCalledWith({ totp_code: '654321', setup_token: 'setup-token' })
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('displays retained digits when returning from the QR-code step', async () => {
    const wrapper = await openCodeStep()
    await wrapper.findAll('button').find(button => button.text() === 'common.back')!.trigger('click')
    await wrapper.get('.btn-primary').trigger('click')
    expect(wrapper.findAll('input[maxlength="1"]').map(input => (input.element as HTMLInputElement).value)).toEqual(['1', '2', '3', '4', '5', '6'])
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenCalledWith({ totp_code: '123456', setup_token: 'setup-token' })
  })

  it('submits normally entered digits', async () => {
    const wrapper = await openCodeStep()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenCalledWith({ totp_code: '123456', setup_token: 'setup-token' })
    expect(wrapper.emitted('success')).toHaveLength(1)
  })
})

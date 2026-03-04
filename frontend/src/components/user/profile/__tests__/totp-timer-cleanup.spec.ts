import { beforeEach, afterEach, describe, expect, it, vi REDACTED from 'vitest'
import { mount REDACTED from '@vue/test-utils'
import TotpSetupModal from '@/components/user/profile/TotpSetupModal.vue'
import TotpDisableDialog from '@/components/user/profile/TotpDisableDialog.vue'

const mocks = vi.hoisted(() => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
  getVerificationMethod: vi.fn(),
  sendVerifyCode: vi.fn()
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  REDACTED)
REDACTED))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mocks.showSuccess,
    showError: mocks.showError
  REDACTED)
REDACTED))

vi.mock('@/api', () => ({
  totpAPI: {
    getVerificationMethod: mocks.getVerificationMethod,
    sendVerifyCode: mocks.sendVerifyCode,
    initiateSetup: vi.fn(),
    enable: vi.fn(),
    disable: vi.fn()
  REDACTED
REDACTED))

const flushPromises = async () => {
  await Promise.resolve()
  await Promise.resolve()
REDACTED

describe('TOTP 弹窗定时器清理', () => {
  let intervalSeed = 1000
  let setIntervalSpy: ReturnType<typeof vi.spyOn>
  let clearIntervalSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    intervalSeed = 1000
    mocks.showSuccess.mockReset()
    mocks.showError.mockReset()
    mocks.getVerificationMethod.mockReset()
    mocks.sendVerifyCode.mockReset()

    mocks.getVerificationMethod.mockResolvedValue({ method: 'email' REDACTED)
    mocks.sendVerifyCode.mockResolvedValue({ success: true REDACTED)

    setIntervalSpy = vi.spyOn(window, 'setInterval').mockImplementation(((handler: TimerHandler) => {
      void handler
      intervalSeed += 1
      return intervalSeed as unknown as number
    REDACTED) as typeof window.setInterval)
    clearIntervalSpy = vi.spyOn(window, 'clearInterval')
  REDACTED)

  afterEach(() => {
    setIntervalSpy.mockRestore()
    clearIntervalSpy.mockRestore()
  REDACTED)

  it('TotpSetupModal 卸载时清理倒计时定时器', async () => {
    const wrapper = mount(TotpSetupModal)
    await flushPromises()

    const sendButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('profile.totp.sendCode'))

    expect(sendButton).toBeTruthy()
    await sendButton!.trigger('click')
    await flushPromises()

    expect(setIntervalSpy).toHaveBeenCalledTimes(1)
    const timerId = setIntervalSpy.mock.results[0]?.value

    wrapper.unmount()

    expect(clearIntervalSpy).toHaveBeenCalledWith(timerId)
  REDACTED)

  it('TotpDisableDialog 卸载时清理倒计时定时器', async () => {
    const wrapper = mount(TotpDisableDialog)
    await flushPromises()

    const sendButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('profile.totp.sendCode'))

    expect(sendButton).toBeTruthy()
    await sendButton!.trigger('click')
    await flushPromises()

    expect(setIntervalSpy).toHaveBeenCalledTimes(1)
    const timerId = setIntervalSpy.mock.results[0]?.value

    wrapper.unmount()

    expect(clearIntervalSpy).toHaveBeenCalledWith(timerId)
  REDACTED)
REDACTED)

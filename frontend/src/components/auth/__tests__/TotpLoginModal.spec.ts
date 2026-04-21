import { mount REDACTED from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'

const { showErrorMock REDACTED = vi.hoisted(() => ({
  showErrorMock: vi.fn(),
REDACTED))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  REDACTED),
REDACTED))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  REDACTED),
REDACTED))

describe('TotpLoginModal', () => {
  beforeEach(() => {
    showErrorMock.mockReset()
  REDACTED)

  it('sends verification errors to toast and does not render inline red text', async () => {
    const wrapper = mount(TotpLoginModal, {
      props: {
        tempToken: 'temp-token',
        userEmailMasked: 'u***@example.com',
      REDACTED,
    REDACTED)

    ;(wrapper.vm as unknown as { setError: (message: string) => void REDACTED).setError('Invalid code')
    await wrapper.vm.$nextTick()

    expect(showErrorMock).toHaveBeenCalledWith('Invalid code')
    expect(wrapper.text()).not.toContain('Invalid code')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  REDACTED)
REDACTED)

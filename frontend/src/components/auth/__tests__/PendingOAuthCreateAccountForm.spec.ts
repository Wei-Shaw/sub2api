import { defineComponent, h REDACTED from 'vue'
import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import PendingOAuthCreateAccountForm from '../PendingOAuthCreateAccountForm.vue'

const sendVerifyCode = vi.fn()
const sendPendingOAuthVerifyCode = vi.fn()
const getPublicSettings = vi.fn()
const showError = vi.fn()
const turnstileReset = vi.fn()
const verifyTencent = vi.fn()

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    sendVerifyCode: (...args: any[]) => sendVerifyCode(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCode(...args),
    getPublicSettings: (...args: any[]) => getPublicSettings(...args)
  REDACTED
REDACTED)

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
  REDACTED)
REDACTED))

describe('PendingOAuthCreateAccountForm', () => {
  beforeEach(() => {
    sendVerifyCode.mockReset()
    sendPendingOAuthVerifyCode.mockReset()
    getPublicSettings.mockReset()
    showError.mockReset()
    turnstileReset.mockReset()
    verifyTencent.mockReset()
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: ''
    REDACTED)
  REDACTED)

  it('acquires separate proofs for pending OAuth send-code and create-account', async () => {
    getPublicSettings.mockResolvedValue({
      email_verify_enabled: true,
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id'
    REDACTED)
    sendPendingOAuthVerifyCode.mockResolvedValue({ countdown: 0 REDACTED)
    verifyTencent
      .mockResolvedValueOnce({ ticket: 'ticket-1', randstr: '@rand-1' REDACTED)
      .mockResolvedValueOnce({ ticket: 'ticket-2', randstr: '@rand-2' REDACTED)
    const CaptchaChallengeStub = defineComponent({
      setup(_, { expose REDACTED) {
        expose({ verifyTencent, reset: turnstileReset REDACTED)
        return () => h('div')
      REDACTED
    REDACTED)

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'oidc',
        initialEmail: 'user@example.com',
        isSubmitting: false
      REDACTED,
      global: {
        stubs: { TurnstileWidget: CaptchaChallengeStub REDACTED
      REDACTED
    REDACTED)

    await flushPromises()
    await wrapper.get('[data-testid="oidc-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="oidc-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="oidc-create-account-send-code"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="oidc-create-account-submit"]').trigger('click')
    await flushPromises()

    expect(verifyTencent).toHaveBeenCalledTimes(2)
    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com',
      tencent_captcha_ticket: 'ticket-1',
      tencent_captcha_randstr: '@rand-1'
    REDACTED)
    expect(wrapper.emitted('submit')).toEqual([
      [
        expect.objectContaining({
          tencentCaptchaTicket: 'ticket-2',
          tencentCaptchaRandstr: '@rand-2'
        REDACTED)
      ]
    ])
    expect(turnstileReset).toHaveBeenCalledTimes(2)
  REDACTED)

  it('emits trimmed email, password, and verify code on submit', async () => {
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      REDACTED
    REDACTED)

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue(' 246810 ')
    await wrapper.get('form').trigger('submit.prevent')

    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'user@example.com',
          password: 'secret-123',
          verifyCode: '246810'
        REDACTED
      ]
    ])
  REDACTED)

  it('renders action labels through i18n keys', () => {
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      REDACTED
    REDACTED)

    expect(wrapper.text()).toContain('auth.createAccount')
    expect(wrapper.text()).toContain('auth.alreadyHaveAccount')
  REDACTED)

  it('hides email verification controls when public settings disable email verification', async () => {
    getPublicSettings.mockResolvedValue({
      email_verify_enabled: false,
      turnstile_enabled: false,
      turnstile_site_key: ''
    REDACTED)

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      REDACTED
    REDACTED)

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')

    expect(wrapper.find('[data-testid="linuxdo-create-account-verify-code"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="linuxdo-create-account-send-code"]').exists()).toBe(false)
    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'prefill@example.com',
          password: 'secret-123',
          verifyCode: ''
        REDACTED
      ]
    ])
  REDACTED)

  it('shows and emits invitation code when invitation-only signup is enabled', async () => {
    getPublicSettings.mockResolvedValue({
      invitation_code_enabled: true,
      email_verify_enabled: true,
      turnstile_enabled: false,
      turnstile_site_key: ''
    REDACTED)

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      REDACTED
    REDACTED)

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="linuxdo-create-account-invitation-code"]').setValue(' INVITE123 ')
    await wrapper.get('form').trigger('submit.prevent')

    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'prefill@example.com',
          password: 'secret-123',
          verifyCode: '246810',
          invitationCode: 'INVITE123'
        REDACTED
      ]
    ])
  REDACTED)

  it('sends a verify code for the trimmed email value', async () => {
    sendPendingOAuthVerifyCode.mockResolvedValue({
      message: 'sent',
      countdown: 60
    REDACTED)

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      REDACTED
    REDACTED)

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com'
    REDACTED)
  REDACTED)

  it('shows send-code failures via toast without rendering inline error text', async () => {
    sendPendingOAuthVerifyCode.mockRejectedValue(new Error('send failed'))

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      REDACTED
    REDACTED)

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('user@example.com')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('send failed')
    expect(wrapper.text()).not.toContain('send failed')
  REDACTED)

  it('consumes the captcha proof when sending a verify code fails', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    REDACTED)
    sendPendingOAuthVerifyCode.mockRejectedValue(new Error('send failed'))

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'oidc',
        initialEmail: 'user@example.com',
        isSubmitting: false
      REDACTED,
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'proof-token\')">verify</button>',
            methods: { reset: turnstileReset REDACTED
          REDACTED
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()
    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="oidc-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(turnstileReset).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="oidc-create-account-send-code"]').attributes('disabled')).toBeDefined()
  REDACTED)

  it('requires a turnstile token before sending a verify code when turnstile is enabled', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    REDACTED)
    sendPendingOAuthVerifyCode.mockResolvedValue({
      message: 'sent',
      countdown: 60
    REDACTED)

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      REDACTED,
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'turnstile-token\')">verify</button>',
            methods: { reset: vi.fn() REDACTED
          REDACTED
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')

    expect(wrapper.get('[data-testid="linuxdo-create-account-send-code"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com',
      turnstile_token: 'turnstile-token'
    REDACTED)
  REDACTED)
REDACTED)

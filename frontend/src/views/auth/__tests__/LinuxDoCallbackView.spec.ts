import { beforeEach, describe, expect, it, vi REDACTED from 'vitest'
import { flushPromises, mount REDACTED from '@vue/test-utils'

import LinuxDoCallbackView from '../LinuxDoCallbackView.vue'

const replace = vi.fn()
const showSuccess = vi.fn()
const showError = vi.fn()
const setToken = vi.fn()
const exchangePendingOAuthCompletion = vi.fn()
const completeLinuxDoOAuthRegistration = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {REDACTED
  REDACTED),
  useRouter: () => ({
    replace
  REDACTED)
REDACTED))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    REDACTED)
  REDACTED
REDACTED)

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    setToken
  REDACTED),
  useAppStore: () => ({
    showSuccess,
    showError
  REDACTED)
REDACTED))

vi.mock('@/api/auth', () => ({
  exchangePendingOAuthCompletion: (...args: any[]) => exchangePendingOAuthCompletion(...args),
  completeLinuxDoOAuthRegistration: (...args: any[]) => completeLinuxDoOAuthRegistration(...args)
REDACTED))

describe('LinuxDoCallbackView', () => {
  beforeEach(() => {
    replace.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    setToken.mockReset()
    exchangePendingOAuthCompletion.mockReset()
    completeLinuxDoOAuthRegistration.mockReset()
  REDACTED)

  it('does not send adoption decisions during the initial exchange', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({
      access_token: 'access-token',
      refresh_token: 'refresh-token',
      expires_in: 3600,
      redirect: '/dashboard',
      adoption_required: true
    REDACTED)
    setToken.mockResolvedValue({REDACTED)

    mount(LinuxDoCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(1)
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledWith()
  REDACTED)

  it('waits for explicit adoption confirmation before finishing a non-invitation login', async () => {
    exchangePendingOAuthCompletion
      .mockResolvedValueOnce({
        redirect: '/dashboard',
        adoption_required: true,
        suggested_display_name: 'LinuxDo Nick',
        suggested_avatar_url: 'https://cdn.example/linuxdo.png'
      REDACTED)
      .mockResolvedValueOnce({
        access_token: 'access-token',
        refresh_token: 'refresh-token',
        expires_in: 3600,
        redirect: '/dashboard'
      REDACTED)
    setToken.mockResolvedValue({REDACTED)

    const wrapper = mount(LinuxDoCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('LinuxDo Nick')
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[1].setValue(false)

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(1)
    await buttons[0].trigger('click')
    await flushPromises()

    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(2)
    expect(exchangePendingOAuthCompletion).toHaveBeenNthCalledWith(1)
    expect(exchangePendingOAuthCompletion).toHaveBeenNthCalledWith(2, {
      adoptDisplayName: true,
      adoptAvatar: false
    REDACTED)
    expect(setToken).toHaveBeenCalledWith('access-token')
    expect(replace).toHaveBeenCalledWith('/dashboard')
  REDACTED)

  it('renders adoption choices for invitation flow and submits the selected values', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({
      error: 'invitation_required',
      redirect: '/dashboard',
      adoption_required: true,
      suggested_display_name: 'LinuxDo Nick',
      suggested_avatar_url: 'https://cdn.example/linuxdo.png'
    REDACTED)
    completeLinuxDoOAuthRegistration.mockResolvedValue({
      access_token: 'access-token',
      refresh_token: 'refresh-token',
      expires_in: 3600,
      token_type: 'Bearer'
    REDACTED)
    setToken.mockResolvedValue({REDACTED)

    const wrapper = mount(LinuxDoCallbackView, {
      global: {
        stubs: {
          AuthLayout: { template: '<div><slot /></div>' REDACTED,
          Icon: true,
          RouterLink: { template: '<a><slot /></a>' REDACTED,
          transition: false
        REDACTED
      REDACTED
    REDACTED)

    await flushPromises()

    expect(wrapper.text()).toContain('LinuxDo Nick')
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(1)
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledWith()

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(2)

    await checkboxes[0].setValue(false)
    await wrapper.find('input[type="text"]').setValue('invite-code')
    await wrapper.find('button').trigger('click')

    expect(completeLinuxDoOAuthRegistration).toHaveBeenCalledWith('invite-code', {
      adoptDisplayName: false,
      adoptAvatar: true
    REDACTED)
  REDACTED)
REDACTED)

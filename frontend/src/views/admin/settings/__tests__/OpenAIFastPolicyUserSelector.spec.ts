import { flushPromises, mount REDACTED from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi REDACTED from 'vitest'

import OpenAIFastPolicyUserSelector from '../OpenAIFastPolicyUserSelector.vue'

const messages: Record<string, string> = {
  'admin.settings.openaiFastPolicy.userDeleted': '(deleted)',
  'admin.settings.openaiFastPolicy.userIdFallback': 'User #{idREDACTED',
  'admin.settings.openaiFastPolicy.removeUser': 'Remove user',
  'admin.settings.openaiFastPolicy.userSearchPlaceholder': 'Search users',
  'admin.settings.openaiFastPolicy.userSearchEmpty': 'No users found',
  'common.loading': 'Loading',
REDACTED

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const message = messages[key] ?? key
      return params
        ? Object.entries(params).reduce(
            (value, [name, replacement]) => value.replace(`{${nameREDACTEDREDACTED`, String(replacement)),
            message,
          )
        : message
    REDACTED,
  REDACTED),
REDACTED))

const mockSearchUsers = vi.fn()
const mockGetUserById = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      searchUsers: (...args: unknown[]) => mockSearchUsers(...args),
    REDACTED,
    users: {
      getById: (...args: unknown[]) => mockGetUserById(...args),
    REDACTED,
  REDACTED,
REDACTED))

describe('OpenAIFastPolicyUserSelector', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockSearchUsers.mockReset()
    mockGetUserById.mockReset()
  REDACTED)

  afterEach(() => {
    vi.useRealTimers()
  REDACTED)

  it('hydrates existing IDs to email labels without changing the saved IDs', async () => {
    mockGetUserById.mockResolvedValue({
      id: 7,
      email: 'existing@example.com',
      deleted_at: null,
    REDACTED)

    const wrapper = mount(OpenAIFastPolicyUserSelector, {
      props: { modelValue: [7] REDACTED,
      global: { stubs: { Icon: true REDACTED REDACTED,
    REDACTED)
    await flushPromises()

    expect(mockGetUserById).toHaveBeenCalledWith(7, true)
    expect(wrapper.text()).toContain('existing@example.com')
    expect(wrapper.text()).toContain('#7')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  REDACTED)

  it('searches after one character and adds the selected user ID', async () => {
    mockSearchUsers.mockResolvedValue([
      { id: 9, email: 'alice@example.com', deleted: false REDACTED,
    ])

    const wrapper = mount(OpenAIFastPolicyUserSelector, {
      props: { modelValue: [] REDACTED,
      global: { stubs: { Icon: true REDACTED REDACTED,
    REDACTED)
    const input = wrapper.get('input')
    await input.trigger('focus')
    await input.setValue('a')
    await input.trigger('input')
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(mockSearchUsers).toHaveBeenCalledWith('a')
    const result = wrapper.findAll('button').find((button) =>
      button.text().includes('alice@example.com'),
    )
    expect(result).toBeDefined()
    await result!.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[[9]]])
  REDACTED)

  it('keeps an unresolved saved ID visible and removable', async () => {
    mockGetUserById.mockRejectedValue(new Error('not found'))

    const wrapper = mount(OpenAIFastPolicyUserSelector, {
      props: { modelValue: [42] REDACTED,
      global: { stubs: { Icon: true REDACTED REDACTED,
    REDACTED)
    await flushPromises()

    expect(wrapper.text()).toContain('User #42')
    await wrapper.get('button[aria-label="Remove user"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[[]]])
  REDACTED)
REDACTED)

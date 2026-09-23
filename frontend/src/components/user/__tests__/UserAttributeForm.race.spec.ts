import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserAttributeForm from '../UserAttributeForm.vue'
const mocks = vi.hoisted(() => ({ getUserAttributeValues: vi.fn(), listEnabledDefinitions: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { userAttributes: mocks } }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  mocks.listEnabledDefinitions.mockResolvedValue([{ id: 1, name: 'Team', type: 'text' }])
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())
function deferred() {
  let resolve!: (value: unknown) => void
  const promise = new Promise(res => { resolve = res })
  return { promise, resolve }
}
function open() {
  return mount(UserAttributeForm, { props: { userId: 1, modelValue: {} }, global: { stubs: { Select: true } } })
}
describe('user attribute request ownership', () => {
  it('does not emit previous user values after a newer user has loaded', async () => {
    const old = deferred()
    mocks.getUserAttributeValues.mockReturnValueOnce(old.promise).mockResolvedValueOnce([{ attribute_id: 1, value: 'current' }])
    const w = open(); await w.setProps({ userId: 2 }); await flushPromises()
    old.resolve([{ attribute_id: 1, value: 'old' }]); await flushPromises()
    expect(w.get('input').element.value).toBe('current')
    expect(w.emitted('update:modelValue')).toEqual([[{ 1: 'current' }]])
  })
  it('does not restore values after resetting to a new user', async () => {
    const old = deferred(); mocks.getUserAttributeValues.mockReturnValueOnce(old.promise)
    const w = open(); await flushPromises(); await w.setProps({ userId: undefined })
    old.resolve([{ attribute_id: 1, value: 'old' }]); await flushPromises()
    expect(w.get('input').element.value).toBe('')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })
  it('clears the previous values before a new user request fails', async () => {
    mocks.getUserAttributeValues.mockResolvedValueOnce([{ attribute_id: 1, value: 'old' }]).mockRejectedValueOnce(new Error('unavailable'))
    const w = open(); await flushPromises(); await w.setProps({ userId: 2 }); await flushPromises()
    expect(w.get('input').element.value).toBe('')
  })
})

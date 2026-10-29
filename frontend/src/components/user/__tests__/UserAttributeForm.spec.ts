import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import UserAttributeForm from '../UserAttributeForm.vue'

const mocks = vi.hoisted(() => ({ listEnabledDefinitions: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { userAttributes: mocks } }))
enableAutoUnmount(afterEach)

beforeEach(() => {
  mocks.listEnabledDefinitions.mockResolvedValue([
    { id: 1, name: 'Count', type: 'number', validation: { min: -10, max: 100 } },
    { id: 2, name: 'Note', type: 'text' }
  ])
})

describe('UserAttributeForm numeric values', () => {
  it.each(['42', '0', '-3'])('emits %s as a string accepted by the values API', async (value) => {
    const wrapper = mount(UserAttributeForm, { props: { modelValue: {} }, global: { stubs: { Select: true } } })
    await flushPromises()
    await wrapper.get('input[type="text"]').setValue('keep me')
    await wrapper.get('input[type="number"]').setValue(value)
    const emitted = wrapper.emitted('update:modelValue')!.at(-1)![0]
    expect(JSON.parse(JSON.stringify(emitted))).toEqual({ 1: value, 2: 'keep me' })
  })

  it('emits an empty string when the number is cleared', async () => {
    const wrapper = mount(UserAttributeForm, { props: { modelValue: {} }, global: { stubs: { Select: true } } })
    await flushPromises()
    const input = wrapper.get('input[type="number"]')
    await input.setValue('42')
    await input.setValue('')
    expect(wrapper.emitted('update:modelValue')!.at(-1)).toEqual([{ 1: '' }])
    expect(input.attributes()).toMatchObject({ min: '-10', max: '100', type: 'number' })
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ModelTagInput from '../ModelTagInput.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

function pressTab(input: Element, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true })
  input.dispatchEvent(event)
  return event
}

describe('model tag keyboard navigation', () => {
  it.each(['Enter', 'Tab', 'Backspace'])('leaves composing %s to the input method', async (key) => {
    const wrapper = mount(ModelTagInput, { props: { models: ['existing'] } })
    const input = wrapper.get('input')
    if (key !== 'Backspace') await input.setValue('prefix')
    const event = new KeyboardEvent('keydown', { key, isComposing: true, bubbles: true, cancelable: true })
    input.element.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('keeps tags on Delete and removes the last tag only on Backspace', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: ['existing'] } })
    await wrapper.get('input').trigger('keydown', { key: 'Delete' })
    expect(wrapper.emitted('update:models')).toBeUndefined()
    await wrapper.get('input').trigger('keydown', { key: 'Backspace' })
    expect(wrapper.emitted('update:models')).toEqual([[[]]])
  })

  it('commits ordinary Enter input and prevents form submission', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: [] } })
    const input = wrapper.get('input'); await input.setValue('new-model')
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    input.element.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    expect(wrapper.emitted('update:models')).toEqual([[['new-model']]])
  })

  it.each([false, true])('allows leaving an empty input with Tab (shift: %s)', (shift) => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    expect(pressTab(wrapper.get('input').element, shift).defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('allows leaving a whitespace-only input', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: [] } })
    await wrapper.get('input').setValue('   ')
    expect(pressTab(wrapper.get('input').element).defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('still commits a pending model with Tab, then allows the next Tab to leave', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    const input = wrapper.get('input')
    await input.setValue('gpt-4.1')
    expect(pressTab(input.element).defaultPrevented).toBe(true)
    await nextTick()
    expect(wrapper.emitted('update:models')).toEqual([[['gpt-4o', 'gpt-4.1']]])
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(pressTab(input.element).defaultPrevented).toBe(false)
  })
})

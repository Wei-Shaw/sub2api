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

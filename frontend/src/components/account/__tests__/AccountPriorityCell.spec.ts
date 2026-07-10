import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountPriorityCell from '../AccountPriorityCell.vue'

describe('AccountPriorityCell', () => {
  it('saves a changed integer on blur', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000, label: 'Priority' }
    })
    const input = wrapper.get('input')

    await input.setValue('50')
    await input.trigger('blur')

    expect(wrapper.emitted('save')).toEqual([[50]])
  })

  it('does not submit the same value twice for enter and blur', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = wrapper.get('input')

    await input.setValue('50')
    await input.trigger('keydown', { key: 'Enter' })
    await input.trigger('blur')

    expect(wrapper.emitted('save')).toEqual([[50]])
  })

  it('restores the original value without saving on escape', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = wrapper.get('input')

    await input.setValue('50')
    await input.trigger('keydown', { key: 'Escape' })

    expect((input.element as HTMLInputElement).value).toBe('1000')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('rejects invalid priorities and restores the current value', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = wrapper.get('input')

    await input.setValue('0')
    await input.trigger('blur')

    expect((input.element as HTMLInputElement).value).toBe('1000')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('disables the input and shows progress while saving', () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000, saving: true }
    })

    expect(wrapper.get('input').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="priority-saving"]').exists()).toBe(true)
  })

  it('restores the current value when saving finishes without an update', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = wrapper.get('input')

    await input.setValue('50')
    await input.trigger('blur')
    await wrapper.setProps({ saving: true })
    await wrapper.setProps({ saving: false })

    expect((input.element as HTMLInputElement).value).toBe('1000')
  })
})

import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountPriorityCell from '../AccountPriorityCell.vue'

const startEditing = async (wrapper: ReturnType<typeof mount>) => {
  await wrapper.get('[data-test="priority-value"]').trigger('dblclick')
  return wrapper.get('input')
}

describe('AccountPriorityCell', () => {
  it('shows a value until it is double-clicked', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000, editLabel: 'Double-click to edit' }
    })
    const value = wrapper.get('[data-test="priority-value"]')

    expect(value.text()).toBe('1000')
    expect(value.attributes('title')).toBe('Double-click to edit')
    expect(wrapper.find('input').exists()).toBe(false)

    await value.trigger('click')
    expect(wrapper.find('input').exists()).toBe(false)

    const input = await startEditing(wrapper)
    expect((input.element as HTMLInputElement).value).toBe('1000')
  })

  it('saves a changed integer on blur', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000, label: 'Priority' }
    })
    const input = await startEditing(wrapper)

    await input.setValue('50')
    await input.trigger('blur')

    expect(wrapper.emitted('save')).toEqual([[50]])
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-value"]').text()).toBe('50')
  })

  it('saves once on enter and exits editing', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = await startEditing(wrapper)

    await input.setValue('50')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('save')).toEqual([[50]])
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('restores the original value without saving on escape', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = await startEditing(wrapper)

    await input.setValue('50')
    await input.trigger('keydown', { key: 'Escape' })

    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-value"]').text()).toBe('1000')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('rejects invalid priorities and restores the current value', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = await startEditing(wrapper)

    await input.setValue('0')
    await input.trigger('blur')

    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-value"]').text()).toBe('1000')
    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('shows progress without entering editing while saving', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000, saving: true }
    })

    await wrapper.get('[data-test="priority-value"]').trigger('dblclick')

    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('[data-test="priority-saving"]').exists()).toBe(true)
  })

  it('restores the current value when saving finishes without an update', async () => {
    const wrapper = mount(AccountPriorityCell, {
      props: { value: 1000 }
    })
    const input = await startEditing(wrapper)

    await input.setValue('50')
    await input.trigger('blur')
    expect(wrapper.get('[data-test="priority-value"]').text()).toBe('50')

    await wrapper.setProps({ saving: true })
    await wrapper.setProps({ saving: false })

    expect(wrapper.get('[data-test="priority-value"]').text()).toBe('1000')
  })
})

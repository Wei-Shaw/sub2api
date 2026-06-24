import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Toggle from '@/components/common/Toggle.vue'

describe('Toggle', () => {
  it('emits the next value when enabled', async () => {
    const wrapper = mount(Toggle, {
      props: {
        modelValue: false,
      },
    })

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])
  })

  it('does not emit when disabled', async () => {
    const wrapper = mount(Toggle, {
      props: {
        modelValue: true,
        disabled: true,
      },
    })

    const button = wrapper.get('button')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('aria-disabled')).toBe('true')

    await button.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})

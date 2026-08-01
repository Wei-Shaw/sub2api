import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ModelMultiSelect from '../ModelMultiSelect.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('ModelMultiSelect', () => {
  it('selects failed models independently and supports all or none', async () => {
    const wrapper = mount(ModelMultiSelect, {
      props: {
        modelValue: ['luna', 'sol'],
        options: [
          { value: 'luna', label: 'Luna' },
          { value: 'sol', label: 'Sol' }
        ],
        id: 'failed-models',
        ariaLabel: 'Failed models',
        allLabel: 'All failed models',
        clearLabel: 'Clear'
      },
      global: { stubs: { Icon: true } }
    })

    const trigger = wrapper.get('.select-trigger')
    expect(trigger.attributes()).toMatchObject({ id: 'failed-models', 'aria-label': 'Failed models' })
    expect(wrapper.text()).toContain('All failed models')
    await trigger.trigger('click')
    expect(trigger.attributes('aria-controls')).toBe('failed-models-options')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(wrapper.get('[role="group"]').attributes('aria-label')).toBe('Failed models')
    await wrapper.findAll('input[type="checkbox"]')[0].trigger('change')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['sol']])

    await wrapper.findAll('button').find(button => button.text() === 'Clear')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([[]])

    await wrapper.findAll('button').filter(button => button.text() === 'All failed models').pop()!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[2]).toEqual([['luna', 'sol']])
  })
})

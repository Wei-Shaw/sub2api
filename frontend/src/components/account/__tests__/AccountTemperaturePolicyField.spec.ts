import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountTemperaturePolicyField from '../AccountTemperaturePolicyField.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AccountTemperaturePolicyField', () => {
  it('renders the three account modes and no unchanged mode by default', () => {
    const wrapper = mount(AccountTemperaturePolicyField, {
      props: { mode: 'inherit', temperature: null }
    })

    expect(wrapper.find('[data-testid="temperature-mode-inherit"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="temperature-mode-override"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="temperature-mode-omit"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="temperature-mode-unchanged"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="temperature-value"]').exists()).toBe(false)
  })

  it('emits mode changes and shows a numeric input for override', async () => {
    const wrapper = mount(AccountTemperaturePolicyField, {
      props: { mode: 'inherit', temperature: null }
    })

    await wrapper.get('[data-testid="temperature-mode-override"]').trigger('click')
    expect(wrapper.emitted('update:mode')).toEqual([['override']])

    await wrapper.setProps({ mode: 'override' })
    const input = wrapper.get('[data-testid="temperature-value"]')
    expect(input.attributes('type')).toBe('number')
    expect(input.attributes('required')).toBeDefined()

    await input.setValue('0')
    expect(wrapper.emitted('update:temperature')?.at(-1)).toEqual([0])
  })

  it('emits null when the override input is cleared', async () => {
    const wrapper = mount(AccountTemperaturePolicyField, {
      props: { mode: 'override', temperature: 0.7 }
    })

    await wrapper.get('[data-testid="temperature-value"]').setValue('')
    expect(wrapper.emitted('update:temperature')?.at(-1)).toEqual([null])
  })

  it('supports bulk unchanged mode when requested', () => {
    const wrapper = mount(AccountTemperaturePolicyField, {
      props: { mode: 'unchanged', temperature: null, allowUnchanged: true }
    })

    expect(wrapper.find('[data-testid="temperature-mode-unchanged"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="temperature-mode-unchanged"]').attributes('aria-pressed')).toBe(
      'true'
    )
  })
})

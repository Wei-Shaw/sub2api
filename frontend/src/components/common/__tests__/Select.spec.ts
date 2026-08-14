import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import FormField from '../FormField.vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const originalInnerWidth = window.innerWidth
let unmountWrapper: (() => void) | undefined

const setViewportWidth = (width: number) => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

const mockTriggerRect = (left: number, width: number) => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: left,
    y: 20,
    top: 20,
    right: left + width,
    bottom: 60,
    left,
    width,
    height: 40,
    toJSON: () => ({}),
  })
}

const openSelect = async () => {
  const wrapper = mount(Select, {
    props: {
      modelValue: null,
      options: [
        {
          value: 'example',
          label: 'very-long-unbroken-option-value-that-must-not-overflow',
        },
      ],
    },
  })
  unmountWrapper = () => wrapper.unmount()

  await wrapper.get('button').trigger('click')
  await nextTick()

  return document.body.querySelector<HTMLElement>('.select-dropdown-portal')
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  setViewportWidth(originalInnerWidth)
  vi.restoreAllMocks()
})

describe('Select accessible name', () => {
  const options = [{ value: 'monthly', label: 'Monthly' }]

  it('does not render aria-label when FormField already links a real label', async () => {
    const wrapper = mount(
      {
        components: { FormField, Select },
        data: () => ({ options }),
        template: `
          <FormField label="Billing cycle">
            <template #default="{ id }">
              <Select :id="id" :model-value="null" :options="options" />
            </template>
          </FormField>
        `,
      },
      { attachTo: document.body }
    )
    unmountWrapper = () => wrapper.unmount()
    await nextTick()

    const trigger = wrapper.get('button')
    const label = wrapper.get('label')

    // The label is genuinely wired to this control...
    expect(label.attributes('for')).toBe(trigger.attributes('id'))
    expect(label.text()).toContain('Billing cycle')
    // ...so the control must not shout a name of its own over it.
    expect(trigger.attributes('aria-label')).toBeUndefined()
  })

  it('renders the aria-label a consumer explicitly passes', () => {
    const wrapper = mount(Select, {
      props: { modelValue: null, options, ariaLabel: 'Billing cycle' },
    })
    unmountWrapper = () => wrapper.unmount()

    expect(wrapper.get('button').attributes('aria-label')).toBe('Billing cycle')
  })

  it('forwards a consumer aria-labelledby to the trigger instead of naming itself', () => {
    const wrapper = mount(Select, {
      props: { modelValue: null, options },
      attrs: { 'aria-labelledby': 'billing-heading' },
    })
    unmountWrapper = () => wrapper.unmount()

    const trigger = wrapper.get('button')
    expect(trigger.attributes('aria-labelledby')).toBe('billing-heading')
    expect(trigger.attributes('aria-label')).toBeUndefined()
  })

  it('falls back to its own text — not an invented label — with no naming source', () => {
    const wrapper = mount(Select, { props: { modelValue: null, options } })
    unmountWrapper = () => wrapper.unmount()

    const trigger = wrapper.get('button')
    expect(trigger.attributes('aria-label')).toBeUndefined()
    expect(trigger.attributes('aria-labelledby')).toBeUndefined()
    // The name comes from content, which is localized: the placeholder key here,
    // the selected option's label once there is a value.
    expect(trigger.text()).toContain('common.selectOption')
  })

  it('keeps the clear affordance labelled from the translation catalog', () => {
    const wrapper = mount(Select, {
      props: { modelValue: 'monthly', options, clearable: true },
    })
    unmountWrapper = () => wrapper.unmount()

    expect(wrapper.get('.select-clear').attributes('aria-label')).toBe('common.clear')
  })
})

describe('Select dropdown viewport constraints', () => {
  it('preserves the existing 200px minimum width when space is available', async () => {
    setViewportWidth(1024)
    mockTriggerRect(20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('996px')
  })

  it('shrinks the minimum width to fit near the right viewport edge', async () => {
    setViewportWidth(320)
    mockTriggerRect(220, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('220px')
    expect(dropdown?.style.minWidth).toBe('92px')
    expect(dropdown?.style.maxWidth).toBe('92px')
  })

  it('clamps a trigger left of the viewport to the safe padding', async () => {
    setViewportWidth(320)
    mockTriggerRect(-20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('8px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('304px')
  })

  it('clamps an offscreen-right trigger position to the viewport boundary', async () => {
    setViewportWidth(320)
    mockTriggerRect(400, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('312px')
    expect(dropdown?.style.minWidth).toBe('0px')
    expect(dropdown?.style.maxWidth).toBe('0px')
  })
})

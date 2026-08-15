import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'

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

describe('Select multiple mode', () => {
  it('supports a bare multiple prop and keeps the dropdown open while toggling values', async () => {
    const Host = defineComponent({
      components: { AppSelect: Select },
      setup() {
        const selected = ref<number[]>([])
        const options = [
          { value: 1, label: 'Group 1' },
          { value: 2, label: 'Group 2' },
          { value: 3, label: 'Group 3' },
          { value: 4, label: 'Group 4' },
          { value: 5, label: 'Group 5' },
          { value: 6, label: 'Group 6' },
        ]
        return { selected, options }
      },
      template: '<AppSelect v-model="selected" :options="options" multiple />',
    })

    const wrapper = mount(Host)
    unmountWrapper = () => wrapper.unmount()

    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
    expect(dropdown?.getAttribute('aria-multiselectable')).toBe('true')
    expect(dropdown?.querySelectorAll('.select-checkbox')).toHaveLength(6)
    expect(dropdown?.querySelector('.select-search')).not.toBeNull()

    const options = dropdown?.querySelectorAll<HTMLElement>('.select-option')
    options?.[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect((wrapper.vm as any).selected).toEqual([1])
    expect(document.body.querySelector('.select-dropdown-portal')).not.toBeNull()

    options?.[1].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect((wrapper.vm as any).selected).toEqual([1, 2])

    options?.[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect((wrapper.vm as any).selected).toEqual([2])
  })
})

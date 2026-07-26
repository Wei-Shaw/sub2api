import { mount REDACTED from '@vue/test-utils'
import { afterEach, describe, expect, it, vi REDACTED from 'vitest'
import { nextTick REDACTED from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key REDACTED),
  REDACTED
REDACTED)

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
REDACTED)

describe('Select dropdown viewport constraints', () => {
  it('limits the teleported dropdown to the available viewport width', async () => {
    Object.defineProperty(window, 'innerWidth', {
      configurable: true,
      value: 320,
    REDACTED)

    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 220,
      y: 20,
      top: 20,
      right: 300,
      bottom: 60,
      left: 220,
      width: 80,
      height: 40,
      toJSON: () => ({REDACTED),
    REDACTED)

    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        options: [
          {
            value: 'example',
            label: 'very-long-unbroken-option-value-that-must-not-overflow',
          REDACTED,
        ],
      REDACTED,
    REDACTED)

    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('220px')
    expect(dropdown?.style.minWidth).toBe('80px')
    expect(dropdown?.style.maxWidth).toBe('92px')

    wrapper.unmount()
  REDACTED)
REDACTED)

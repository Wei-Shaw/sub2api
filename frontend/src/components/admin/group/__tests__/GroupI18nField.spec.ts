import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, reactive } from 'vue'

import GroupI18nField from '../GroupI18nField.vue'
import type { GroupI18n } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${params.language}` : key
    })
  }
})

// 用例把组件挂到 document.body 上以断言焦点，逐例卸载避免互相残留
enableAutoUnmount(afterEach)

// 挂载一个 v-model 双向绑定的宿主，贴近分组表单里的真实用法。
const mountField = (
  initial: { text: string; i18n: GroupI18n },
  fieldProps: Record<string, unknown> = {}
) => {
  const state = reactive({ text: initial.text, i18n: initial.i18n })
  const host = defineComponent({
    setup() {
      return () =>
        h(GroupI18nField, {
          label: 'Name',
          field: 'name',
          ...fieldProps,
          modelValue: state.text,
          'onUpdate:modelValue': (value: string) => {
            state.text = value
          },
          i18n: state.i18n,
          'onUpdate:i18n': (value: GroupI18n) => {
            state.i18n = value
          }
        })
    }
  })
  return { wrapper: mount(host, { attachTo: document.body }), state }
}

type Wrapper = ReturnType<typeof mountField>['wrapper']

const trigger = (wrapper: Wrapper) => wrapper.get('[data-testid="group-i18n-menu"]')
const menuItems = (wrapper: Wrapper) => wrapper.findAll('button[data-locale]')
const choose = async (wrapper: Wrapper, locale: string) => {
  await trigger(wrapper).trigger('click')
  await wrapper.get(`button[data-locale="${locale}"]`).trigger('click')
}

describe('GroupI18nField', () => {
  it('renders the label and the switcher on one row above a single input', () => {
    const { wrapper } = mountField({ text: '标准', i18n: { en: { name: 'Standard' } } })

    const labelRow = wrapper.get('label').element.parentElement!
    expect(wrapper.get('label').text()).toBe('Name')
    expect(labelRow.contains(trigger(wrapper).element)).toBe(true)
    expect(labelRow.contains(wrapper.get('input').element)).toBe(false)
    expect(wrapper.findAll('input')).toHaveLength(1)
  })

  it('starts on the default content', async () => {
    const { wrapper, state } = mountField({ text: '标准', i18n: { en: { name: 'Standard' } } })
    const input = wrapper.get('input')

    expect(input.element.value).toBe('标准')
    expect(trigger(wrapper).text()).toBe('admin.groups.i18nField.default')

    await input.setValue('标准版')

    expect(state.text).toBe('标准版')
    expect(state.i18n).toEqual({ en: { name: 'Standard' } })
  })

  it('lists the default content and an add or edit entry per interface language', async () => {
    const { wrapper } = mountField({
      text: '标准',
      i18n: { en: { name: 'Standard' }, zh: { description: '仅描述' } }
    })

    await trigger(wrapper).trigger('click')

    expect(menuItems(wrapper).map((item) => [item.attributes('data-locale'), item.text()])).toEqual([
      ['default', 'admin.groups.i18nField.default'],
      ['en', 'admin.groups.i18nField.edit:English'],
      ['zh', 'admin.groups.i18nField.add:中文']
    ])
  })

  it('switches the same input to the chosen language version', async () => {
    const { wrapper, state } = mountField({ text: '标准', i18n: { en: { name: 'Standard' } } })

    await choose(wrapper, 'en')

    expect(menuItems(wrapper)).toHaveLength(0)
    expect(wrapper.findAll('input')).toHaveLength(1)
    const input = wrapper.get('input')
    expect(input.element.value).toBe('Standard')
    expect(document.activeElement).toBe(input.element)
    expect(trigger(wrapper).text()).toBe('English')

    await input.setValue('Standard v2')

    expect(state.text).toBe('标准')
    expect(state.i18n).toEqual({ en: { name: 'Standard v2' } })
  })

  it('starts a new language version empty and keeps the other field of that language', async () => {
    const { wrapper, state } = mountField({
      text: '标准',
      i18n: { en: { description: 'Pro + Max pool' } }
    })

    await choose(wrapper, 'en')
    const input = wrapper.get('input')
    expect(input.element.value).toBe('')
    expect(input.attributes('placeholder')).toBe('admin.groups.i18nField.placeholder:English')

    await input.setValue('Standard')

    expect(state.i18n).toEqual({ en: { name: 'Standard', description: 'Pro + Max pool' } })
  })

  it('returns to the default content from a language version', async () => {
    const { wrapper } = mountField({ text: '标准', i18n: { en: { name: 'Standard' } } })

    await choose(wrapper, 'en')
    await choose(wrapper, 'zh')
    expect(wrapper.get('input').element.value).toBe('')

    await choose(wrapper, 'default')

    expect(wrapper.get('input').element.value).toBe('标准')
    expect(trigger(wrapper).text()).toBe('admin.groups.i18nField.default')
  })

  it('marks the version being edited in the menu', async () => {
    const { wrapper } = mountField({ text: '标准', i18n: {} })

    await choose(wrapper, 'zh')
    await trigger(wrapper).trigger('click')

    const checked = menuItems(wrapper)
      .filter((item) => item.find('svg').exists())
      .map((item) => item.attributes('data-locale'))
    expect(checked).toEqual(['zh'])
  })

  it('requires only the default content and keeps the form placeholder for it', async () => {
    const { wrapper } = mountField(
      { text: '', i18n: {} },
      { required: true, placeholder: 'Enter name', 'data-tour': 'group-form-name' }
    )
    const input = wrapper.get('input')
    expect(input.attributes('required')).toBeDefined()
    expect(input.attributes('placeholder')).toBe('Enter name')
    expect(input.attributes('data-tour')).toBe('group-form-name')

    await choose(wrapper, 'en')

    expect(wrapper.get('input').attributes('required')).toBeUndefined()
  })

  it('switches a single textarea for multiline fields', async () => {
    const { wrapper, state } = mountField(
      { text: '号池', i18n: {} },
      { field: 'description', multiline: true, rows: '3' }
    )
    expect(wrapper.findAll('textarea')).toHaveLength(1)
    expect(wrapper.get('textarea').attributes('rows')).toBe('3')

    await choose(wrapper, 'en')
    await wrapper.get('textarea').setValue('Pro + Max pool')

    expect(wrapper.findAll('textarea')).toHaveLength(1)
    expect(state.i18n).toEqual({ en: { description: 'Pro + Max pool' } })
    expect(state.text).toBe('号池')
  })

  it('closes the menu when clicking elsewhere', async () => {
    const { wrapper } = mountField({ text: '标准', i18n: {} })
    await trigger(wrapper).trigger('click')
    expect(menuItems(wrapper)).toHaveLength(3)

    document.body.click()
    await wrapper.vm.$nextTick()

    expect(menuItems(wrapper)).toHaveLength(0)
  })
})

import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createPinia } from 'pinia'
import { defineComponent, h, ref } from 'vue'
import HeaderOverrideEditor from '../HeaderOverrideEditor.vue'
import type { HeaderOverrideRow } from '../credentialsBuilder'

const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

function mountEditor(initial: HeaderOverrideRow[], allowVariables = true) {
  const rows = ref<HeaderOverrideRow[]>(initial)
  const Host = defineComponent({
    setup: () => () =>
      h(HeaderOverrideEditor, {
        rows: rows.value,
        allowVariables,
        'onUpdate:rows': (next: HeaderOverrideRow[]) => {
          rows.value = next
        }
      })
  })
  const wrapper = mount(Host, { attachTo: document.body, global: { plugins: [i18n, createPinia()] } })
  return { wrapper, rows }
}

describe('HeaderOverrideEditor', () => {
  it('adds a suggested row when no value input is being edited', async () => {
    const { wrapper, rows } = mountEditor([])
    await wrapper.get('[data-testid="header-override-variable-user_id"]').trigger('click')
    expect(rows.value).toEqual([{ name: 'x-gateway-user-id', value: '{{user_id}}' }])

    // 连点同一个变量不产生重名行
    await wrapper.get('[data-testid="header-override-variable-user_id"]').trigger('click')
    expect(rows.value).toHaveLength(1)
    wrapper.unmount()
  })

  it('leaves the name empty when the suggested name is taken by another value', async () => {
    const { wrapper, rows } = mountEditor([{ name: 'X-Gateway-User-Id', value: 'fixed' }])
    await wrapper.get('[data-testid="header-override-variable-user_id"]').trigger('click')
    expect(rows.value).toEqual([
      { name: 'X-Gateway-User-Id', value: 'fixed' },
      { name: '', value: '{{user_id}}' }
    ])
    wrapper.unmount()
  })

  it('inserts at the caret of the value input being edited', async () => {
    const { wrapper, rows } = mountEditor([{ name: 'x-trace', value: 'u=;end' }])
    const input = wrapper.get('[data-testid="header-override-value"]').element as HTMLInputElement
    input.focus()
    input.setSelectionRange(2, 2)
    await wrapper.get('[data-testid="header-override-variable-request_id"]').trigger('click')
    expect(rows.value).toEqual([{ name: 'x-trace', value: 'u={{request_id}};end' }])
    wrapper.unmount()
  })

  it('shows a preview for valid variables and an inline error for unknown ones', async () => {
    const { wrapper } = mountEditor([
      { name: 'x-user', value: 'id-{{user_id}}' },
      { name: 'x-bad', value: '{{nope}}' }
    ])
    const previews = wrapper.findAll('[data-testid="header-override-row-preview"]')
    expect(previews).toHaveLength(1)
    expect(previews[0].text()).toContain('x-user: id-10086')
    expect(wrapper.findAll('[data-testid="header-override-row-error"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('hides the variable picker and flags variables when the account cannot report identity', () => {
    const { wrapper } = mountEditor([{ name: 'x-user', value: '{{user_id}}' }], false)
    expect(wrapper.find('[data-testid="header-override-variables"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="header-override-variables-unavailable"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="header-override-row-error"]')).toHaveLength(1)
    wrapper.unmount()
  })
})

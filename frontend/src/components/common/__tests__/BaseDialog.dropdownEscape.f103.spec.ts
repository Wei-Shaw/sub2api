import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'
import ProxySelector from '../ProxySelector.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy: vi.fn() } } }))

let f103Wrapper: ReturnType<typeof mount> | null = null

afterEach(() => {
  f103Wrapper?.unmount()
  f103Wrapper = null
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

// 在 BaseDialog 中挂载一个下拉组件，返回 close 监听 spy。
async function f103MountInDialog(child: () => ReturnType<typeof h>) {
  const onClose = vi.fn()
  const Host = defineComponent({
    setup() {
      return () => h(BaseDialog, { show: true, title: 'Edit', onClose }, { default: child })
    }
  })
  f103Wrapper = mount(Host, { attachTo: document.body, global: { stubs: { Icon: true } } })
  await nextTick()
  await nextTick()
  return onClose
}

const f103Escape = (target: EventTarget) =>
  target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))

describe('F1-03: Escape closing a dropdown must not close the surrounding BaseDialog', () => {
  it('Select: first Escape closes only the dropdown, second Escape closes the dialog', async () => {
    const value = ref<string | null>('user')
    const onClose = await f103MountInDialog(() =>
      h(Select, {
        modelValue: value.value,
        searchable: false,
        options: [
          { value: 'user', label: 'User' },
          { value: 'admin', label: 'Admin' }
        ],
        'onUpdate:modelValue': (v: string | null) => { value.value = v }
      })
    )
    const trigger = document.querySelector<HTMLButtonElement>('.select-trigger')!
    trigger.click()
    await nextTick()
    await nextTick()
    const listbox = document.querySelector('[role="listbox"]')
    expect(listbox).not.toBeNull()
    expect(document.activeElement).toBe(listbox)

    f103Escape(document.activeElement!)
    await nextTick()
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(onClose).not.toHaveBeenCalled()

    f103Escape(document.activeElement!)
    await nextTick()
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('ProxySelector: first Escape closes only the dropdown, second Escape closes the dialog', async () => {
    const onClose = await f103MountInDialog(() => h(ProxySelector, { modelValue: null, proxies: [] }))
    const trigger = document.querySelector<HTMLButtonElement>('.select-trigger')!
    trigger.click()
    await nextTick()
    expect(trigger.classList.contains('select-trigger-open')).toBe(true)

    f103Escape(trigger)
    await nextTick()
    expect(trigger.classList.contains('select-trigger-open')).toBe(false)
    expect(onClose).not.toHaveBeenCalled()

    f103Escape(trigger)
    await nextTick()
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'
import ProxySelector from '../ProxySelector.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { testProxy: vi.fn() } } }))

let f109Wrapper: ReturnType<typeof mount> | null = null

afterEach(() => {
  f109Wrapper?.unmount()
  f109Wrapper = null
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

// 在 BaseDialog 中挂载下拉组件，旁边放一个普通输入框作为“对话框内的其他位置”。
async function f109MountInDialog(child: () => ReturnType<typeof h>) {
  const Host = defineComponent({
    setup() {
      return () =>
        h(BaseDialog, { show: true, title: 'Edit' }, {
          default: () => [h('input', { class: 'f109-other', type: 'text' }), child()]
        })
    }
  })
  f109Wrapper = mount(Host, { attachTo: document.body, global: { stubs: { Icon: true } } })
  await nextTick()
  await nextTick()
}

describe('F1-09: dropdowns inside BaseDialog close on a click elsewhere in the dialog', () => {
  it('Select closes when another field in the same dialog is clicked', async () => {
    await f109MountInDialog(() =>
      h(Select, {
        modelValue: 'user',
        options: [
          { value: 'user', label: 'User' },
          { value: 'admin', label: 'Admin' }
        ]
      })
    )
    const trigger = document.querySelector<HTMLButtonElement>('.select-trigger')!
    trigger.click()
    await nextTick()
    expect(trigger.getAttribute('aria-expanded')).toBe('true')

    // 点击下拉内部不应关闭
    document.querySelector<HTMLElement>('[role="listbox"]')!.click()
    await nextTick()
    expect(trigger.getAttribute('aria-expanded')).toBe('true')

    document.querySelector<HTMLInputElement>('.f109-other')!.click()
    await nextTick()
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
  })

  it('ProxySelector closes when another field in the same dialog is clicked', async () => {
    await f109MountInDialog(() => h(ProxySelector, { modelValue: null, proxies: [] }))
    const trigger = document.querySelector<HTMLButtonElement>('.select-trigger')!
    trigger.click()
    await nextTick()
    expect(trigger.classList.contains('select-trigger-open')).toBe(true)

    // 点击下拉内部（搜索框）不应关闭
    document.querySelector<HTMLInputElement>('.select-search-input')!.click()
    await nextTick()
    expect(trigger.classList.contains('select-trigger-open')).toBe(true)

    document.querySelector<HTMLInputElement>('.f109-other')!.click()
    await nextTick()
    expect(trigger.classList.contains('select-trigger-open')).toBe(false)
  })
})

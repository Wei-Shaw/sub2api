import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

import { useEscapeKey } from '@/composables/useEscapeKey'

function withComposable(setup: () => void): ReturnType<typeof mount> {
  const Host = defineComponent({
    setup() {
      setup()
      return () => h('div')
    }
  })
  return mount(Host, { attachTo: document.body })
}

describe('useEscapeKey', () => {
  it('Escape 触发回调', () => {
    const onEscape = vi.fn()
    withComposable(() => {
      useEscapeKey(onEscape, { enabled: () => true })
    })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(onEscape).toHaveBeenCalledTimes(1)
  })

  it('非 Escape 键不触发', () => {
    const onEscape = vi.fn()
    withComposable(() => {
      useEscapeKey(onEscape, { enabled: () => true })
    })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(onEscape).not.toHaveBeenCalled()
  })

  it('enabled 返回 false 时不触发', () => {
    const onEscape = vi.fn()
    withComposable(() => {
      useEscapeKey(onEscape, { enabled: () => false })
    })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(onEscape).not.toHaveBeenCalled()
  })

  it('输入法组合期的 Escape 不触发（属于 IME 取消候选词）', () => {
    const onEscape = vi.fn()
    withComposable(() => {
      useEscapeKey(onEscape, { enabled: () => true })
    })

    const composing = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    Object.defineProperty(composing, 'isComposing', { value: true })
    document.dispatchEvent(composing)
    expect(onEscape).not.toHaveBeenCalled()

    // keyCode 229 同样是 IME 组合期
    const legacy = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    Object.defineProperty(legacy, 'keyCode', { value: 229 })
    document.dispatchEvent(legacy)
    expect(onEscape).not.toHaveBeenCalled()
  })

  it('阻止事件继续冒泡（避免连带触发外层弹窗的 Escape）', () => {
    const outer = vi.fn()
    document.addEventListener('keydown', outer)

    withComposable(() => {
      useEscapeKey(() => {}, { enabled: () => true })
    })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(outer).not.toHaveBeenCalled()

    document.removeEventListener('keydown', outer)
  })

  it('卸载后不再监听', () => {
    const onEscape = vi.fn()
    const wrapper = withComposable(() => {
      useEscapeKey(onEscape, { enabled: () => true })
    })
    wrapper.unmount()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(onEscape).not.toHaveBeenCalled()
  })
})

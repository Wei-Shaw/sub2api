import { describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'

import { useClickOutside } from '@/composables/useClickOutside'

/** 把 composable 装进一个可挂载的宿主组件 */
function withComposable(
  setup: () => void
): ReturnType<typeof mount> {
  const Host = defineComponent({
    setup() {
      setup()
      return () => h('div')
    }
  })
  return mount(Host, { attachTo: document.body })
}

describe('useClickOutside', () => {
  it('默认走捕获阶段，能在 @click.stop 容器内收到点击', () => {
    const onOutside = vi.fn()
    // 模拟 BaseDialog 的内容层：@click.stop 阻断冒泡
    const host = document.createElement('div')
    host.addEventListener('click', (e) => e.stopPropagation())
    document.body.appendChild(host)

    withComposable(() => {
      useClickOutside(() => true, onOutside)
    })

    host.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onOutside).toHaveBeenCalledTimes(1)

    document.body.innerHTML = ''
  })

  it('enabled 返回 false 时不触发', () => {
    const onOutside = vi.fn()
    withComposable(() => {
      useClickOutside(() => true, onOutside, { enabled: () => false })
    })

    document.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onOutside).not.toHaveBeenCalled()
  })

  it('isOutside 返回 false 时不触发', () => {
    const onOutside = vi.fn()
    withComposable(() => {
      useClickOutside(() => false, onOutside)
    })

    document.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onOutside).not.toHaveBeenCalled()
  })

  it('capture: false 时退回冒泡阶段监听', () => {
    const onOutside = vi.fn()
    withComposable(() => {
      useClickOutside(() => true, onOutside, { capture: false })
    })

    document.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onOutside).toHaveBeenCalledTimes(1)
  })

  it('卸载后不再监听', () => {
    const onOutside = vi.fn()
    const wrapper = withComposable(() => {
      useClickOutside(() => true, onOutside)
    })
    wrapper.unmount()

    document.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onOutside).not.toHaveBeenCalled()
  })
})

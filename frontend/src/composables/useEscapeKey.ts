import { onMounted, onUnmounted } from 'vue'

export interface UseEscapeKeyOptions {
  /** 仅在返回 true 时消费 Escape（避免未展开状态吞掉弹窗关闭）。 */
  enabled: () => boolean
}

/**
 * 捕获阶段的 Escape 监听：下拉展开时优先收起下拉，并阻止事件继续
 * 冒泡到 BaseDialog 的 Escape（否则会连整个弹窗一起关掉）。
 */
export function useEscapeKey(onEscape: () => void, options: UseEscapeKeyOptions) {
  const listener = (event: KeyboardEvent) => {
    if (event.key !== 'Escape') return
    // 输入法组合期的 Escape 属于 IME（取消候选词），不能当作关闭手势消费。
    if (event.isComposing || event.keyCode === 229) return
    if (!options.enabled()) return
    event.stopPropagation()
    onEscape()
  }

  onMounted(() => {
    document.addEventListener('keydown', listener, true)
  })
  onUnmounted(() => {
    document.removeEventListener('keydown', listener, true)
  })
}

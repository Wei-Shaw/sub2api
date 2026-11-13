import { onMounted, onUnmounted, type Ref } from 'vue'

export interface UseClickOutsideOptions {
  /** 捕获阶段监听（默认 true）。弹窗内容层普遍有 @click.stop，冒泡阶段收不到事件。 */
  capture?: boolean
  /** 仅在返回 true 时参与判定（例如下拉未展开时跳过）。 */
  enabled?: () => boolean
}

/**
 * 统一的 click-outside 监听。
 *
 * 必须默认走捕获阶段：BaseDialog 的内容层带 `@click.stop`，冒泡阶段的
 * document 监听在弹窗内永远不会触发 —— 点击弹窗里的下拉浮层收不到事件，
 * 浮层关不掉。
 * 捕获阶段先于任何 stopPropagation 执行，弹窗内点击也能命中。
 */
export function useClickOutside(
  isOutside: (event: MouseEvent) => boolean,
  onOutside: () => void,
  options: UseClickOutsideOptions = {},
) {
  const capture = options.capture ?? true
  const listener = (event: MouseEvent) => {
    if (options.enabled && !options.enabled()) return
    if (isOutside(event)) onOutside()
  }

  onMounted(() => {
    document.addEventListener('click', listener, capture)
  })
  onUnmounted(() => {
    document.removeEventListener('click', listener, capture)
  })
}

/** 判定点击是否落在 ref 之外；ref 为空时视为「不在内部」。 */
export function targetInside(ref: Ref<HTMLElement | null> | null | undefined): (event: MouseEvent) => boolean {
  return (event: MouseEvent) => {
    const target = event.target as Node | null
    if (!target) return true
    const el = ref?.value
    return !el || !el.contains(target)
  }
}

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId, useTemplateRef, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const tooltipId = useId()
let pointerInside = false
let focusInside = false
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const contentRef = useTemplateRef<HTMLElement>('content')
const placement = ref<'top' | 'bottom'>('top')
const tooltipStyle = ref({ top: '0px', left: '0px', maxHeight: 'none' })

const VIEWPORT_PADDING = 8
const TRIGGER_GAP = 8

function openTooltip() {
  show.value = true
  nextTick(updatePosition)
}

function closeTooltip() {
  show.value = false
}

function onEnter() {
  if (props.trigger !== 'hover') return
  pointerInside = true
  openTooltip()
}

function isInside(container: HTMLElement | null, target: EventTarget | null): boolean {
  return target instanceof Node && !!container?.contains(target)
}

// 悬停模式下指针在触发图标与提示框之间往返时保持打开，便于选中提示里的文字。
function onLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(tooltipRef.value, event.relatedTarget)) return
  pointerInside = false
  if (!focusInside) closeTooltip()
}

function onTooltipLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(triggerRef.value, event.relatedTarget)) return
  pointerInside = false
  if (!focusInside) closeTooltip()
}

function onFocusIn() {
  if (props.trigger !== 'hover') return
  focusInside = true
  openTooltip()
}

function onFocusOut(event: FocusEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(triggerRef.value, event.relatedTarget) || isInside(tooltipRef.value, event.relatedTarget)) return
  focusInside = false
  if (!pointerInside) closeTooltip()
}

function onClick(event: MouseEvent) {
  if (props.trigger !== 'click') return
  event.stopPropagation()
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip()
}

function onDocumentClick(event: MouseEvent) {
  if (props.trigger !== 'click' || !show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeTooltip()
  }
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  const tooltip = tooltipRef.value
  const content = contentRef.value
  if (!el || !tooltip || !content) return

  // fixed 定位使用视口坐标；滚动页面时不能再叠加 scrollX/Y。
  const rect = el.getBoundingClientRect()
  const width = tooltip.offsetWidth
  // 定位外层保留箭头与悬停桥；只让内容容器滚动，并以其自然内容高度计算总高。
  const chromeHeight = Math.max(0, tooltip.offsetHeight - content.offsetHeight)
  const height = content.scrollHeight + chromeHeight
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const center = rect.left + rect.width / 2
  const minCenter = VIEWPORT_PADDING + width / 2
  const maxCenter = viewportWidth - VIEWPORT_PADDING - width / 2
  const left = minCenter <= maxCenter
    ? Math.min(Math.max(center, minCenter), maxCenter)
    : viewportWidth / 2

  const above = Math.max(0, rect.top - TRIGGER_GAP - VIEWPORT_PADDING)
  const below = Math.max(0, viewportHeight - rect.bottom - TRIGGER_GAP - VIEWPORT_PADDING)
  placement.value = height > above && below > above ? 'bottom' : 'top'
  const availableHeight = placement.value === 'top' ? above : below

  tooltipStyle.value = {
    top: `${placement.value === 'top' ? rect.top - TRIGGER_GAP : rect.bottom + TRIGGER_GAP}px`,
    left: `${left}px`,
    maxHeight: `${Math.max(0, availableHeight - chromeHeight)}px`,
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<template>
  <div
    ref="trigger"
    class="group relative ml-1 inline-flex items-center align-middle"
    :aria-describedby="show ? tooltipId : undefined"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @click="onClick"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    </slot>

    <!-- Teleport to body to escape modal overflow clipping -->
    <Teleport to="body">
      <!-- before: 伪元素沿当前放置方向延伸，盖住提示框与触发图标之间的空隙，让指针能连续移入提示框。 -->
      <div
        :id="tooltipId"
        ref="tooltip"
        v-show="show"
        role="tooltip"
        :class="[
          'fixed z-[99999] max-w-[calc(100vw-1rem)] rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 selection:bg-primary-200 selection:text-gray-900 before:absolute before:inset-x-0 before:h-3 dark:bg-gray-800 dark:selection:bg-primary-200 dark:selection:text-gray-900',
          props.widthClass,
          placement === 'top'
            ? '-translate-x-1/2 -translate-y-full before:top-full'
            : '-translate-x-1/2 before:bottom-full',
        ]"
        :style="{ top: tooltipStyle.top, left: tooltipStyle.left }"
        @mouseenter="onEnter"
        @mouseleave="onTooltipLeave"
        @focusin="onFocusIn"
        @focusout="onFocusOut"
      >
        <button
          v-if="props.trigger === 'click'"
          type="button"
          class="absolute right-1.5 top-1.5 rounded p-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white"
          aria-label="Close"
          @click.stop="closeTooltip"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <div
          ref="content"
          class="tooltip-content overflow-y-auto overscroll-contain"
          :style="{ maxHeight: tooltipStyle.maxHeight }"
        >
          <slot>{{ content }}</slot>
        </div>
        <div
          class="absolute left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"
          :class="placement === 'top' ? '-bottom-1' : '-top-1'"
        ></div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId, useTemplateRef, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const tooltipStyle = ref({ top: '0px', left: '0px' })
const tooltipId = useId()
// 自定义触发器自带可聚焦元素（如链接）时不再额外占一个 Tab 位，也避免 role=button 吞掉链接语义。
const triggerHasFocusable = ref(false)
let openedByFocus = false

function openTooltip(byFocus = false) {
  openedByFocus = byFocus
  show.value = true
  nextTick(updatePosition)
}

function closeTooltip() {
  show.value = false
}

function onEnter() {
  if (props.trigger !== 'hover') return
  openTooltip()
}

function isInside(container: HTMLElement | null, target: EventTarget | null): boolean {
  return target instanceof Node && !!container?.contains(target)
}

// 悬停模式下指针在触发图标与提示框之间往返时保持打开，便于选中提示里的文字。
function onLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(tooltipRef.value, event.relatedTarget)) return
  closeTooltip()
}

function onTooltipLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(triggerRef.value, event.relatedTarget)) return
  closeTooltip()
}

// 键盘聚焦等同悬停；只关闭由聚焦打开的提示，避免鼠标点进提示框选字时被 focusout 关掉。
function onFocusIn() {
  if (props.trigger !== 'hover' || show.value) return
  openTooltip(true)
}

function onFocusOut(event: FocusEvent) {
  if (!openedByFocus || isInside(tooltipRef.value, event.relatedTarget)) return
  closeTooltip()
}

function onClick(event: Event) {
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
  if (event.key === 'Escape' && show.value) {
    closeTooltip()
  }
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  tooltipStyle.value = {
    top: `${rect.top + window.scrollY}px`,
    left: `${rect.left + rect.width / 2 + window.scrollX}px`,
  }
}

onMounted(() => {
  triggerHasFocusable.value = !!triggerRef.value?.querySelector('a[href], button, input, select, textarea, [tabindex]')
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
    :tabindex="triggerHasFocusable ? undefined : 0"
    :role="triggerHasFocusable ? undefined : 'button'"
    :aria-describedby="triggerHasFocusable ? undefined : tooltipId"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @click="onClick"
    @focusin="onFocusIn"
    @focusout="onFocusOut"
    @keydown.enter.self.prevent="onClick"
    @keydown.space.self.prevent="onClick"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-fg-subtle transition-colors hover:text-accent"
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
      <!-- before: 伪元素向下延伸一段透明区域，盖住提示框与触发图标之间的空隙，让指针能连续移入提示框。 -->
      <div
        :id="tooltipId"
        ref="tooltip"
        v-show="show"
        role="tooltip"
        :class="[
          'fixed z-[99999] max-w-[calc(100vw-2rem)] -translate-x-1/2 -translate-y-full rounded-sm border border-border-strong bg-surface-raised px-3 py-2 text-meta leading-relaxed text-fg shadow-overlay before:absolute before:inset-x-0 before:top-full before:h-3',
          props.widthClass,
        ]"
        :style="{ top: `calc(${tooltipStyle.top} - 8px)`, left: tooltipStyle.left, borderTop: '2px solid rgb(var(--accent))' }"
        @mouseleave="onTooltipLeave"
      >
        <button
          v-if="props.trigger === 'click'"
          type="button"
          class="absolute right-1 top-1 rounded-sm p-1 text-fg-subtle transition-colors hover:bg-accent-weak hover:text-accent-strong"
          :aria-label="t('common.close')"
          @click.stop="closeTooltip"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <slot>{{ content }}</slot>
        <div class="absolute -bottom-[5px] left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 border-b border-r border-border-strong bg-surface-raised"></div>
      </div>
    </Teleport>
  </div>
</template>

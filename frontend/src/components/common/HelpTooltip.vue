<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
  tooltipClass?: string
  openDelayMs?: number
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
  openDelayMs: 0,
})

const show = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const placement = ref<'top' | 'bottom'>('top')
const tooltipStyle = ref({ top: '0px', left: '0px', maxHeight: 'none' })
let openTimer: ReturnType<typeof setTimeout> | null = null

function clearOpenTimer() {
  if (openTimer == null) return
  clearTimeout(openTimer)
  openTimer = null
}

function showTooltip() {
  openTimer = null
  show.value = true
  nextTick(updatePosition)
}

function openTooltip(immediate = false) {
  clearOpenTimer()
  const delay = immediate ? 0 : Math.max(0, props.openDelayMs)
  if (delay === 0) {
    showTooltip()
    return
  }
  openTimer = setTimeout(showTooltip, delay)
}

function closeTooltip() {
  clearOpenTimer()
  show.value = false
}

function onEnter() {
  if (props.trigger !== 'hover') return
  openTooltip()
}

function onFocusIn() {
  if (props.trigger !== 'hover') return
  openTooltip(true)
}

function onLeave() {
  if (props.trigger !== 'hover') return
  closeTooltip()
}

function onClick(event: MouseEvent) {
  if (props.trigger !== 'click') return
  event.stopPropagation()
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip(true)
}

function onDocumentClick(event: MouseEvent) {
  if (props.trigger !== 'click' || !show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (props.trigger !== 'click') return
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
  if (!el) return
  const rect = el.getBoundingClientRect()
  const tooltipRect = tooltipRef.value?.getBoundingClientRect()
  const tooltipWidth = tooltipRect?.width ?? 0
  const tooltipHeight = tooltipRect?.height ?? 0
  const viewportPadding = 8
  const gap = 8
  const desiredLeft = rect.left + rect.width / 2
  const minLeft = viewportPadding + tooltipWidth / 2
  const maxLeft = window.innerWidth - viewportPadding - tooltipWidth / 2
  const spaceAbove = Math.max(0, rect.top - gap - viewportPadding)
  const spaceBelow = Math.max(0, window.innerHeight - rect.bottom - gap - viewportPadding)
  const placeAbove = tooltipHeight <= spaceAbove || (tooltipHeight > spaceBelow && spaceAbove >= spaceBelow)
  placement.value = placeAbove ? 'top' : 'bottom'
  tooltipStyle.value = {
    top: `${placeAbove ? rect.top - gap : rect.bottom + gap}px`,
    left: `${Math.min(Math.max(desiredLeft, minLeft), Math.max(minLeft, maxLeft))}px`,
    maxHeight: `${placeAbove ? spaceAbove : spaceBelow}px`,
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  clearOpenTimer()
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
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @focusin="onFocusIn"
    @focusout="onLeave"
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
      <div
        ref="tooltip"
        v-show="show"
        role="tooltip"
        :class="[
          'fixed z-[99999] -translate-x-1/2 overflow-y-auto rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 dark:bg-gray-800',
          placement === 'top' ? '-translate-y-full' : '',
          props.widthClass,
          props.tooltipClass,
        ]"
        :style="tooltipStyle"
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
        <slot>{{ content }}</slot>
        <div
          class="absolute left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"
          :class="placement === 'top' ? '-bottom-1' : '-top-1'"
        ></div>
      </div>
    </Teleport>
  </div>
</template>

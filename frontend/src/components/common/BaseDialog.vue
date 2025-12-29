<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div ref="dialogRef" :class="['modal-content', widthClasses]" @click.stop>
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title REDACTEDREDACTED
            </h3>
            <button
              @click="emit('close')"
              class="-mr-2 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300"
              aria-label="Close modal"
            >
              <svg
                class="h-5 w-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- Body -->
          <div class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick REDACTED from 'vue'

// 生成唯一ID以避免多个对话框时ID冲突
let dialogIdCounter = 0
const dialogId = `modal-title-${++dialogIdCounterREDACTED`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
REDACTED

interface Emits {
  (e: 'close'): void
REDACTED

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false
REDACTED)

const emit = defineEmits<Emits>()

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  REDACTED
  return widths[props.width]
REDACTED)

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  REDACTED
REDACTED

const handleEscape = (event: KeyboardEvent) => {
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  REDACTED
REDACTED

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      document.body.classList.add('modal-open')

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (dialogRef.value) {
        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      REDACTED
    REDACTED else {
      document.body.classList.remove('modal-open')
      // 恢复之前的焦点
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      REDACTED
      previousActiveElement = null
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  // 确保组件卸载时移除滚动锁定
  document.body.classList.remove('modal-open')
REDACTED)
</script>

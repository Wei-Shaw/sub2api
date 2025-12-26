<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="modal-overlay"
      aria-labelledby="modal-title"
      role="dialog"
      aria-modal="true"
      @click.self="handleClose"
    >
      <!-- Modal panel -->
      <div :class="['modal-content', sizeClasses]" @click.stop>
        <!-- Header -->
        <div class="modal-header">
          <h3 id="modal-title" class="modal-title">
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
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted REDACTED from 'vue'

type ModalSize = 'sm' | 'md' | 'lg' | 'xl' | '2xl' | 'full'

interface Props {
  show: boolean
  title: string
  size?: ModalSize
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
REDACTED

interface Emits {
  (e: 'close'): void
REDACTED

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  closeOnEscape: true,
  closeOnClickOutside: false
REDACTED)

const emit = defineEmits<Emits>()

const sizeClasses = computed(() => {
  const sizes: Record<ModalSize, string> = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-5xl',
    full: 'max-w-4xl'
  REDACTED
  return sizes[props.size]
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

// Prevent body scroll when modal is open
watch(
  () => props.show,
  (isOpen) => {
    console.log('[Modal] show changed to:', isOpen)
    if (isOpen) {
      document.body.style.overflow = 'hidden'
    REDACTED else {
      document.body.style.overflow = ''
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  document.body.style.overflow = ''
REDACTED)
</script>

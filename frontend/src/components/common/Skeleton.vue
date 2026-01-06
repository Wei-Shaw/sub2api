<template>
  <div
    :class="[
      'animate-pulse bg-gray-200 dark:bg-dark-700',
      variant === 'circle' ? 'rounded-full' : 'rounded-lg',
      customClass
    ]"
    :style="style"
  ></div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'

interface Props {
  variant?: 'rect' | 'circle' | 'text'
  width?: string | number
  height?: string | number
  class?: string
REDACTED

const props = withDefaults(defineProps<Props>(), {
  variant: 'rect',
  width: '100%'
REDACTED)

const customClass = computed(() => props.class || '')

const style = computed(() => {
  const s: Record<string, string> = {REDACTED
  
  if (props.width) {
    s.width = typeof props.width === 'number' ? `${props.widthREDACTEDpx` : props.width
  REDACTED
  
  if (props.height) {
    s.height = typeof props.height === 'number' ? `${props.heightREDACTEDpx` : props.height
  REDACTED else if (props.variant === 'text') {
    s.height = '1em'
    s.marginTop = '0.25em'
    s.marginBottom = '0.25em'
  REDACTED
  
  return s
REDACTED)
</script>

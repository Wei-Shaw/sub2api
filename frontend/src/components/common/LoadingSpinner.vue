<template>
  <div
    :class="['spinner', sizeClasses, colorClass]"
    role="status"
    :aria-label="t('common.loading')"
  >
    <span class="sr-only">{{ t('common.loading') REDACTEDREDACTED</span>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'
type SpinnerColor = 'primary' | 'secondary' | 'white' | 'gray'

interface Props {
  size?: SpinnerSize
  color?: SpinnerColor
REDACTED

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  color: 'primary'
REDACTED)

const sizeClasses = computed(() => {
  const sizes: Record<SpinnerSize, string> = {
    sm: 'w-4 h-4 border-2',
    md: 'w-8 h-8 border-2',
    lg: 'w-12 h-12 border-[3px]',
    xl: 'w-16 h-16 border-4'
  REDACTED
  return sizes[props.size]
REDACTED)

const colorClass = computed(() => {
  const colors: Record<SpinnerColor, string> = {
    primary: 'text-primary-500',
    secondary: 'text-gray-500 dark:text-dark-400',
    white: 'text-white',
    gray: 'text-gray-400 dark:text-dark-500'
  REDACTED
  return colors[props.color]
REDACTED)
</script>

<style scoped>
.spinner {
  @apply inline-block rounded-full border-solid border-current border-r-transparent;
  animation: spin 0.75s linear infinite;
REDACTED

@keyframes spin {
  from {
    transform: rotate(0deg);
  REDACTED
  to {
    transform: rotate(360deg);
  REDACTED
REDACTED
</style>

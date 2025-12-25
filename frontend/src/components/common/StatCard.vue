<template>
  <div class="stat-card">
    <div :class="['stat-icon', iconClass]">
      <component v-if="icon" :is="icon" class="h-6 w-6" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <p class="stat-label truncate">{{ title REDACTEDREDACTED</p>
      <div class="mt-1 flex items-baseline gap-2">
        <p class="stat-value">{{ formattedValue REDACTEDREDACTED</p>
        <span v-if="change !== undefined" :class="['stat-trend', trendClass]">
          <svg
            v-if="changeType !== 'neutral'"
            :class="['h-3 w-3', changeType === 'down' && 'rotate-180']"
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path
              fill-rule="evenodd"
              d="M5.293 9.707a1 1 0 010-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 01-1.414 1.414L11 7.414V15a1 1 0 11-2 0V7.414L6.707 9.707a1 1 0 01-1.414 0z"
              clip-rule="evenodd"
            />
          </svg>
          {{ formattedChange REDACTEDREDACTED
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { Component REDACTED from 'vue'

type ChangeType = 'up' | 'down' | 'neutral'
type IconVariant = 'primary' | 'success' | 'warning' | 'danger'

interface Props {
  title: string
  value: number | string
  icon?: Component
  iconVariant?: IconVariant
  change?: number
  changeType?: ChangeType
  formatValue?: (value: number | string) => string
REDACTED

const props = withDefaults(defineProps<Props>(), {
  changeType: 'neutral',
  iconVariant: 'primary'
REDACTED)

const formattedValue = computed(() => {
  if (props.formatValue) {
    return props.formatValue(props.value)
  REDACTED
  if (typeof props.value === 'number') {
    return props.value.toLocaleString()
  REDACTED
  return props.value
REDACTED)

const formattedChange = computed(() => {
  if (props.change === undefined) return ''
  const absChange = Math.abs(props.change)
  return `${absChangeREDACTED%`
REDACTED)

const iconClass = computed(() => {
  const classes: Record<IconVariant, string> = {
    primary: 'stat-icon-primary',
    success: 'stat-icon-success',
    warning: 'stat-icon-warning',
    danger: 'stat-icon-danger'
  REDACTED
  return classes[props.iconVariant]
REDACTED)

const trendClass = computed(() => {
  const classes: Record<ChangeType, string> = {
    up: 'stat-trend-up',
    down: 'stat-trend-down',
    neutral: 'text-gray-500 dark:text-dark-400'
  REDACTED
  return classes[props.changeType]
REDACTED)
</script>

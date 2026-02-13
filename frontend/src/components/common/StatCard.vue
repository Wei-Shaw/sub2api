<template>
  <div class="stat-card">
    <div :class="['stat-icon', iconClass]">
      <component v-if="icon" :is="icon" class="h-6 w-6" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <p class="stat-label truncate">{{ title REDACTEDREDACTED</p>
      <div class="mt-1 flex items-baseline gap-2">
        <p class="stat-value" :title="String(formattedValue)">{{ formattedValue REDACTEDREDACTED</p>
        <span v-if="change !== undefined" :class="['stat-trend', trendClass]">
          <Icon
            v-if="changeType !== 'neutral'"
            name="arrowUp"
            size="xs"
            :class="changeType === 'down' && 'rotate-180'"
          />
          {{ formattedChange REDACTEDREDACTED
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { Component REDACTED from 'vue'
import Icon from '@/components/icons/Icon.vue'

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

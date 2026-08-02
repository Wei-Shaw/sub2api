<template>
  <div class="card p-4">
    <div class="flex items-center gap-3">
      <div :class="['rounded-lg p-2', colors.background]">
        <Icon :name="icon" size="md" :class="colors.foreground" :stroke-width="2" />
      </div>
      <div class="min-w-0">
        <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ label }}</p>
        <p class="truncate text-xl font-bold text-gray-900 dark:text-white" :title="String(value)"><slot name="value">{{ value }}</slot></p>
        <div class="truncate text-xs text-gray-500 dark:text-gray-400" :title="detail"><slot name="detail">{{ detail }}</slot></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@/components/icons'

type DashboardIcon = 'key' | 'server' | 'chart' | 'creditCard' | 'dollar' | 'cube' | 'database' | 'bolt' | 'clock'
type DashboardColor = 'blue' | 'purple' | 'green' | 'emerald' | 'amber' | 'indigo' | 'violet' | 'rose'

const props = defineProps<{
  icon: DashboardIcon
  color: DashboardColor
  label: string
  value: string | number
  detail: string
}>()

const colorClasses: Record<DashboardColor, { background: string; foreground: string }> = {
  blue: { background: 'bg-blue-100 dark:bg-blue-900/30', foreground: 'text-blue-600 dark:text-blue-400' },
  purple: { background: 'bg-purple-100 dark:bg-purple-900/30', foreground: 'text-purple-600 dark:text-purple-400' },
  green: { background: 'bg-green-100 dark:bg-green-900/30', foreground: 'text-green-600 dark:text-green-400' },
  emerald: { background: 'bg-emerald-100 dark:bg-emerald-900/30', foreground: 'text-emerald-600 dark:text-emerald-400' },
  amber: { background: 'bg-amber-100 dark:bg-amber-900/30', foreground: 'text-amber-600 dark:text-amber-400' },
  indigo: { background: 'bg-indigo-100 dark:bg-indigo-900/30', foreground: 'text-indigo-600 dark:text-indigo-400' },
  violet: { background: 'bg-violet-100 dark:bg-violet-900/30', foreground: 'text-violet-600 dark:text-violet-400' },
  rose: { background: 'bg-rose-100 dark:bg-rose-900/30', foreground: 'text-rose-600 dark:text-rose-400' },
}

const colors = computed(() => colorClasses[props.color])
</script>

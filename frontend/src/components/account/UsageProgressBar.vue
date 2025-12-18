<template>
  <div class="flex items-center gap-1">
    <!-- Label badge (fixed width for alignment) -->
    <span
      :class="[
        'text-[10px] font-medium px-1 rounded w-[32px] text-center shrink-0',
        labelClass
      ]"
    >
      {{ label REDACTEDREDACTED
    </span>

    <!-- Progress bar container -->
    <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden shrink-0">
      <div
        :class="['h-full transition-all duration-300', barClass]"
        :style="{ width: barWidth REDACTED"
      ></div>
    </div>

    <!-- Percentage -->
    <span :class="['text-[10px] font-medium w-[32px] text-right shrink-0', textClass]">
      {{ displayPercent REDACTEDREDACTED
    </span>

    <!-- Reset time -->
    <span v-if="resetsAt" class="text-[10px] text-gray-400 shrink-0">
      {{ formatResetTime REDACTEDREDACTED
    </span>

    <!-- Window stats (only for 5h window) -->
    <span v-if="windowStats" class="text-[10px] text-gray-400 shrink-0 ml-1">
      ({{ formatStats REDACTEDREDACTED)
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import type { WindowStats REDACTED from '@/types'

const props = defineProps<{
  label: string
  utilization: number  // Percentage (0-100+)
  resetsAt?: string | null
  color: 'indigo' | 'emerald' | 'purple'
  windowStats?: WindowStats | null
REDACTED>()

// Label background colors
const labelClass = computed(() => {
  const colors = {
    indigo: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300',
    emerald: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
    purple: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300'
  REDACTED
  return colors[props.color]
REDACTED)

// Progress bar color based on utilization
const barClass = computed(() => {
  if (props.utilization >= 100) {
    return 'bg-red-500'
  REDACTED else if (props.utilization >= 80) {
    return 'bg-amber-500'
  REDACTED else {
    return 'bg-green-500'
  REDACTED
REDACTED)

// Text color based on utilization
const textClass = computed(() => {
  if (props.utilization >= 100) {
    return 'text-red-600 dark:text-red-400'
  REDACTED else if (props.utilization >= 80) {
    return 'text-amber-600 dark:text-amber-400'
  REDACTED else {
    return 'text-gray-600 dark:text-gray-400'
  REDACTED
REDACTED)

// Bar width (capped at 100%)
const barWidth = computed(() => {
  return `${Math.min(props.utilization, 100)REDACTED%`
REDACTED)

// Display percentage (cap at 999% for readability)
const displayPercent = computed(() => {
  const percent = Math.round(props.utilization)
  return percent > 999 ? '>999%' : `${percentREDACTED%`
REDACTED)

// Format reset time
const formatResetTime = computed(() => {
  if (!props.resetsAt) return 'N/A'
  const date = new Date(props.resetsAt)
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()

  if (diffMs <= 0) return 'Now'

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${daysREDACTEDd ${diffHours % 24REDACTEDh`
  REDACTED else if (diffHours > 0) {
    return `${diffHoursREDACTEDh ${diffMinsREDACTEDm`
  REDACTED else {
    return `${diffMinsREDACTEDm`
  REDACTED
REDACTED)

// Format window stats
const formatStats = computed(() => {
  if (!props.windowStats) return ''
  const { requests, tokens, cost REDACTED = props.windowStats

  // Format tokens (e.g., 1234567 -> 1.2M)
  const formatTokens = (t: number): string => {
    if (t >= 1000000) return `${(t / 1000000).toFixed(1)REDACTEDM`
    if (t >= 1000) return `${(t / 1000).toFixed(1)REDACTEDK`
    return t.toString()
  REDACTED

  return `${requestsREDACTEDreq ${formatTokens(tokens)REDACTEDtok $${cost.toFixed(2)REDACTED`
REDACTED)
</script>

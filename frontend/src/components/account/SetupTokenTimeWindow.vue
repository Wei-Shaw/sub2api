<template>
  <div class="space-y-1">
    <!-- 5h Time Window Progress -->
    <div v-if="hasWindowInfo" class="flex items-center gap-1">
      <!-- Label badge -->
      <span class="text-[10px] font-medium px-1 rounded w-[32px] text-center shrink-0 bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
        5h
      </span>

      <!-- Progress bar container -->
      <div class="w-8 h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden shrink-0">
        <div
          :class="['h-full transition-all duration-300', barColorClass]"
          :style="{ width: progressWidth REDACTED"
        ></div>
      </div>

      <!-- Percentage -->
      <span :class="['text-[10px] font-medium w-[32px] text-right shrink-0', textColorClass]">
        {{ displayPercent REDACTEDREDACTED
      </span>

      <!-- Reset time -->
      <span class="text-[10px] text-gray-400 shrink-0">
        {{ formatResetTime REDACTEDREDACTED
      </span>
    </div>

    <!-- No recent activity (had activity but window expired > 1 hour) -->
    <div v-else-if="hasExpiredWindow" class="flex items-center gap-1">
      <span class="text-[10px] font-medium px-1 rounded w-[32px] text-center shrink-0 bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
        5h
      </span>
      <span class="text-[10px] text-gray-400 italic">
        No recent activity
      </span>
    </div>

    <!-- No window info yet (never had activity) -->
    <div v-else class="flex items-center gap-1">
      <span class="text-[10px] font-medium px-1 rounded w-[32px] text-center shrink-0 bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">
        5h
      </span>
      <span class="text-[10px] text-gray-400 italic">
        No activity yet
      </span>
    </div>

    <!-- Hint -->
    <div class="text-[10px] text-gray-400 italic">
      Setup Token (time-based)
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted REDACTED from 'vue'
import type { Account REDACTED from '@/types'

const props = defineProps<{
  account: Account
REDACTED>()

// Update timer
const currentTime = ref(new Date())
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  // Update every second for more accurate countdown
  timer = setInterval(() => {
    currentTime.value = new Date()
  REDACTED, 1000)
REDACTED)

onUnmounted(() => {
  if (timer) {
    clearInterval(timer)
  REDACTED
REDACTED)

// Check if we have window information but it's been expired for more than 1 hour
const hasExpiredWindow = computed(() => {
  if (!props.account.session_window_start || !props.account.session_window_end) {
    return false
  REDACTED

  const end = new Date(props.account.session_window_end).getTime()
  const now = currentTime.value.getTime()
  const expiredMs = now - end

  // Window exists and expired more than 1 hour ago
  return expiredMs > 1000 * 60 * 60
REDACTED)

// Check if we have valid window information (not expired for more than 1 hour)
const hasWindowInfo = computed(() => {
  if (!props.account.session_window_start || !props.account.session_window_end) {
    return false
  REDACTED

  // If window is expired more than 1 hour, don't show progress bar
  if (hasExpiredWindow.value) {
    return false
  REDACTED

  return true
REDACTED)

// Calculate time-based progress (0-100)
const timeProgress = computed(() => {
  if (!props.account.session_window_start || !props.account.session_window_end) {
    return 0
  REDACTED

  const start = new Date(props.account.session_window_start).getTime()
  const end = new Date(props.account.session_window_end).getTime()
  const now = currentTime.value.getTime()

  // Window hasn't started yet
  if (now < start) {
    return 0
  REDACTED

  // Window has ended
  if (now >= end) {
    return 100
  REDACTED

  // Calculate progress within window
  const total = end - start
  const elapsed = now - start
  return Math.round((elapsed / total) * 100)
REDACTED)

// Progress bar width
const progressWidth = computed(() => {
  return `${Math.min(timeProgress.value, 100)REDACTED%`
REDACTED)

// Display percentage
const displayPercent = computed(() => {
  return `${timeProgress.valueREDACTED%`
REDACTED)

// Progress bar color based on progress
const barColorClass = computed(() => {
  if (timeProgress.value >= 100) {
    return 'bg-red-500'
  REDACTED else if (timeProgress.value >= 80) {
    return 'bg-amber-500'
  REDACTED else {
    return 'bg-green-500'
  REDACTED
REDACTED)

// Text color based on progress
const textColorClass = computed(() => {
  if (timeProgress.value >= 100) {
    return 'text-red-600 dark:text-red-400'
  REDACTED else if (timeProgress.value >= 80) {
    return 'text-amber-600 dark:text-amber-400'
  REDACTED else {
    return 'text-gray-600 dark:text-gray-400'
  REDACTED
REDACTED)

// Format reset time (time remaining until window end)
const formatResetTime = computed(() => {
  if (!props.account.session_window_end) {
    return 'N/A'
  REDACTED

  const end = new Date(props.account.session_window_end)
  const now = currentTime.value
  const diffMs = end.getTime() - now.getTime()

  if (diffMs <= 0) {
    // 窗口已过期，计算过期了多久
    const expiredMs = Math.abs(diffMs)
    const expiredHours = Math.floor(expiredMs / (1000 * 60 * 60))

    if (expiredHours >= 1) {
      return 'No recent activity'
    REDACTED
    return 'Window expired'
  REDACTED

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
  const diffSecs = Math.floor((diffMs % (1000 * 60)) / 1000)

  if (diffHours > 0) {
    return `${diffHoursREDACTEDh ${diffMinsREDACTEDm`
  REDACTED else if (diffMins > 0) {
    return `${diffMinsREDACTEDm ${diffSecsREDACTEDs`
  REDACTED else {
    return `${diffSecsREDACTEDs`
  REDACTED
REDACTED)
</script>

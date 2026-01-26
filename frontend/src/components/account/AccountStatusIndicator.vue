<template>
  <div class="flex items-center gap-2">
    <!-- Rate Limit Display (429) - Two-line layout -->
    <div v-if="isRateLimited" class="flex flex-col items-center gap-1">
      <span class="badge text-xs badge-warning">{{ t('admin.accounts.status.rateLimited') REDACTEDREDACTED</span>
      <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ rateLimitCountdown REDACTEDREDACTED</span>
    </div>

    <!-- Overload Display (529) - Two-line layout -->
    <div v-else-if="isOverloaded" class="flex flex-col items-center gap-1">
      <span class="badge text-xs badge-danger">{{ t('admin.accounts.status.overloaded') REDACTEDREDACTED</span>
      <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ overloadCountdown REDACTEDREDACTED</span>
    </div>

    <!-- Main Status Badge (shown when not rate limited/overloaded) -->
    <template v-else>
      <button
        v-if="isTempUnschedulable"
        type="button"
        :class="['badge text-xs', statusClass, 'cursor-pointer']"
        :title="t('admin.accounts.status.viewTempUnschedDetails')"
        @click="handleTempUnschedClick"
      >
        {{ statusText REDACTEDREDACTED
      </button>
      <span v-else :class="['badge text-xs', statusClass]">
        {{ statusText REDACTEDREDACTED
      </span>
    </template>

    <!-- Error Info Indicator -->
    <div v-if="hasError && account.error_message" class="group/error relative">
      <svg
        class="h-4 w-4 cursor-help text-red-500 transition-colors hover:text-red-600 dark:text-red-400 dark:hover:text-red-300"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z"
        />
      </svg>
      <!-- Tooltip - 向下显示 -->
      <div
        class="invisible absolute left-0 top-full z-[100] mt-1.5 min-w-[200px] max-w-[300px] rounded-lg bg-gray-800 px-3 py-2 text-xs text-white opacity-0 shadow-xl transition-all duration-200 group-hover/error:visible group-hover/error:opacity-100 dark:bg-gray-900"
      >
        <div class="whitespace-pre-wrap break-words leading-relaxed text-gray-300">
          {{ account.error_message REDACTEDREDACTED
        </div>
        <!-- 上方小三角 -->
        <div
          class="absolute bottom-full left-3 border-[6px] border-transparent border-b-gray-800 dark:border-b-gray-900"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import type { Account REDACTED from '@/types'
import { formatCountdownWithSuffix REDACTED from '@/utils/format'

const { t REDACTED = useI18n()

const props = defineProps<{
  account: Account
REDACTED>()

const emit = defineEmits<{
  (e: 'show-temp-unsched', account: Account): void
REDACTED>()

// Computed: is rate limited (429)
const isRateLimited = computed(() => {
  if (!props.account.rate_limit_reset_at) return false
  return new Date(props.account.rate_limit_reset_at) > new Date()
REDACTED)

// Computed: is overloaded (529)
const isOverloaded = computed(() => {
  if (!props.account.overload_until) return false
  return new Date(props.account.overload_until) > new Date()
REDACTED)

// Computed: is temp unschedulable
const isTempUnschedulable = computed(() => {
  if (!props.account.temp_unschedulable_until) return false
  return new Date(props.account.temp_unschedulable_until) > new Date()
REDACTED)

// Computed: has error status
const hasError = computed(() => {
  return props.account.status === 'error'
REDACTED)

// Computed: countdown text for rate limit (429)
const rateLimitCountdown = computed(() => {
  return formatCountdownWithSuffix(props.account.rate_limit_reset_at)
REDACTED)

// Computed: countdown text for overload (529)
const overloadCountdown = computed(() => {
  return formatCountdownWithSuffix(props.account.overload_until)
REDACTED)

// Computed: status badge class
const statusClass = computed(() => {
  if (hasError.value) {
    return 'badge-danger'
  REDACTED
  if (isTempUnschedulable.value) {
    return 'badge-warning'
  REDACTED
  if (!props.account.schedulable) {
    return 'badge-gray'
  REDACTED
  switch (props.account.status) {
    case 'active':
      return 'badge-success'
    case 'inactive':
      return 'badge-gray'
    case 'error':
      return 'badge-danger'
    default:
      return 'badge-gray'
  REDACTED
REDACTED)

// Computed: status text
const statusText = computed(() => {
  if (hasError.value) {
    return t('admin.accounts.status.error')
  REDACTED
  if (isTempUnschedulable.value) {
    return t('admin.accounts.status.tempUnschedulable')
  REDACTED
  if (!props.account.schedulable) {
    return t('admin.accounts.status.paused')
  REDACTED
  return t(`admin.accounts.status.${props.account.statusREDACTED`)
REDACTED)

const handleTempUnschedClick = () => {
  if (!isTempUnschedulable.value) return
  emit('show-temp-unsched', props.account)
REDACTED
</script>
